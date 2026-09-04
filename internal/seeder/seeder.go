package seeder

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
)

const resetAllDataSQL = `
	DO $$
	DECLARE
		table_list TEXT;
	BEGIN
		SELECT STRING_AGG(FORMAT('%I.%I', schema_name, table_name), ', ')
		INTO table_list
		FROM (VALUES
			('public', 'notifications'),
			('public', 'medical_record_audit_events'),
			('public', 'medical_record_attachments'),
			('public', 'encounter_diagnoses'),
			('public', 'consultation_note_revisions'),
			('public', 'vital_sign_revisions'),
			('public', 'medical_encounters'),
			('public', 'patient_record_events'),
			('public', 'appointment_reminders'),
			('public', 'appointment_status_events'),
			('public', 'appointments'),
			('public', 'appointment_daily_counters'),
			('public', 'doctor_schedule_change_events'),
			('public', 'doctor_schedule_change_items'),
			('public', 'doctor_schedule_change_requests'),
			('public', 'doctor_hospital_schedules'),
			('public', 'doctor_hospital_affiliation_events'),
			('public', 'doctor_hospital_affiliations'),
			('public', 'doctor_hospital_invitation_schedules'),
			('public', 'doctor_hospital_invitation_events'),
			('public', 'doctor_hospital_contracts'),
			('public', 'doctor_hospital_invitations'),
			('public', 'hospital_rooms'),
			('public', 'hospital_departments'),
			('public', 'hospital_user_roles'),
			('public', 'user_hospitals'),
			('public', 'doctor_profiles'),
			('public', 'patient_profiles'),
			('public', 'patient_records'),
			('public', 'role_permissions'),
			('public', 'user_roles'),
			('public', 'hospitals'),
			('public', 'permissions'),
			('public', 'roles'),
			('public', 'users')
		) AS allowed(schema_name, table_name)
		WHERE TO_REGCLASS(FORMAT('%I.%I', schema_name, table_name)) IS NOT NULL;

		IF table_list IS NOT NULL THEN
			-- An unknown table that references an application table must make the
			-- reset fail instead of being silently emptied as a dependency.
			EXECUTE 'TRUNCATE TABLE ' || table_list || ' RESTART IDENTITY';
		END IF;
	END;
	$$;
`

const envSuperadminSeedKey = "medikaone:user:env-superadmin"

// Run idempotently synchronizes system RBAC definitions and development demo
// data. The hardcoded demo credentials are intentional for the current stage.
func Run(db *gorm.DB) error {
	start := time.Now()
	if err := db.Transaction(seedAll); err != nil {
		return err
	}
	fmt.Printf("[seeder] done in %s\n", time.Since(start))
	return nil
}

// ResetAllAndSeed atomically removes every row owned by the application and
// recreates the declarative seed set. Migrations must be applied before this
// function is called. PostgreSQL can roll TRUNCATE back, so a seed failure
// leaves the previous staging data intact.
func ResetAllAndSeed(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(resetAllDataSQL).Error; err != nil {
			return fmt.Errorf("truncate application data: %w", err)
		}
		return seedAll(tx)
	})
}

// ClearAllData is the destructive first phase of the guarded full staging
// reset. It is kept separate from migrations so legacy duplicate data cannot
// prevent the recovery command from applying new unique indexes.
func ClearAllData(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(resetAllDataSQL).Error; err != nil {
			return fmt.Errorf("truncate application data: %w", err)
		}
		return nil
	})
}

func seedAll(tx *gorm.DB) error {
	if err := SeedRoles(tx); err != nil {
		return fmt.Errorf("seed roles: %w", err)
	}
	if err := SeedPermissions(tx); err != nil {
		return fmt.Errorf("seed permissions: %w", err)
	}
	if err := SeedSampleUsers(tx); err != nil {
		return fmt.Errorf("seed sample users: %w", err)
	}
	// Apply the environment-managed account after the built-in fixtures so an
	// explicitly configured password and identity always win on a permitted
	// collision with the canonical superadmin fixture.
	if err := seedEnvironmentSuperadmin(tx); err != nil {
		return err
	}
	if err := SeedHospitals(tx); err != nil {
		return fmt.Errorf("seed hospitals: %w", err)
	}
	if err := SeedUserHospitals(tx); err != nil {
		return fmt.Errorf("seed user hospitals: %w", err)
	}
	return nil
}

