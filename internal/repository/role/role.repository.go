package role

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) WithTx(tx *gorm.DB) *Repository { return &Repository{db: tx} }

// =====================
// Global (non-tenant)
// =====================

func (r *Repository) FindBySlug(ctx context.Context, slug string) (*entity.Role, error) {
	var out entity.Role
	if err := r.db.WithContext(ctx).Where("UPPER(slug) = UPPER(?) AND active = TRUE AND deleted_at IS NULL", slug).First(&out).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

func (r *Repository) Assign(ctx context.Context, userID, roleID string) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO user_roles (user_id, role_id, created_at)
		VALUES (?, ?, NOW())
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, userID, roleID).Error
}

func (r *Repository) UserHasRole(ctx context.Context, userID, roleSlug string) (bool, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Table("user_roles ur").
		Joins("JOIN roles r ON r.id = ur.role_id").
		Joins("JOIN users u ON u.id = ur.user_id").
		Where(`ur.user_id = ? AND UPPER(r.slug) = UPPER(?)
			AND r.active = TRUE AND r.deleted_at IS NULL
			AND u.status = 'active' AND u.deleted_at IS NULL`, userID, roleSlug).
		Count(&cnt).Error
	return cnt > 0, err
}

func (r *Repository) ListPermissionsByUser(ctx context.Context, userID string) ([]entity.Permission, error) {
	var perms []entity.Permission
	q := `
SELECT DISTINCT p.id, p.name, p.slug, p.description, p.is_active, p.created_at, p.updated_at, p.deleted_at
FROM user_roles ur
JOIN users u ON u.id = ur.user_id
JOIN roles r ON r.id = ur.role_id
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ur.user_id = ?
  AND u.status = 'active' AND u.deleted_at IS NULL
  AND r.active = TRUE AND r.deleted_at IS NULL
  AND p.is_active = TRUE AND p.deleted_at IS NULL
`
	if err := r.db.WithContext(ctx).Raw(q, userID).Scan(&perms).Error; err != nil {
		return nil, err
	}
	return perms, nil
}

// Ambil role.id dari role.slug
func (r *Repository) GetRoleIDBySlug(ctx context.Context, slug string) (string, error) {
	var id string
	const q = `SELECT id FROM roles WHERE UPPER(slug) = UPPER(?) AND active = TRUE AND deleted_at IS NULL LIMIT 1`
	if err := r.db.WithContext(ctx).Raw(q, slug).Scan(&id).Error; err != nil {
		return "", err
	}
	if id == "" {
		return "", gorm.ErrRecordNotFound
	}
	return id, nil
}

// Cek apakah user punya role global 'SUPER_ADMIN'
func (r *Repository) IsUserSuperAdmin(ctx context.Context, userID string) (bool, error) {
	type row struct{ C int64 }
	var out row
	q := `
SELECT COUNT(1) AS c
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
JOIN users u ON u.id = ur.user_id
WHERE ur.user_id = ? AND UPPER(r.slug) = UPPER(?)
  AND r.active = TRUE AND r.deleted_at IS NULL
  AND u.status = 'active' AND u.deleted_at IS NULL`
	if err := r.db.WithContext(ctx).Raw(q, userID, constant.RoleSuperAdmin).Scan(&out).Error; err != nil {
		return false, err
	}
	return out.C > 0, nil
}

// =====================
// Tenant-scoped (hospital)
// =====================

// Assign role pada user di scope hospital (idempotent)
func (r *Repository) AssignHospitalRole(ctx context.Context, hospitalID, userID, roleID string) error {
	row := map[string]any{
		"hospital_id": hospitalID,
		"user_id":     userID,
		"role_id":     roleID,
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "hospital_id"}, {Name: "user_id"}, {Name: "role_id"}},
			DoNothing: true,
		}).
		Table("hospital_user_roles").
		Create(row).Error
}

// Daftar permissions user pada hospital tertentu
func (r *Repository) ListHospitalPermissionsByUser(ctx context.Context, hospitalID, userID string) ([]entity.Permission, error) {
	var perms []entity.Permission
	q := `
SELECT DISTINCT p.id, p.name, p.slug, p.description, p.is_active, p.created_at, p.updated_at, p.deleted_at
FROM hospital_user_roles hur
JOIN user_hospitals uh ON uh.user_id = hur.user_id AND uh.hospital_id = hur.hospital_id
JOIN hospitals h ON h.id = hur.hospital_id
JOIN users u ON u.id = hur.user_id
JOIN roles r ON r.id = hur.role_id
JOIN role_permissions rp ON rp.role_id = hur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE hur.hospital_id = ? AND hur.user_id = ?
  AND uh.is_active = TRUE AND uh.deleted_at IS NULL
  AND h.is_active = TRUE AND h.deleted_at IS NULL
  AND u.status = 'active' AND u.deleted_at IS NULL
  AND r.active = TRUE AND r.deleted_at IS NULL
  AND p.is_active = TRUE AND p.deleted_at IS NULL
`
	if err := r.db.WithContext(ctx).Raw(q, hospitalID, userID).Scan(&perms).Error; err != nil {
		return nil, err
	}
	return perms, nil
}

