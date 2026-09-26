package doctor_hospital

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/Cendana-Project/medikaone-api/internal/scheduleconflict"
)

var (
	ErrInvitationNotFound        = errors.New("doctor hospital invitation not found")
	ErrInvitationExists          = errors.New("an open invitation or affiliation already exists")
	ErrInvitationExpired         = errors.New("doctor hospital invitation expired")
	ErrInvalidInvitationState    = errors.New("invalid doctor hospital invitation state")
	ErrPlacementNotFound         = errors.New("department or room not found")
	ErrMasterDepartmentNotFound  = errors.New("master department not found or inactive")
	ErrScheduleConflict          = errors.New("doctor schedule conflicts with an active affiliation")
	ErrAffiliationNotFound       = errors.New("doctor hospital affiliation not found")
	ErrNotificationNotFound      = errors.New("notification not found")
	ErrDoctorNotEligible         = errors.New("doctor is not eligible")
	ErrHospitalWorkerDOBRequired = errors.New("hospital worker date of birth is required")
	ErrHospitalWorkerUnderage    = errors.New("hospital worker minimum age is not met")
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type Document struct {
	Filename   string
	MIMEType   string
	Bucket     string
	ObjectPath string
	FileSize   int64
	SHA256     string
}

type Schedule struct {
	DayOfWeek           int
	ScheduleDate        *string
	StartTime           string
	EndTime             string
	Timezone            string
	BookingMode         string
	SlotDurationMinutes int
	Capacity            int
}

type CreateInvitationInput struct {
	InvitationID string
	HospitalID   string
	DoctorID     string
	DepartmentID string
	RoomID       *string
	InvitedBy    string
	Message      *string
	ExpiresAt    time.Time
	Contract     Document
	Schedules    []Schedule
	Now          time.Time
}

type ContractDocument struct {
	Filename   string
	MIMEType   string
	Bucket     string
	ObjectPath string
	FileSize   int64
	SHA256     string
}

const eligibleDoctorSelect = `
	SELECT u.id, u.email, dp.medikaone_id AS doctor_medikaone_id,
	       COALESCE(u.username, '') AS username,
	       COALESCE(u.phone, '') AS phone,
	       COALESCE(u.first_name, '') AS first_name,
	       COALESCE(u.last_name, '') AS last_name,
	       COALESCE(dp.sip_number, '') AS sip_number,
	       COALESCE(dp.specialty, '') AS specialty
	FROM users u
	JOIN doctor_profiles dp ON dp.user_id = u.id
	JOIN roles role ON UPPER(role.slug) = ?
	WHERE role.active = TRUE
	  AND role.deleted_at IS NULL
	  AND (
	       EXISTS (SELECT 1 FROM user_roles membership WHERE membership.user_id = u.id AND membership.role_id = role.id)
	       OR EXISTS (
	           SELECT 1 FROM hospital_user_roles assignment
	           JOIN user_hospitals membership ON membership.user_id = assignment.user_id AND membership.hospital_id = assignment.hospital_id
	           JOIN hospitals hospital ON hospital.id = assignment.hospital_id
	           WHERE assignment.user_id = u.id AND assignment.role_id = role.id
	             AND membership.is_active = TRUE AND membership.deleted_at IS NULL
	             AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL
	       )
	  )
	  AND u.deleted_at IS NULL
	  AND u.status = 'active'
	  AND u.verified_at IS NOT NULL
	  AND NULLIF(TRIM(dp.sip_number), '') IS NOT NULL
	  AND u.dob IS NOT NULL
	  AND (u.dob + INTERVAL '15 years') < CURRENT_DATE`

func (r *Repository) SearchEligibleDoctors(ctx context.Context, identity string, limit int) ([]response.DoctorSearchResult, error) {
	identity = strings.TrimSpace(identity)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	loweredIdentity := strings.ToLower(identity)
	containsPattern := "%" + escapeLikePattern(loweredIdentity) + "%"
	normalizedPhone := normalizePhoneIdentity(identity)

	query := eligibleDoctorSelect + `
	  AND (
	       u.id::text = ?
	       OR dp.medikaone_id = ?
	       OR LOWER(u.email) LIKE ?
	       OR LOWER(COALESCE(u.username, '')) LIKE ?
	       OR LOWER(COALESCE(dp.sip_number, '')) LIKE ?
	       OR LOWER(COALESCE(u.first_name, '')) LIKE ?
	       OR LOWER(COALESCE(u.last_name, '')) LIKE ?
	       OR LOWER(CONCAT_WS(' ', u.first_name, u.last_name)) LIKE ?
	       OR LOWER(COALESCE(dp.specialty, '')) LIKE ?
	       OR (? <> '' AND REGEXP_REPLACE(COALESCE(u.phone, ''), '[^0-9]', '', 'g') = ?)
	  )
	ORDER BY CASE
	           WHEN u.id::text = ?
	             OR LOWER(u.email) = ?
	             OR LOWER(COALESCE(u.username, '')) = ?
	             OR LOWER(COALESCE(dp.sip_number, '')) = ?
	             OR (? <> '' AND REGEXP_REPLACE(COALESCE(u.phone, ''), '[^0-9]', '', 'g') = ?)
	             OR LOWER(CONCAT_WS(' ', u.first_name, u.last_name)) = ?
	           THEN 0 ELSE 1
	         END,
	         LOWER(COALESCE(u.first_name, '')),
	         LOWER(COALESCE(u.last_name, '')),
	         u.id
	LIMIT ?`

	args := []any{
		constant.RoleDoctor,
		identity,
		strings.ToUpper(identity),
		containsPattern, containsPattern, containsPattern, containsPattern,
		containsPattern, containsPattern, containsPattern,
		normalizedPhone, normalizedPhone,
		identity, loweredIdentity, loweredIdentity, loweredIdentity,
		normalizedPhone, normalizedPhone, loweredIdentity,
		limit,
	}
	out := make([]response.DoctorSearchResult, 0)
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) FindEligibleDoctorByID(ctx context.Context, doctorID string) (*response.DoctorSearchResult, error) {
	var out response.DoctorSearchResult
	query := eligibleDoctorSelect + " AND u.id = ? LIMIT 1"
	if err := r.db.WithContext(ctx).Raw(query, constant.RoleDoctor, doctorID).Scan(&out).Error; err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &out, nil
}

func escapeLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// GetDoctorMedikaOneID includes historical/inactive profiles: suspending a
// hospital affiliation must not require a doctor to remain invite-eligible.
func (r *Repository) GetDoctorMedikaOneID(ctx context.Context, doctorID string) (string, error) {
	var identity string
	result := r.db.WithContext(ctx).Raw(`SELECT medikaone_id FROM doctor_profiles WHERE user_id = ?`, doctorID).Scan(&identity)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", ErrAffiliationNotFound
	}
	return identity, nil
}

