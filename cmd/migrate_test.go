package cmd

import (
	"testing"
)

func TestRequireSafeMigrationAction(t *testing.T) {
	for _, action := range []string{"down", "down-to", "reset"} {
		if err := requireSafeMigrationAction(action); err == nil {
			t.Fatalf("must reject generic migrate %s", action)
		}
	}
	if err := requireSafeMigrationAction("up"); err != nil {
		t.Fatalf("rejected safe migration: %v", err)
	}
}
