package seeder

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/scrypt"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
)

func seedPasswordHash(rawPassword string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := scrypt.Key([]byte(rawPassword), salt, 1<<15, 8, 1, 64)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key) + ":" + base64.StdEncoding.EncodeToString(salt), nil
}

// CreateDemoUserActive declaratively restores a seeded account. Existing
// rows receive the fixture password and canonical identity fields; otherwise a
// pending row could retain an attacker-controlled password while gaining the
// fixture's privileged role. Only SUPER_ADMIN and PATIENT are global roles;
// hospital staff roles are assigned exclusively by SeedUserHospitals.
func CreateDemoUserActive(db *gorm.DB, seedKey, email, firstName, lastName, rawPassword, roleSlug string) (*entity.User, error) {
	seedKey = strings.TrimSpace(seedKey)
	email = strings.ToLower(strings.TrimSpace(email))
	if seedKey == "" || email == "" || rawPassword == "" || roleSlug == "" {
		return nil, errors.New("seed key/email/password/role wajib diisi")
	}

	isGlobalRole := roleSlug == constant.RoleSuperAdmin || roleSlug == constant.RolePatient
	var role entity.Role
	if isGlobalRole {
		if err := db.Where("slug = ? AND active = TRUE AND deleted_at IS NULL", roleSlug).First(&role).Error; err != nil {
			return nil, err
		}
	}
	passwordHash, err := seedPasswordHash(rawPassword)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var user entity.User
	err = db.Unscoped().
		Where("seed_key = ?", seedKey).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = entity.User{
			SeedKey:      &seedKey,
			Email:        email,
			FirstName:    firstName,
			LastName:     lastName,
			PasswordHash: passwordHash,
			Status:       "active",
			VerifiedAt:   &now,
		}
		if err := db.Create(&user).Error; err != nil {
			return nil, fmt.Errorf("create fixture %q: %w; an untagged row may already own its canonical identity", seedKey, err)
		}
	} else if err != nil {
		return nil, err
	} else {
		if err := db.Unscoped().Model(&entity.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"seed_key":      seedKey,
			"email":         email,
			"first_name":    firstName,
			"last_name":     lastName,
			"password_hash": passwordHash,
			"status":        "active",
			"verified_at":   now,
			"updated_at":    now,
			"deleted_at":    nil,
		}).Error; err != nil {
			return nil, err
		}
		user.SeedKey = &seedKey
		user.Email = email
		user.FirstName = firstName
		user.LastName = lastName
		user.PasswordHash = passwordHash
		user.Status = "active"
		user.VerifiedAt = &now
		user.DeletedAt = gorm.DeletedAt{}
	}

	if isGlobalRole {
		if err := db.Exec(`
			INSERT INTO user_roles (user_id, role_id, created_at)
			VALUES (?, ?, NOW())
			ON CONFLICT (user_id, role_id) DO NOTHING
		`, user.ID, role.ID).Error; err != nil {
			return nil, err
		}
	} else if err := db.Exec(`
		DELETE FROM user_roles ur
		USING roles r
		WHERE ur.role_id = r.id AND ur.user_id = ? AND UPPER(r.slug) = UPPER(?)
	`, user.ID, roleSlug).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