func normalizePhoneIdentity(value string) string {
	var digits strings.Builder
	for _, char := range value {
		switch {
		case char >= '0' && char <= '9':
			digits.WriteRune(char)
		case char == '+', char == '-', char == '.', char == ' ', char == '(', char == ')':
			continue
		default:
			return ""
		}
	}
	if digits.Len() < 7 {
		return ""
	}
	return digits.String()
}

func activeMasterDepartment(tx *gorm.DB, masterDepartmentID string) (*entity.MasterDepartment, error) {
	var master entity.MasterDepartment
	if err := tx.Raw(`SELECT id, code, name, category, sort_order, is_active, created_at, updated_at
		FROM master_departments WHERE id = ? AND is_active = TRUE FOR SHARE`, masterDepartmentID).Scan(&master).Error; err != nil {
		return nil, err
	}
	if master.ID == "" {
		return nil, ErrMasterDepartmentNotFound
	}
	return &master, nil
}

func (r *Repository) CreateDepartment(ctx context.Context, hospitalID, masterDepartmentID string, now time.Time) (*entity.HospitalDepartment, error) {
	department := &entity.HospitalDepartment{}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var id string
		if err := tx.Raw(`SELECT id FROM hospitals WHERE id = ? AND is_active = TRUE AND deleted_at IS NULL FOR SHARE`, hospitalID).Scan(&id).Error; err != nil {
			return err
		}
		if id == "" {
			return ErrPlacementNotFound
		}
		master, err := activeMasterDepartment(tx, masterDepartmentID)
		if err != nil {
			return err
		}
		department = &entity.HospitalDepartment{
			ID: uuid.NewString(), HospitalID: hospitalID, MasterDepartmentID: &master.ID,
			Code: master.Code, Name: master.Name, IsActive: true, CreatedAt: now, UpdatedAt: now,
		}
		return tx.Create(department).Error
	}); err != nil {
		return nil, err
	}
	return department, nil
}

func (r *Repository) ListDepartments(ctx context.Context, hospitalID string) ([]entity.HospitalDepartment, error) {
	var rows []entity.HospitalDepartment
	err := r.db.WithContext(ctx).
		Where("hospital_id = ? AND is_active = TRUE", hospitalID).
		Order("name ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) CreateRoom(ctx context.Context, hospitalID, departmentID, code, name string, now time.Time) (*entity.HospitalRoom, error) {
	ok, err := r.DepartmentExists(ctx, hospitalID, departmentID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPlacementNotFound
	}
	room := &entity.HospitalRoom{
		ID: uuid.NewString(), HospitalID: hospitalID, DepartmentID: departmentID,
		Code: code, Name: name, IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockActivePlacement(tx, hospitalID, departmentID, nil); err != nil {
			return err
		}
		return tx.Create(room).Error
	}); err != nil {
		return nil, err
	}
	return room, nil
}

func (r *Repository) ListRooms(ctx context.Context, hospitalID, departmentID string) ([]entity.HospitalRoom, error) {
	q := r.db.WithContext(ctx).Where("hospital_id = ? AND is_active = TRUE", hospitalID)
	if departmentID != "" {
		q = q.Where("department_id = ?", departmentID)
	}
	var rows []entity.HospitalRoom
	err := q.Order("name ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) DepartmentExists(ctx context.Context, hospitalID, departmentID string) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM hospital_departments
			WHERE id = ? AND hospital_id = ? AND is_active = TRUE
		)`, departmentID, hospitalID).Scan(&exists).Error
	return exists, err
}

func (r *Repository) RoomMatchesDepartment(ctx context.Context, hospitalID, departmentID, roomID string) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM hospital_rooms
			WHERE id = ? AND hospital_id = ? AND department_id = ? AND is_active = TRUE
		)`, roomID, hospitalID, departmentID).Scan(&exists).Error
	return exists, err
}

func (r *Repository) HasActiveScheduleConflict(ctx context.Context, doctorID string, schedules []Schedule) (bool, error) {
	return hasActiveScheduleConflict(r.db.WithContext(ctx), doctorID, schedules, time.Now().UTC())
}

func hasActiveScheduleConflict(db *gorm.DB, doctorID string, schedules []Schedule, now time.Time) (bool, error) {
	proposed := conflictSchedules(schedules)
	if conflict, err := scheduleconflict.AnyOverlap(proposed, now); err != nil || conflict {
		return conflict, err
	}
	var active []scheduleconflict.Schedule
	if err := db.Raw(`
		SELECT existing.day_of_week, existing.schedule_date::text AS schedule_date,
		       TO_CHAR(existing.start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(existing.end_time, 'HH24:MI') AS end_time, existing.timezone
		FROM doctor_hospital_affiliations affiliation
		JOIN doctor_hospital_schedules existing ON existing.affiliation_id = affiliation.id
		WHERE affiliation.doctor_id = ? AND affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL
		  AND existing.is_active = TRUE
		  AND (existing.schedule_date IS NULL OR ((existing.schedule_date + existing.end_time) AT TIME ZONE existing.timezone) > ?)`,
		doctorID, now).Scan(&active).Error; err != nil {
		return false, err
	}
	return scheduleconflict.AnyConflict(proposed, active, now)
}

func conflictSchedules(schedules []Schedule) []scheduleconflict.Schedule {
	result := make([]scheduleconflict.Schedule, 0, len(schedules))
	for _, schedule := range schedules {
		timezone := schedule.Timezone
		if timezone == "" {
			timezone = "Asia/Jakarta"
		}
		result = append(result, scheduleconflict.Schedule{
			DayOfWeek: schedule.DayOfWeek, ScheduleDate: schedule.ScheduleDate,
			StartTime: schedule.StartTime, EndTime: schedule.EndTime, Timezone: timezone,
		})
	}
	return result
}

func invitationScheduleConflict(db *gorm.DB, doctorID, invitationID string, now time.Time) (bool, error) {
	var proposed []scheduleconflict.Schedule
	if err := db.Raw(`
		SELECT day_of_week, schedule_date::text AS schedule_date,
		       TO_CHAR(start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(end_time, 'HH24:MI') AS end_time, timezone
		FROM doctor_hospital_invitation_schedules WHERE invitation_id = ?`, invitationID).Scan(&proposed).Error; err != nil {
		return false, err
	}
	if conflict, err := scheduleconflict.AnyOverlap(proposed, now); err != nil || conflict {
		return conflict, err
	}
	var active []scheduleconflict.Schedule
	if err := db.Raw(`
		SELECT existing.day_of_week, existing.schedule_date::text AS schedule_date,
		       TO_CHAR(existing.start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(existing.end_time, 'HH24:MI') AS end_time, existing.timezone
		FROM doctor_hospital_affiliations affiliation
		JOIN doctor_hospital_schedules existing ON existing.affiliation_id = affiliation.id
		WHERE affiliation.doctor_id = ? AND affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL
		  AND existing.is_active = TRUE
		  AND (existing.schedule_date IS NULL OR ((existing.schedule_date + existing.end_time) AT TIME ZONE existing.timezone) > ?)`,
		doctorID, now).Scan(&active).Error; err != nil {
		return false, err
	}
	return scheduleconflict.AnyConflict(proposed, active, now)
}

