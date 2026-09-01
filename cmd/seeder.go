package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/bootstrap"
	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/infrastructure"
	"github.com/Cendana-Project/medikaone-api/internal/seeder"
)

const (
	confirmResetAll                         = "RESET-ALL-STAGING-DATA"
	confirmResetDemo                        = "RESET-DEMO-STAGING-DATA"
	stagingDatabaseFingerprintEnvName       = "STAGING_DATABASE_FINGERPRINT"
	stagingResetAdvisoryLockKey       int64 = 0x4d6564696b614f6e // "MedikaOn"
	maintenanceLeaseTTL                     = 2 * time.Minute
	maintenanceRenewInterval                = 30 * time.Second
	databaseAdminAdvisoryLockQuery          = "SELECT pg_try_advisory_xact_lock($1)"
)

var errDatabaseAdminOperationInProgress = errors.New("another database migration/reset operation is already running")

type databaseMaintenanceKind string

const (
	maintenanceKindMigration databaseMaintenanceKind = "migration"
	maintenanceKindResetAll  databaseMaintenanceKind = "reset-all"
	maintenanceKindResetSeed databaseMaintenanceKind = "reset-seed"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Synchronize initial and demo seed data",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireDemoSeedEnvironment(); err != nil {
			return err
		}
		db, closeDB, err := openSeederDB(config.Env.Database.DSN)
		if err != nil {
			return err
		}
		defer closeDB()
		return seeder.Run(db)
	},
}

func requireDemoSeedEnvironment() error {
	if config.Env == nil {
		return fmt.Errorf("refusing seed: ENV is not configured")
	}
	switch strings.ToLower(strings.TrimSpace(config.Env.Env)) {
	case "development", "test":
		return requireLocalDemoSeedTarget(config.Env.Database.DSN)
	default:
		return fmt.Errorf("refusing demo seed: ENV must be development or test; use a guarded reset command for staging")
	}
}

func requireLocalDemoSeedTarget(dsn string) error {
	cfg, err := pgx.ParseConfig(strings.TrimSpace(dsn))
	if err != nil || strings.TrimSpace(cfg.Host) == "" {
		return errors.New("refusing demo seed: database target is invalid")
	}
	if len(cfg.Fallbacks) != 0 {
		return errors.New("refusing demo seed: database target must not contain fallbacks")
	}
	host := strings.TrimSpace(cfg.Host)
	if strings.EqualFold(host, "localhost") || strings.HasPrefix(host, "/") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return errors.New("refusing demo seed: development/test DATABASE_DSN must target localhost, a loopback IP, or a local Unix socket")
}

func newStagingResetAllCmd() *cobra.Command {
	var confirmation string
	command := &cobra.Command{
		Use:   "staging-reset-all",
		Short: "Clear all staging data, migrate, and seed it again",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireStagingConfirmation(confirmation, confirmResetAll); err != nil {
				return err
			}
			if err := requireStagingDatabaseTarget(); err != nil {
				return err
			}

			return runWithMaintenanceAndDatabaseAdminLockOptions(cmd.Context(), true, databaseMaintenanceOptions{
				allowFailedTakeover:           true,
				additionalFailedTakeoverKinds: []databaseMaintenanceKind{maintenanceKindMigration, maintenanceKindResetSeed},
				kind:                          maintenanceKindResetAll,
				recoveryCommand:               "staging-reset-all",
				clearAuthOnSuccess:            true,
			}, func(operationCtx context.Context, adminDSN string) error {
				db, closeDB, err := openSeederDB(adminDSN)
				if err != nil {
					return err
				}
				defer closeDB()
				// Mark maintenance persistent before the first destructive commit.
				// If this process is interrupted at any point from here onward,
				// readiness and application traffic remain fail-closed until this
				// recovery command is rerun successfully.
				if err := retainDatabaseMaintenance(operationCtx); err != nil {
					return err
				}
				// Clear first so this recovery command can repair staging databases
				// whose legacy duplicate rows would intentionally block a hardening
				// migration. Each phase is transactional and the entire sequence is
				// protected by the dedicated transaction-scoped advisory lock.
				if err := seeder.ClearAllData(db.WithContext(operationCtx)); err != nil {
					return err
				}
				if err := bootstrap.StartMigrateContext(operationCtx, "up", "", nil); err != nil {
					return err
				}
				// Truncate once more in the same transaction as seeding. This removes
				// writes from direct clients or stale deployments during migration.
				return seeder.ResetAllAndSeed(db.WithContext(operationCtx))
			})
		},
	}
	command.Flags().StringVar(&confirmation, "confirm", "", "required destructive-operation confirmation token")
	return command
}

