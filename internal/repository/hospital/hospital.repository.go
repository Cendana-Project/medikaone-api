package hospital

import (
	"context"
	"errors"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"

	"gorm.io/gorm"
)

type Hospital struct {
	ID       string  `gorm:"column:id"`
	Code     *string `gorm:"column:code"`
	Name     string  `gorm:"column:name"`
	IsActive bool    `gorm:"column:is_active"`
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) WithTx(tx *gorm.DB) *Repository { return &Repository{db: tx} }

func (r *Repository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

func (r *Repository) Create(ctx context.Context, h *entity.Hospital) error {
	return r.db.WithContext(ctx).Create(h).Error
}

func (r *Repository) FindByID(ctx context.Context, id string) (*Hospital, error) {
	var row Hospital
	if err := r.db.WithContext(ctx).
		Raw(`SELECT id, code, name, is_active FROM hospitals WHERE id::text = ? AND is_active = TRUE AND deleted_at IS NULL LIMIT 1`, id).
		Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

func (r *Repository) FindByCode(ctx context.Context, code string) (*Hospital, error) {
	var row Hospital
	if err := r.db.WithContext(ctx).
		Raw(`SELECT id, code, name, is_active FROM hospitals WHERE LOWER(code) = LOWER(?) AND is_active = TRUE AND deleted_at IS NULL LIMIT 1`, code).
		Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

// EnsureMembership creates or reactivates a membership. A primary membership
// is unique per user and is changed atomically.
func (r *Repository) EnsureMembership(ctx context.Context, userID, hospitalID string, setPrimary bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if setPrimary {
			if err := tx.Exec(`
				UPDATE user_hospitals
				SET is_primary = FALSE, updated_at = NOW()
				WHERE user_id = ? AND hospital_id <> ? AND deleted_at IS NULL
			`, userID, hospitalID).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`
			INSERT INTO user_hospitals (user_id, hospital_id, is_active, is_primary, created_at, updated_at, deleted_at)
			VALUES (?, ?, TRUE, ?, NOW(), NOW(), NULL)
			ON CONFLICT (user_id, hospital_id) DO UPDATE SET
				is_active = TRUE,
				is_primary = CASE WHEN EXCLUDED.is_primary THEN TRUE ELSE user_hospitals.is_primary END,
				updated_at = NOW(),
				deleted_at = NULL
		`, userID, hospitalID, setPrimary).Error; err != nil {
			return err
		}
		return nil
	})
}

// AssignHospitalRole: assign role di scope hospital
func (r *Repository) AssignHospitalRole(ctx context.Context, userID, hospitalID, roleID string) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO hospital_user_roles (hospital_id, user_id, role_id)
		VALUES (?, ?, ?)
		ON CONFLICT (hospital_id, user_id, role_id) DO NOTHING
	`, hospitalID, userID, roleID).Error
}

// Helper: resolve hospital id dari id / code
func (r *Repository) ResolveHospitalID(ctx context.Context, idOrCode string) (string, error) {
	var h Hospital
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, code, name, is_active
		FROM hospitals
		WHERE (id::text = ? OR LOWER(code) = LOWER(?))
		  AND is_active = TRUE
		  AND deleted_at IS NULL
		LIMIT 1
	`, idOrCode, idOrCode).Scan(&h).Error
	if err == nil && h.ID == "" {
		err = gorm.ErrRecordNotFound
	}
	return retID(&h, err)
}

func retID(h *Hospital, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if h == nil || h.ID == "" || !h.IsActive {
		return "", errors.New("hospital not found or inactive")
	}
	return h.ID, nil
}

func (r *Repository) IsUserLinkedToHospital(ctx context.Context, userID, hospitalID string) (bool, error) {
	var cnt int64
	if err := r.db.WithContext(ctx).
		Table("user_hospitals uh").
		Joins("JOIN users u ON u.id = uh.user_id").
		Joins("JOIN hospitals h ON h.id = uh.hospital_id").
		Where(`uh.user_id = ? AND uh.hospital_id = ?
			AND uh.is_active = TRUE AND uh.deleted_at IS NULL
			AND u.status = 'active' AND u.deleted_at IS NULL
			AND h.is_active = TRUE AND h.deleted_at IS NULL`, userID, hospitalID).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}
func (r *Repository) IsCodeExists(ctx context.Context, code string) (bool, error) {
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&entity.Hospital{}).
		Where("LOWER(code)=LOWER(?) AND deleted_at IS NULL", code).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func (r *Repository) IsNameExists(ctx context.Context, name string) (bool, error) {
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&entity.Hospital{}).
		Where("LOWER(name)=LOWER(?) AND deleted_at IS NULL", name).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}
