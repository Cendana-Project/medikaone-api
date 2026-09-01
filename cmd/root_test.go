package cmd

import (
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/spf13/cobra"
)

func TestValidationScopeForCommand(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   config.ValidationScope
	}{
		{name: "server", want: config.ValidationServer},
		{name: "seed", want: config.ValidationDatabase},
		{name: "database-fingerprint", want: config.ValidationDatabaseTarget},
		{name: "staging-reset-all", want: config.ValidationMaintenance},
		{name: "staging-reset-seed", want: config.ValidationMaintenance},
		{name: "migrate", action: "create", want: config.ValidationLocalFileCommand},
		{name: "migrate", action: "status", want: config.ValidationDatabase},
		{name: "migrate", action: "UP", want: config.ValidationMigration},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.action, func(t *testing.T) {
			command := &cobra.Command{Use: tt.name}
			if tt.name == "migrate" {
				command.Flags().String("action", tt.action, "")
			}
			if got := validationScopeForCommand(command); got != tt.want {
				t.Fatalf("scope = %q, want %q", got, tt.want)
			}
		})
	}
}