func newStagingResetSeedCmd() *cobra.Command {
	var confirmation string
	command := &cobra.Command{
		Use:   "staging-reset-seed",
		Short: "Recreate only recognized staging demo users and seed definitions",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireStagingConfirmation(confirmation, confirmResetDemo); err != nil {
				return err
			}
			if err := requireStagingDatabaseTarget(); err != nil {
				return err
			}
			return runWithMaintenanceAndDatabaseAdminLockOptions(cmd.Context(), true, databaseMaintenanceOptions{
				allowFailedTakeover: true,
				kind:                maintenanceKindResetSeed,
				recoveryCommand:     "staging-reset-seed",
			}, func(operationCtx context.Context, adminDSN string) error {
				db, closeDB, err := openSeederDB(adminDSN)
				if err != nil {
					return err
				}
				defer closeDB()
				// From this point onward both the migration and demo-data reset may
				// commit changes. Persist the typed sentinel first so a crash cannot
				// reopen application traffic to a partially updated database.
				if err := retainDatabaseMaintenance(operationCtx); err != nil {
					return err
				}
				if err := bootstrap.StartMigrateContext(operationCtx, "up", "", nil); err != nil {
					return err
				}
				return seeder.ResetDemoAndSeed(db.WithContext(operationCtx))
			})
		},
	}
	command.Flags().StringVar(&confirmation, "confirm", "", "required destructive-operation confirmation token")
	return command
}

func newDatabaseFingerprintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "database-fingerprint",
		Short: "Print a credential-free fingerprint for the configured database target",
		RunE: func(cmd *cobra.Command, args []string) error {
			dsn, err := databaseAdminDSN(true)
			if err != nil {
				return err
			}
			fingerprint, err := databaseTargetFingerprint(dsn)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), fingerprint)
			return err
		},
	}
}

func requireStagingDatabaseTarget() error {
	if err := requireMatchingApplicationAndAdminTargets(); err != nil {
		return err
	}
	expected := strings.TrimSpace(os.Getenv(stagingDatabaseFingerprintEnvName))
	if expected == "" {
		return fmt.Errorf("refusing reset: %s is required", stagingDatabaseFingerprintEnvName)
	}
	dsn, err := databaseAdminDSN(true)
	if err != nil {
		return err
	}
	actual, err := databaseTargetFingerprint(dsn)
	if err != nil {
		return fmt.Errorf("refusing reset: invalid database target")
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("refusing reset: configured database fingerprint does not match the staging target")
	}
	return nil
}

func databaseTargetFingerprint(rawDSN string) (string, error) {
	identity, err := databaseTargetIdentity(rawDSN, false)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])[:24], nil
}

func databaseTargetIdentity(rawDSN string, normalizeNeonPooler bool) (string, error) {
	return databaseTargetIdentityWithUser(rawDSN, normalizeNeonPooler, true)
}

// databaseTargetRoutingIdentity identifies where a DSN connects without
// requiring the application and migration connections to share credentials.
// The destructive-operation fingerprint still uses databaseTargetIdentity so
// it remains bound to the configured admin user.
func databaseTargetRoutingIdentity(rawDSN string, normalizeNeonPooler bool) (string, error) {
	return databaseTargetIdentityWithUser(rawDSN, normalizeNeonPooler, false)
}

