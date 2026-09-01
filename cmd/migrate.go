/*
Copyright © 2024 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/bootstrap"
	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/spf13/cobra"
)

// migrateCmd represents the migrate command
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "perform database migration",
	Long:  `perform database migration`,
	RunE: func(cmd *cobra.Command, args []string) error {
		action, _ := cmd.Flags().GetString("action")
		action = strings.ToLower(strings.TrimSpace(action))
		if action == "reset" {
			return fmt.Errorf("migrate reset is guarded; use staging-reset-all")
		}
		migrationName, _ := cmd.Flags().GetString("name")
		version, _ := cmd.Flags().GetInt64("version")
		if err := requireSafeMigrationAction(action); err != nil {
			return err
		}

		switch action {
		case "up", "up-by-one", "up-to":
			environment := strings.ToLower(strings.TrimSpace(config.Env.Env))
			requireExplicitAdminDSN := environment == "staging" || environment == "production"
			if config.Env.Database.AdminDSN != "" {
				if err := requireMatchingApplicationAndAdminTargets(); err != nil {
					return err
				}
			}
			operation := func(ctx context.Context, _ string) error {
				return bootstrap.StartMigrateContext(ctx, action, migrationName, &version)
			}
			if !requireExplicitAdminDSN {
				return runWithDatabaseAdminLock(cmd.Context(), false, operation)
			}
			return runWithMaintenanceAndDatabaseAdminLockOptions(
				cmd.Context(),
				true,
				databaseMaintenanceOptions{
					allowFailedTakeover: true,
					kind:                maintenanceKindMigration,
					recoveryCommand:     "migrate --action=up",
				},
				func(ctx context.Context, adminDSN string) error {
					// A migration may commit one or more versions independently. Make
					// maintenance persistent before it starts so a crash or later
					// migration failure cannot reopen traffic to a partial schema.
					if err := retainDatabaseMaintenance(ctx); err != nil {
						return err
					}
					return operation(ctx, adminDSN)
				},
			)
		default:
			return bootstrap.StartMigrateContext(cmd.Context(), action, migrationName, &version)
		}
	},
}

func requireSafeMigrationAction(action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "down", "down-to", "reset":
		return fmt.Errorf("refusing destructive migrate action %q; use a guarded reset command", action)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.PersistentFlags().String("action", "up", "action create|up|up-by-one|up-to|status")
	migrateCmd.PersistentFlags().Int64("version", 1, "version")
	migrateCmd.PersistentFlags().String("name", "", "migration name")
}
