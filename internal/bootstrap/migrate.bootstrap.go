package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/infrastructure"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func StartMigrate(actionType string, name string, version *int64) error {
	return StartMigrateContext(context.Background(), actionType, name, version)
}

func StartMigrateContext(ctx context.Context, actionType string, name string, version *int64) error {
	return runMigration(ctx, actionType, name, version)
}

func runMigration(ctx context.Context, actionType string, name string, version *int64) error {
	migrationDir := "migration/db"
	validAction := map[string]bool{
		"create": true, "up": true, "up-by-one": true, "up-to": true,
		"status": true,
	}
	if !validAction[actionType] {
		return errors.New("invalid migration command")
	}
	if actionType == "create" {
		name = strings.TrimSpace(name)
		if name == "" {
			return errors.New("migration name is required")
		}
		// goose.Create only writes a local file; it does not use the *sql.DB
		// parameter. Keep this path offline so creating a migration never opens
		// an admin connection or depends on PostgreSQL availability.
		if err := goose.Create(nil, migrationDir, name, "sql"); err != nil {
			return fmt.Errorf("migration create failed: %w", err)
		}
		return nil
	}

	dsn := config.Env.Database.DSN
	if config.Env.Database.AdminDSN != "" {
		dsn = config.Env.Database.AdminDSN
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return errors.New("open migration database")
	}
	defer db.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return errors.New("connect to migration database")
	}
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}

	switch actionType {
	case "up":
		err = goose.UpContext(ctx, db, migrationDir)
	case "up-by-one":
		err = goose.UpByOneContext(ctx, db, migrationDir)
	case "up-to":
		if version == nil || *version <= 0 {
			return errors.New("positive migration version is required")
		}
		err = goose.UpToContext(ctx, db, migrationDir, *version)
	case "status":
		err = goose.StatusContext(ctx, db, migrationDir)
	}

	if err != nil {
		return fmt.Errorf("migration %s failed: %w", actionType, err)
	}
	if requiresGuardedMigrationPostcondition(config.Env, actionType) {
		if err := verifyRequiredMigrationApplied(ctx, db); err != nil {
			return err
		}
	}
	return nil
}

func requiresGuardedMigrationPostcondition(cfg *config.EnvConfig, action string) bool {
	if cfg == nil {
		return false
	}
	environment := strings.ToLower(strings.TrimSpace(cfg.Env))
	if environment != "staging" && environment != "production" {
		return false
	}
	switch action {
	case "up", "up-by-one", "up-to":
		return true
	default:
		return false
	}
}

func verifyRequiredMigrationApplied(ctx context.Context, db *sql.DB) error {
	var applied bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM (
				SELECT DISTINCT ON (version_id) version_id, is_applied
				FROM public.goose_db_version
				WHERE version_id = $1
				ORDER BY version_id, id DESC
			) AS latest
			WHERE is_applied
		)
	`, infrastructure.RequiredDatabaseMigrationVersion).Scan(&applied); err != nil {
		return errors.New("verify required database migration after migrate")
	}
	if !applied {
		return fmt.Errorf(
			"required database migration %d is not applied; maintenance remains active",
			infrastructure.RequiredDatabaseMigrationVersion,
		)
	}
	return nil
}
