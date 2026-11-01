package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/seeder"
	"github.com/Cendana-Project/medikaone-api/internal/storage"
)

type demoSeedOptions struct {
	withWorkflows bool
	outputPath    string
	baseURL       string
	contracts     storage.Client
	medical       storage.Client
}

func addDemoSeedFlags(command *cobra.Command) {
	command.Flags().Bool("with-workflows", false, "Seed genuine practice, appointment, clinical, and PDF-backed prescription scenarios; requires configured Supabase storage")
	command.Flags().String("bruno-env", "", "Write actual demo fixture variables to this local Bruno environment YAML file (requires --with-workflows; overwrites the specified file)")
}

func prepareDemoSeed(command *cobra.Command) (demoSeedOptions, error) {
	var options demoSeedOptions
	options.withWorkflows, _ = command.Flags().GetBool("with-workflows")
	options.outputPath, _ = command.Flags().GetString("bruno-env")
	options.outputPath = strings.TrimSpace(options.outputPath)
	if !options.withWorkflows {
		if options.outputPath != "" {
			return options, fmt.Errorf("--bruno-env requires --with-workflows")
		}
		return options, nil
	}
	options.baseURL = strings.TrimRight(strings.TrimSpace(config.Env.Server.BaseURL), "/")
	parsed, err := url.Parse(options.baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return options, fmt.Errorf("--with-workflows requires absolute HTTP(S) SERVER_BASE_URL without credentials, query, or fragment")
	}
	options.contracts, err = storage.NewSupabaseClient(config.Env.Storage)
	if err != nil {
		return options, fmt.Errorf("--with-workflows contract storage: %w", err)
	}
	options.medical, err = storage.NewSupabaseClientForBucket(config.Env.Storage, config.Env.Storage.MedicalBucket)
	if err != nil {
		return options, fmt.Errorf("--with-workflows medical storage: %w", err)
	}
	return options, nil
}

func seedDemoData(ctx context.Context, db *gorm.DB, options demoSeedOptions, reset bool) (*seeder.DemoManifest, error) {
	if options.withWorkflows {
		if reset {
			return seeder.ResetAllAndSeedWithWorkflows(ctx, db, options.contracts, options.medical, time.Now(), options.baseURL)
		}
		return seeder.RunWithWorkflows(ctx, db, options.contracts, options.medical, time.Now(), options.baseURL)
	}
	if reset {
		return nil, seeder.ResetAllAndSeed(db.WithContext(ctx))
	}
	return nil, seeder.Run(db.WithContext(ctx))
}

// Export outside the maintenance operation: failure to write a local convenience
// file must not leave an otherwise successfully seeded staging API offline.
func exportDemoEnvironment(command *cobra.Command, options demoSeedOptions, manifest *seeder.DemoManifest) error {
	if options.outputPath == "" {
		return nil
	}
	if manifest == nil {
		return fmt.Errorf("seed completed but no workflow environment was generated")
	}
	if err := manifest.WriteBrunoEnvironment(options.outputPath); err != nil {
		return fmt.Errorf("database seed completed, but Bruno export failed: %w", err)
	}
	_, err := fmt.Fprintf(command.OutOrStdout(), "[seeder] Bruno environment written to %s (fixture IDs and public demo defaults only; no server secrets or issued tokens)\n", options.outputPath)
	return err
}

func init() {
	addDemoSeedFlags(seedCmd)
}
