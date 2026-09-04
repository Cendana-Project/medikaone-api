package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

// RequiredDatabaseMigrationVersion is the latest required schema version that every
// staging/production web process and guarded migration must observe before
// traffic can resume.
const RequiredDatabaseMigrationVersion int64 = 20260904150000

func OpenDBConn() (*gorm.DB, error) {
	safeLogging := config.Env.Env == constant.ProductionEnvironment || config.Env.Env == "staging"
	db, err := openDBConn(config.Env.Database, safeLogging, safeLogging)
	if err != nil {
		return nil, errors.New(redactConnectionError(err, config.Env.Database.DSN))
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, errors.New("failed to get database connection pool")
	}
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()
	if err := verifyDatabaseMigrationVersion(startupCtx, db); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	DB = db
	registerHealthCheck("database", func(ctx context.Context) error {
		if err := sqlDB.PingContext(ctx); err != nil {
			return errors.New("database unavailable")
		}
		if err := verifyDatabaseMigrationVersion(ctx, db); err != nil {
			return errors.New("database schema is not ready")
		}
		return nil
	})

	logrus.Info("database connection established")
	return db, nil
}

// OpenIsolatedDBConn opens a pool without replacing the server's package-global
// pool or health check. Administrative commands use it with their direct DSN.
func OpenIsolatedDBConn(dsn string) (*gorm.DB, error) {
	cfg := config.Env.Database
	cfg.DSN = dsn
	db, err := openDBConn(cfg, true, false)
	if err != nil {
		return nil, errors.New(redactConnectionError(err, dsn))
	}
	return db, nil
}

func CloseDB() error {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func openDBConn(cfg config.Database, safeLogging, requireRestrictedUser bool) (*gorm.DB, error) {
	logLevel := gormLogLevel(config.Env.LogLevel)
	if safeLogging && logLevel > gormLogger.Warn {
		logLevel = gormLogger.Warn
	}
	logger := gormLogger.New(log.New(os.Stdout, "", log.LstdFlags), gormLogger.Config{
		SlowThreshold:             500 * time.Millisecond,
		LogLevel:                  logLevel,
		IgnoreRecordNotFoundError: true,
		ParameterizedQueries:      safeLogging,
		Colorful:                  !safeLogging,
	})

	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		PrepareStmt:    true,
		TranslateError: true,
		Logger:         logger,
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.MaxConnLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := verifyConnectedDatabaseTarget(ctx, db, cfg.DSN, requireRestrictedUser); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	// database/sql already reconnects broken connections and preserves the pool
	// referenced by repositories. Replacing a package-global *gorm.DB would leave
	// those repositories pointing at a stale pool.
	return db, nil
}

func verifyConnectedDatabaseTarget(ctx context.Context, db *gorm.DB, dsn string, requireRestrictedUser bool) error {
	parsed, err := pgx.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return errors.New("verify connected database target")
	}
	var result struct {
		DatabaseName              string
		SessionUser               string
		CurrentUser               string
		SchemaName                string
		CanCreate                 bool
		ControlsPublicTableOwner  bool
		CanTruncatePublicTable    bool
		CanModifyMigrationHistory bool
		CanUseMigrationSequence   bool
	}
	err = db.WithContext(ctx).Raw(`
		SELECT current_database() AS database_name,
		       session_user AS session_user,
		       current_user AS current_user,
		       current_schema() AS schema_name,
		       has_schema_privilege(current_user, 'public', 'CREATE') AS can_create,
		       EXISTS (
		           SELECT 1
		           FROM pg_catalog.pg_class AS c
		           JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
		           WHERE n.nspname = 'public'
		             AND c.relkind IN ('r', 'p')
		             AND pg_catalog.pg_has_role(current_user, c.relowner, 'MEMBER')
		       ) AS controls_public_table_owner,
		       EXISTS (
		           SELECT 1
		           FROM pg_catalog.pg_class AS c
		           JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
		           WHERE n.nspname = 'public'
		             AND c.relkind IN ('r', 'p')
		             AND pg_catalog.has_table_privilege(current_user, c.oid, 'TRUNCATE')
		       ) AS can_truncate_public_table,
		       EXISTS (
		           SELECT 1
		           FROM pg_catalog.pg_class AS c
		           JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
		           WHERE n.nspname = 'public'
		             AND c.relname = 'goose_db_version'
		             AND c.relkind IN ('r', 'p')
		             AND (
		                 pg_catalog.has_table_privilege(current_user, c.oid, 'INSERT')
		                 OR pg_catalog.has_table_privilege(current_user, c.oid, 'UPDATE')
		                 OR pg_catalog.has_table_privilege(current_user, c.oid, 'DELETE')
		             )
		       ) AS can_modify_migration_history,
		       EXISTS (
		           SELECT 1
		           FROM pg_catalog.pg_class AS sequence
		           JOIN pg_catalog.pg_depend AS dependency
		             ON dependency.classid = 'pg_catalog.pg_class'::regclass
		            AND dependency.objid = sequence.oid
		            AND dependency.deptype IN ('a', 'i')
		           JOIN pg_catalog.pg_class AS owner_table
		             ON dependency.refclassid = 'pg_catalog.pg_class'::regclass
		            AND dependency.refobjid = owner_table.oid
		           JOIN pg_catalog.pg_namespace AS owner_namespace
		             ON owner_namespace.oid = owner_table.relnamespace
		           WHERE sequence.relkind = 'S'
		             AND owner_namespace.nspname = 'public'
		             AND owner_table.relname = 'goose_db_version'
		             AND (
		                 pg_catalog.has_sequence_privilege(current_user, sequence.oid, 'USAGE')
		                 OR pg_catalog.has_sequence_privilege(current_user, sequence.oid, 'SELECT')
		                 OR pg_catalog.has_sequence_privilege(current_user, sequence.oid, 'UPDATE')
		             )
		       ) AS can_use_migration_sequence
	`).Scan(&result).Error
	if err != nil {
		return errors.New("verify connected database target")
	}
	if result.DatabaseName != parsed.Database || result.SessionUser != parsed.User ||
		result.CurrentUser != parsed.User || result.SchemaName != "public" {
		return fmt.Errorf("connected database/user/schema does not match configured public target")
	}
	if err := validateRuntimeDatabasePrivileges(
		requireRestrictedUser,
		result.CanCreate,
		result.ControlsPublicTableOwner,
		result.CanTruncatePublicTable,
		result.CanModifyMigrationHistory,
		result.CanUseMigrationSequence,
	); err != nil {
		return err
	}
	return nil
}