func seedEnvironmentSuperadmin(tx *gorm.DB) error {
	email := strings.TrimSpace(os.Getenv("SUPERADMIN_EMAIL"))
	if email == "" {
		return nil
	}
	password := os.Getenv("SUPERADMIN_PASSWORD")
	if password == "" {
		return fmt.Errorf("env SUPERADMIN_PASSWORD required when SUPERADMIN_EMAIL set")
	}
	seedKey, err := envSuperadminKey(email)
	if err != nil {
		return err
	}
	firstName := strings.TrimSpace(os.Getenv("SUPERADMIN_FIRST_NAME"))
	if firstName == "" {
		firstName = "Super"
	}
	lastName := strings.TrimSpace(os.Getenv("SUPERADMIN_LAST_NAME"))
	if lastName == "" {
		lastName = "Admin"
	}
	if _, err := CreateDemoUserActive(
		tx, seedKey, email, firstName, lastName, password, constant.RoleSuperAdmin,
	); err != nil {
		return fmt.Errorf("seed super_admin: %w", err)
	}
	return nil
}

// ResetDemoAndSeed atomically removes only users owned by the built-in demo
// fixture, then recreates all declarative seed data. It intentionally keeps:
//   - every non-demo user and their global role assignments;
//   - demo hospitals, including memberships belonging to non-demo users;
//   - custom roles, permissions, and custom permission mappings.
func ResetDemoAndSeed(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := resetDemoUsers(tx); err != nil {
			return err
		}
		return seedAll(tx)
	})
}

// Flush is retained for local tooling, but now has safe semantics: only exact
// built-in demo users are removed. It never deletes roles or assignments from
// real users.
func Flush(db *gorm.DB) error {
	return db.Transaction(resetDemoUsers)
}

func resetDemoUsers(tx *gorm.DB) error {
	if err := ensureNoLegacyFixtureCollisions(tx); err != nil {
		return err
	}
	result := tx.Exec(`DELETE FROM users WHERE seed_key IN ?`, demoUserSeedKeys())
	if result.Error != nil {
		return fmt.Errorf("delete demo users: %w", result.Error)
	}
	fmt.Printf("[seeder] removed %d recognized demo users\n", result.RowsAffected)
	return nil
}

func envSuperadminKey(email string) (string, error) {
	for _, fixture := range sampleUserSeeds() {
		if strings.EqualFold(strings.TrimSpace(email), fixture.Email) {
			if fixture.RoleSlug != constant.RoleSuperAdmin {
				return "", fmt.Errorf(
					"refusing SUPERADMIN_EMAIL %q: address belongs to the %s demo fixture",
					strings.ToLower(strings.TrimSpace(email)), fixture.RoleSlug,
				)
			}
			return demoUserSeedKey(fixture.Email), nil
		}
	}
	return envSuperadminSeedKey, nil
}

func ensureNoLegacyFixtureCollisions(tx *gorm.DB) error {
	var userCount int64
	if err := tx.Raw(`
		SELECT COUNT(*) FROM users
		WHERE seed_key IS NULL AND LOWER(email) IN ?
	`, demoUserEmails()).Scan(&userCount).Error; err != nil {
		return fmt.Errorf("inspect legacy user fixtures: %w", err)
	}
	var hospitalCount int64
	if err := tx.Raw(`
		SELECT COUNT(*) FROM hospitals
		WHERE seed_key IS NULL AND LOWER(code) IN ?
	`, demoHospitalCodes()).Scan(&hospitalCount).Error; err != nil {
		return fmt.Errorf("inspect legacy hospital fixtures: %w", err)
	}
	if userCount > 0 || hospitalCount > 0 {
		return fmt.Errorf("refusing demo reset: found untagged rows using fixture identities; resolve them manually or run the guarded full staging reset once")
	}
	return nil
}

func uniquePermSlugsFromDefaults() []string {
	var all []string
	for _, slugs := range constant.DefaultRolePermissions {
		all = append(all, slugs...)
	}
	slices.Sort(all)
	return slices.Compact(all)
}
