package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/config"
)

func TestStartMigrateRejectsInvalidCommandBeforeConnecting(t *testing.T) {
	err := StartMigrate("unknown", "", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid migration command") {
		t.Fatalf("StartMigrate() error = %v, want invalid command", err)
	}
}

func TestStartMigrateRejectsDestructiveCommandsBeforeConnecting(t *testing.T) {
	for _, action := range []string{"down", "down-to", "reset"} {
		err := StartMigrate(action, "", nil)
		if err == nil || !strings.Contains(err.Error(), "invalid migration command") {
			t.Fatalf("StartMigrate(%q) error = %v, want invalid command", action, err)
		}
	}
}

func TestStartMigrateCreateDoesNotNeedDatabase(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("migration", "db"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := StartMigrate("create", "offline_example", nil); err != nil {
		t.Fatalf("StartMigrate(create) error = %v", err)
	}
	files, err := filepath.Glob(filepath.Join("migration", "db", "*_offline_example.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("created migration files = %v, want exactly one", files)
	}
}

func TestGuardedMigrationRequiresSchemaPostcondition(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *config.EnvConfig
		action string
		want   bool
	}{
		{name: "staging up", cfg: &config.EnvConfig{Env: "staging"}, action: "up", want: true},
		{name: "production up-to", cfg: &config.EnvConfig{Env: "production"}, action: "up-to", want: true},
		{name: "staging status", cfg: &config.EnvConfig{Env: "staging"}, action: "status"},
		{name: "development up", cfg: &config.EnvConfig{Env: "development"}, action: "up"},
		{name: "missing config", cfg: nil, action: "up"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := requiresGuardedMigrationPostcondition(test.cfg, test.action); got != test.want {
				t.Fatalf("requiresGuardedMigrationPostcondition() = %v, want %v", got, test.want)
			}
		})
	}
}