// ListRolesByUser: daftar role global (aktif) milik user
func (r *Repository) ListRolesByUser(ctx context.Context, userID string) ([]entity.Role, error) {
	var roles []entity.Role
	const q = `
SELECT r.id, r.name, r.slug, r.description, r.active, r.created_at, r.updated_at, r.deleted_at
FROM user_roles ur
JOIN users u ON u.id = ur.user_id
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = ? AND r.active = TRUE
  AND r.deleted_at IS NULL
  AND u.status = 'active' AND u.deleted_at IS NULL
ORDER BY CASE UPPER(r.slug)
  WHEN 'SUPER_ADMIN' THEN 1
  WHEN 'ADMIN' THEN 2
  WHEN 'DOCTOR' THEN 3
  WHEN 'NURSE' THEN 4
  WHEN 'RECEPTIONIST' THEN 5
  WHEN 'BOD' THEN 6
  WHEN 'PATIENT' THEN 7
  ELSE 100 END, UPPER(r.slug)`
	if err := r.db.WithContext(ctx).Raw(q, userID).Scan(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// ListHospitalRolesByUser: role aktif user pada hospital tertentu
func (r *Repository) ListHospitalRolesByUser(ctx context.Context, hospitalID, userID string) ([]entity.Role, error) {
	var roles []entity.Role
	const q = `
SELECT r.id, r.name, r.slug, r.description, r.active, r.created_at, r.updated_at, r.deleted_at
FROM hospital_user_roles hur
JOIN user_hospitals uh ON uh.user_id = hur.user_id AND uh.hospital_id = hur.hospital_id
JOIN hospitals h ON h.id = hur.hospital_id
JOIN users u ON u.id = hur.user_id
JOIN roles r ON r.id = hur.role_id
WHERE hur.hospital_id = ? AND hur.user_id = ?
  AND uh.is_active = TRUE AND uh.deleted_at IS NULL
  AND h.is_active = TRUE AND h.deleted_at IS NULL
  AND u.status = 'active' AND u.deleted_at IS NULL
  AND r.active = TRUE AND r.deleted_at IS NULL
ORDER BY CASE UPPER(r.slug)
  WHEN 'SUPER_ADMIN' THEN 1
  WHEN 'ADMIN' THEN 2
  WHEN 'DOCTOR' THEN 3
  WHEN 'NURSE' THEN 4
  WHEN 'RECEPTIONIST' THEN 5
  WHEN 'BOD' THEN 6
  WHEN 'PATIENT' THEN 7
  ELSE 100 END, UPPER(r.slug)`
	if err := r.db.WithContext(ctx).Raw(q, hospitalID, userID).Scan(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// =====================
// >>> Validasi Hospital Admin (pakai constants) <<<
// =====================

func (r *Repository) UserHasHospitalRole(ctx context.Context, hospitalID, userID, roleSlug string) (bool, error) {
	type row struct{ C int64 }
	var out row
	const q = `
SELECT COUNT(1) AS c
FROM hospital_user_roles hur
JOIN user_hospitals uh ON uh.user_id = hur.user_id AND uh.hospital_id = hur.hospital_id
JOIN hospitals h ON h.id = hur.hospital_id
JOIN users u ON u.id = hur.user_id
JOIN roles r ON r.id = hur.role_id
WHERE hur.hospital_id = ? AND hur.user_id = ? AND UPPER(r.slug) = UPPER(?)
  AND uh.is_active = TRUE AND uh.deleted_at IS NULL
  AND h.is_active = TRUE AND h.deleted_at IS NULL
  AND u.status = 'active' AND u.deleted_at IS NULL
  AND r.active = TRUE AND r.deleted_at IS NULL`
	if err := r.db.WithContext(ctx).Raw(q, hospitalID, userID, roleSlug).Scan(&out).Error; err != nil {
		return false, err
	}
	return out.C > 0, nil
}

// IsUserHospitalAdmin true jika user adalah ADMIN (scoped ke hospital) sesuai constants.
func (r *Repository) IsUserHospitalAdmin(ctx context.Context, hospitalID, userID string) (bool, error) {
	return r.UserHasHospitalRole(ctx, hospitalID, userID, constant.RoleAdmin)
}

// UserHasAnyActiveHospitalRole checks a tenant role without granting any global
// role. It is suitable for profile gates such as a hospital-created doctor.
func (r *Repository) UserHasAnyActiveHospitalRole(ctx context.Context, userID, roleSlug string) (bool, error) {
	var exists bool
	const q = `
SELECT EXISTS(
	SELECT 1
	FROM hospital_user_roles hur
	JOIN user_hospitals uh ON uh.user_id = hur.user_id AND uh.hospital_id = hur.hospital_id
	JOIN hospitals h ON h.id = hur.hospital_id
	JOIN users u ON u.id = hur.user_id
	JOIN roles r ON r.id = hur.role_id
	WHERE hur.user_id = ? AND UPPER(r.slug) = UPPER(?)
	  AND uh.is_active = TRUE AND uh.deleted_at IS NULL
	  AND h.is_active = TRUE AND h.deleted_at IS NULL
	  AND u.status = 'active' AND u.deleted_at IS NULL
	  AND r.active = TRUE AND r.deleted_at IS NULL
)`
	if err := r.db.WithContext(ctx).Raw(q, userID, roleSlug).Scan(&exists).Error; err != nil {
		return false, err
	}
	return exists, nil
}
