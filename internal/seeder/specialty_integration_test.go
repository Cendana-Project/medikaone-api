package seeder

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	specialtyrepo "github.com/Cendana-Project/medikaone-api/internal/repository/specialty"
	userrepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	specialtyservice "github.com/Cendana-Project/medikaone-api/internal/service/specialty"
	specialtyhttp "github.com/Cendana-Project/medikaone-api/internal/transport/http/specialty"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

// Call on the isolated empty integration schema instead of its initial Up.
// It finishes at the latest schema and removes only its own temporary users.
func assertSpecialtyMigrationRetention(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, migrationDirectory(t), 20261004065141); err != nil {
		t.Fatalf("migrate pre-specialty schema: %v", err)
	}
	fixtures := []struct {
		legacy string
		code   string
	}{
		{"  umum  ", "SP-UMUM"},
		{"MATA", "SP-MATA"},
		{"Paru", "SP-PARU"},
		{"Penyakit Dalam", "SP-PENYAKIT-DALAM"},
		{" sp-anak ", "SP-ANAK"},
		{"Unknown specialty", ""},
		{"Mata / Anak", ""},
		{"Dokter Anak", ""},
	}
	ids := make([]string, len(fixtures))
	for i, fixture := range fixtures {
		ids[i] = uuid.NewString()
		if _, err := db.Exec(`INSERT INTO users (id,email,password_hash) VALUES ($1,$2,'integration-only')`, ids[i], ids[i]+"@specialty-migration.test"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO doctor_profiles (user_id,specialty) VALUES ($1,$2)`, ids[i], fixture.legacy); err != nil {
			t.Fatal(err)
		}
	}
	applyMigrations(t, db)
	for i, fixture := range fixtures {
		var legacy, code string
		if err := db.QueryRow(`SELECT profile.specialty_legacy, COALESCE(master.code,'')
			FROM doctor_profiles profile LEFT JOIN master_specialties master ON master.id=profile.specialty_id
			WHERE profile.user_id=$1`, ids[i]).Scan(&legacy, &code); err != nil {
			t.Fatal(err)
		}
		if legacy != fixture.legacy || code != fixture.code {
			t.Fatalf("legacy %q became archive=%q code=%q, want code=%q", fixture.legacy, legacy, code, fixture.code)
		}
		if _, err := db.Exec(`DELETE FROM users WHERE id=$1`, ids[i]); err != nil {
			t.Fatal(err)
		}
	}
}

// Call after normal development seeds. All mutations are rolled back.
func assertSpecialtyCatalogIntegration(t *testing.T, db *gorm.DB) {
	t.Helper()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := SeedMasterSpecialties(tx); err != nil {
		t.Fatal(err)
	}
	var firstID string
	if err := tx.Raw(`SELECT id FROM master_specialties WHERE code='SP-MATA'`).Scan(&firstID).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedMasterSpecialties(tx); err != nil {
		t.Fatal(err)
	}
	var secondID string
	if err := tx.Raw(`SELECT id FROM master_specialties WHERE code='SP-MATA'`).Scan(&secondID).Error; err != nil || firstID != secondID {
		t.Fatalf("idempotent seed changed master identity: %v", err)
	}
	unknownID := uuid.NewString()
	if err := tx.Exec(`INSERT INTO master_specialties(id,code,name,category,sort_order,is_active)
		VALUES (?, 'SP-TEST-LOCAL', 'Local %_ field', 'GENERAL', 999, FALSE)`, unknownID).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedMasterSpecialties(tx); err != nil {
		t.Fatal(err)
	}
	var unknownCount int64
	if err := tx.Table("master_specialties").Where("id=? AND NOT is_active", unknownID).Count(&unknownCount).Error; err != nil || unknownCount != 1 {
		t.Fatalf("seed must preserve unknown rows: %v", err)
	}
	repo := specialtyrepo.NewRepository(tx)
	svc := specialtyservice.NewService(repo)
	rows, err := svc.ListSpecialties(context.Background(), request.SpecialtyQuery{Search: "mAtA", Limit: 100})
	if err != nil || len(rows) != 1 || rows[0].Code != "SP-MATA" {
		t.Fatalf("canonical case-insensitive name search: rows=%v err=%v", rows, err)
	}
	rows, err = svc.ListSpecialties(context.Background(), request.SpecialtyQuery{Search: "SP-PARU", Limit: 100})
	if err != nil || len(rows) != 1 || rows[0].Code != "SP-PARU" {
		t.Fatalf("code search: rows=%v err=%v", rows, err)
	}
	rows, err = svc.ListSpecialties(context.Background(), request.SpecialtyQuery{Code: "SP-TEST-LOCAL", Limit: 100})
	if err != nil || rows == nil || len(rows) != 0 {
		t.Fatalf("inactive catalog must return nonnil empty list: rows=%v err=%v", rows, err)
	}
	rows, err = svc.ListSpecialties(context.Background(), request.SpecialtyQuery{Search: "%_", Limit: 100})
	if err != nil || len(rows) != 0 {
		t.Fatalf("search wildcards must be literal: rows=%v err=%v", rows, err)
	}
	var userID string
	if err := tx.Raw(`SELECT user_id FROM doctor_profiles ORDER BY user_id LIMIT 1`).Scan(&userID).Error; err != nil || userID == "" {
		t.Fatalf("seeded doctor profile prerequisite: %v", err)
	}
	for _, selection := range []struct {
		name string
		id   string
	}{
		{"inactive", unknownID},
		{"missing", uuid.NewString()},
	} {
		tx.SavePoint("specialty_invalid_selection")
		err := tx.Exec(`UPDATE doctor_profiles SET specialty_id=? WHERE user_id=?`, selection.id, userID).Error
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
			t.Fatalf("%s selection must fail DB guard: %v", selection.name, err)
		}
		tx.RollbackTo("specialty_invalid_selection")
	}
	if err := tx.Exec(`UPDATE doctor_profiles SET specialty_id=NULL WHERE user_id=?`, userID).Error; err != nil {
		t.Fatalf("nullable specialty selection: %v", err)
	}
	if err := tx.Exec(`UPDATE doctor_profiles SET specialty_id=? WHERE user_id=?`, firstID, userID).Error; err != nil {
		t.Fatalf("active specialty selection: %v", err)
	}
	if err := tx.Exec(`UPDATE master_specialties SET is_active=FALSE WHERE id=?`, firstID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`UPDATE doctor_profiles SET updated_at=NOW() WHERE user_id=?`, userID).Error; err != nil {
		t.Fatalf("unrelated update after deactivation: %v", err)
	}
	for _, invalidSQL := range []string{
		`INSERT INTO master_specialties(code,name,category,sort_order) VALUES ('SP-MATA','Duplicate','GENERAL',1)`,
		`INSERT INTO master_specialties(code,name,category,sort_order) VALUES ('sp-invalid','Invalid','GENERAL',1)`,
		`INSERT INTO master_specialties(code,name,category,sort_order) VALUES ('SP-INVALID','Invalid','SURGICAL_SPECIALIST',1)`,
	} {
		tx.SavePoint("specialty_invalid_master")
		if err := tx.Exec(invalidSQL).Error; err == nil {
			t.Fatalf("invalid master row accepted: %s", invalidSQL)
		}
		tx.RollbackTo("specialty_invalid_master")
	}
	router := gin.New()
	router.GET("/v1/specialties", specialtyhttp.NewController(svc).ListSpecialties)
	for _, query := range []string{"?q=", "?q=a&q=b", "?category=SURGICAL_SPECIALIST", "?code=sp-mata", "?limit=0", "?limit=101", "?offset=-1", "?q=%ZZ"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/specialties"+query, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid query %q returned %d", query, recorder.Code)
		}
	}

	// The runtime role can select specialties, but must never need master DML
	// privileges merely to update a doctor's choice.
	runtimeRole := "specialty_runtime_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	for _, statement := range []string{
		"CREATE ROLE " + runtimeRole + " NOLOGIN",
		"GRANT USAGE ON SCHEMA public TO " + runtimeRole,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + runtimeRole,
		"REVOKE INSERT, UPDATE, DELETE ON master_specialties FROM " + runtimeRole,
		"SET LOCAL ROLE " + runtimeRole,
	} {
		if err := tx.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	var activeID string
	if err := tx.Raw(`SELECT id FROM master_specialties WHERE code='SP-ANAK'`).Scan(&activeID).Error; err != nil {
		t.Fatal(err)
	}
	if err := userrepo.NewRepository(tx).ApplyUnifiedProfileUpdate(context.Background(), userID, userrepo.UnifiedProfileUpdate{
		DoctorFields: map[string]any{"specialty_id": activeID},
	}); err != nil {
		t.Fatalf("runtime selection with read-only master privileges: %v", err)
	}
	var selectedID string
	if err := tx.Raw(`SELECT specialty_id FROM doctor_profiles WHERE user_id=?`, userID).Scan(&selectedID).Error; err != nil || selectedID != activeID {
		t.Fatalf("runtime selection persisted %q, expected %q: %v", selectedID, activeID, err)
	}
	tx.SavePoint("specialty_runtime_readonly")
	err = tx.Exec(`UPDATE master_specialties SET name='Forbidden' WHERE id=?`, activeID).Error
	var permissionError *pgconn.PgError
	if !errors.As(err, &permissionError) || permissionError.Code != "42501" {
		t.Fatalf("runtime role must not modify master catalogue: %v", err)
	}
	tx.RollbackTo("specialty_runtime_readonly")
	if err := tx.Exec("RESET ROLE").Error; err != nil {
		t.Fatal(err)
	}
}
