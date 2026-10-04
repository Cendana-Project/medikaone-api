package doctor

import (
	"context"
	"math"
	"strings"

	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/directorycriteria"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	directoryrepo "github.com/Cendana-Project/medikaone-api/internal/repository/directory"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type Filter struct {
	Gender             string
	MinExperienceYears *int
	MaxExperienceYears *int
	AvailableFrom      string
	AvailableTo        string
	OnlyAvailable      bool
	Query              string
	Specialty          string
	HospitalID         string
	DepartmentCode     string
	DepartmentID       string
	City               string
	AvailableOn        string
	BookingMode        string
	Latitude           *float64
	Longitude          *float64
	RadiusKM           *float64
	Recommended        bool
	Page               int
	Limit              int
}

const publicDoctorColumns = `
	doctor.id::text AS doctor_id, profile.medikaone_id AS doctor_medikaone_id,
	COALESCE(doctor.first_name, '') AS first_name, COALESCE(doctor.last_name, '') AS last_name,
	TRIM(CONCAT_WS(' ', doctor.first_name, doctor.last_name)) AS full_name,
	COALESCE(profile.sip_number, '') AS sip_number, COALESCE(profile.specialty, '') AS specialty,
	doctor.gender, profile.practice_started_on::text,
	EXTRACT(YEAR FROM AGE((CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date, profile.practice_started_on))::integer AS experience_years`

const publicDoctorTables = `
	FROM users doctor
	JOIN doctor_profiles profile ON profile.user_id = doctor.id`

const publicDoctorConditions = `
	WHERE doctor.status = 'active' AND doctor.deleted_at IS NULL AND doctor.verified_at IS NOT NULL
	  AND NULLIF(BTRIM(profile.sip_number), '') IS NOT NULL
	  AND EXISTS (
		SELECT 1 FROM roles role
		WHERE UPPER(role.slug) = 'DOCTOR' AND role.active = TRUE AND role.deleted_at IS NULL
		  AND (
			EXISTS (SELECT 1 FROM user_roles membership WHERE membership.user_id = doctor.id AND membership.role_id = role.id)
			OR EXISTS (
				SELECT 1 FROM hospital_user_roles assignment
				JOIN user_hospitals membership ON membership.user_id = assignment.user_id AND membership.hospital_id = assignment.hospital_id
				JOIN hospitals hospital ON hospital.id = assignment.hospital_id
				WHERE assignment.user_id = doctor.id AND assignment.role_id = role.id
				  AND membership.is_active = TRUE AND membership.deleted_at IS NULL
				  AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL
			)
		  )
	  )`

const publicDoctorFrom = publicDoctorTables + publicDoctorConditions

// Reused for eligibility and nearest practice, so a nearby affiliation cannot
// satisfy distance while a different one satisfies department/date/mode filters.
func practiceQuery(filter Filter) (string, []any) {
	where := ` FROM doctor_hospital_affiliations affiliation
		JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
		JOIN hospital_departments department ON department.id = affiliation.department_id
		LEFT JOIN hospital_rooms room ON room.id = affiliation.room_id
		WHERE affiliation.doctor_id = doctor.id
		  AND affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL
		  AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL AND department.is_active = TRUE
		  AND (affiliation.room_id IS NULL OR room.is_active = TRUE)`
	args := make([]any, 0)
	for _, condition := range []struct{ value, sql string }{
		{filter.HospitalID, " AND affiliation.hospital_id = ?"},
		{filter.DepartmentCode, " AND LOWER(department.code) = LOWER(?)"},
		{filter.DepartmentID, " AND department.id = ?"},
		{filter.City, " AND LOWER(hospital.city) = LOWER(?)"},
	} {
		if condition.value != "" {
			where += condition.sql
			args = append(args, condition.value)
		}
	}
	if filter.AvailableOn != "" || filter.BookingMode != "" || filter.Recommended || filter.Latitude != nil {
		predicate, values := directoryrepo.SchedulePredicate(directorycriteria.Availability{
			Date: filter.AvailableOn, From: filter.AvailableFrom, To: filter.AvailableTo,
			OnlyAvailable: filter.OnlyAvailable, BookingMode: filter.BookingMode,
		})
		where += " AND EXISTS (SELECT 1 FROM doctor_hospital_schedules schedule WHERE schedule.affiliation_id = affiliation.id AND " + predicate + ")"
		args = append(args, values...)
	}
	return where, args
}

