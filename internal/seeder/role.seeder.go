package seeder

import (
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
)

func roleSeeds() []entity.Role {
	return []entity.Role{
		{Name: "Super Admin", Slug: constant.RoleSuperAdmin},
		{Name: "Admin (Hospital)", Slug: constant.RoleAdmin},
		{Name: "Patient", Slug: constant.RolePatient},
		{Name: "Doctor", Slug: constant.RoleDoctor},
		{Name: "Nurse", Slug: constant.RoleNurse},
		{Name: "Receptionist", Slug: constant.RoleReceptionist},
		{Name: "BOD", Slug: constant.RoleBOD},
	}
}

// SeedRoles memastikan 7 role tersedia (idempotent).
func SeedRoles(db *gorm.DB) error {
	roles := roleSeeds()
	for _, r := range roles {
		if err := db.Exec(`
			INSERT INTO roles (name, slug, active, created_at, updated_at, deleted_at)
			VALUES (?, ?, TRUE, NOW(), NOW(), NULL)
			ON CONFLICT (slug) DO UPDATE SET
				name = EXCLUDED.name,
				active = TRUE,
				updated_at = NOW(),
				deleted_at = NULL
		`, r.Name, r.Slug).Error; err != nil {
			return err
		}
	}
	return nil
}