func databaseTargetIdentityWithUser(rawDSN string, normalizeNeonPooler, includeUser bool) (string, error) {
	cfg, err := pgx.ParseConfig(strings.TrimSpace(rawDSN))
	if err != nil || cfg.Host == "" || cfg.Database == "" || cfg.User == "" {
		return "", fmt.Errorf("invalid database DSN")
	}

	targetSet := map[string]struct{}{
		normalizeDatabaseHost(cfg.Host, normalizeNeonPooler) + ":" + strconv.Itoa(int(cfg.Port)): {},
	}
	for _, fallback := range cfg.Fallbacks {
		targetSet[normalizeDatabaseHost(fallback.Host, normalizeNeonPooler)+":"+strconv.Itoa(int(fallback.Port))] = struct{}{}
	}
	targets := make([]string, 0, len(targetSet))
	for target := range targetSet {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	runtimeKeys := make([]string, 0, len(cfg.RuntimeParams))
	for key := range cfg.RuntimeParams {
		runtimeKeys = append(runtimeKeys, key)
	}
	sort.Strings(runtimeKeys)
	identityParts := []string{
		"targets=" + strings.Join(targets, ","),
		"database=" + cfg.Database,
	}
	if includeUser {
		identityParts = append(identityParts, "user="+cfg.User)
	}
	for _, key := range runtimeKeys {
		identityParts = append(identityParts, "runtime:"+key+"="+cfg.RuntimeParams[key])
	}
	identity := strings.Join(identityParts, "|")
	return identity, nil
}

func normalizeDatabaseHost(host string, normalizeNeonPooler bool) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if normalizeNeonPooler && strings.HasSuffix(host, ".neon.tech") {
		labels := strings.Split(host, ".")
		labels[0] = strings.TrimSuffix(labels[0], "-pooler")
		host = strings.Join(labels, ".")
	}
	return host
}

func requireMatchingApplicationAndAdminTargets() error {
	if config.Env == nil {
		return errors.New("refusing reset: database configuration is unavailable")
	}
	adminDSN, err := databaseAdminDSN(true)
	if err != nil {
		return err
	}
	appIdentity, err := databaseTargetRoutingIdentity(config.Env.Database.DSN, true)
	if err != nil {
		return errors.New("refusing reset: invalid application database target")
	}
	adminIdentity, err := databaseTargetRoutingIdentity(adminDSN, true)
	if err != nil {
		return errors.New("refusing reset: invalid database admin target")
	}
	if appIdentity != adminIdentity {
		return errors.New("refusing reset: DATABASE_ADMIN_DSN does not match the effective DATABASE_DSN target")
	}
	return nil
}

func requireStagingConfirmation(actual, expected string) error {
	if config.Env == nil || !strings.EqualFold(strings.TrimSpace(config.Env.Env), "staging") {
		return fmt.Errorf("refusing reset: ENV must be staging")
	}
	if actual != expected {
		return fmt.Errorf("refusing reset: pass --confirm=%s", expected)
	}
	return nil
}