func filterQuery(filter Filter) (string, []any) {
	where := publicDoctorFrom
	args := make([]any, 0)
	practice, practiceArgs := practiceQuery(filter)
	if filter.Latitude != nil && filter.Longitude != nil {
		where = publicDoctorTables + ` LEFT JOIN LATERAL (
			SELECT affiliation.id AS affiliation_id, hospital.id AS hospital_id, hospital.name AS hospital_name,
			       department.id AS department_id, department.name AS department_name, hospital.latitude, hospital.longitude,
			       CASE WHEN hospital.latitude IS NULL OR hospital.longitude IS NULL THEN NULL ELSE
			       6371.0088 * ACOS(LEAST(1.0, GREATEST(-1.0,
			           SIN(RADIANS(?::double precision)) * SIN(RADIANS(hospital.latitude::double precision)) +
			           COS(RADIANS(?::double precision)) * COS(RADIANS(hospital.latitude::double precision)) *
			           COS(RADIANS(hospital.longitude::double precision - ?::double precision))))) END AS distance_km
			` + practice + ` ORDER BY distance_km ASC NULLS LAST, hospital.id, affiliation.id LIMIT 1
		) nearest ON TRUE ` + publicDoctorConditions + " AND nearest.affiliation_id IS NOT NULL"
		args = append(args, *filter.Latitude, *filter.Latitude, *filter.Longitude)
		args = append(args, practiceArgs...)
		if filter.RadiusKM != nil {
			where += " AND nearest.distance_km <= ?"
			args = append(args, *filter.RadiusKM)
		}
	}
	if filter.Query != "" {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(filter.Query)) + "%"
		where += ` AND (LOWER(CONCAT_WS(' ', doctor.first_name, doctor.last_name)) LIKE ?
			OR LOWER(profile.medikaone_id) LIKE ? OR LOWER(profile.sip_number) LIKE ?
			OR LOWER(COALESCE(profile.specialty, '')) LIKE ?)`
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if filter.Specialty != "" {
		where += ` AND LOWER(profile.specialty) = LOWER(?)`
		args = append(args, filter.Specialty)
	}
	if filter.Gender != "" {
		where += " AND doctor.gender = ?"
		args = append(args, filter.Gender)
	}
	const experience = "EXTRACT(YEAR FROM AGE((CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date, profile.practice_started_on))"
	if filter.MinExperienceYears != nil {
		where += " AND " + experience + " >= ?"
		args = append(args, *filter.MinExperienceYears)
	}
	if filter.MaxExperienceYears != nil {
		where += " AND " + experience + " <= ?"
		args = append(args, *filter.MaxExperienceYears)
	}
	if filter.Latitude == nil && (filter.HospitalID != "" || filter.DepartmentCode != "" || filter.DepartmentID != "" || filter.City != "" || filter.AvailableOn != "" || filter.BookingMode != "" || filter.Recommended) {
		where += " AND EXISTS (SELECT 1 " + practice + ")"
		args = append(args, practiceArgs...)
	}
	return where, args
}