func (r *Repository) CreateInvitation(ctx context.Context, input CreateInvitationInput) (*response.DoctorHospitalInvitation, error) {
	invitationID := input.InvitationID
	if invitationID == "" {
		invitationID = uuid.NewString()
	}
	notificationID := uuid.NewString()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockActivePlacement(tx, input.HospitalID, input.DepartmentID, input.RoomID); err != nil {
			return err
		}
		if err := expirePendingInvitations(tx, input.HospitalID, input.DoctorID, input.Now); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(CAST(? AS text), 0))", input.DoctorID).Error; err != nil {
			return err
		}
		if err := lockAndValidateHospitalWorkerDOB(tx, input.DoctorID, input.Now); err != nil {
			return err
		}
		if len(input.Schedules) > 0 {
			conflict, err := hasActiveScheduleConflict(tx, input.DoctorID, input.Schedules, input.Now)
			if err != nil {
				return err
			}
			if conflict {
				return ErrScheduleConflict
			}
		}

		var exists bool
		if err := tx.Raw(`
			SELECT EXISTS(
				SELECT 1 FROM doctor_hospital_affiliations
				WHERE hospital_id = ? AND doctor_id = ? AND department_id = ?
				  AND deleted_at IS NULL
				  AND COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid)
				      = COALESCE(?::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
				UNION ALL
				SELECT 1 FROM doctor_hospital_invitations
				WHERE hospital_id = ? AND doctor_id = ? AND department_id = ?
				  AND COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid)
				      = COALESCE(?::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
				  AND status = 'PENDING' AND deleted_at IS NULL
			)`, input.HospitalID, input.DoctorID, input.DepartmentID, input.RoomID,
			input.HospitalID, input.DoctorID, input.DepartmentID, input.RoomID).Scan(&exists).Error; err != nil {
			return err
		}
		if exists {
			return ErrInvitationExists
		}

		if err := tx.Exec(`
			INSERT INTO doctor_hospital_invitations (
				id, hospital_id, doctor_id, department_id, room_id, invited_by,
				status, message, expires_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, 'PENDING', ?, ?, ?, ?)`,
			invitationID, input.HospitalID, input.DoctorID, input.DepartmentID, input.RoomID,
			input.InvitedBy, input.Message, input.ExpiresAt, input.Now, input.Now).Error; err != nil {
			return err
		}

		if err := tx.Exec(`
			INSERT INTO doctor_hospital_contracts (
				id, invitation_id, original_filename, original_mime_type,
				original_bucket, original_object_path, original_file_size, original_sha256,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), invitationID, input.Contract.Filename, input.Contract.MIMEType,
			input.Contract.Bucket, input.Contract.ObjectPath, input.Contract.FileSize,
			input.Contract.SHA256, input.Now, input.Now).Error; err != nil {
			return err
		}
		if err := insertInvitationEvent(tx, invitationID, input.InvitedBy, "CREATED", input.Now); err != nil {
			return err
		}

		for _, schedule := range input.Schedules {
			if err := tx.Exec(`
				INSERT INTO doctor_hospital_invitation_schedules (
					id, invitation_id, day_of_week, schedule_date, start_time, end_time, timezone,
					booking_mode, slot_duration_minutes, capacity, created_at
				) VALUES (?, ?, ?, ?::date, ?::time, ?::time, ?, ?, ?, ?, ?)`,
				uuid.NewString(), invitationID, schedule.DayOfWeek, schedule.ScheduleDate, schedule.StartTime,
				schedule.EndTime, schedule.Timezone, schedule.BookingMode,
				schedule.SlotDurationMinutes, schedule.Capacity, input.Now).Error; err != nil {
				return err
			}
		}

		data, _ := json.Marshal(map[string]any{
			"invitation_id": invitationID,
			"hospital_id":   input.HospitalID,
			"event":         "DOCTOR_HOSPITAL_INVITATION_CREATED",
		})
		if err := tx.Exec(`
			INSERT INTO notifications (id, user_id, type, title, body, data, created_at)
			SELECT ?, ?, 'DOCTOR_HOSPITAL_INVITATION',
			       'Undangan rumah sakit',
			       'Anda menerima undangan untuk bergabung dengan ' || name || '.',
			       ?::jsonb, ?
			FROM hospitals WHERE id = ?`,
			notificationID, input.DoctorID, string(data), input.Now, input.HospitalID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetInvitationForHospital(ctx, input.HospitalID, invitationID, input.Now)
}

func expirePendingInvitations(tx *gorm.DB, hospitalID, doctorID string, now time.Time) error {
	return tx.Transaction(func(expiryTx *gorm.DB) error {
		var invitations []entity.DoctorHospitalInvitation
		q := expiryTx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? AND expires_at <= ? AND deleted_at IS NULL", entity.DoctorHospitalInvitationPending, now)
		if hospitalID != "" {
			q = q.Where("hospital_id = ?", hospitalID)
		}
		if doctorID != "" {
			q = q.Where("doctor_id = ?", doctorID)
		}
		if err := q.Find(&invitations).Error; err != nil {
			return err
		}
		for i := range invitations {
			result := expiryTx.Model(&entity.DoctorHospitalInvitation{}).
				Where("id = ? AND status = ?", invitations[i].ID, entity.DoctorHospitalInvitationPending).
				Updates(map[string]any{"status": entity.DoctorHospitalInvitationExpired, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				if err := insertInvitationEvent(expiryTx, invitations[i].ID, nil, "EXPIRED", now); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *Repository) ListInvitationsForDoctor(ctx context.Context, doctorID, status string, now time.Time) ([]response.DoctorHospitalInvitation, error) {
	if err := expirePendingInvitations(r.db.WithContext(ctx), "", doctorID, now); err != nil {
		return nil, err
	}
	return r.listInvitations(ctx, "doctor", doctorID, status)
}

func (r *Repository) ListInvitationsForHospital(ctx context.Context, hospitalID, status string, now time.Time) ([]response.DoctorHospitalInvitation, error) {
	if err := expirePendingInvitations(r.db.WithContext(ctx), hospitalID, "", now); err != nil {
		return nil, err
	}
	return r.listInvitations(ctx, "hospital", hospitalID, status)
}

func (r *Repository) listInvitations(ctx context.Context, ownerType, ownerID, status string) ([]response.DoctorHospitalInvitation, error) {
	where := "i.doctor_id = ?"
	if ownerType == "hospital" {
		where = "i.hospital_id = ?"
	}
	where += " AND i.deleted_at IS NULL"
	args := []any{ownerID}
	if status != "" {
		where += " AND i.status = ?"
		args = append(args, status)
	}
	var rows []response.DoctorHospitalInvitation
	query := invitationSelect + " WHERE " + where + " ORDER BY i.created_at DESC LIMIT 100"
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if err := r.attachInvitationSchedules(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) GetInvitationForDoctor(ctx context.Context, doctorID, invitationID string, now time.Time) (*response.DoctorHospitalInvitation, error) {
	if err := expirePendingInvitations(r.db.WithContext(ctx), "", doctorID, now); err != nil {
		return nil, err
	}
	return r.getInvitation(ctx, "i.doctor_id = ? AND i.id = ?", doctorID, invitationID)
}

func (r *Repository) GetInvitationForHospital(ctx context.Context, hospitalID, invitationID string, now time.Time) (*response.DoctorHospitalInvitation, error) {
	if err := expirePendingInvitations(r.db.WithContext(ctx), hospitalID, "", now); err != nil {
		return nil, err
	}
	return r.getInvitation(ctx, "i.hospital_id = ? AND i.id = ?", hospitalID, invitationID)
}

func (r *Repository) getInvitation(ctx context.Context, where string, args ...any) (*response.DoctorHospitalInvitation, error) {
	where += " AND i.deleted_at IS NULL"
	var out response.DoctorHospitalInvitation
	if err := r.db.WithContext(ctx).Raw(invitationSelect+" WHERE "+where+" LIMIT 1", args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, ErrInvitationNotFound
	}
	rows := []response.DoctorHospitalInvitation{out}
	if err := r.attachInvitationSchedules(ctx, rows); err != nil {
		return nil, err
	}
	return &rows[0], nil
}

const invitationSelect = `
	SELECT i.id, i.hospital_id, h.code AS hospital_code, h.name AS hospital_name,
	       i.doctor_id, dp.medikaone_id AS doctor_medikaone_id, u.email AS doctor_email, u.first_name AS doctor_first_name,
	       u.last_name AS doctor_last_name, COALESCE(dp.sip_number, '') AS sip_number,
	       COALESCE(dp.specialty, '') AS specialty,
	       i.department_id, department.name AS department_name,
	       i.room_id, room.name AS room_name, i.invited_by, i.supersedes_invitation_id,
	       i.status, i.message,
	       i.rejection_reason, i.expires_at, i.responded_at, i.created_at,
	       contract.original_filename AS contract_filename,
	       contract.signed_filename AS signed_contract_name
	FROM doctor_hospital_invitations i
	JOIN hospitals h ON h.id = i.hospital_id
	JOIN users u ON u.id = i.doctor_id
	JOIN doctor_profiles dp ON dp.user_id = i.doctor_id
	JOIN hospital_departments department ON department.id = i.department_id
	LEFT JOIN hospital_rooms room ON room.id = i.room_id
	JOIN doctor_hospital_contracts contract ON contract.invitation_id = i.id`

func (r *Repository) attachInvitationSchedules(ctx context.Context, invitations []response.DoctorHospitalInvitation) error {
	for i := range invitations {
		var schedules []response.DoctorHospitalSchedule
		if err := r.db.WithContext(ctx).Raw(`
			SELECT id, day_of_week, schedule_date::text AS schedule_date,
			       TO_CHAR(start_time, 'HH24:MI') AS start_time,
			       TO_CHAR(end_time, 'HH24:MI') AS end_time,
			       timezone, booking_mode, slot_duration_minutes, capacity
			FROM doctor_hospital_invitation_schedules
			WHERE invitation_id = ?
			ORDER BY day_of_week, start_time`, invitations[i].ID).Scan(&schedules).Error; err != nil {
			return err
		}
		invitations[i].Schedules = schedules
	}
	return nil
}

func (r *Repository) AcceptInvitation(ctx context.Context, invitationID, doctorID string, now time.Time) error {
	if err := expirePendingInvitations(r.db.WithContext(ctx), "", doctorID, now); err != nil {
		return err
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockInvitationHospital(tx, invitationID, "", doctorID); err != nil {
			return err
		}
		var invitation entity.DoctorHospitalInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND doctor_id = ? AND deleted_at IS NULL", invitationID, doctorID).
			First(&invitation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvitationNotFound
			}
			return err
		}
		if invitation.Status == entity.DoctorHospitalInvitationExpired {
			return ErrInvitationExpired
		}
		if invitation.Status != entity.DoctorHospitalInvitationPending {
			return ErrInvalidInvitationState
		}
		if err := lockActivePlacement(tx, invitation.HospitalID, invitation.DepartmentID, invitation.RoomID); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(CAST(? AS text), 0))", doctorID).Error; err != nil {
			return err
		}
		if err := lockAndValidateHospitalWorkerDOB(tx, doctorID, now); err != nil {
			return err
		}

		conflict, err := invitationScheduleConflict(tx, doctorID, invitationID, now)
		if err != nil {
			return err
		}
		if conflict {
			return ErrScheduleConflict
		}

		affiliationID := uuid.NewString()
		if err := tx.Exec(`
			INSERT INTO doctor_hospital_affiliations (
				id, hospital_id, doctor_id, department_id, room_id, invitation_id,
				status, joined_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE', ?, ?, ?)`,
			affiliationID, invitation.HospitalID, doctorID, invitation.DepartmentID,
			invitation.RoomID, invitationID, now, now, now).Error; err != nil {
			return err
		}

		if err := tx.Exec(`
			INSERT INTO doctor_hospital_schedules (
				id, affiliation_id, day_of_week, schedule_date, start_time, end_time, timezone,
				booking_mode, slot_duration_minutes, capacity, is_active, created_at, updated_at
			)
			SELECT gen_random_uuid(), ?, day_of_week, schedule_date, start_time, end_time, timezone,
			       booking_mode, slot_duration_minutes, capacity, TRUE, ?, ?
			FROM doctor_hospital_invitation_schedules
			WHERE invitation_id = ?`, affiliationID, now, now, invitationID).Error; err != nil {
			return err
		}

		if err := tx.Exec(`
			INSERT INTO user_hospitals (user_id, hospital_id, is_active, is_primary, created_at)
			VALUES (?, ?, TRUE, FALSE, ?)
			ON CONFLICT (user_id, hospital_id)
			DO UPDATE SET is_active = TRUE`, doctorID, invitation.HospitalID, now).Error; err != nil {
			return err
		}

		var doctorRoleID string
		if err := tx.Raw("SELECT id FROM roles WHERE UPPER(slug) = ? LIMIT 1", constant.RoleDoctor).
			Scan(&doctorRoleID).Error; err != nil {
			return err
		}
		if doctorRoleID == "" {
			return fmt.Errorf("doctor role is not seeded")
		}
		if err := tx.Exec(`
			INSERT INTO hospital_user_roles (hospital_id, user_id, role_id, created_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (hospital_id, user_id, role_id) DO NOTHING`,
			invitation.HospitalID, doctorID, doctorRoleID, now).Error; err != nil {
			return err
		}

		if err := tx.Model(&invitation).Updates(map[string]any{
			"status": entity.DoctorHospitalInvitationAccepted, "responded_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := insertInvitationEvent(tx, invitationID, doctorID, "ACCEPTED", now); err != nil {
			return err
		}
		if err := insertAffiliationEvent(tx, affiliationID, doctorID, "ACTIVATED", nil, entity.DoctorHospitalAffiliationActive, now); err != nil {
			return err
		}

		data, _ := json.Marshal(map[string]any{
			"invitation_id": invitationID, "hospital_id": invitation.HospitalID,
			"doctor_id": doctorID, "event": "DOCTOR_HOSPITAL_INVITATION_ACCEPTED",
		})
		return tx.Exec(`
			INSERT INTO notifications (id, user_id, type, title, body, data, created_at)
			VALUES (?, ?, 'DOCTOR_HOSPITAL_INVITATION_ACCEPTED',
			        'Undangan dokter diterima', 'Dokter menerima undangan rumah sakit.', ?::jsonb, ?)`,
			uuid.NewString(), invitation.InvitedBy, string(data), now).Error
	})
	return mapScheduleConflictWriteError(err)
}

