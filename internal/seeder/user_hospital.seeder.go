package seeder

import (
	"fmt"

	"gorm.io/gorm"
)

func getHospitalIDBySeedKey(db *gorm.DB, seedKey string) (string, error) {
	var id string
	if err := db.Raw(`
		SELECT id FROM hospitals
		WHERE seed_key = ? AND is_active = TRUE AND deleted_at IS NULL
		LIMIT 1
	`, seedKey).Scan(&id).Error; err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("seeded hospital not found: %s", seedKey)
	}
	return id, nil
}

func getUserIDBySeedKey(db *gorm.DB, seedKey string) (string, error) {
	var id string
	if err := db.Raw(`
		SELECT id FROM users
		WHERE seed_key = ? AND status = 'active' AND deleted_at IS NULL
		LIMIT 1
	`, seedKey).Scan(&id).Error; err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("seeded user not found: %s", seedKey)
	}
	return id, nil
}

func getActiveRoleIDBySlug(db *gorm.DB, slug string) (string, error) {
	var id string
	if err := db.Raw(`
		SELECT id FROM roles
		WHERE UPPER(slug) = UPPER(?) AND active = TRUE AND deleted_at IS NULL
		LIMIT 1
	`, slug).Scan(&id).Error; err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("role not found: %s", slug)
	}
	return id, nil
}

// SeedUserHospitals creates both the active membership and its tenant-scoped
// role. These are deliberately separate from the user's global demo role.
func SeedUserHospitals(db *gorm.DB) error {
	hospitalID, err := getHospitalIDBySeedKey(db, "medikaone:hospital:general-jakarta")
	if err != nil {
		return err
	}

	// Derive tenant assignments from the canonical user fixtures instead of
	// maintaining a second, drift-prone account list. SUPER_ADMIN and PATIENT
	// are intentionally global-only; every other fixture role is tenant-scoped.
	for _, assignment := range sampleUserSeeds() {
		userID, userErr := getUserIDBySeedKey(db, demoUserSeedKey(assignment.Email))
		if userErr != nil {
			return userErr
		}
		if isGlobalDemoRole(assignment.RoleSlug) {
			// Reconcile legacy/partial demo seeds. Global-only fixture accounts do
			// not need a tenant membership in order to select a hospital.
			if err := db.Exec(`
				DELETE FROM hospital_user_roles
				WHERE hospital_id = ? AND user_id = ?
			`, hospitalID, userID).Error; err != nil {
				return fmt.Errorf("remove tenant roles from global fixture %s: %w", assignment.Email, err)
			}
			if err := db.Exec(`
				DELETE FROM user_hospitals
				WHERE hospital_id = ? AND user_id = ?
			`, hospitalID, userID).Error; err != nil {
				return fmt.Errorf("remove tenant membership from global fixture %s: %w", assignment.Email, err)
			}
			continue
		}

		roleID, roleErr := getActiveRoleIDBySlug(db, assignment.RoleSlug)
		if roleErr != nil {
			return roleErr
		}

		if err := db.Exec(`
			UPDATE user_hospitals
			SET is_primary = FALSE, updated_at = NOW()
			WHERE user_id = ? AND hospital_id <> ? AND deleted_at IS NULL
		`, userID, hospitalID).Error; err != nil {
			return fmt.Errorf("reset primary membership for %s: %w", assignment.Email, err)
		}
		if err := db.Exec(`
			INSERT INTO user_hospitals (
				user_id, hospital_id, is_active, is_primary, created_at, updated_at, deleted_at
			)
			VALUES (?, ?, TRUE, TRUE, NOW(), NOW(), NULL)
			ON CONFLICT (user_id, hospital_id) DO UPDATE SET
				is_active = TRUE,
				is_primary = TRUE,
				updated_at = NOW(),
				deleted_at = NULL
		`, userID, hospitalID).Error; err != nil {
			return fmt.Errorf("link user %s to hospital: %w", assignment.Email, err)
		}
		// A demo tenant account has one canonical role in its canonical
		// hospital. Remove stale roles left by older/partial seed runs.
		if err := db.Exec(`
			DELETE FROM hospital_user_roles
			WHERE hospital_id = ? AND user_id = ? AND role_id <> ?
		`, hospitalID, userID, roleID).Error; err != nil {
			return fmt.Errorf("reconcile tenant roles for %s: %w", assignment.Email, err)
		}
		if err := db.Exec(`
			INSERT INTO hospital_user_roles (hospital_id, user_id, role_id, created_at)
			VALUES (?, ?, ?, NOW())
			ON CONFLICT (hospital_id, user_id, role_id) DO NOTHING
		`, hospitalID, userID, roleID).Error; err != nil {
			return fmt.Errorf("assign tenant role %s to %s: %w", assignment.RoleSlug, assignment.Email, err)
		}
	}

	return nil
}
