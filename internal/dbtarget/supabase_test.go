package dbtarget

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

const testProject = "abcdefghijklmnopqrst"

func parse(t *testing.T, dsn string) *pgx.ConnConfig {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestSupabaseSessionAndDirectRoles(t *testing.T) {
	for _, tc := range []struct {
		dsn, role string
		pooled    bool
	}{
		{"postgresql://medikaone_app." + testProject + ":secret@aws-0-ap-southeast-1.pooler.supabase.com:5432/postgres?sslmode=verify-full", "medikaone_app", true},
		{"postgresql://postgres." + testProject + ":secret@aws-1-eu-central-1.pooler.supabase.com:5432/postgres?sslmode=verify-full", "postgres", true},
		{"postgresql://migration.owner." + testProject + ":secret@aws-0-ap-southeast-1.pooler.supabase.com:5432/postgres?sslmode=verify-full", "migration.owner", true},
		{"postgresql://medikaone_app:secret@db." + testProject + ".supabase.co:5432/postgres?sslmode=verify-full", "medikaone_app", false},
	} {
		cfg := parse(t, tc.dsn)
		endpoint, err := Supabase(cfg)
		if err != nil || endpoint == nil || endpoint.ProjectRef != testProject || endpoint.Role != tc.role || endpoint.SessionPooler != tc.pooled {
			t.Fatalf("endpoint=%#v err=%v", endpoint, err)
		}
		if err = ValidateAdmin(cfg); err != nil {
			t.Fatal(err)
		}
		if err = VerifySession(cfg, "postgres", tc.role, tc.role, "public"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSupabaseRejectsUnsupportedConnectionModes(t *testing.T) {
	for _, dsn := range []string{
		"postgresql://postgres." + testProject + ":secret@aws-0-ap-southeast-1.pooler.supabase.com:6543/postgres?sslmode=verify-full",
		"postgresql://postgres:secret@db." + testProject + ".supabase.co:6543/postgres?sslmode=verify-full",
		"postgresql://postgres:secret@aws-0-ap-southeast-1.pooler.supabase.com:5432/postgres?sslmode=verify-full",
		"postgresql://postgres.invalid:secret@aws-0-ap-southeast-1.pooler.supabase.com:5432/postgres?sslmode=verify-full",
		"postgresql://postgres:secret@db.invalid.supabase.co:5432/postgres?sslmode=verify-full",
		"postgresql://postgres." + testProject + ":secret@aws-0-ap-southeast-1.pooler.supabase.com:5432/postgres?sslmode=prefer",
	} {
		cfg := parse(t, dsn)
		if _, err := Supabase(cfg); err == nil {
			t.Fatal("unsupported Supabase endpoint accepted")
		}
		if err := ValidateAdmin(cfg); err == nil {
			t.Fatal("unsupported admin endpoint accepted")
		}
	}
}

func TestSessionVerificationStillChecksDatabaseRoleAndSchema(t *testing.T) {
	cfg := parse(t, "postgresql://postgres."+testProject+":secret@aws-0-ap-southeast-1.pooler.supabase.com:5432/postgres?sslmode=verify-full")
	for _, fields := range [][4]string{
		{"other", "postgres", "postgres", "public"},
		{"postgres", "medikaone_app", "medikaone_app", "public"},
		{"postgres", "postgres", "medikaone_app", "public"},
		{"postgres", "postgres", "postgres", "private"},
	} {
		if err := VerifySession(cfg, fields[0], fields[1], fields[2], fields[3]); err == nil {
			t.Fatal("mismatched actual session accepted")
		}
	}
}

func TestGenericHostsRetainLiteralRoles(t *testing.T) {
	for _, host := range []string{"postgres.example.test", "aws-0-region.pooler.supabase.com.example.test", "db." + testProject + ".supabase.co.example.test"} {
		cfg := parse(t, "postgresql://postgres."+testProject+":secret@"+host+":5432/postgres?sslmode=verify-full")
		endpoint, err := Supabase(cfg)
		if err != nil || endpoint != nil {
			t.Fatal("unrelated host treated as Supabase")
		}
		if err = VerifySession(cfg, "postgres", "postgres", "postgres", "public"); err == nil {
			t.Fatal("unrelated host had its username translated")
		}
		if err = VerifySession(cfg, "postgres", cfg.User, cfg.User, "public"); err != nil {
			t.Fatal(err)
		}
	}
}