func databaseAdminDSN(requireExplicit bool) (string, error) {
	if config.Env == nil {
		return "", errors.New("database configuration is unavailable")
	}
	dsn := strings.TrimSpace(config.Env.Database.AdminDSN)
	if dsn == "" {
		if requireExplicit {
			return "", errors.New("DATABASE_ADMIN_DSN with a direct/non-pooler PostgreSQL URL is required")
		}
		dsn = strings.TrimSpace(config.Env.Database.DSN)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || cfg.Host == "" || cfg.Database == "" || cfg.User == "" {
		return "", errors.New("invalid database admin DSN")
	}
	if len(cfg.Fallbacks) != 0 {
		return "", errors.New("database admin DSN must contain exactly one direct target without fallbacks")
	}
	hosts := []string{cfg.Host}
	for _, fallback := range cfg.Fallbacks {
		hosts = append(hosts, fallback.Host)
	}
	for _, host := range hosts {
		normalized := strings.ToLower(host)
		if strings.Contains(normalized, "-pooler") || strings.Contains(normalized, "pgbouncer") {
			return "", errors.New("database admin DSN must use a direct/non-pooler endpoint")
		}
	}
	return dsn, nil
}

func runWithDatabaseAdminLock(
	ctx context.Context,
	requireExplicitDSN bool,
	operation func(context.Context, string) error,
) error {
	dsn, err := databaseAdminDSN(requireExplicitDSN)
	if err != nil {
		return err
	}
	setupCtx, cancelSetup := context.WithTimeout(ctx, 10*time.Second)
	defer cancelSetup()
	guardConn, err := pgx.Connect(setupCtx, dsn)
	if err != nil {
		return errors.New("connect database admin lock")
	}
	defer func() {
		releaseCtx, cancelRelease := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelRelease()
		_ = guardConn.Close(releaseCtx)
	}()

	guardTx, err := guardConn.Begin(setupCtx)
	if err != nil {
		return errors.New("begin database admin guard transaction")
	}
	guardTxFinished := false
	defer func() {
		if guardTxFinished {
			return
		}
		rollbackCtx, cancelRollback := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelRollback()
		_ = guardTx.Rollback(rollbackCtx)
	}()
	if err := verifyConnectedAdminTarget(setupCtx, guardTx, dsn); err != nil {
		return err
	}

	var acquired bool
	if err := guardTx.QueryRow(
		setupCtx,
		databaseAdminAdvisoryLockQuery,
		stagingResetAdvisoryLockKey,
	).Scan(&acquired); err != nil {
		return errors.New("acquire database admin advisory lock")
	}
	if !acquired {
		return errDatabaseAdminOperationInProgress
	}
	cancelSetup()

	operationCtx, cancelOperation := context.WithCancel(ctx)
	defer cancelOperation()
	heartbeatCtx, stopHeartbeat := context.WithCancel(context.Background())
	heartbeatDone := make(chan struct{})
	lockLost := make(chan struct{}, 1)
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				pingCtx, cancelPing := context.WithTimeout(heartbeatCtx, 2*time.Second)
				_, err := guardTx.Exec(pingCtx, "SELECT 1")
				cancelPing()
				if err != nil {
					select {
					case lockLost <- struct{}{}:
					default:
					}
					cancelOperation()
					return
				}
			}
		}
	}()

	operationErr := operation(operationCtx, dsn)
	stopHeartbeat()
	<-heartbeatDone
	finalPingCtx, cancelFinalPing := context.WithTimeout(context.Background(), 2*time.Second)
	_, finalPingErr := guardTx.Exec(finalPingCtx, "SELECT 1")
	cancelFinalPing()
	select {
	case <-lockLost:
		return errors.Join(operationErr, errors.New("database admin lock connection was lost; operation canceled"))
	default:
	}
	if finalPingErr != nil {
		return errors.Join(operationErr, errors.New("database admin lock connection was lost before completion"))
	}

	commitCtx, cancelCommit := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelCommit()
	if err := guardTx.Commit(commitCtx); err != nil {
		return errors.Join(operationErr, errors.New("release database admin lock"))
	}
	guardTxFinished = true
	return operationErr
}

const (
	maintenanceRunningPrefix = "running:"
	maintenanceFailedPrefix  = "failed:"
)

var (
	acquireMaintenanceScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
  return 1
end
if current == ARGV[1] then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
  return 1
end
for i = 3, #ARGV do
  local failed_prefix = ARGV[i]
  if string.sub(current, 1, string.len(failed_prefix)) == failed_prefix then
    redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
    return 2
  end
end
return 0`)
	renewMaintenanceScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current == ARGV[1] then
  redis.call('PEXPIRE', KEYS[1], ARGV[3])
  return 1
end
if current == ARGV[2] then
  return 2
end
return 0`)
	retainMaintenanceScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current == ARGV[2] then
  return 1
