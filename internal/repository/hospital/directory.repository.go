package hospital

import (
	"context"
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"gorm.io/gorm"
)

func (r *Repository) directoryQuery(ctx context.Context, latitude, longitude *float64) *gorm.DB {
	distance := "NULL::double precision AS distance_km"
	var args []any
	if latitude != nil && longitude != nil {
		distance = `CASE WHEN h.latitude IS NULL OR h.longitude IS NULL THEN NULL ELSE
            6371.0088 * ACOS(LEAST(1.0,GREATEST(-1.0,
            SIN(RADIANS(?::double precision))*SIN(RADIANS(h.latitude::double precision)) +
            COS(RADIANS(?::double precision))*COS(RADIANS(h.latitude::double precision))*COS(RADIANS(h.longitude::double precision-?::double precision))))) END AS distance_km`
		args = []any{*latitude, *latitude, *longitude}
	}
	inner := r.db.WithContext(ctx).Table("hospitals AS h").Select(`h.*, ratings.rating_average, COALESCE(ratings.rating_count,0) AS rating_count, `+distance, args...).
		Joins(`LEFT JOIN (SELECT hospital_id, AVG(rating)::double precision AS rating_average, COUNT(*) AS rating_count FROM hospital_reviews WHERE deleted_at IS NULL GROUP BY hospital_id) ratings ON ratings.hospital_id=h.id`).
		Where("h.is_active=TRUE AND h.deleted_at IS NULL")
	return r.db.WithContext(ctx).Table("(?) AS directory", inner).Select(publicHospitalColumns + ", distance_km, rating_average, rating_count")
}
func likePattern(value string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(value)) + "%"
}

func (r *Repository) ListDirectory(ctx context.Context, q request.HospitalDirectoryQuery) ([]response.Hospital, error) {
	query := r.directoryQuery(ctx, q.Latitude, q.Longitude)
	if q.Search != "" {
		p := likePattern(q.Search)
		query = query.Where("LOWER(name) LIKE ? OR LOWER(COALESCE(code,'')) LIKE ?", p, p)
	}
	if q.City != "" {
		query = query.Where("LOWER(city)=LOWER(?)", q.City)
	}
	if q.Department != "" {
		p := likePattern(q.Department)
		query = query.Where(`EXISTS(SELECT 1 FROM hospital_departments d WHERE d.hospital_id=directory.id AND d.is_active AND (LOWER(d.name) LIKE ? OR LOWER(d.code) LIKE ?))`, p, p)
	}
	if q.DepartmentCode != "" {
		query = query.Where(`EXISTS(SELECT 1 FROM hospital_departments d WHERE d.hospital_id=directory.id AND d.is_active AND LOWER(d.code)=LOWER(?))`, q.DepartmentCode)
	}
	if q.DepartmentID != "" {
		query = query.Where(`EXISTS(SELECT 1 FROM hospital_departments d WHERE d.hospital_id=directory.id AND d.is_active AND d.id=?)`, q.DepartmentID)
	}
	if q.MinRating != nil {
		query = query.Where("rating_average >= ?", *q.MinRating)
	}
	if q.RadiusKM != nil {
		query = query.Where("distance_km <= ?", *q.RadiusKM)
	}
	if q.Recommended {
		query = query.Where(`EXISTS(
			SELECT 1 FROM doctor_hospital_affiliations affiliation
			JOIN users doctor ON doctor.id = affiliation.doctor_id
			JOIN hospital_departments department ON department.id = affiliation.department_id
			JOIN doctor_hospital_schedules schedule ON schedule.affiliation_id = affiliation.id
			WHERE affiliation.hospital_id = directory.id AND affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL
			  AND doctor.status = 'active' AND doctor.deleted_at IS NULL AND department.is_active = TRUE AND schedule.is_active = TRUE
			  AND (schedule.schedule_date IS NULL OR ((schedule.schedule_date + schedule.end_time) AT TIME ZONE schedule.timezone) > CURRENT_TIMESTAMP)
		)`)
	}
	switch q.Sort {
	case "distance":
		query = query.Order("distance_km ASC NULLS LAST, name ASC, id ASC")
	case "rating":
		query = query.Order("rating_average DESC NULLS LAST, rating_count DESC, name ASC, id ASC")
	default:
		query = query.Order("name ASC, id ASC")
	}
	rows := make([]response.Hospital, 0)
	err := query.Limit(q.Limit).Offset(q.Offset).Scan(&rows).Error
	return rows, err
}

func (r *Repository) ListDepartmentOptions(ctx context.Context, q request.DepartmentDirectoryQuery) ([]response.DepartmentOption, error) {
	query := r.db.WithContext(ctx).Table("hospital_departments AS department").
		Select(`UPPER(department.code) AS code, MIN(department.name) AS name,
			COUNT(DISTINCT department.hospital_id) AS hospital_count,
			COUNT(DISTINCT affiliated_doctor.id) AS doctor_count`).
		Joins("JOIN hospitals hospital ON hospital.id = department.hospital_id").
		Joins(`LEFT JOIN doctor_hospital_affiliations affiliation
			ON affiliation.department_id = department.id AND affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL`).
		Joins(`LEFT JOIN users affiliated_doctor
			ON affiliated_doctor.id = affiliation.doctor_id AND affiliated_doctor.status = 'active' AND affiliated_doctor.deleted_at IS NULL`).
		Where("department.is_active = TRUE AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL")
	if q.Search != "" {
		pattern := likePattern(q.Search)
		query = query.Where("LOWER(department.code) LIKE ? OR LOWER(department.name) LIKE ?", pattern, pattern)
	}
	if q.HospitalID != "" {
		query = query.Where("department.hospital_id = ?", q.HospitalID)
	}
	rows := make([]response.DepartmentOption, 0)
	err := query.Group("UPPER(department.code)").Order("MIN(department.name) ASC, UPPER(department.code) ASC").
		Limit(q.Limit).Offset(q.Offset).Scan(&rows).Error
	return rows, err
}
func (r *Repository) GetDirectory(ctx context.Context, id string, latitude, longitude *float64) (*response.Hospital, error) {
	var row response.Hospital
	if err := r.directoryQuery(ctx, latitude, longitude).Where("id=?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
func (r *Repository) DirectoryDepartments(ctx context.Context, ids []string) ([]response.HospitalDepartmentSummary, error) {
	rows := make([]response.HospitalDepartmentSummary, 0)
	err := r.db.WithContext(ctx).Table("hospital_departments").Select("id,hospital_id,code,name").Where("hospital_id IN ? AND is_active=TRUE", ids).Order("name ASC, id ASC").Scan(&rows).Error
	return rows, err
}
func (r *Repository) DirectoryImages(ctx context.Context, ids []string, coverOnly bool) ([]response.HospitalImage, error) {
	rows := make([]response.HospitalImage, 0)
	q := r.db.WithContext(ctx).Table("hospital_images").Where("hospital_id IN ? AND deleted_at IS NULL", ids)
	if coverOnly {
		q = q.Where("is_cover=TRUE")
	}
	err := q.Order("sort_order ASC,created_at ASC,id ASC").Scan(&rows).Error
	return rows, err
}
