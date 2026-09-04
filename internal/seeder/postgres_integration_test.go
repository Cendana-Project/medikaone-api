package seeder

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	appointmentrepo "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

const (
	testDatabaseDSNEnv               = "TEST_DATABASE_DSN"
	testDatabaseResetConfirmationEnv = "TEST_DATABASE_RESET_CONFIRMATION"
	testDatabaseResetConfirmation    = "RESET-MEDIKAONE-TEST-DATABASE"
)

func TestPostgresSeederIntegration(t *testing.T) {
	dsn, expectedTarget := requireIsolatedTestDatabase(t)
	t.Setenv("SUPERADMIN_EMAIL", "")
	t.Setenv("SUPERADMIN_PASSWORD", "")

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("open isolated PostgreSQL test database")
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal("TEST_DATABASE_DSN is not reachable")
	}
	var currentDatabase, currentUser string
	if err := sqlDB.QueryRowContext(ctx, `SELECT CURRENT_DATABASE(), CURRENT_USER`).Scan(&currentDatabase, &currentUser); err != nil {
		t.Fatal("cannot verify TEST_DATABASE_DSN target")
	}
	if currentDatabase != expectedTarget.database || currentUser != expectedTarget.user {
		t.Fatalf(
			"refusing destructive integration test: connected database/user %q/%q does not match configured target %q/%q",
			currentDatabase, currentUser, expectedTarget.database, expectedTarget.user,
		)
	}

	// TEST_DATABASE_DSN must name a disposable database. Every integration run
	// starts with its public schema empty and leaves it empty on cleanup.
	t.Cleanup(func() { resetPublicSchema(t, sqlDB) })
	resetPublicSchema(t, sqlDB)

	if ok := t.Run("clear empty and partial schema", func(t *testing.T) {
		db := openIntegrationGORM(t, sqlDB)
		if err := ClearAllData(db); err != nil {
			t.Fatalf("ClearAllData() on empty schema: %v", err)
		}
		if _, err := sqlDB.Exec(`CREATE TABLE users (id BIGSERIAL PRIMARY KEY, email TEXT)`); err != nil {
			t.Fatalf("create partial schema: %v", err)
		}
		if _, err := sqlDB.Exec(`CREATE TABLE custom_keep (id BIGSERIAL PRIMARY KEY)`); err != nil {
			t.Fatalf("create unowned test table: %v", err)
		}
		if _, err := sqlDB.Exec(`CREATE TABLE goose_db_version (id BIGSERIAL PRIMARY KEY)`); err != nil {
			t.Fatalf("create migration history placeholder: %v", err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO users (email) VALUES ('partial@example.test')`); err != nil {
			t.Fatalf("populate partial schema: %v", err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO custom_keep DEFAULT VALUES`); err != nil {
			t.Fatalf("populate unowned test table: %v", err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO goose_db_version DEFAULT VALUES`); err != nil {
			t.Fatalf("populate migration history placeholder: %v", err)
		}
		if err := ClearAllData(db); err != nil {
			t.Fatalf("ClearAllData() on partial schema: %v", err)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users`); got != 0 {
			t.Fatalf("partial users count after ClearAllData = %d, want 0", got)
		}
		var restartedID int
		if err := sqlDB.QueryRow(`INSERT INTO users (email) VALUES ('after-reset@example.test') RETURNING id`).Scan(&restartedID); err != nil {
			t.Fatalf("insert after ClearAllData: %v", err)
		}
		if restartedID != 1 {
			t.Fatalf("users identity after ClearAllData = %d, want 1", restartedID)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM custom_keep`); got != 1 {
			t.Fatalf("unowned table rows after ClearAllData = %d, want 1", got)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM goose_db_version`); got != 1 {
			t.Fatalf("migration history rows after ClearAllData = %d, want 1", got)
		}

		// An unknown table that references application data must make the reset
		// fail closed. It must never be silently included in the destructive set.
		if _, err := sqlDB.Exec(`CREATE TABLE external_user_notes (id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL REFERENCES users(id))`); err != nil {
			t.Fatalf("create external FK table: %v", err)
		}
		if _, err := sqlDB.Exec(`INSERT INTO external_user_notes (user_id) VALUES (1)`); err != nil {
			t.Fatalf("populate external FK table: %v", err)
		}
		if err := ClearAllData(db); err == nil {
			t.Fatal("ClearAllData() with an unknown referencing table unexpectedly succeeded")
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users`); got != 1 {
			t.Fatalf("users rows after failed closed reset = %d, want 1", got)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM external_user_notes`); got != 1 {
			t.Fatalf("external rows after failed closed reset = %d, want 1", got)
		}
	}); !ok {
		t.FailNow()
	}

	resetPublicSchema(t, sqlDB)
	applyMigrations(t, sqlDB)
	db := openIntegrationGORM(t, sqlDB)

	if ok := t.Run("goose bootstraps empty schema", func(t *testing.T) {
		// A second Up must be a no-op, proving an already-bootstrapped database
		// does not rerun or roll back migrations.
		applyMigrations(t, sqlDB)
		for _, table := range []string{
			"users", "roles", "permissions", "user_roles", "role_permissions",
			"patient_profiles", "doctor_profiles", "hospitals", "user_hospitals",
			"hospital_user_roles", "hospital_departments", "hospital_rooms",
			"doctor_hospital_invitations", "doctor_hospital_contracts",
			"doctor_hospital_invitation_events", "doctor_hospital_invitation_schedules",
			"doctor_hospital_affiliations", "doctor_hospital_affiliation_events",
			"doctor_hospital_schedules", "notifications",
			"doctor_schedule_change_requests", "doctor_schedule_change_items",
			"doctor_schedule_change_events", "appointment_daily_counters",
			"appointments", "appointment_status_events", "appointment_reminders",
			"patient_records", "patient_record_events",
		} {
			var exists bool
			if err := sqlDB.QueryRow(`SELECT TO_REGCLASS($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil {
				t.Fatalf("inspect table %s: %v", table, err)
			}
			if !exists {
				t.Fatalf("Goose Up did not create table %s", table)
			}
		}
		for _, index := range []string{
			"ux_users_seed_key", "ux_users_email_lower", "ux_users_username_lower", "ux_users_nik_active",
			"ux_hospitals_seed_key", "ux_hospitals_code_lower", "ux_hospitals_name_lower",
			"ux_user_hospitals_one_primary",
		} {
			if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1`, index); got != 1 {
				t.Fatalf("partial unique index %s count = %d, want 1", index, got)
			}
		}
		for _, column := range []string{"updated_at", "deleted_at"} {
			if got := scalarInt(t, sqlDB, `
				SELECT COUNT(*) FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'user_hospitals' AND column_name = $1
			`, column); got != 1 {
				t.Fatalf("user_hospitals.%s column count = %d, want 1", column, got)
			}
		}
		wantMigrations := migrationFileCount(t)
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM goose_db_version WHERE is_applied AND version_id > 0`); got != wantMigrations {
			t.Fatalf("applied Goose migration count = %d, want %d", got, wantMigrations)
		}
	}); !ok {
		t.FailNow()
	}

	if ok := t.Run("declarative seeder populates relational fixture", func(t *testing.T) {
		if err := Run(db); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		stableIDs := map[string]string{
			"user":       scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'superadmin@medikaone.id' AND deleted_at IS NULL`),
			"role":       scalarString(t, sqlDB, `SELECT id::text FROM roles WHERE slug = 'SUPER_ADMIN' AND deleted_at IS NULL`),
			"permission": scalarString(t, sqlDB, `SELECT id::text FROM permissions WHERE slug = 'user.view' AND deleted_at IS NULL`),
			"hospital":   scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code = 'HSP-MO-001' AND deleted_at IS NULL`),
		}
		if err := Run(db); err != nil {
			t.Fatalf("second idempotent Run() error = %v", err)
		}
		currentIDs := map[string]string{
			"user":       scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'superadmin@medikaone.id' AND deleted_at IS NULL`),
			"role":       scalarString(t, sqlDB, `SELECT id::text FROM roles WHERE slug = 'SUPER_ADMIN' AND deleted_at IS NULL`),
			"permission": scalarString(t, sqlDB, `SELECT id::text FROM permissions WHERE slug = 'user.view' AND deleted_at IS NULL`),
			"hospital":   scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code = 'HSP-MO-001' AND deleted_at IS NULL`),
		}
		for fixture, want := range stableIDs {
			if got := currentIDs[fixture]; got != want {
				t.Fatalf("%s fixture ID after second Run = %q, want stable ID %q", fixture, got, want)
			}
		}
		if got, want := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`), len(sampleUserSeeds()); got != want {
			t.Fatalf("active seeded users = %d, want %d", got, want)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM roles WHERE deleted_at IS NULL`); got != 7 {
			t.Fatalf("active seeded roles = %d, want 7", got)
		}
		wantPermissions := len(uniquePermSlugsFromDefaults())
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM permissions WHERE deleted_at IS NULL`); got != wantPermissions {
			t.Fatalf("active seeded permissions = %d, want %d", got, wantPermissions)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM user_roles`); got != 4 {
			t.Fatalf("seeded global role assignments = %d, want 4", got)
		}
		wantRolePermissions := 0
		for _, permissions := range constant.DefaultRolePermissions {
			wantRolePermissions += len(permissions)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM role_permissions`); got != wantRolePermissions {
			t.Fatalf("seeded role permission assignments = %d, want %d", got, wantRolePermissions)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM hospitals WHERE deleted_at IS NULL`); got != 2 {
			t.Fatalf("active seeded hospitals = %d, want 2", got)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM user_hospitals WHERE deleted_at IS NULL`); got != 7 {
			t.Fatalf("seeded hospital memberships = %d, want 7", got)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM hospital_user_roles`); got != 7 {
			t.Fatalf("seeded hospital role assignments = %d, want 7", got)
		}
		if _, err := sqlDB.Exec(`UPDATE users SET seed_key = 'forged:fixture' WHERE seed_key IS NOT NULL`); err == nil {
			t.Fatal("database allowed immutable user fixture provenance to change")
		}
		if _, err := sqlDB.Exec(`UPDATE hospitals SET seed_key = 'forged:fixture' WHERE seed_key IS NOT NULL`); err == nil {
			t.Fatal("database allowed immutable hospital fixture provenance to change")
		}
	}); !ok {
		t.FailNow()
	}

	if ok := t.Run("soft-deleted user identifiers can be reused", func(t *testing.T) {
		const insertUser = `
			INSERT INTO users (email, username, nik, password_hash, status, created_at)
			VALUES ($1, $2, $3, 'integration-hash', 'active', NOW())
			RETURNING id`
		var deletedID string
		if err := sqlDB.QueryRow(insertUser, "reuse@example.test", "ReusableUser", "9999000011112222").Scan(&deletedID); err != nil {
			t.Fatalf("insert user to soft-delete: %v", err)
		}
		if _, err := sqlDB.Exec(`UPDATE users SET deleted_at = NOW() WHERE id = $1`, deletedID); err != nil {
			t.Fatalf("soft-delete user: %v", err)
		}
		if _, err := sqlDB.Exec(`
			INSERT INTO users (email, username, nik, password_hash, status, created_at)
			VALUES ('REUSE@example.test', 'reusableuser', '9999000011112222', 'integration-hash', 'active', NOW())
		`); err != nil {
			t.Fatalf("reuse identifiers from soft-deleted user: %v", err)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users WHERE LOWER(email) = LOWER('reuse@example.test')`); got != 2 {
			t.Fatalf("all reused-identifier user rows = %d, want 2", got)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users WHERE LOWER(email) = LOWER('reuse@example.test') AND deleted_at IS NULL`); got != 1 {
			t.Fatalf("active reused-identifier user rows = %d, want 1", got)
		}
		if _, err := sqlDB.Exec(`
			INSERT INTO users (email, username, nik, password_hash, status, created_at)
			VALUES ('reuse@example.test', 'another-user', '9999000011113333', 'integration-hash', 'active', NOW())
		`); !isUniqueViolation(err) {
			t.Fatalf("duplicate active case-insensitive email error = %v, want PostgreSQL unique violation", err)
		}
	}); !ok {
		t.FailNow()
	}

	if ok := t.Run("hospital identifiers reuse and seeded hospital reactivation", func(t *testing.T) {
		var deletedID string
		if err := sqlDB.QueryRow(`
			INSERT INTO hospitals (code, name, is_active, created_at, updated_at)
			VALUES ('HSP-REUSE-IT', 'Reusable Integration Hospital', TRUE, NOW(), NOW())
			RETURNING id
		`).Scan(&deletedID); err != nil {
			t.Fatalf("insert hospital to soft-delete: %v", err)
		}
		if _, err := sqlDB.Exec(`UPDATE hospitals SET deleted_at = NOW(), is_active = FALSE WHERE id = $1`, deletedID); err != nil {
			t.Fatalf("soft-delete hospital: %v", err)
		}
		if _, err := sqlDB.Exec(`
			INSERT INTO hospitals (code, name, is_active, created_at, updated_at)
			VALUES ('hsp-reuse-it', 'reusable integration hospital', TRUE, NOW(), NOW())
		`); err != nil {
			t.Fatalf("reuse identifiers from soft-deleted hospital: %v", err)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM hospitals WHERE LOWER(code) = LOWER('HSP-REUSE-IT')`); got != 2 {
			t.Fatalf("all reused-identifier hospital rows = %d, want 2", got)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM hospitals WHERE LOWER(code) = LOWER('HSP-REUSE-IT') AND deleted_at IS NULL`); got != 1 {
			t.Fatalf("active reused-identifier hospital rows = %d, want 1", got)
		}
		if _, err := sqlDB.Exec(`
			INSERT INTO hospitals (code, name, is_active, created_at, updated_at)
			VALUES ('HSP-REUSE-IT', 'Another Active Hospital', TRUE, NOW(), NOW())
		`); !isUniqueViolation(err) {
			t.Fatalf("duplicate active case-insensitive hospital code error = %v, want PostgreSQL unique violation", err)
		}

		var seededID string
		if err := sqlDB.QueryRow(`SELECT id FROM hospitals WHERE code = 'HSP-MO-001' AND deleted_at IS NULL`).Scan(&seededID); err != nil {
			t.Fatalf("find seeded hospital: %v", err)
		}
		if _, err := sqlDB.Exec(`UPDATE hospitals SET deleted_at = NOW(), is_active = FALSE WHERE id = $1`, seededID); err != nil {
			t.Fatalf("soft-delete seeded hospital: %v", err)
		}
		if err := SeedHospitals(db); err != nil {
			t.Fatalf("SeedHospitals() reactivation error = %v", err)
		}
		var reactivatedID string
		if err := sqlDB.QueryRow(`SELECT id FROM hospitals WHERE LOWER(code) = LOWER('HSP-MO-001') AND deleted_at IS NULL`).Scan(&reactivatedID); err != nil {
			t.Fatalf("find reactivated hospital: %v", err)
		}
		if reactivatedID != seededID {
			t.Fatalf("SeedHospitals created a replacement row %s; want original %s reactivated", reactivatedID, seededID)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM hospitals WHERE LOWER(code) = LOWER('HSP-MO-001') AND deleted_at IS NULL`); got != 1 {
			t.Fatalf("active canonical hospital count = %d, want 1", got)
		}

		// If an unrelated active row takes a fixture's public identifier while
		// the owned fixture is deleted, the seeder must fail instead of adopting
		// or overwriting that unrelated row.
		if _, err := sqlDB.Exec(`UPDATE hospitals SET deleted_at = NOW(), is_active = FALSE WHERE id = $1`, seededID); err != nil {
			t.Fatalf("soft-delete seeded hospital before conflict: %v", err)
		}
		var replacementID string
		if err := sqlDB.QueryRow(`
			INSERT INTO hospitals (code, name, is_active, created_at, updated_at)
			VALUES ('hsp-mo-001', 'Unowned Active Replacement', TRUE, NOW(), NOW())
			RETURNING id
		`).Scan(&replacementID); err != nil {
			t.Fatalf("insert active unowned hospital replacement: %v", err)
		}
		if err := SeedHospitals(db); !isUniqueViolation(err) {
			t.Fatalf("SeedHospitals() identifier conflict error = %v, want PostgreSQL unique violation", err)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM hospitals WHERE id = $1 AND deleted_at IS NOT NULL`, seededID); got != 1 {
			t.Fatal("owned hospital was unexpectedly reactivated across an identifier conflict")
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM hospitals WHERE id = $1 AND deleted_at IS NULL`, replacementID); got != 1 {
			t.Fatal("unowned hospital replacement was unexpectedly changed across an identifier conflict")
		}
	}); !ok {
		t.FailNow()
	}

	if ok := t.Run("failed full reset rolls back truncate and partial seed", func(t *testing.T) {
		var sentinelID string
		if err := sqlDB.QueryRow(`
			INSERT INTO users (email, password_hash, status, created_at)
			VALUES ('rollback-sentinel@example.test', 'integration-hash', 'active', NOW())
			RETURNING id
		`).Scan(&sentinelID); err != nil {
			t.Fatalf("insert rollback sentinel: %v", err)
		}
		const sentinelRoleName = "Rollback Sentinel Patient Role"
		if result, err := sqlDB.Exec(`UPDATE roles SET name = $1 WHERE slug = 'PATIENT'`, sentinelRoleName); err != nil {
			t.Fatalf("mutate role before reset: %v", err)
		} else if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
			t.Fatalf("mutated role rows = %d, err = %v; want 1", affected, rowsErr)
		}
		before := tableCounts(t, sqlDB,
			"users", "roles", "permissions", "user_roles", "role_permissions",
			"hospitals", "user_hospitals", "hospital_user_roles",
		)

		t.Setenv("SUPERADMIN_EMAIL", "force-seed-failure@example.test")
		t.Setenv("SUPERADMIN_PASSWORD", "")
		err := ResetAllAndSeed(db)
		if err == nil || !strings.Contains(err.Error(), "SUPERADMIN_PASSWORD") {
			t.Fatalf("ResetAllAndSeed() error = %v, want forced seed failure", err)
		}
		after := tableCounts(t, sqlDB,
			"users", "roles", "permissions", "user_roles", "role_permissions",
			"hospitals", "user_hospitals", "hospital_user_roles",
		)
		for table, want := range before {
			if got := after[table]; got != want {
				t.Fatalf("%s count after rollback = %d, want %d", table, got, want)
			}
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users WHERE id = $1 AND email = 'rollback-sentinel@example.test'`, sentinelID); got != 1 {
			t.Fatal("transactional reset did not restore the pre-reset sentinel user")
		}
		var roleName string
		if err := sqlDB.QueryRow(`SELECT name FROM roles WHERE slug = 'PATIENT'`).Scan(&roleName); err != nil {
			t.Fatalf("read role after rollback: %v", err)
		}
		if roleName != sentinelRoleName {
			t.Fatalf("role name after rollback = %q, want %q", roleName, sentinelRoleName)
		}
	}); !ok {
		t.FailNow()
	}

	if ok := t.Run("successful full reset removes non-seed data", func(t *testing.T) {
		t.Setenv("SUPERADMIN_EMAIL", "")
		t.Setenv("SUPERADMIN_PASSWORD", "")
		if err := ResetAllAndSeed(db); err != nil {
			t.Fatalf("ResetAllAndSeed() error = %v", err)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users WHERE email = 'rollback-sentinel@example.test'`); got != 0 {
			t.Fatalf("sentinel users after successful full reset = %d, want 0", got)
		}
		if got, want := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`), len(sampleUserSeeds()); got != want {
			t.Fatalf("active users after full reset = %d, want %d", got, want)
		}
	}); !ok {
		t.FailNow()
	}

	if ok := t.Run("environment superadmin overrides only canonical fixture", func(t *testing.T) {
		const configuredPassword = "Environment!Pass9"
		t.Setenv("SUPERADMIN_EMAIL", "SUPERADMIN@medikaone.id")
		t.Setenv("SUPERADMIN_PASSWORD", configuredPassword)
		t.Setenv("SUPERADMIN_FIRST_NAME", "Environment")
		if err := Run(db); err != nil {
			t.Fatalf("Run() with canonical environment superadmin: %v", err)
		}
		hash := scalarString(t, sqlDB, `SELECT password_hash FROM users WHERE seed_key = $1`, demoUserSeedKey("superadmin@medikaone.id"))
		matched, err := util.VerifyPasswordScrypt(context.Background(), hash, configuredPassword)
		if err != nil || !matched {
			t.Fatalf("environment superadmin password match = %v, err = %v", matched, err)
		}
		if firstName := scalarString(t, sqlDB, `SELECT first_name FROM users WHERE seed_key = $1`, demoUserSeedKey("superadmin@medikaone.id")); firstName != "Environment" {
			t.Fatalf("environment first name = %q, want Environment", firstName)
		}

		t.Setenv("SUPERADMIN_EMAIL", "patient001@medikaone.id")
		if err := Run(db); err == nil || !strings.Contains(err.Error(), "belongs to the PATIENT demo fixture") {
			t.Fatalf("cross-role environment superadmin error = %v, want fail-closed collision", err)
		}
		if got := scalarInt(t, sqlDB, `
			SELECT COUNT(*) FROM user_roles ur
			JOIN users u ON u.id = ur.user_id
			JOIN roles r ON r.id = ur.role_id
			WHERE u.email = 'patient001@medikaone.id' AND r.slug = 'SUPER_ADMIN'
		`); got != 0 {
			t.Fatalf("patient fixture superadmin assignments = %d, want 0", got)
		}
	}); !ok {
		t.FailNow()
	}

	if ok := t.Run("demo reset deletes only immutable fixture provenance", func(t *testing.T) {
		var realUserID string
		if err := sqlDB.QueryRow(`
			INSERT INTO users (email, password_hash, status, created_at)
			VALUES ('real-user@example.test', 'integration-hash', 'active', NOW())
			RETURNING id
		`).Scan(&realUserID); err != nil {
			t.Fatalf("insert real user: %v", err)
		}
		oldFixtureID := scalarString(t, sqlDB, `
			SELECT id::text FROM users WHERE seed_key = $1
		`, demoUserSeedKey("superadmin@medikaone.id"))
		if err := ResetDemoAndSeed(db); err != nil {
			t.Fatalf("ResetDemoAndSeed() error = %v", err)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users WHERE id = $1`, realUserID); got != 1 {
			t.Fatal("demo reset removed an unowned real user")
		}
		newFixtureID := scalarString(t, sqlDB, `
			SELECT id::text FROM users WHERE seed_key = $1
		`, demoUserSeedKey("superadmin@medikaone.id"))
		if newFixtureID == oldFixtureID {
			t.Fatal("demo reset did not recreate the owned fixture user")
		}

		patientKey := demoUserSeedKey("patient001@medikaone.id")
		if _, err := sqlDB.Exec(`UPDATE users SET email = 'moved-fixture@example.test' WHERE seed_key = $1`, patientKey); err != nil {
			t.Fatalf("move fixture identity: %v", err)
		}
		var unownedCollisionID string
		if err := sqlDB.QueryRow(`
			INSERT INTO users (email, password_hash, status, created_at)
			VALUES ('patient001@medikaone.id', 'integration-hash', 'active', NOW())
			RETURNING id
		`).Scan(&unownedCollisionID); err != nil {
			t.Fatalf("insert unowned canonical identity: %v", err)
		}
		if err := ResetDemoAndSeed(db); err == nil || !strings.Contains(err.Error(), "untagged rows") {
			t.Fatalf("ResetDemoAndSeed() collision error = %v, want fail-closed provenance error", err)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users WHERE seed_key = $1 AND email = 'moved-fixture@example.test'`, patientKey); got != 1 {
			t.Fatal("failed demo reset changed the owned fixture")
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM users WHERE id = $1 AND seed_key IS NULL`, unownedCollisionID); got != 1 {
			t.Fatal("failed demo reset changed the unowned colliding user")
		}
	}); !ok {
		t.FailNow()
	}

	if ok := t.Run("appointment booking serializes competing patients", func(t *testing.T) {
		t.Setenv("SUPERADMIN_EMAIL", "")
		t.Setenv("SUPERADMIN_PASSWORD", "")
		if err := ResetAllAndSeed(db); err != nil {
			t.Fatalf("reset before appointment concurrency test: %v", err)
		}
		hospitalID := scalarString(t, sqlDB, `SELECT id::text FROM hospitals WHERE code = 'HSP-MO-001'`)
		doctorID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'doctor001@medikaone.id'`)
		adminID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'admin001@medikaone.id'`)
		patientIDs := []string{
			scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'patient001@medikaone.id'`),
			scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'patient002@medikaone.id'`),
		}
		departmentID, invitationID, affiliationID, scheduleID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
		now := time.Now().UTC()
		if _, err := sqlDB.Exec(`
			INSERT INTO hospital_departments (id, hospital_id, code, name, created_at, updated_at)
			VALUES ($1, $2, 'IT-APPT', 'Integration Appointment', $3, $3)`, departmentID, hospitalID, now); err != nil {
			t.Fatalf("prepare appointment department: %v", err)
		}
		if _, err := sqlDB.Exec(`
			INSERT INTO doctor_hospital_invitations (
				id, hospital_id, doctor_id, department_id, invited_by, status, expires_at,
				responded_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, 'ACCEPTED', $6, $7, $7, $7)`,
			invitationID, hospitalID, doctorID, departmentID, adminID, now.Add(24*time.Hour), now); err != nil {
			t.Fatalf("prepare appointment invitation: %v", err)
		}
		if _, err := sqlDB.Exec(`
			INSERT INTO doctor_hospital_affiliations (
				id, hospital_id, doctor_id, department_id, invitation_id, status,
				joined_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, 'ACTIVE', $6, $6, $6)`,
			affiliationID, hospitalID, doctorID, departmentID, invitationID, now); err != nil {
			t.Fatalf("prepare appointment affiliation: %v", err)
		}
		if _, err := sqlDB.Exec(`
			INSERT INTO doctor_hospital_schedules (
				id, affiliation_id, day_of_week, start_time, end_time, timezone,
				booking_mode, slot_duration_minutes, capacity, is_active, created_at, updated_at
			) VALUES ($1, $2, 1, '08:00', '09:00', 'Asia/Jakarta', 'FIXED_SLOT', 30, 1, TRUE, $3, $3)`,
			scheduleID, affiliationID, now); err != nil {
			t.Fatalf("prepare appointment schedule: %v", err)
		}
		appointmentDate := "2026-09-07"
		start := time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)
		schedule := appointmentrepo.Schedule{
			ID: scheduleID, AffiliationID: affiliationID, HospitalID: hospitalID,
			HospitalCode: "HSP-MO-001", DoctorID: doctorID, DepartmentID: departmentID,
			DayOfWeek: 1, StartTime: "08:00", EndTime: "09:00", Timezone: "Asia/Jakarta",
			BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 1,
		}
		repo := appointmentrepo.NewRepository(db)
		errs := make(chan error, len(patientIDs))
		var wg sync.WaitGroup
		for _, patientID := range patientIDs {
			patientID := patientID
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _, err := repo.Book(context.Background(), appointmentrepo.BookInput{
					PatientID: patientID, Schedule: schedule, AppointmentDate: appointmentDate,
					ScheduledStartAt: start, ScheduledEndAt: start.Add(30 * time.Minute),
					ReasonForVisit: "integration test", ConsentVersion: "it-v1",
					IdempotencyKey: uuid.NewString(), IdempotencyRequestHash: strings.Repeat("a", 64), Now: now,
				})
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		succeeded, rejected := 0, 0
		for err := range errs {
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, appointmentrepo.ErrSlotUnavailable):
				rejected++
			default:
				t.Fatalf("unexpected concurrent booking error: %v", err)
			}
		}
		if succeeded != 1 || rejected != 1 {
			t.Fatalf("concurrent bookings succeeded/rejected = %d/%d, want 1/1", succeeded, rejected)
		}
		if got := scalarInt(t, sqlDB, `SELECT COUNT(*) FROM appointments WHERE schedule_id = $1`, scheduleID); got != 1 {
			t.Fatalf("persisted competing appointments = %d, want 1", got)
		}

		receptionistID := scalarString(t, sqlDB, `SELECT id::text FROM users WHERE email = 'receptionist001@medikaone.id'`)
		walkIn, replay, err := repo.CreateWalkIn(context.Background(), appointmentrepo.WalkInInput{
			ActorID: receptionistID,
			Patient: appointmentrepo.WalkInPatientInput{
				FirstName: "Walk In", LastName: "Claim", Email: stringPointer("patient001@medikaone.id"),
				Phone: "081200000001", DateOfBirth: "1990-01-01", Gender: "L",
				IdentityType: "OTHER", IdentityNumber: "IT-CLAIM-001", IdentityNormalized: "ITCLAIM001",
			},
			Schedule: schedule, AppointmentDate: appointmentDate,
			ScheduledStartAt: start.Add(30 * time.Minute), ScheduledEndAt: start.Add(60 * time.Minute),
			ReasonForVisit: "walk-in integration test", ConsentVersion: "it-v1",
			IdempotencyKey: uuid.NewString(), IdempotencyRequestHash: strings.Repeat("b", 64), Now: now,
		})
		if err != nil {
			t.Fatalf("create walk-in appointment: %v", err)
		}
		if replay || walkIn.Status != "WAITING_VITALS" || walkIn.Source != "WALK_IN" || walkIn.PatientID != nil {
			t.Fatalf("walk-in result = %#v, replay=%v; want new unclaimed WAITING_VITALS appointment", walkIn, replay)
		}
		claimed, err := repo.ClaimPatientRecord(
			context.Background(), patientIDs[0], "OTHER", "IT-CLAIM-001", "1990-01-01", now.Add(time.Minute),
		)
		if err != nil {
			t.Fatalf("claim walk-in patient record: %v", err)
		}
		if claimed.UserID == nil || *claimed.UserID != patientIDs[0] {
			t.Fatalf("claimed patient user = %v, want %s", claimed.UserID, patientIDs[0])
		}
		if got := scalarString(t, sqlDB, `SELECT patient_id::text FROM appointments WHERE id = $1`, walkIn.ID); got != patientIDs[0] {
			t.Fatalf("walk-in appointment patient after claim = %s, want %s", got, patientIDs[0])
		}

		overrideReason := "Dokter menyetujui pasien tambahan"
		override, replay, err := repo.CreateWalkIn(context.Background(), appointmentrepo.WalkInInput{
			ActorID: adminID,
			Patient: appointmentrepo.WalkInPatientInput{
				FirstName: "Walk In", LastName: "Override", Phone: "081299999999",
				DateOfBirth: "1991-02-03", Gender: "P", IdentityType: "OTHER",
				IdentityNumber: "IT-OVERRIDE-002", IdentityNormalized: "ITOVERRIDE002",
			},
			Schedule: schedule, AppointmentDate: appointmentDate,
			ScheduledStartAt: start.Add(30 * time.Minute), ScheduledEndAt: start.Add(60 * time.Minute),
			ReasonForVisit: "capacity override integration test", ConsentVersion: "it-v1",
			IdempotencyKey: uuid.NewString(), IdempotencyRequestHash: strings.Repeat("c", 64),
			CapacityOverride: true, CapacityOverrideReason: &overrideReason, Now: now,
		})
		if err != nil {
			t.Fatalf("create capacity-override walk-in appointment: %v", err)
		}
		if replay || !override.CapacityOverridden || override.CapacityOverrideReason == nil || *override.CapacityOverrideReason != overrideReason {
			t.Fatalf("capacity override result = %#v, replay=%v", override, replay)
		}
	}); !ok {
		t.FailNow()
	}
}