end
if current == ARGV[1] then
  redis.call('SET', KEYS[1], ARGV[2])
  return 1
end
return 0`)
	releaseMaintenanceScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then
  return 1
end
if current == ARGV[1] or current == ARGV[2] then
  redis.call('DEL', KEYS[1])
  return 1
end
return 0`)
)

type databaseMaintenanceOptions struct {
	allowFailedTakeover           bool
	additionalFailedTakeoverKinds []databaseMaintenanceKind
	kind                          databaseMaintenanceKind
	recoveryCommand               string
	clearAuthOnSuccess            bool
}

type databaseMaintenanceLease struct {
	rdb          *redis.Client
	key          string
	runningValue string
	failedPrefix string
	failedValue  string
	ttl          time.Duration
}

type databaseMaintenanceContextKey struct{}

func newDatabaseMaintenanceLease(
	rdb *redis.Client,
	key string,
	kind databaseMaintenanceKind,
	owner string,
	ttl time.Duration,
) *databaseMaintenanceLease {
	runningPrefix := maintenanceRunningPrefix + string(kind) + ":"
	failedPrefix := maintenanceFailedPrefix + string(kind) + ":"
	return &databaseMaintenanceLease{
		rdb:          rdb,
		key:          key,
		runningValue: runningPrefix + owner,
		failedPrefix: failedPrefix,
		failedValue:  failedPrefix + owner,
		ttl:          ttl,
	}
}

// acquire returns true when this run recovered a persistent failed sentinel.
// Repeating the script with the same owner is safe after an ambiguous network
// response because it only renews that owner's lease.
func (l *databaseMaintenanceLease) acquire(
	ctx context.Context,
	allowFailedTakeover bool,
	additionalKinds ...databaseMaintenanceKind,
) (bool, error) {
	args := []any{l.runningValue, l.ttl.Milliseconds()}
	if allowFailedTakeover {
		args = append(args, l.failedPrefix)
		seen := map[string]struct{}{l.failedPrefix: {}}
		for _, kind := range additionalKinds {
			if !kind.valid() {
				return false, errors.New("invalid additional maintenance recovery kind")
			}
			prefix := maintenanceFailedPrefix + string(kind) + ":"
			if _, exists := seen[prefix]; exists {
				continue
			}
			seen[prefix] = struct{}{}
			args = append(args, prefix)
		}
	}
	result, err := acquireMaintenanceScript.Run(
		ctx,
		l.rdb,
		[]string{l.key},
		args...,
	).Int64()
	if err != nil {
		return false, errors.New("acquire maintenance lease")
	}
	switch result {
	case 1:
		return false, nil
	case 2:
		return true, nil
	default:
		return false, errors.New("another maintenance operation is already running or a matching recovery command is required")
	}
}

func (l *databaseMaintenanceLease) renew(ctx context.Context) error {
	result, err := renewMaintenanceScript.Run(
		ctx, l.rdb, []string{l.key}, l.runningValue, l.failedValue, l.ttl.Milliseconds(),
	).Int64()
	if err != nil || (result != 1 && result != 2) {
		return errors.New("maintenance lease was lost")
	}
	return nil
}

func (l *databaseMaintenanceLease) retain(ctx context.Context) error {
	result, err := retainMaintenanceScript.Run(
		ctx, l.rdb, []string{l.key}, l.runningValue, l.failedValue,
	).Int64()
	if err != nil || result != 1 {
		return errors.New("retain fail-closed database maintenance sentinel")
	}
	return nil
}

func (l *databaseMaintenanceLease) release(ctx context.Context) error {
	result, err := releaseMaintenanceScript.Run(
		ctx, l.rdb, []string{l.key}, l.runningValue, l.failedValue,
	).Int64()
	if err != nil || result != 1 {
		return errors.New("release database maintenance lease")
	}
	return nil
}

func retainDatabaseMaintenance(ctx context.Context) error {
	retain, ok := ctx.Value(databaseMaintenanceContextKey{}).(func(context.Context) error)
	if !ok || retain == nil {
		return errors.New("fail-closed database maintenance is unavailable")
	}
	return retain(ctx)
}