func validateRuntimeDatabasePrivileges(
	restricted, canCreate, controlsTableOwner, canTruncate, canModifyMigrationHistory, canUseMigrationSequence bool,
) error {
	if !restricted {
		return nil
	}
	if canCreate {
		return errors.New("runtime database user must not have CREATE privilege on schema public")
	}
	if controlsTableOwner {
		return errors.New("runtime database user must not own or be a member of an owner role for tables in schema public")
	}
	if canTruncate {
		return errors.New("runtime database user must not have TRUNCATE privilege on tables in schema public")
	}
	if canModifyMigrationHistory {
		return errors.New("runtime database user must have read-only access to public.goose_db_version")
	}
	if canUseMigrationSequence {
		return errors.New("runtime database user must not have privileges on sequences owned by public.goose_db_version")
	}
	return nil
}

func verifyDatabaseMigrationVersion(ctx context.Context, db *gorm.DB) error {
	var state struct {
		Version         int64
		RequiredApplied bool
	}
	if err := db.WithContext(ctx).Raw(`
		WITH latest AS (
			SELECT DISTINCT ON (version_id) version_id, is_applied
			FROM public.goose_db_version
			WHERE version_id > 0
			ORDER BY version_id, id DESC
		)
		SELECT COALESCE(MAX(version_id) FILTER (WHERE is_applied), 0)::bigint AS version,
		       COALESCE(BOOL_OR(version_id = ? AND is_applied), FALSE) AS required_applied
		FROM latest
	`, RequiredDatabaseMigrationVersion).Scan(&state).Error; err != nil {
		return errors.New("required database migration history is unavailable")
	}
	return validateDatabaseMigrationState(state.Version, state.RequiredApplied)
}

func validateDatabaseMigrationState(version int64, requiredApplied bool) error {
	if !requiredApplied {
		return fmt.Errorf("required database migration %d is not applied", RequiredDatabaseMigrationVersion)
	}
	return validateDatabaseMigrationVersion(version)
}

func validateDatabaseMigrationVersion(version int64) error {
	if version < RequiredDatabaseMigrationVersion {
		return fmt.Errorf(
			"database migration version %d is older than required version %d",
			version,
			RequiredDatabaseMigrationVersion,
		)
	}
	return nil
}

func gormLogLevel(level string) gormLogger.LogLevel {
	switch level {
	case "panic", "fatal", "error":
		return gormLogger.Error
	case "warn", "warning":
		return gormLogger.Warn
	case "debug", "trace":
		return gormLogger.Info
	default:
		return gormLogger.Warn
	}
}
