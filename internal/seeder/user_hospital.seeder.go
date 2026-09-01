package seeder

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
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

	assignments := []struct {
		email string
		role  string
	}{
		{"admin001@medikaone.id", constant.RoleAdmin},
		{"nurse001@medikaone.id", constant.RoleNurse},
		{"receptionist001@medikaone.id", constant.RoleReceptionist},
		{"bod001@medikaone.id", constant.RoleBOD},
		{"doctor001@medikaone.id", constant.RoleDoctor},
		{"doctor002@medikaone.id", constant.RoleDoctor},
		{"doctor003@medikaone.id", constant.RoleDoctor},
	}

	for _, assignment := range assignments {
		userID, userErr := getUserIDBySeedKey(db, demoUserSeedKey(assignment.email))
		if userErr != nil {
			return userErr
		}
		roleID, roleErr := getActiveRoleIDBySlug(db, assignment.role)
		if roleErr != nil {
			return roleErr
		}

		if err := db.Exec(`
			UPDATE user_hospitals
			SET is_primary = FALSE, updated_at = NOW()
			WHERE user_id = ? AND hospital_id <> ? AND deleted_at IS NULL
		`, userID, hospitalID).Error; err != nil {
			return fmt.Errorf("reset primary membership for %s: %w", assignment.email, err)
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
			return fmt.Errorf("link user %s to hospital: %w", assignment.email, err)
		}
		if err := db.Exec(`
			INSERT INTO hospital_user_roles (hospital_id, user_id, role_id, created_at)
			VALUES (?, ?, ?, NOW())
			ON CONFLICT (hospital_id, user_id, role_id) DO NOTHING
		`, hospitalID, userID, roleID).Error; err != nil {
			return fmt.Errorf("assign tenant role %s to %s: %w", assignment.role, assignment.email, err)
		}
	}

	return nil
}
