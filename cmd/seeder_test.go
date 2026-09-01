package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

func TestRequireStagingConfirmation(t *testing.T) {
	previous := config.Env
	t.Cleanup(func() { config.Env = previous })

	config.Env = &config.EnvConfig{Env: "production"}
	if err := requireStagingConfirmation(confirmResetAll, confirmResetAll); err == nil {
		t.Fatal("production reset must be rejected")
	}

	config.Env = &config.EnvConfig{Env: "staging"}
	if err := requireStagingConfirmation("wrong", confirmResetAll); err == nil {
		t.Fatal("incorrect confirmation must be rejected")
	}
	if err := requireStagingConfirmation(confirmResetAll, confirmResetAll); err != nil {
		t.Fatalf("valid staging confirmation rejected: %v", err)
	}
	if err := requireStagingConfirmation(confirmResetDemo, confirmResetDemo); err != nil {
		t.Fatalf("valid demo reset confirmation rejected: %v", err)
	}
}

func TestRequireStagingDatabaseTarget(t *testing.T) {
	previous := config.Env
	t.Cleanup(func() { config.Env = previous })
	config.Env = &config.EnvConfig{Database: config.Database{
		DSN:      "postgresql://staging_user:secret@ep-staging-pooler.us-east-1.aws.neon.tech:5432/medikaone?sslmode=require",
		AdminDSN: "postgresql://staging_user:secret@ep-staging.us-east-1.aws.neon.tech:5432/medikaone?sslmode=require",
	}}

	fingerprint, err := databaseTargetFingerprint(config.Env.Database.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(stagingDatabaseFingerprintEnvName, fingerprint)
	if err := requireStagingDatabaseTarget(); err != nil {
		t.Fatalf("matching fingerprint rejected: %v", err)
	}

	t.Setenv(stagingDatabaseFingerprintEnvName, "000000000000000000000000")
	if err := requireStagingDatabaseTarget(); err == nil {
		t.Fatal("mismatched database fingerprint must be rejected")
	}
}

func TestRequireStagingDatabaseTargetRejectsMismatchedAdminTarget(t *testing.T) {
	previous := config.Env
	t.Cleanup(func() { config.Env = previous })
	config.Env = &config.EnvConfig{Database: config.Database{
		DSN:      "postgresql://staging_user:secret@app-pooler.example.com:5432/medikaone?sslmode=require",
		AdminDSN: "postgresql://staging_user:secret@production.example.com:5432/medikaone?sslmode=require",
	}}
	fingerprint, err := databaseTargetFingerprint(config.Env.Database.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(stagingDatabaseFingerprintEnvName, fingerprint)
	if err := requireStagingDatabaseTarget(); err == nil {
		t.Fatal("admin DSN for another database target must be rejected even when its fingerprint is configured")
	}
}

func TestDatabaseAdminDSNRejectsPooler(t *testing.T) {
	previous := config.Env
	t.Cleanup(func() { config.Env = previous })
	config.Env = &config.EnvConfig{Database: config.Database{
		AdminDSN: "postgresql://owner:secret@ep-example-pooler.example.com:5432/medikaone?sslmode=require",
	}}
	if _, err := databaseAdminDSN(true); err == nil {
		t.Fatal("pooled admin endpoint must be rejected")
	}
}

func TestDatabaseAdminDSNRejectsFallbackTargets(t *testing.T) {
	previous := config.Env
	t.Cleanup(func() { config.Env = previous })
	config.Env = &config.EnvConfig{Database: config.Database{
		AdminDSN: "postgresql://owner:secret@staging.example.com:5432,production.example.com:5432/medikaone?sslmode=require",
	}}
	if _, err := databaseAdminDSN(true); err == nil || !strings.Contains(err.Error(), "without fallbacks") {
		t.Fatalf("databaseAdminDSN() error = %v, want fallback rejection", err)
	}
}

func TestDatabaseTargetMatchAllowsSeparateUsersButIncludesRuntimeParameters(t *testing.T) {
	base := "postgresql://owner:secret@ep-example-pooler.us-east-1.aws.neon.tech:5432/medikaone?sslmode=require&search_path=public"
	direct := "postgresql://migration_owner:other@ep-example.us-east-1.aws.neon.tech:5432/medikaone?sslmode=require&search_path=public"
	baseIdentity, err := databaseTargetRoutingIdentity(base, true)
	if err != nil {
		t.Fatal(err)
	}
	directIdentity, err := databaseTargetRoutingIdentity(direct, true)
	if err != nil {
		t.Fatal(err)
	}
	if baseIdentity != directIdentity {
		t.Fatal("equivalent pooled/direct targets should match regardless of database user or password")
	}
	nonNeonPooler, err := databaseTargetRoutingIdentity(
		"postgresql://owner:secret@database-pooler.example.com:5432/medikaone?sslmode=require&search_path=public",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if nonNeonPooler == baseIdentity || normalizeDatabaseHost("database-pooler.example.com", true) == "database.example.com" {
		t.Fatal("pooler hostname normalization must be restricted to Neon endpoints")
	}
	for _, other := range []string{
		"postgresql://owner:secret@ep-example.us-east-1.aws.neon.tech:5432/other?sslmode=require&search_path=public",
		"postgresql://owner:secret@ep-example.us-east-1.aws.neon.tech:5432/medikaone?sslmode=require&search_path=private",
	} {
		identity, parseErr := databaseTargetRoutingIdentity(other, true)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if identity == baseIdentity {
			t.Fatalf("different effective target matched: %s", other)
		}
	}
}

func TestRequireMatchingApplicationAndAdminTargetsAllowsSeparateUsers(t *testing.T) {
	previous := config.Env
	t.Cleanup(func() { config.Env = previous })
	config.Env = &config.EnvConfig{Database: config.Database{
		DSN:      "postgresql://app_user:secret@ep-example-pooler.us-east-1.aws.neon.tech:5432/medikaone?sslmode=require&search_path=public",
		AdminDSN: "postgresql://migration_owner:other@ep-example.us-east-1.aws.neon.tech:5432/medikaone?sslmode=require&search_path=public",
	}}

	if err := requireMatchingApplicationAndAdminTargets(); err != nil {
		t.Fatalf("least-privilege application/admin users were rejected: %v", err)
	}
}

func TestDatabaseTargetFingerprintDoesNotDependOnPassword(t *testing.T) {
	one, err := databaseTargetFingerprint("postgresql://owner:first@db.example.com:5432/medikaone?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	two, err := databaseTargetFingerprint("postgresql://owner:second@db.example.com:5432/medikaone?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	if one != two {
		t.Fatal("credential rotation must not change the database target fingerprint")
	}
}

func TestDatabaseTargetFingerprintRemainsBoundToAdminUser(t *testing.T) {
	one, err := databaseTargetFingerprint("postgresql://owner_a:secret@db.example.com:5432/medikaone?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	two, err := databaseTargetFingerprint("postgresql://owner_b:secret@db.example.com:5432/medikaone?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	if one == two {
		t.Fatal("database fingerprint must remain bound to the configured admin user")
	}
}

func TestVerifyConnectedAdminTargetRemainsBoundToAdminUser(t *testing.T) {
	dsn := "postgresql://migration_owner:secret@db.example.com:5432/medikaone?sslmode=require"
	wrongUser := staticPGXQueryRower{values: []string{"medikaone", "app_user", "app_user", "public"}}
	if err := verifyConnectedAdminTarget(context.Background(), wrongUser, dsn); err == nil {
		t.Fatal("connected application user must not satisfy admin connection verification")
	}

	adminUser := staticPGXQueryRower{values: []string{"medikaone", "migration_owner", "migration_owner", "public"}}
	if err := verifyConnectedAdminTarget(context.Background(), adminUser, dsn); err != nil {
		t.Fatalf("configured admin connection was rejected: %v", err)
	}
}

type staticPGXQueryRower struct {
	values []string
}

func (s staticPGXQueryRower) QueryRow(context.Context, string, ...any) pgx.Row {
	return staticPGXRow{values: s.values}
}

type staticPGXRow struct {
	values []string
}

func (s staticPGXRow) Scan(dest ...any) error {
	if len(dest) != len(s.values) {
		return fmt.Errorf("scan destination count %d, want %d", len(dest), len(s.values))
	}
	for i, value := range s.values {
		ptr, ok := dest[i].(*string)
		if !ok {
			return fmt.Errorf("scan destination %d is not *string", i)
		}
		*ptr = value
	}
	return nil
}

func TestDatabaseTargetFingerprintUsesEffectivePGXTarget(t *testing.T) {
	base, err := databaseTargetFingerprint("postgresql://owner:secret@db.example.com:5432/medikaone?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	variants := []string{
		"postgresql://owner:secret@db.example.com:5432/medikaone?sslmode=require&host=other.example.com",
		"postgresql://owner:secret@db.example.com:5432/medikaone?sslmode=require&port=6543",
		"postgresql://owner:secret@db.example.com:5432/medikaone?sslmode=require&dbname=other",
		"postgresql://owner:secret@db.example.com:5432/medikaone?sslmode=require&user=other",
		"postgresql://owner:secret@db.example.com:5432/medikaone?sslmode=require&search_path=private",
		"postgresql://owner:secret@db.example.com:5432/medikaone?sslmode=require&options=-csearch_path%3Dprivate",
	}
	for _, variant := range variants {
		got, err := databaseTargetFingerprint(variant)
		if err != nil {
			t.Fatalf("fingerprint variant: %v", err)
		}
		if got == base {
			t.Fatalf("effective target override did not change fingerprint for %q", variant)
		}
	}
}

func TestRequireDemoSeedEnvironment(t *testing.T) {
	previous := config.Env
	t.Cleanup(func() { config.Env = previous })

	config.Env = &config.EnvConfig{Env: "production"}
	if err := requireDemoSeedEnvironment(); err == nil {
		t.Fatal("production demo seed must be rejected")
	}
	config.Env = &config.EnvConfig{Env: "staging"}
	if err := requireDemoSeedEnvironment(); err == nil {
		t.Fatal("unguarded staging demo seed must be rejected")
	}
	for _, environment := range []string{"development", "test"} {
		config.Env = &config.EnvConfig{Env: environment, Database: config.Database{
			DSN: "postgresql://postgres:postgres@localhost:5432/medikaone?sslmode=disable",
		}}
		if err := requireDemoSeedEnvironment(); err != nil {
			t.Fatalf("%s seed was rejected: %v", environment, err)
		}
	}
}

func TestRequireDemoSeedEnvironmentRejectsRemoteOrFallbackDatabase(t *testing.T) {
	previous := config.Env
	t.Cleanup(func() { config.Env = previous })

	for _, dsn := range []string{
		"postgresql://owner:secret@staging.example.com:5432/medikaone?sslmode=require",
		"postgresql://owner:secret@localhost:5432,staging.example.com:5432/medikaone?sslmode=require",
	} {
		config.Env = &config.EnvConfig{Env: "development", Database: config.Database{DSN: dsn}}
		if err := requireDemoSeedEnvironment(); err == nil {
			t.Fatalf("remote/fallback demo seed target was accepted: %s", dsn)
		}
	}
}

func TestDatabaseMaintenanceLeaseOwnershipAndRecovery(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_REDIS_DSN"))
	if dsn == "" {
		t.Skip("TEST_REDIS_DSN is not set")
	}
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		t.Fatalf("parse TEST_REDIS_DSN: %v", err)
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("TEST_REDIS_DSN is not reachable: %v", err)
	}

	key := "medikaone:test:maintenance:" + uuid.NewString()
	t.Cleanup(func() { _ = rdb.Del(context.Background(), key).Err() })
	first := newDatabaseMaintenanceLease(rdb, key, maintenanceKindResetAll, "first-owner", 10*time.Second)
	second := newDatabaseMaintenanceLease(rdb, key, maintenanceKindResetAll, "second-owner", 10*time.Second)
	migration := newDatabaseMaintenanceLease(rdb, key, maintenanceKindMigration, "migration-owner", 10*time.Second)
	resetSeed := newDatabaseMaintenanceLease(rdb, key, maintenanceKindResetSeed, "reset-seed-owner", 10*time.Second)

	tookOver, err := first.acquire(ctx, false)
	if err != nil || tookOver {
		t.Fatalf("first acquire = takeover %v, error %v", tookOver, err)
	}
	// An ambiguous response can safely repeat the same owner's acquire.
	if tookOver, err := first.acquire(ctx, false); err != nil || tookOver {
		t.Fatalf("idempotent acquire = takeover %v, error %v", tookOver, err)
	}
	if _, err := second.acquire(ctx, false); err == nil {
		t.Fatal("second owner acquired a live maintenance lease")
	}
	if err := first.renew(ctx); err != nil {
		t.Fatalf("renew owned lease: %v", err)
	}

	if err := first.retain(ctx); err != nil {
		t.Fatalf("retain failed sentinel: %v", err)
	}
	if ttl, err := rdb.TTL(ctx, key).Result(); err != nil || ttl != -1 {
		t.Fatalf("failed sentinel TTL = %v, error %v; want persistent", ttl, err)
	}
	if _, err := second.acquire(ctx, false); err == nil {
		t.Fatal("normal operation took over a failed sentinel")
	}
	if _, err := migration.acquire(ctx, true); err == nil {
		t.Fatal("migration took over a reset-all failed sentinel")
	}
	if _, err := resetSeed.acquire(ctx, true); err == nil {
		t.Fatal("reset-seed took over a reset-all failed sentinel")
	}
	tookOver, err = second.acquire(ctx, true)
	if err != nil || !tookOver {
		t.Fatalf("recovery acquire = takeover %v, error %v", tookOver, err)
	}
	if err := first.release(ctx); err == nil {
		t.Fatal("previous owner released the recovery owner's lease")
	}
	if err := second.retain(ctx); err != nil {
		t.Fatalf("recovery retain: %v", err)
	}
	if err := second.release(ctx); err != nil {
		t.Fatalf("release recovery sentinel: %v", err)
	}
	if err := second.release(ctx); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	if exists, err := rdb.Exists(ctx, key).Result(); err != nil || exists != 0 {
		t.Fatalf("maintenance key exists = %d, error %v", exists, err)
	}

	if tookOver, err := migration.acquire(ctx, true); err != nil || tookOver {
		t.Fatalf("migration acquire = takeover %v, error %v", tookOver, err)
	}
	if err := migration.retain(ctx); err != nil {
		t.Fatalf("retain migration sentinel: %v", err)
	}
	if _, err := second.acquire(ctx, true); err == nil {
		t.Fatal("reset-all took over a migration sentinel without explicit cross-kind recovery")
	}
	if tookOver, err := second.acquire(ctx, true, maintenanceKindMigration); err != nil || !tookOver {
		t.Fatalf("guarded reset-all migration recovery = takeover %v, error %v", tookOver, err)
	}
	if err := second.release(ctx); err != nil {
		t.Fatalf("release reset-all migration recovery: %v", err)
	}
	if tookOver, err := migration.acquire(ctx, true); err != nil || tookOver {
		t.Fatalf("migration reacquire = takeover %v, error %v", tookOver, err)
	}
	if err := migration.retain(ctx); err != nil {
		t.Fatalf("retain migration sentinel again: %v", err)
	}
	migrationRecovery := newDatabaseMaintenanceLease(
		rdb, key, maintenanceKindMigration, "migration-recovery-owner", 10*time.Second,
	)
	if tookOver, err := migrationRecovery.acquire(ctx, true); err != nil || !tookOver {
		t.Fatalf("migration recovery acquire = takeover %v, error %v", tookOver, err)
	}
	if err := migrationRecovery.release(ctx); err != nil {
		t.Fatalf("release migration recovery sentinel: %v", err)
	}
}

func TestDatabaseMaintenanceValuesAreSeparatedByKindAndOwner(t *testing.T) {
	resetAll := newDatabaseMaintenanceLease(nil, "key", maintenanceKindResetAll, "same-owner", time.Minute)
	migration := newDatabaseMaintenanceLease(nil, "key", maintenanceKindMigration, "same-owner", time.Minute)
	resetSeed := newDatabaseMaintenanceLease(nil, "key", maintenanceKindResetSeed, "same-owner", time.Minute)

	values := map[string]struct{}{}
	for _, lease := range []*databaseMaintenanceLease{resetAll, migration, resetSeed} {
		if !strings.HasSuffix(lease.runningValue, ":same-owner") ||
			!strings.HasSuffix(lease.failedValue, ":same-owner") {
			t.Fatalf("maintenance values are not owner-aware: running=%q failed=%q", lease.runningValue, lease.failedValue)
		}
		values[lease.runningValue] = struct{}{}
		values[lease.failedValue] = struct{}{}
	}
	if len(values) != 6 {
		t.Fatalf("maintenance values are not kind-separated: %v", values)
	}
	if got := resetAll.failedPrefix; got != "failed:reset-all:" {
		t.Fatalf("reset-all failed prefix = %q", got)
	}
}

func TestDatabaseMaintenanceKindValidation(t *testing.T) {
	for _, kind := range []databaseMaintenanceKind{
		maintenanceKindMigration,
		maintenanceKindResetAll,
		maintenanceKindResetSeed,
	} {
		if !kind.valid() {
			t.Fatalf("known maintenance kind %q was rejected", kind)
		}
	}
	for _, kind := range []databaseMaintenanceKind{"", "reset", "reset-all:other"} {
		if kind.valid() {
			t.Fatalf("unknown maintenance kind %q was accepted", kind)
		}
	}
}

func TestClearRedisNamespaceExcept(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_REDIS_DSN"))
	if dsn == "" {
		t.Skip("TEST_REDIS_DSN is not set")
	}
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		t.Fatalf("parse TEST_REDIS_DSN: %v", err)
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	prefix := "medikaone:test:clear:" + uuid.NewString() + ":"
	maintenanceKey := prefix + "maintenance"
	activeKey := prefix + "active-requests"
	staleKeys := []string{prefix + "registration:one", prefix + "refresh:two", prefix + "rate:three"}
	allKeys := append(append([]string{}, staleKeys...), maintenanceKey, activeKey)
	t.Cleanup(func() { _ = rdb.Del(context.Background(), allKeys...).Err() })
	for _, key := range allKeys {
		if err := rdb.Set(ctx, key, "value", time.Minute).Err(); err != nil {
			t.Fatalf("seed Redis key %q: %v", key, err)
		}
	}

	if err := clearRedisNamespaceExcept(ctx, rdb, prefix, maintenanceKey, activeKey); err != nil {
		t.Fatalf("clearRedisNamespaceExcept() error = %v", err)
	}
	if count, err := rdb.Exists(ctx, staleKeys...).Result(); err != nil || count != 0 {
		t.Fatalf("stale namespace keys remaining = %d, error %v", count, err)
	}
	if count, err := rdb.Exists(ctx, maintenanceKey, activeKey).Result(); err != nil || count != 2 {
		t.Fatalf("preserved keys remaining = %d, error %v; want 2", count, err)
	}
}

func TestDatabaseAdminTransactionLockIsExclusiveAndReleases(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	firstConn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect first TEST_DATABASE_DSN session: %v", err)
	}
	t.Cleanup(func() { _ = firstConn.Close(context.Background()) })
	secondConn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect second TEST_DATABASE_DSN session: %v", err)
	}
	t.Cleanup(func() { _ = secondConn.Close(context.Background()) })

	firstTx, err := firstConn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first guard transaction: %v", err)
	}
	defer func() { _ = firstTx.Rollback(context.Background()) }()
	secondTx, err := secondConn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin second guard transaction: %v", err)
	}
	defer func() { _ = secondTx.Rollback(context.Background()) }()

	lockKey := stagingResetAdvisoryLockKey + 1
	var acquired bool
	if err := firstTx.QueryRow(ctx, databaseAdminAdvisoryLockQuery, lockKey).Scan(&acquired); err != nil || !acquired {
		t.Fatalf("first transaction acquire = %v, error %v", acquired, err)
	}
	if err := secondTx.QueryRow(ctx, databaseAdminAdvisoryLockQuery, lockKey).Scan(&acquired); err != nil || acquired {
		t.Fatalf("concurrent transaction acquire = %v, error %v; want false", acquired, err)
	}
	if err := firstTx.Commit(ctx); err != nil {
		t.Fatalf("commit first guard transaction: %v", err)
	}
	if err := secondTx.QueryRow(ctx, databaseAdminAdvisoryLockQuery, lockKey).Scan(&acquired); err != nil || !acquired {
		t.Fatalf("acquire after transaction release = %v, error %v", acquired, err)
	}
}