func (r *Repository) RejectInvitation(ctx context.Context, invitationID, doctorID string, message *string, now time.Time) error {
	if err := expirePendingInvitations(r.db.WithContext(ctx), "", doctorID, now); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invitation entity.DoctorHospitalInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND doctor_id = ? AND deleted_at IS NULL", invitationID, doctorID).First(&invitation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvitationNotFound
			}
			return err
		}
		if invitation.Status == entity.DoctorHospitalInvitationExpired {
			return ErrInvitationExpired
		}
		if invitation.Status != entity.DoctorHospitalInvitationPending {
			return ErrInvalidInvitationState
		}
		if err := tx.Model(&invitation).Updates(map[string]any{
			"status": entity.DoctorHospitalInvitationRejected, "rejection_reason": message,
			"responded_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"rejection_reason": message})
		if err := tx.Exec(`INSERT INTO doctor_hospital_invitation_events
			(id, invitation_id, actor_id, event_type, metadata, created_at)
			VALUES (?, ?, ?, 'REJECTED', ?::jsonb, ?)`,
			uuid.NewString(), invitationID, doctorID, string(metadata), now).Error; err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]any{
			"invitation_id": invitationID, "hospital_id": invitation.HospitalID,
			"doctor_id": doctorID, "event": "DOCTOR_HOSPITAL_INVITATION_REJECTED",
			"rejection_reason": message,
		})
		return tx.Exec(`
			INSERT INTO notifications (id, user_id, type, title, body, data, created_at)
			VALUES (?, ?, 'DOCTOR_HOSPITAL_INVITATION_REJECTED',
			        'Undangan dokter ditolak', 'Dokter menolak undangan rumah sakit.', ?::jsonb, ?)`,
			uuid.NewString(), invitation.InvitedBy, string(data), now).Error
	})
}