func stringPointer(value string) *string { return &value }

type integrationDatabaseTarget struct {
	database string
	user     string
}

func requireIsolatedTestDatabase(t *testing.T) (string, integrationDatabaseTarget) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(testDatabaseDSNEnv))
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is empty; skipping PostgreSQL integration tests")
	}
	database, err := validateIntegrationDatabaseTarget(dsn, os.Getenv(testDatabaseResetConfirmationEnv))
	if err != nil {
		t.Fatal(err)
	}
	return dsn, database
}

func validateIntegrationDatabaseTarget(dsn, confirmation string) (integrationDatabaseTarget, error) {
	if confirmation != testDatabaseResetConfirmation {
		return integrationDatabaseTarget{}, fmt.Errorf("refusing destructive integration test: %s must equal %q", testDatabaseResetConfirmationEnv, testDatabaseResetConfirmation)
	}
	parsed, err := pgx.ParseConfig(strings.TrimSpace(dsn))
	if err != nil || parsed.Database == "" || parsed.User == "" {
		return integrationDatabaseTarget{}, errors.New("TEST_DATABASE_DSN is invalid")
	}
	database := strings.ToLower(parsed.Database)
	clearlyTestOnly := database == "test" ||
		strings.HasPrefix(database, "test_") || strings.HasPrefix(database, "test-") ||
		strings.HasSuffix(database, "_test") || strings.HasSuffix(database, "-test")
	if !clearlyTestOnly {
		return integrationDatabaseTarget{}, fmt.Errorf("refusing destructive integration test: database name %q is not explicitly test-only", parsed.Database)
	}
	return integrationDatabaseTarget{database: parsed.Database, user: parsed.User}, nil
}

