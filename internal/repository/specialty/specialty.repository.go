package specialty

import (
	"context"
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) ListSpecialties(ctx context.Context, q request.SpecialtyQuery) ([]response.Specialty, error) {
	query := r.db.WithContext(ctx).Table("master_specialties").
		Select("id, code, name, category").Where("is_active = TRUE")
	if q.Search != "" {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(q.Search)) + "%"
		query = query.Where("(LOWER(name) LIKE ? OR LOWER(code) LIKE ?)", pattern, pattern)
	}
	if q.Code != "" {
		query = query.Where("code = ?", q.Code)
	}
	if q.Category != "" {
		query = query.Where("category = ?", q.Category)
	}
	rows := make([]response.Specialty, 0)
	err := query.Order("sort_order ASC, name ASC, id ASC").Limit(q.Limit).Offset(q.Offset).Scan(&rows).Error
	return rows, err
}