func (r *Repository) CancelInvitation(ctx context.Context, invitationID, hospitalID, actorID string, now time.Time) error {
	if err := expirePendingInvitations(r.db.WithContext(ctx), hospitalID, "", now); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invitation entity.DoctorHospitalInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND hospital_id = ? AND deleted_at IS NULL", invitationID, hospitalID).First(&invitation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvitationNotFound
			}
			return err
		}
		if invitation.Status == entity.DoctorHospitalInvitationExpired {
			return ErrInvitationExpired
		}
		if invitation.Status != entity.DoctorHospitalInvitationPending {
			return ErrInvalidInvitationState
		}
		if err := tx.Model(&invitation).Updates(map[string]any{
			"status": entity.DoctorHospitalInvitationCancelled, "cancelled_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := insertInvitationEvent(tx, invitationID, actorID, "CANCELLED", now); err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]any{
			"invitation_id": invitationID, "hospital_id": hospitalID,
			"event": "DOCTOR_HOSPITAL_INVITATION_CANCELLED",
		})
		return tx.Exec(`
			INSERT INTO notifications (id, user_id, type, title, body, data, created_at)
			VALUES (?, ?, 'DOCTOR_HOSPITAL_INVITATION_CANCELLED',
			        'Undangan rumah sakit dibatalkan', 'Rumah sakit membatalkan undangan Anda.', ?::jsonb, ?)`,
			uuid.NewString(), invitation.DoctorID, string(data), now).Error
	})
}

