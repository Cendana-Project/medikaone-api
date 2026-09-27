package infrastructure

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestValidateRuntimeDatabasePrivileges(t *testing.T) {
	tests := []struct {
		name                    string
		restricted              bool
		canCreate               bool
		controlsOwner           bool
		canTruncate             bool
		canModifyHistory        bool
		canUseMigrationSequence bool
		wantError               string
	}{
		{name: "restricted least privilege", restricted: true},
		{name: "schema create", restricted: true, canCreate: true, wantError: "CREATE"},
		{name: "owner membership", restricted: true, controlsOwner: true, wantError: "owner role"},
		{name: "truncate", restricted: true, canTruncate: true, wantError: "TRUNCATE"},
		{name: "migration history DML", restricted: true, canModifyHistory: true, wantError: "read-only"},
		{name: "migration history sequence", restricted: true, canUseMigrationSequence: true, wantError: "sequences owned"},
		{
			name:                    "administrative connection",
			canCreate:               true,
			controlsOwner:           true,
			canTruncate:             true,
			canModifyHistory:        true,
			canUseMigrationSequence: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRuntimeDatabasePrivileges(
				tc.restricted, tc.canCreate, tc.controlsOwner, tc.canTruncate,
				tc.canModifyHistory, tc.canUseMigrationSequence,
			)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("validateRuntimeDatabasePrivileges() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("validateRuntimeDatabasePrivileges() error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestValidateDatabaseMigrationVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version int64
		wantErr bool
	}{
		{name: "missing", version: 0, wantErr: true},
		{name: "older", version: RequiredDatabaseMigrationVersion - 1, wantErr: true},
		{name: "minimum", version: RequiredDatabaseMigrationVersion},
		{name: "newer", version: RequiredDatabaseMigrationVersion + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDatabaseMigrationVersion(tc.version)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateDatabaseMigrationVersion(%d) error = %v, wantErr %v", tc.version, err, tc.wantErr)
			}
		})
	}
}

func TestRequiredMigrationCannotBeReplacedByAnewerVersion(t *testing.T) {
	if err := validateDatabaseMigrationState(RequiredDatabaseMigrationVersion+1, false); err == nil {
		t.Fatal("newer maximum version bypassed the missing required migration")
	}
	if err := validateDatabaseMigrationState(RequiredDatabaseMigrationVersion+1, true); err != nil {
		t.Fatalf("applied required migration with newer maximum was rejected: %v", err)
	}
}

func TestRequiredDatabaseMigrationIsLatest(t *testing.T) {
	entries, err := os.ReadDir("../../migration/db")
	if err != nil {
		t.Fatal(err)
	}
	var latest int64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		prefix, _, found := strings.Cut(name, "_")
		if !found {
			continue
		}
		version, parseErr := strconv.ParseInt(prefix, 10, 64)
		if parseErr != nil {
			t.Fatalf("parse migration version from %q: %v", name, parseErr)
		}
		if version > latest {
			latest = version
		}
	}
	if latest != RequiredDatabaseMigrationVersion {
		t.Fatalf("RequiredDatabaseMigrationVersion = %d, latest migration = %d", RequiredDatabaseMigrationVersion, latest)
	}
}
