package seeder

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
)

// SeedPermissions:
// 1) memastikan semua permission ada (idempotent)
// 2) assign permission ke setiap role sesuai DefaultRolePermissions
func SeedPermissions(db *gorm.DB) error {
	// --- 1) DAFTAR PERMISSION (rapi per domain) ---
	toCreate := []entity.Permission{
		// user & role & permission
		{Name: "User View", Slug: constant.PermissionUserView, IsActive: true},
		{Name: "User Create", Slug: constant.PermissionUserCreate, IsActive: true},
		{Name: "User Update", Slug: constant.PermissionUserUpdate, IsActive: true},
		{Name: "User Delete", Slug: constant.PermissionUserDelete, IsActive: true},

		{Name: "Role View", Slug: constant.PermissionRoleView, IsActive: true},
		{Name: "Role Assign", Slug: constant.PermissionRoleAssign, IsActive: true},

		{Name: "Permission View", Slug: constant.PermissionPermissionView, IsActive: true},

		// profile
		{Name: "Patient View", Slug: constant.PermissionPatientView, IsActive: true},
		{Name: "Patient Edit", Slug: constant.PermissionPatientEdit, IsActive: true},
		{Name: "Doctor View", Slug: constant.PermissionDoctorView, IsActive: true},
		{Name: "Doctor Edit", Slug: constant.PermissionDoctorEdit, IsActive: true},

		// emr & billing
		{Name: "EMR View", Slug: constant.PermissionEMRView, IsActive: true},
		{Name: "EMR Edit", Slug: constant.PermissionEMREdit, IsActive: true},
		{Name: "Billing View", Slug: constant.PermissionBillingView, IsActive: true},
		{Name: "Billing Edit", Slug: constant.PermissionBillingEdit, IsActive: true},

		// appointment
		{Name: "Appointment View", Slug: constant.PermissionAppointmentView, IsActive: true},
		{Name: "Appointment Edit", Slug: constant.PermissionAppointmentEdit, IsActive: true},
	}

	// upsert per slug (idempotent)
	for _, p := range toCreate {
		if err := db.Exec(`
			INSERT INTO permissions (name, slug, is_active, created_at, updated_at, deleted_at)
			VALUES (?, ?, TRUE, NOW(), NOW(), NULL)
			ON CONFLICT (slug) DO UPDATE SET
				name = EXCLUDED.name,
				is_active = TRUE,
				updated_at = NOW(),
				deleted_at = NULL;
		`, p.Name, p.Slug).Error; err != nil {
			return fmt.Errorf("insert permission %s: %w", p.Slug, err)
		}
	}

	// helper ambil id
	getRoleID := func(slug string) (string, error) {
		var id string
		if err := db.Raw(`SELECT id FROM roles WHERE slug = ? LIMIT 1`, slug).Scan(&id).Error; err != nil {
			return "", err
		}
		if id == "" {
			return "", fmt.Errorf("role not found: %s", slug)
		}
		return id, nil
	}
	getPermID := func(slug string) (string, error) {
		var id string
		if err := db.Raw(`SELECT id FROM permissions WHERE slug = ? LIMIT 1`, slug).Scan(&id).Error; err != nil {
			return "", err
		}
		if id == "" {
			return "", fmt.Errorf("permission not found: %s", slug)
		}
		return id, nil
	}

	// --- 2) ASSIGN PERMISSIONS KE ROLE ---
	for roleSlug, permSlugs := range constant.DefaultRolePermissions {
		rid, err := getRoleID(roleSlug)
		if err != nil {
			return err
		}
		// Synchronize only permissions managed by this seeder. Custom permission
		// mappings are intentionally preserved.
		if err := db.Exec(`
			DELETE FROM role_permissions rp
			USING permissions p
			WHERE rp.role_id = ?
			  AND rp.permission_id = p.id
			  AND p.slug IN ?
			  AND p.slug NOT IN ?
		`, rid, uniquePermSlugsFromDefaults(), permSlugs).Error; err != nil {
			return fmt.Errorf("sync permissions for %s: %w", roleSlug, err)
		}
		for _, ps := range permSlugs {
			pid, err := getPermID(ps)
			if err != nil {
				return err
			}
			if err := db.Exec(`
				INSERT INTO role_permissions (role_id, permission_id, created_at)
				VALUES (?, ?, NOW())
				ON CONFLICT (role_id, permission_id) DO NOTHING;
			`, rid, pid).Error; err != nil {
				return fmt.Errorf("assign %s -> %s: %w", roleSlug, ps, err)
			}
		}
	}

	return nil
}