func (r *Repository) ResendInvitation(ctx context.Context, invitationID, hospitalID, invitedBy string, expiresAt, now time.Time) (*response.DoctorHospitalInvitation, error) {
	if err := expirePendingInvitations(r.db.WithContext(ctx), hospitalID, "", now); err != nil {
		return nil, err
	}
	newID := uuid.NewString()
	var doctorID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockInvitationHospital(tx, invitationID, hospitalID, ""); err != nil {
			return err
		}
		var invitation entity.DoctorHospitalInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND hospital_id = ? AND deleted_at IS NULL", invitationID, hospitalID).First(&invitation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvitationNotFound
			}
			return err
		}
		switch invitation.Status {
		case entity.DoctorHospitalInvitationRejected,
			entity.DoctorHospitalInvitationCancelled,
			entity.DoctorHospitalInvitationExpired:
		default:
			return ErrInvalidInvitationState
		}
		doctorID = invitation.DoctorID
		if err := lockActivePlacement(tx, hospitalID, invitation.DepartmentID, invitation.RoomID); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(CAST(? AS text), 0))", doctorID).Error; err != nil {
			return err
		}
		if err := lockAndValidateHospitalWorkerDOB(tx, doctorID, now); err != nil {
			return err
		}

		var exists bool
		if err := tx.Raw(`
			SELECT EXISTS(
				SELECT 1 FROM doctor_hospital_affiliations
				WHERE hospital_id = ? AND doctor_id = ? AND department_id = ?
				  AND deleted_at IS NULL
				  AND COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid)
				      = COALESCE(?::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
			)`, hospitalID, doctorID, invitation.DepartmentID, invitation.RoomID).Scan(&exists).Error; err != nil {
			return err
		}
		if exists {
			return ErrInvitationExists
		}

		if err := tx.Exec(`
			INSERT INTO doctor_hospital_invitations (
				id, hospital_id, doctor_id, department_id, room_id, invited_by,
				supersedes_invitation_id, status, message, expires_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, 'PENDING', ?, ?, ?, ?)`,
			newID, hospitalID, doctorID, invitation.DepartmentID, invitation.RoomID,
			invitedBy, invitationID, invitation.Message, expiresAt, now, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO doctor_hospital_contracts (
				id, invitation_id, original_filename, original_mime_type,
				original_bucket, original_object_path, original_file_size,
				original_sha256, created_at, updated_at
			)
			SELECT gen_random_uuid(), ?, original_filename, original_mime_type,
			       original_bucket, original_object_path, original_file_size,
			       original_sha256, ?, ?
			FROM doctor_hospital_contracts WHERE invitation_id = ?`,
			newID, now, now, invitationID).Error; err != nil {
			return err
		}
		if err := insertInvitationEvent(tx, newID, invitedBy, "RESENT", now); err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO doctor_hospital_invitation_schedules (
				id, invitation_id, day_of_week, schedule_date, start_time, end_time, timezone,
				booking_mode, slot_duration_minutes, capacity, created_at
			)
			SELECT gen_random_uuid(), ?, day_of_week, schedule_date, start_time, end_time, timezone,
			       booking_mode, slot_duration_minutes, capacity, ?
			FROM doctor_hospital_invitation_schedules WHERE invitation_id = ?`,
			newID, now, invitationID).Error; err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]any{
			"invitation_id": newID, "hospital_id": hospitalID,
			"event": "DOCTOR_HOSPITAL_INVITATION_RESENT",
		})
		return tx.Exec(`
			INSERT INTO notifications (id, user_id, type, title, body, data, created_at)
			SELECT ?, ?, 'DOCTOR_HOSPITAL_INVITATION',
			       'Undangan rumah sakit dikirim ulang',
			       'Undangan untuk bergabung dengan ' || name || ' telah dikirim ulang.',
			       ?::jsonb, ? FROM hospitals WHERE id = ?`,
			uuid.NewString(), doctorID, string(data), now, hospitalID).Error
	})
	if err != nil {
		return nil, err
	}
	return r.GetInvitationForHospital(ctx, hospitalID, newID, now)
}

func (r *Repository) GetContractForDoctor(ctx context.Context, invitationID, doctorID, version string) (*ContractDocument, error) {
	return r.getContract(ctx, "i.id = ? AND i.doctor_id = ?", version, invitationID, doctorID)
}

func (r *Repository) GetContractForHospital(ctx context.Context, invitationID, hospitalID, version string) (*ContractDocument, error) {
	return r.getContract(ctx, "i.id = ? AND i.hospital_id = ?", version, invitationID, hospitalID)
}

func (r *Repository) getContract(ctx context.Context, where, version string, args ...any) (*ContractDocument, error) {
	where += " AND i.deleted_at IS NULL"
	prefix := "original"
	if strings.EqualFold(version, "signed") {
		prefix = "signed"
	}
	query := fmt.Sprintf(`
		SELECT contract.%[1]s_filename AS filename,
		       contract.%[1]s_mime_type AS mime_type,
		       contract.%[1]s_bucket AS bucket,
		       contract.%[1]s_object_path AS object_path,
		       contract.%[1]s_file_size AS file_size,
		       contract.%[1]s_sha256 AS sha256
		FROM doctor_hospital_contracts contract
		JOIN doctor_hospital_invitations i ON i.id = contract.invitation_id
		WHERE %[2]s LIMIT 1`, prefix, where)
	var out ContractDocument
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	if out.Filename == "" || out.ObjectPath == "" {
		return nil, ErrInvitationNotFound
	}
	return &out, nil
}

func insertInvitationEvent(tx *gorm.DB, invitationID string, actorID any, eventType string, now time.Time) error {
	return tx.Exec(`
		INSERT INTO doctor_hospital_invitation_events (
			id, invitation_id, actor_id, event_type, metadata, created_at
		) VALUES (?, ?, ?, ?, '{}'::jsonb, ?)`,
		uuid.NewString(), invitationID, actorID, eventType, now).Error
}

func insertAffiliationEvent(tx *gorm.DB, affiliationID, actorID, eventType string, fromStatus *string, toStatus string, now time.Time) error {
	return tx.Exec(`
		INSERT INTO doctor_hospital_affiliation_events (
			id, affiliation_id, actor_id, event_type, from_status, to_status, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, '{}'::jsonb, ?)`,
		uuid.NewString(), affiliationID, actorID, eventType, fromStatus, toStatus, now).Error
}

func (r *Repository) ListHospitalDoctors(ctx context.Context, hospitalID, status string) ([]response.HospitalDoctor, error) {
	return r.listAffiliations(ctx, hospitalID, "", status)
}

func (r *Repository) ListDoctorAffiliations(ctx context.Context, doctorID, status string) ([]response.HospitalDoctor, error) {
	return r.listAffiliations(ctx, "", doctorID, status)
}