func (r *Repository) ListDoctors(ctx context.Context, filter Filter) (*response.PublicDoctorPage, error) {
	where, args := filterQuery(filter)
	out := &response.PublicDoctorPage{Items: make([]response.PublicDoctor, 0), Page: filter.Page, Limit: filter.Limit}
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) `+where, args...).Scan(&out.Total).Error; err != nil {
		return nil, err
	}
	order := "LOWER(doctor.first_name), LOWER(doctor.last_name), doctor.id"
	if filter.Recommended {
		order = `(
			SELECT COALESCE(AVG(review.rating)::double precision, 0)
			FROM doctor_hospital_affiliations ranked_affiliation
			JOIN hospitals ranked_hospital ON ranked_hospital.id = ranked_affiliation.hospital_id
			JOIN hospital_departments ranked_department ON ranked_department.id = ranked_affiliation.department_id AND ranked_department.is_active = TRUE
			LEFT JOIN hospital_reviews review ON review.hospital_id = ranked_hospital.id AND review.deleted_at IS NULL
			WHERE ranked_affiliation.doctor_id = doctor.id AND ranked_affiliation.status = 'ACTIVE'
			  AND ranked_affiliation.deleted_at IS NULL AND ranked_hospital.is_active = TRUE AND ranked_hospital.deleted_at IS NULL
		) DESC, (
			SELECT COUNT(*) FROM doctor_hospital_affiliations ranked_affiliation
			JOIN hospitals ranked_hospital ON ranked_hospital.id = ranked_affiliation.hospital_id AND ranked_hospital.is_active = TRUE AND ranked_hospital.deleted_at IS NULL
			JOIN hospital_departments ranked_department ON ranked_department.id = ranked_affiliation.department_id AND ranked_department.is_active = TRUE
			JOIN doctor_hospital_schedules ranked_schedule ON ranked_schedule.affiliation_id = ranked_affiliation.id
			WHERE ranked_affiliation.doctor_id = doctor.id AND ranked_affiliation.status = 'ACTIVE'
			  AND ranked_affiliation.deleted_at IS NULL AND ranked_schedule.is_active = TRUE
			  AND (ranked_schedule.schedule_date IS NULL OR ((ranked_schedule.schedule_date + ranked_schedule.end_time) AT TIME ZONE ranked_schedule.timezone) > CURRENT_TIMESTAMP)
		) DESC, LOWER(doctor.first_name), LOWER(doctor.last_name), doctor.id`
	}
	columns := publicDoctorColumns
	withLocation := filter.Latitude != nil && filter.Longitude != nil
	if withLocation {
		order = "nearest.distance_km ASC NULLS LAST, " + order
		columns += `, nearest.affiliation_id::text AS nearest_affiliation_id,
			nearest.hospital_id::text AS nearest_hospital_id, nearest.hospital_name AS nearest_hospital_name,
			nearest.department_id::text AS nearest_department_id, nearest.department_name AS nearest_department_name,
			nearest.latitude AS nearest_latitude, nearest.longitude AS nearest_longitude, nearest.distance_km AS nearest_distance_km`
	}
	query := `SELECT ` + columns + where + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)
	if !withLocation {
		if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&out.Items).Error; err != nil {
			return nil, err
		}
		return out, nil
	}
	var rows []doctorLocationRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.NearestDistanceKM != nil && row.NearestLatitude != nil && row.NearestLongitude != nil {
			row.NearestPractice = &response.DoctorPracticeLocation{
				AffiliationID: row.NearestAffiliationID, HospitalID: row.NearestHospitalID, HospitalName: row.NearestHospitalName,
				DepartmentID: row.NearestDepartmentID, DepartmentName: row.NearestDepartmentName,
				Latitude: *row.NearestLatitude, Longitude: *row.NearestLongitude,
				DistanceKM: math.Round(*row.NearestDistanceKM*100) / 100,
			}
		}
		out.Items = append(out.Items, row.PublicDoctor)
	}
	return out, nil
}

type doctorLocationRow struct {
	response.PublicDoctor
	NearestAffiliationID  string
	NearestHospitalID     string
	NearestHospitalName   string
	NearestDepartmentID   string
	NearestDepartmentName string
	NearestLatitude       *float64
	NearestLongitude      *float64
	NearestDistanceKM     *float64 `gorm:"column:nearest_distance_km"`
}

// GetDoctor accepts either the relational UUID or the human-facing MedikaOne ID.
func (r *Repository) GetDoctor(ctx context.Context, identity string) (*response.PublicDoctorDetail, error) {
	var out response.PublicDoctorDetail
	result := r.db.WithContext(ctx).Raw(`SELECT `+publicDoctorColumns+publicDoctorFrom+
		` AND (doctor.id::text = ? OR profile.medikaone_id = ?) LIMIT 1`, identity, strings.ToUpper(identity)).Scan(&out.PublicDoctor)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	out.Affiliations = make([]response.PublicDoctorAffiliation, 0)
	if err := r.db.WithContext(ctx).Raw(`
		SELECT affiliation.id::text AS affiliation_id, affiliation.hospital_id::text,
		       COALESCE(hospital.code, '') AS hospital_code, hospital.name AS hospital_name,
		       affiliation.department_id::text, department.name AS department_name,
		       affiliation.room_id::text, room.name AS room_name
		FROM doctor_hospital_affiliations affiliation
		JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
		JOIN hospital_departments department ON department.id = affiliation.department_id
		LEFT JOIN hospital_rooms room ON room.id = affiliation.room_id
		WHERE affiliation.doctor_id = ? AND affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL
		  AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL AND department.is_active = TRUE
		  AND (room.id IS NULL OR room.is_active = TRUE)
		ORDER BY hospital.name, department.name, affiliation.id
	`, out.DoctorID).Scan(&out.Affiliations).Error; err != nil {
		return nil, err
	}
	if len(out.Affiliations) == 0 {
		return &out, nil
	}
	ids := make([]string, 0, len(out.Affiliations))
	for i := range out.Affiliations {
		ids = append(ids, out.Affiliations[i].AffiliationID)
		out.Affiliations[i].Schedules = make([]response.DoctorHospitalSchedule, 0)
	}
	var schedules []struct {
		AffiliationID string
		response.DoctorHospitalSchedule
	}
	if err := r.db.WithContext(ctx).Raw(`
		SELECT id::text, affiliation_id::text, day_of_week, schedule_date::text AS schedule_date,
		       TO_CHAR(start_time, 'HH24:MI') AS start_time, TO_CHAR(end_time, 'HH24:MI') AS end_time,
		       timezone, booking_mode, slot_duration_minutes, capacity
		FROM doctor_hospital_schedules
		WHERE affiliation_id IN ? AND is_active = TRUE
		  AND (schedule_date IS NULL OR schedule_date >= (CURRENT_TIMESTAMP AT TIME ZONE timezone)::date)
		ORDER BY schedule_date NULLS FIRST, day_of_week, start_time, id
	`, ids).Scan(&schedules).Error; err != nil {
		return nil, err
	}
	byAffiliation := make(map[string][]response.DoctorHospitalSchedule)
	for _, schedule := range schedules {
		byAffiliation[schedule.AffiliationID] = append(byAffiliation[schedule.AffiliationID], schedule.DoctorHospitalSchedule)
	}
	for i := range out.Affiliations {
		if list, ok := byAffiliation[out.Affiliations[i].AffiliationID]; ok {
			out.Affiliations[i].Schedules = list
		}
	}
	return &out, nil
}
