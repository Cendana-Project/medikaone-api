package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var destructiveUpSQL = regexp.MustCompile(`(?im)^\s*(DROP\s+(TABLE|SCHEMA)|TRUNCATE\s+)`)
var destructiveCascadeUpSQL = regexp.MustCompile(`(?im)^\s*DROP\b[^;]*\bCASCADE\b`)

func TestMigrationUpDoesNotDestroySchemaOrTables(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("db", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no SQL migrations found")
	}

	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			sql := string(raw)
			upMarker := strings.Index(sql, "-- +goose Up")
			downMarker := strings.Index(sql, "-- +goose Down")
			if upMarker < 0 || downMarker < 0 || downMarker <= upMarker {
				t.Fatal("migration must contain ordered Goose Up and Down sections")
			}
			upSQL := sql[upMarker:downMarker]
			if statement := destructiveUpSQL.FindString(upSQL); statement != "" {
				t.Fatalf("destructive statement %q is forbidden in migration Up", strings.TrimSpace(statement))
			}
			if statement := destructiveCascadeUpSQL.FindString(upSQL); statement != "" {
				t.Fatalf("destructive DROP ... CASCADE %q is forbidden in migration Up", strings.TrimSpace(statement))
			}
		})
	}
}

func TestHardeningMigrationDownFailsExplicitly(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("db", "20260901010000_harden_user_hospitals.sql"))
	if err != nil {
		t.Fatal(err)
	}
	_, down, found := strings.Cut(string(raw), "-- +goose Down")
	if !found {
		t.Fatal("hardening migration has no Down section")
	}
	if !strings.Contains(down, "intentionally irreversible") || !strings.Contains(down, "RAISE EXCEPTION") {
		t.Fatal("hardening migration Down must fail explicitly instead of restoring unsafe constraints")
	}
}

func TestExaminationMigrationEnforcesAppendOnlyFinalRecords(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("db", "20260904150000_examination.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"trg_vital_revision_immutable",
		"trg_consultation_revision_immutable",
		"trg_diagnosis_immutable",
		"chk_vital_correction_provenance",
		"chk_consultation_correction_provenance",
		"fk_vital_supersedes_same_encounter",
		"fk_consultation_supersedes_same_encounter",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("examination migration is missing %s", required)
		}
	}
}

func TestPrescriptionMigrationEnforcesAppendOnlyIssuedRecords(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("db", "20260905090000_prescription.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"trg_prescription_revision_immutable",
		"trg_prescription_item_immutable",
		"trg_prescription_component_immutable",
		"trg_prescription_revision_validate_issue",
		"fk_prescription_revision_supersedes_same_parent",
		"chk_prescription_item_not_controlled",
		"patient_allergies_snapshot",
		"prescription_documents",
		"revision_id UUID REFERENCES prescription_revisions(id) ON DELETE SET NULL",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("prescription migration is missing %s", required)
		}
	}
}

func TestProfilePhotoMigrationEnforcesPrivateObjectMetadata(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("db", "20260905150000_user_profile_photo.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"avatar_bucket", "avatar_object_path", "avatar_content_type",
		"chk_users_avatar_complete", "chk_users_avatar_content_type",
		"chk_users_avatar_file_size", "ux_users_avatar_object_path",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("profile photo migration is missing %s", required)
		}
	}
}