func TestValidateIntegrationDatabaseTarget(t *testing.T) {
	validDSN := "postgresql://postgres:postgres@localhost:5432/medikaone_test?sslmode=disable"
	if got, err := validateIntegrationDatabaseTarget(validDSN, testDatabaseResetConfirmation); err != nil || got.database != "medikaone_test" || got.user != "postgres" {
		t.Fatalf("valid target = %#v, %v; want medikaone_test/postgres, nil", got, err)
	}
	if _, err := validateIntegrationDatabaseTarget(validDSN, ""); err == nil {
		t.Fatal("missing destructive confirmation unexpectedly accepted")
	}
	for _, unsafeName := range []string{"contest", "latest", "production", "production_test_backup"} {
		dsn := "postgresql://postgres:postgres@localhost:5432/" + unsafeName + "?sslmode=disable"
		if _, err := validateIntegrationDatabaseTarget(dsn, testDatabaseResetConfirmation); err == nil {
			t.Fatalf("unsafe database name %q unexpectedly accepted", unsafeName)
		}
	}
}

func resetPublicSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset isolated test schema: %v", err)
	}
}

func applyMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set Goose dialect: %v", err)
	}
	migrationDir := migrationDirectory(t)
	if err := goose.Up(db, migrationDir); err != nil {
		t.Fatalf("Goose Up on empty schema: %v", err)
	}
}

func migrationFileCount(t *testing.T) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(migrationDirectory(t), "*.sql"))
	if err != nil {
		t.Fatalf("list migration files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no SQL migration files found")
	}
	return len(files)
}

func migrationDirectory(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test location")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "migration", "db")
}

func openIntegrationGORM(t *testing.T, sqlDB *sql.DB) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open GORM over isolated test database: %v", err)
	}
	return db
}

func scalarInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var value int
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatalf("scalar query failed: %v", err)
	}
	return value
}

func scalarString(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var value string
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatalf("scalar query failed: %v", err)
	}
	return value
}

func tableCounts(t *testing.T, db *sql.DB, tables ...string) map[string]int {
	t.Helper()
	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		if strings.ContainsAny(table, `"';.-/\\`) {
			t.Fatalf("unsafe test table name %q", table)
		}
		counts[table] = scalarInt(t, db, fmt.Sprintf("SELECT COUNT(*) FROM %s", table))
	}
	return counts
}

func isUniqueViolation(err error) bool {
	var target interface{ SQLState() string }
	return errors.As(err, &target) && target.SQLState() == "23505"
}