func runWithMaintenanceAndDatabaseAdminLockOptions(
	ctx context.Context,
	requireExplicitDSN bool,
	options databaseMaintenanceOptions,
	operation func(context.Context, string) error,
) error {
	if !options.kind.valid() {
		return errors.New("valid database maintenance kind is required")
	}
	if strings.TrimSpace(options.recoveryCommand) == "" {
		return errors.New("database maintenance recovery command is required")
	}
	if len(options.additionalFailedTakeoverKinds) > 0 && options.kind != maintenanceKindResetAll {
		return errors.New("only staging-reset-all may recover another maintenance operation kind")
	}
	// The database lock is acquired first. A recovery run can therefore take
	// over a persistent failed sentinel only after the previous process has
	// released (or lost) its transaction-scoped advisory lock.
	return runWithDatabaseAdminLock(ctx, requireExplicitDSN, func(lockCtx context.Context, adminDSN string) error {
		return runWithDatabaseMaintenance(lockCtx, options, func(maintenanceCtx context.Context) error {
			return operation(maintenanceCtx, adminDSN)
		})
	})
}

func (kind databaseMaintenanceKind) valid() bool {
	switch kind {
	case maintenanceKindMigration, maintenanceKindResetAll, maintenanceKindResetSeed:
		return true
	default:
		return false
	}
}

func runWithDatabaseMaintenance(
	ctx context.Context,
	options databaseMaintenanceOptions,
	operation func(context.Context) error,
) (returnErr error) {
	if config.Env == nil {
		return errors.New("maintenance configuration is unavailable")
	}
	opts, err := redis.ParseURL(config.Env.Redis.CacheDSN)
	if err != nil {
		return errors.New("invalid Redis maintenance configuration")
	}
	rdb := redis.NewClient(opts)
	defer rdb.Close()
	if err := infrastructure.VerifyRedisNoEviction(ctx, rdb); err != nil {
		return err
	}

	lease := newDatabaseMaintenanceLease(
		rdb,
		config.MaintenanceRedisKey(),
		options.kind,
		uuid.NewString(),
		maintenanceLeaseTTL,
	)
	tookOverFailed, err := lease.acquire(
		ctx,
		options.allowFailedTakeover,
		options.additionalFailedTakeoverKinds...,
	)
	if err != nil {
		return err
	}
	var failClosed atomic.Bool
	failClosed.Store(tookOverFailed)
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if returnErr != nil && failClosed.Load() {
			if retainErr := lease.retain(releaseCtx); retainErr != nil {
				returnErr = errors.Join(returnErr, databaseMaintenanceRetainedError(options.recoveryCommand), retainErr)
				return
			}
			returnErr = errors.Join(returnErr, databaseMaintenanceRetainedError(options.recoveryCommand))
			return
		}
		if releaseErr := lease.release(releaseCtx); releaseErr != nil {
			returnErr = errors.Join(returnErr, releaseErr)
		}
	}()

	drainTimeout := config.Env.Server.WriteTimeout + 5*time.Second
	if drainTimeout < 10*time.Second {
		drainTimeout = 10 * time.Second
	}
	if drainTimeout > time.Minute {
		drainTimeout = time.Minute
	}
	drainCtx, cancelDrain := context.WithTimeout(ctx, drainTimeout)
	defer cancelDrain()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		active, activeErr := rdb.Get(drainCtx, config.ActiveRequestsRedisKey()).Int64()
		if activeErr == redis.Nil || (activeErr == nil && active <= 0) {
			break
		}
		if activeErr != nil {
			return errors.New("inspect active requests during maintenance")
		}
		select {
		case <-drainCtx.Done():
			return errors.New("timed out waiting for active requests to drain")
		case <-ticker.C:
		}
	}
	operationCtx, cancelOperation := context.WithCancel(ctx)
	defer cancelOperation()
	renewalCtx, stopRenewal := context.WithCancel(context.Background())
	renewalDone := make(chan struct{})
	renewalErr := make(chan error, 1)
	go func() {
		defer close(renewalDone)
		ticker := time.NewTicker(maintenanceRenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-renewalCtx.Done():
				return
			case <-ticker.C:
				renewCtx, cancelRenew := context.WithTimeout(renewalCtx, 5*time.Second)
				err := lease.renew(renewCtx)
				cancelRenew()
				if err != nil {
					select {
					case renewalErr <- err:
					default:
					}
					cancelOperation()
					return
				}
			}
		}
	}()

	marker := func(markerCtx context.Context) error {
		failClosed.Store(true)
		return lease.retain(markerCtx)
	}
	operationCtx = context.WithValue(operationCtx, databaseMaintenanceContextKey{}, marker)
	operationErr := operation(operationCtx)
	if operationErr == nil && options.clearAuthOnSuccess {
		operationErr = clearRedisNamespaceExcept(
			operationCtx,
			rdb,
			config.AuthRedisKeyPrefix(),
			config.MaintenanceRedisKey(),
			config.ActiveRequestsRedisKey(),
		)
	}
	stopRenewal()
	<-renewalDone
	select {
	case err := <-renewalErr:
		return errors.Join(operationErr, err)
	default:
		return operationErr
	}
}