func (r *Repository) listAffiliations(ctx context.Context, hospitalID, doctorID, status string) ([]response.HospitalDoctor, error) {
	args := []any{}
	where := `hospital.is_active = TRUE AND hospital.deleted_at IS NULL
		AND u.status = 'active' AND u.deleted_at IS NULL
		AND affiliation.deleted_at IS NULL
		AND department.is_active = TRUE`
	if hospitalID != "" {
		where += " AND affiliation.hospital_id = ?"
		args = append(args, hospitalID)
	}
	if doctorID != "" {
		where += " AND affiliation.doctor_id = ?"
		args = append(args, doctorID)
	}
	if status != "" {
		where += " AND affiliation.status = ?"
		args = append(args, status)
	}
	var rows []response.HospitalDoctor
	if err := r.db.WithContext(ctx).Raw(`
		SELECT affiliation.id AS affiliation_id, affiliation.hospital_id,
		       hospital.name AS hospital_name,
		       affiliation.doctor_id, dp.medikaone_id AS doctor_medikaone_id, u.email, u.first_name, u.last_name,
		       COALESCE(dp.sip_number, '') AS sip_number,
		       COALESCE(dp.specialty, '') AS specialty,
		       affiliation.department_id, department.name AS department,
		       affiliation.room_id, room.name AS room,
		       affiliation.status, affiliation.joined_at
		FROM doctor_hospital_affiliations affiliation
		JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
		JOIN users u ON u.id = affiliation.doctor_id
		JOIN doctor_profiles dp ON dp.user_id = affiliation.doctor_id
		JOIN hospital_departments department ON department.id = affiliation.department_id
		LEFT JOIN hospital_rooms room ON room.id = affiliation.room_id
		WHERE `+where+`
		ORDER BY u.first_name, u.last_name
		LIMIT 100`, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		schedules := make([]response.DoctorHospitalSchedule, 0)
		if err := r.db.WithContext(ctx).Raw(`
			SELECT id, day_of_week, schedule_date::text AS schedule_date, TO_CHAR(start_time, 'HH24:MI') AS start_time,
			       TO_CHAR(end_time, 'HH24:MI') AS end_time, timezone,
			       booking_mode, slot_duration_minutes, capacity
			FROM doctor_hospital_schedules
			WHERE affiliation_id = ? AND is_active = TRUE
			ORDER BY day_of_week, start_time`, rows[i].AffiliationID).Scan(&schedules).Error; err != nil {
			return nil, err
		}
		for j := range schedules {
			schedules[j].Status = entity.DoctorHospitalAffiliationActive
		}
		rows[i].Schedules = schedules
	}
	if err := r.attachPendingScheduleChanges(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) attachPendingScheduleChanges(ctx context.Context, affiliations []response.HospitalDoctor) error {
	if len(affiliations) == 0 {
		return nil
	}

	affiliationIDs := make([]string, 0, len(affiliations))
	affiliationIndex := make(map[string]int, len(affiliations))
	for i := range affiliations {
		affiliationIDs = append(affiliationIDs, affiliations[i].AffiliationID)
		affiliationIndex[affiliations[i].AffiliationID] = i
	}

	changes := make([]response.PendingScheduleChange, 0)
	if err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (affiliation_id)
		       id, affiliation_id, operation, target_schedule_id, requested_by,
		       requested_by_party, status, reason, expires_at, created_at, updated_at
		FROM doctor_schedule_change_requests
		WHERE affiliation_id IN ? AND status = 'PENDING'
		ORDER BY affiliation_id, created_at DESC, id DESC`, affiliationIDs).Scan(&changes).Error; err != nil {
		return err
	}
	if len(changes) == 0 {
		return nil
	}

	changeIDs := make([]string, 0, len(changes))
	changeIndex := make(map[string]int, len(changes))
	for i := range changes {
		changes[i].Schedules = []response.DoctorHospitalSchedule{}
		changeIDs = append(changeIDs, changes[i].ID)
		changeIndex[changes[i].ID] = i
	}

	type pendingScheduleItem struct {
		ChangeRequestID     string
		ID                  string
		DayOfWeek           int
		ScheduleDate        *string
		StartTime           string
		EndTime             string
		Timezone            string
		BookingMode         string
		SlotDurationMinutes int
		Capacity            int
	}
	items := make([]pendingScheduleItem, 0)
	if err := r.db.WithContext(ctx).Raw(`
		SELECT change_request_id, id, day_of_week, schedule_date::text AS schedule_date,
		       TO_CHAR(start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(end_time, 'HH24:MI') AS end_time,
		       timezone, booking_mode, slot_duration_minutes, capacity
		FROM doctor_schedule_change_items
		WHERE change_request_id IN ?
		ORDER BY change_request_id, schedule_date, day_of_week, start_time, id`, changeIDs).Scan(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		i, ok := changeIndex[item.ChangeRequestID]
		if !ok {
			continue
		}
		changes[i].Schedules = append(changes[i].Schedules, response.DoctorHospitalSchedule{
			ID: item.ID, Status: entity.ScheduleChangePending, DayOfWeek: item.DayOfWeek,
			ScheduleDate: item.ScheduleDate, StartTime: item.StartTime, EndTime: item.EndTime,
			Timezone: item.Timezone, BookingMode: item.BookingMode,
			SlotDurationMinutes: item.SlotDurationMinutes, Capacity: item.Capacity,
		})
	}

	for i := range changes {
		if affiliation, ok := affiliationIndex[changes[i].AffiliationID]; ok {
			affiliations[affiliation].PendingScheduleChange = &changes[i]
		}
	}
	return nil
}

func (r *Repository) UpdateAffiliationStatus(ctx context.Context, hospitalID, doctorID, status, actorID string, now time.Time) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var affiliationIDs []string
		if err := tx.Table("doctor_hospital_affiliations").Where("hospital_id = ? AND doctor_id = ? AND deleted_at IS NULL", hospitalID, doctorID).Order("id").Pluck("id", &affiliationIDs).Error; err != nil {
			return err
		}
		for _, id := range affiliationIDs {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "appointment:affiliation:"+id).Error; err != nil {
				return err
			}
		}
		var affiliations []entity.DoctorHospitalAffiliation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("hospital_id = ? AND doctor_id = ? AND deleted_at IS NULL", hospitalID, doctorID).
			Find(&affiliations).Error; err != nil {
			return err
		}
		if len(affiliations) == 0 {
			return ErrAffiliationNotFound
		}
		if status == entity.DoctorHospitalAffiliationActive {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(CAST(? AS text), 0))", doctorID).Error; err != nil {
				return err
			}
			for _, affiliation := range affiliations {
				if err := lockActivePlacement(tx, hospitalID, affiliation.DepartmentID, affiliation.RoomID); err != nil {
					return err
				}
			}
			if err := lockAndValidateHospitalWorkerDOB(tx, doctorID, now); err != nil {
				return err
			}
			var reactivatingIDs []string
			for _, affiliation := range affiliations {
				if affiliation.Status != entity.DoctorHospitalAffiliationActive {
					reactivatingIDs = append(reactivatingIDs, affiliation.ID)
				}
			}
			if len(reactivatingIDs) > 0 {
				conflict, err := affiliationReactivationConflict(tx, doctorID, reactivatingIDs, now)
				if err != nil {
					return err
				}
				if conflict {
					return ErrScheduleConflict
				}
			}
		}
		if err := tx.Model(&entity.DoctorHospitalAffiliation{}).
			Where("hospital_id = ? AND doctor_id = ? AND status <> ? AND deleted_at IS NULL", hospitalID, doctorID, status).
			Updates(map[string]any{"status": status, "updated_at": now}).Error; err != nil {
			return err
		}

		var doctorRoleID string
		if err := tx.Raw("SELECT id FROM roles WHERE UPPER(slug) = ? LIMIT 1", constant.RoleDoctor).
			Scan(&doctorRoleID).Error; err != nil {
			return err
		}
		if doctorRoleID == "" {
			return fmt.Errorf("doctor role is not seeded")
		}
		if status == entity.DoctorHospitalAffiliationActive {
			if err := tx.Exec(`
				INSERT INTO user_hospitals (user_id, hospital_id, is_active, is_primary, created_at)
				VALUES (?, ?, TRUE, FALSE, ?)
				ON CONFLICT (user_id, hospital_id) DO UPDATE SET is_active = TRUE`,
				doctorID, hospitalID, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				INSERT INTO hospital_user_roles (hospital_id, user_id, role_id, created_at)
				VALUES (?, ?, ?, ?)
				ON CONFLICT (hospital_id, user_id, role_id) DO NOTHING`,
				hospitalID, doctorID, doctorRoleID, now).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Exec(`DELETE FROM hospital_user_roles WHERE hospital_id = ? AND user_id = ? AND role_id = ?`,
				hospitalID, doctorID, doctorRoleID).Error; err != nil {
				return err
			}
			var otherRoles bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM hospital_user_roles WHERE hospital_id = ? AND user_id = ?)`,
				hospitalID, doctorID).Scan(&otherRoles).Error; err != nil {
				return err
			}
			if !otherRoles {
				if err := tx.Exec(`UPDATE user_hospitals SET is_active = FALSE WHERE hospital_id = ? AND user_id = ?`,
					hospitalID, doctorID).Error; err != nil {
					return err
				}
			}
		}

		eventType := "SUSPENDED"
		if status == entity.DoctorHospitalAffiliationActive {
			eventType = "REACTIVATED"
		}
		affiliationIDs = make([]string, 0, len(affiliations))
		for _, affiliation := range affiliations {
			affiliationIDs = append(affiliationIDs, affiliation.ID)
			if affiliation.Status == status {
				continue
			}
			previousStatus := affiliation.Status
			if err := insertAffiliationEvent(tx, affiliation.ID, actorID, eventType, &previousStatus, status, now); err != nil {
				return err
			}
		}
		data, _ := json.Marshal(map[string]any{
			"affiliation_ids": affiliationIDs, "hospital_id": hospitalID,
			"status": status, "event": "DOCTOR_HOSPITAL_AFFILIATION_" + eventType,
		})
		return tx.Exec(`
			INSERT INTO notifications (id, user_id, type, title, body, data, created_at)
			VALUES (?, ?, 'DOCTOR_HOSPITAL_AFFILIATION_STATUS',
			        'Status dokter diperbarui', 'Status keanggotaan rumah sakit Anda telah diperbarui.', ?::jsonb, ?)`,
			uuid.NewString(), doctorID, string(data), now).Error
	})
	return mapScheduleConflictWriteError(err)
}

func mapScheduleConflictWriteError(err error) error {
	if err == nil || errors.Is(err, ErrScheduleConflict) {
		return err
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23P01" && postgresError.ConstraintName == "doctor_schedule_no_overlap" {
		return fmt.Errorf("%w: %w", ErrScheduleConflict, err)
	}
	return err
}

func affiliationReactivationConflict(tx *gorm.DB, doctorID string, affiliationIDs []string, now time.Time) (bool, error) {
	var proposed []scheduleconflict.Schedule
	if err := tx.Raw(`
		SELECT schedule.day_of_week, schedule.schedule_date::text AS schedule_date,
		       TO_CHAR(schedule.start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(schedule.end_time, 'HH24:MI') AS end_time, schedule.timezone
		FROM doctor_hospital_schedules schedule
		WHERE schedule.affiliation_id IN ? AND schedule.is_active = TRUE
		  AND (schedule.schedule_date IS NULL OR ((schedule.schedule_date + schedule.end_time) AT TIME ZONE schedule.timezone) > ?)`,
		affiliationIDs, now).Scan(&proposed).Error; err != nil {
		return false, err
	}
	if conflict, err := scheduleconflict.AnyOverlap(proposed, now); err != nil || conflict {
		return conflict, err
	}
	var active []scheduleconflict.Schedule
	if err := tx.Raw(`
		SELECT schedule.day_of_week, schedule.schedule_date::text AS schedule_date,
		       TO_CHAR(schedule.start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(schedule.end_time, 'HH24:MI') AS end_time, schedule.timezone
		FROM doctor_hospital_affiliations affiliation
		JOIN doctor_hospital_schedules schedule ON schedule.affiliation_id = affiliation.id
		WHERE affiliation.doctor_id = ? AND affiliation.id NOT IN ?
		  AND affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL
		  AND schedule.is_active = TRUE
		  AND (schedule.schedule_date IS NULL OR ((schedule.schedule_date + schedule.end_time) AT TIME ZONE schedule.timezone) > ?)`,
		doctorID, affiliationIDs, now).Scan(&active).Error; err != nil {
		return false, err
	}
	return scheduleconflict.AnyConflict(proposed, active, now)
}

func lockAndValidateHospitalWorkerDOB(tx *gorm.DB, userID string, now time.Time) error {
	var user entity.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "dob").
		Where("id = ? AND status = 'active' AND deleted_at IS NULL", userID).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDoctorNotEligible
		}
		return err
	}
	if user.DOB == nil {
		return ErrHospitalWorkerDOBRequired
	}
	if !hospitalWorkerDOBMeetsMinimum(*user.DOB, now) {
		return ErrHospitalWorkerUnderage
	}
	return nil
}

func hospitalWorkerDOBMeetsMinimum(dob, now time.Time) bool {
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	birthDate := time.Date(dob.UTC().Year(), dob.UTC().Month(), dob.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return birthDate.AddDate(15, 0, 0).Before(today)
}

func (r *Repository) ListNotifications(ctx context.Context, userID string, unreadOnly bool) ([]response.Notification, error) {
	where := "user_id = ? AND deleted_at IS NULL"
	if unreadOnly {
		where += " AND read_at IS NULL"
	}
	var rows []response.Notification
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, type, title, body,
		       CASE WHEN data->>'doctor_id' IS NOT NULL THEN data || COALESCE((
		           SELECT jsonb_build_object('doctor_medikaone_id', profile.medikaone_id)
		           FROM doctor_profiles profile WHERE profile.user_id::text = notifications.data->>'doctor_id'
		       ), '{}'::jsonb) ELSE data END AS data,
		       read_at, created_at
		FROM notifications WHERE `+where+`
		ORDER BY created_at DESC LIMIT 100`, userID).Scan(&rows).Error
	return rows, err
}

func (r *Repository) MarkNotificationRead(ctx context.Context, userID, notificationID string, now time.Time) error {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE notifications SET read_at = COALESCE(read_at, ?)
		WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, now, notificationID, userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}
	return nil
}
