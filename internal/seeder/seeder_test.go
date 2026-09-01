package seeder

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var createTableName = regexp.MustCompile(`(?im)^\s*CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+"?([a-zA-Z0-9_]+)"?`)

func TestDemoUserEmailsAreUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for _, email := range demoUserEmails() {
		if _, exists := seen[email]; exists {
			t.Fatalf("duplicate demo email in allowlist: %s", email)
		}
		seen[email] = struct{}{}
	}
	if len(seen) != len(sampleUserSeeds()) {
		t.Fatalf("email allowlist and sample users differ: %d != %d", len(seen), len(sampleUserSeeds()))
	}
}

func TestDemoUserSeedKeysAreUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for _, key := range demoUserSeedKeys() {
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate immutable demo seed key: %s", key)
		}
		seen[key] = struct{}{}
	}
	if len(seen) != len(sampleUserSeeds())+1 {
		t.Fatalf("seed key set size = %d, want %d sample keys plus env superadmin", len(seen), len(sampleUserSeeds()))
	}
}

func TestEnvironmentSuperadminMayOnlyReuseCanonicalSuperadminFixture(t *testing.T) {
	key, err := envSuperadminKey(" SUPERADMIN@medikaone.id ")
	if err != nil {
		t.Fatal(err)
	}
	if want := demoUserSeedKey("superadmin@medikaone.id"); key != want {
		t.Fatalf("canonical env superadmin seed key = %q, want %q", key, want)
	}

	if _, err := envSuperadminKey("patient001@medikaone.id"); err == nil {
		t.Fatal("patient fixture was accepted as the environment superadmin")
	}

	key, err = envSuperadminKey("owner@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if key != envSuperadminSeedKey {
		t.Fatalf("custom env superadmin seed key = %q, want %q", key, envSuperadminSeedKey)
	}
}

func TestSeedPasswordHashUsesAuthCompatibleFormat(t *testing.T) {
	hash, err := seedPasswordHash("Password123")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(hash, ":")
	if len(parts) != 2 {
		t.Fatalf("password hash must be key:salt, got %q", hash)
	}
	key, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil || len(key) != 64 {
		t.Fatalf("invalid derived key: length=%d err=%v", len(key), err)
	}
	salt, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(salt) != 16 {
		t.Fatalf("invalid salt: length=%d err=%v", len(salt), err)
	}
}

func TestInitialMigrationUpNeverDropsTables(t *testing.T) {
	for _, name := range []string{
		"20251021181549_init_core_sql.sql",
		"20251023042912_init_hospital_multi_tenant.sql",
	} {
		path := filepath.Join("..", "..", "migration", "db", name)
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		up, _, found := strings.Cut(string(contents), "-- +goose Down")
		if !found {
			t.Fatalf("%s has no Goose Down section", name)
		}
		if strings.Contains(strings.ToUpper(up), "DROP TABLE") {
			t.Fatalf("%s contains destructive DROP TABLE in Goose Up", name)
		}
	}
}

func TestResetAllCoversEveryApplicationTable(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "migration", "db", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no database migrations found")
	}
	resetSQL := strings.ToLower(resetAllDataSQL)
	for _, path := range files {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, match := range createTableName.FindAllStringSubmatch(string(contents), -1) {
			table := strings.ToLower(match[1])
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(table) + `\b`).MatchString(resetSQL) {
				t.Errorf("application table %q from %s is missing from the full staging reset", table, filepath.Base(path))
			}
		}
	}
	if strings.Contains(resetSQL, "goose_db_version") {
		t.Fatal("full data reset must preserve Goose migration history")
	}
	if regexp.MustCompile(`(?i)\bCASCADE\b`).MatchString(resetSQL) {
		t.Fatal("full data reset must fail closed on unknown foreign-key dependencies")
	}
}