func clearRedisNamespaceExcept(
	ctx context.Context,
	rdb *redis.Client,
	prefix string,
	preservedKeys ...string,
) error {
	if rdb == nil || prefix == "" {
		return errors.New("clear Redis authentication namespace")
	}
	preserved := make(map[string]struct{}, len(preservedKeys))
	for _, key := range preservedKeys {
		if !strings.HasPrefix(key, prefix) {
			return errors.New("preserved Redis key is outside the authentication namespace")
		}
		preserved[key] = struct{}{}
	}

	var cursor uint64
	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, prefix+"*", 256).Result()
		if err != nil {
			return errors.New("scan Redis authentication namespace")
		}
		toDelete := make([]string, 0, len(keys))
		for _, key := range keys {
			if _, keep := preserved[key]; !keep {
				toDelete = append(toDelete, key)
			}
		}
		if len(toDelete) > 0 {
			if err := rdb.Del(ctx, toDelete...).Err(); err != nil {
				return errors.New("clear Redis authentication namespace")
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}

func databaseMaintenanceRetainedError(recoveryCommand string) error {
	return fmt.Errorf(
		"database maintenance remains active; rerun %s after fixing the failure",
		recoveryCommand,
	)
}

type pgxQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func verifyConnectedAdminTarget(ctx context.Context, conn pgxQueryRower, dsn string) error {
	cfg, err := pgx.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return errors.New("verify database admin target")
	}
	var databaseName, sessionUser, currentUser, schemaName string
	if err := conn.QueryRow(ctx, `
		SELECT current_database() AS database_name,
		       session_user AS session_user_name,
		       current_user AS current_user_name,
		       current_schema() AS schema_name
	`).Scan(&databaseName, &sessionUser, &currentUser, &schemaName); err != nil {
		return errors.New("verify database admin target")
	}
	if databaseName != cfg.Database || sessionUser != cfg.User ||
		currentUser != cfg.User || schemaName != "public" {
		return errors.New("connected database/user/schema does not match the configured public database admin target")
	}
	return nil
}

func openSeederDB(dsn string) (*gorm.DB, func(), error) {
	db, err := infrastructure.OpenIsolatedDBConn(dsn)
	if err != nil {
		return nil, func() {}, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, func() {}, fmt.Errorf("get database handle: %w", err)
	}
	return db, func() { _ = sqlDB.Close() }, nil
}

func init() {
	rootCmd.AddCommand(seedCmd)
	rootCmd.AddCommand(newStagingResetAllCmd())
	rootCmd.AddCommand(newStagingResetSeedCmd())
	rootCmd.AddCommand(newDatabaseFingerprintCmd())
}
