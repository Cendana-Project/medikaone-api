package doctor_hospital

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

var (
	ErrInvitationNotFound     = errors.New("doctor hospital invitation not found")
	ErrInvitationExists       = errors.New("an open invitation or affiliation already exists")
	ErrInvitationExpired      = errors.New("doctor hospital invitation expired")
	ErrInvalidInvitationState = errors.New("invalid doctor hospital invitation state")
	ErrPlacementNotFound      = errors.New("department or room not found")
	ErrScheduleConflict       = errors.New("doctor schedule conflicts with an active affiliation")
	ErrAffiliationNotFound    = errors.New("doctor hospital affiliation not found")
	ErrNotificationNotFound   = errors.New("notification not found")
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type DoctorSearchCriteria struct {
	Email       string
	SIPNumber   string
	MedikaOneID string
}

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

func (r *Repository) SearchEligibleDoctor(ctx context.Context, criteria DoctorSearchCriteria) (*response.DoctorSearchResult, error) {
	query := `
		SELECT u.id, u.email, u.first_name, u.last_name,
		       COALESCE(dp.sip_number, '') AS sip_number,
		       COALESCE(dp.specialty, '') AS specialty
		FROM users u
		JOIN doctor_profiles dp ON dp.user_id = u.id
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN roles role ON role.id = ur.role_id AND UPPER(role.slug) = ?
		WHERE u.deleted_at IS NULL
		  AND u.status = 'active'
		  AND u.verified_at IS NOT NULL
		  AND NULLIF(TRIM(dp.sip_number), '') IS NOT NULL`

	args := []any{constant.RoleDoctor}
	switch {
	case criteria.Email != "":
		query += " AND LOWER(u.email) = LOWER(?)"
		args = append(args, criteria.Email)
	case criteria.SIPNumber != "":
		query += " AND LOWER(dp.sip_number) = LOWER(?)"
		args = append(args, criteria.SIPNumber)
	case criteria.MedikaOneID != "":
		query += " AND u.id = ?"
		args = append(args, criteria.MedikaOneID)
	default:
		return nil, gorm.ErrRecordNotFound
	}
	query += " LIMIT 1"

	var out response.DoctorSearchResult
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &out, nil
}

func (r *Repository) CreateDepartment(ctx context.Context, hospitalID, code, name string, now time.Time) (*entity.HospitalDepartment, error) {
	department := &entity.HospitalDepartment{
		ID: uuid.NewString(), HospitalID: hospitalID, Code: code, Name: name,
		IsActive: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := r.db.WithContext(ctx).Create(department).Error; err != nil {
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
	if err := r.db.WithContext(ctx).Create(room).Error; err != nil {
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

func (r *Repository) CreateInvitation(ctx context.Context, input CreateInvitationInput) (*response.DoctorHospitalInvitation, error) {
	invitationID := input.InvitationID
	if invitationID == "" {
		invitationID = uuid.NewString()
	}
	notificationID := uuid.NewString()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := expirePendingInvitations(tx, input.HospitalID, input.DoctorID, input.Now); err != nil {
			return err
		}

		var exists bool
		if err := tx.Raw(`
			SELECT EXISTS(
				SELECT 1 FROM doctor_hospital_affiliations
				WHERE hospital_id = ? AND doctor_id = ? AND department_id = ?
				  AND COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid)
				      = COALESCE(?::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
				UNION ALL
				SELECT 1 FROM doctor_hospital_invitations
				WHERE hospital_id = ? AND doctor_id = ? AND department_id = ?
				  AND COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid)
				      = COALESCE(?::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
				  AND status = 'PENDING'
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
					id, invitation_id, day_of_week, start_time, end_time, timezone,
					booking_mode, slot_duration_minutes, capacity, created_at
				) VALUES (?, ?, ?, ?::time, ?::time, ?, ?, ?, ?, ?)`,
				uuid.NewString(), invitationID, schedule.DayOfWeek, schedule.StartTime,
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
			Where("status = ? AND expires_at <= ?", entity.DoctorHospitalInvitationPending, now)
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
	       i.doctor_id, u.email AS doctor_email, u.first_name AS doctor_first_name,
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
			SELECT id, day_of_week,
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

func (r *Repository) AcceptInvitation(ctx context.Context, invitationID, doctorID string, signed Document, now time.Time) error {
	if err := expirePendingInvitations(r.db.WithContext(ctx), "", doctorID, now); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invitation entity.DoctorHospitalInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND doctor_id = ?", invitationID, doctorID).
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
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(CAST(? AS text), 0))", doctorID).Error; err != nil {
			return err
		}

		var conflict bool
		if err := tx.Raw(`
			SELECT EXISTS(
				SELECT 1
				FROM doctor_hospital_invitation_schedules proposed
				JOIN doctor_hospital_affiliations affiliation
				  ON affiliation.doctor_id = ? AND affiliation.status = 'ACTIVE'
				JOIN doctor_hospital_schedules existing
				  ON existing.affiliation_id = affiliation.id AND existing.is_active = TRUE
				WHERE proposed.invitation_id = ?
				  AND proposed.day_of_week = existing.day_of_week
				  AND proposed.start_time < existing.end_time
				  AND proposed.end_time > existing.start_time
			)`, doctorID, invitationID).Scan(&conflict).Error; err != nil {
			return err
		}
		if conflict {
			return ErrScheduleConflict
		}

		if err := tx.Exec(`
			UPDATE doctor_hospital_contracts
			SET signed_filename = ?, signed_mime_type = ?, signed_bucket = ?,
			    signed_object_path = ?, signed_file_size = ?, signed_sha256 = ?,
			    signed_at = ?, updated_at = ?
			WHERE invitation_id = ?`, signed.Filename, signed.MIMEType, signed.Bucket,
			signed.ObjectPath, signed.FileSize, signed.SHA256, now, now, invitationID).Error; err != nil {
			return err
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
				id, affiliation_id, day_of_week, start_time, end_time, timezone,
				booking_mode, slot_duration_minutes, capacity, is_active, created_at, updated_at
			)
			SELECT gen_random_uuid(), ?, day_of_week, start_time, end_time, timezone,
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
}

func (r *Repository) RejectInvitation(ctx context.Context, invitationID, doctorID string, reason *string, now time.Time) error {
	if err := expirePendingInvitations(r.db.WithContext(ctx), "", doctorID, now); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invitation entity.DoctorHospitalInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND doctor_id = ?", invitationID, doctorID).First(&invitation).Error; err != nil {
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
			"status": entity.DoctorHospitalInvitationRejected, "rejection_reason": reason,
			"responded_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := insertInvitationEvent(tx, invitationID, doctorID, "REJECTED", now); err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]any{
			"invitation_id": invitationID, "hospital_id": invitation.HospitalID,
			"doctor_id": doctorID, "event": "DOCTOR_HOSPITAL_INVITATION_REJECTED",
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
			Where("id = ? AND hospital_id = ?", invitationID, hospitalID).First(&invitation).Error; err != nil {
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
		var invitation entity.DoctorHospitalInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND hospital_id = ?", invitationID, hospitalID).First(&invitation).Error; err != nil {
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

		var exists bool
		if err := tx.Raw(`
			SELECT EXISTS(
				SELECT 1 FROM doctor_hospital_affiliations
				WHERE hospital_id = ? AND doctor_id = ? AND department_id = ?
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
				id, invitation_id, day_of_week, start_time, end_time, timezone,
				booking_mode, slot_duration_minutes, capacity, created_at
			)
			SELECT gen_random_uuid(), ?, day_of_week, start_time, end_time, timezone,
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
		       affiliation.doctor_id, u.email, u.first_name, u.last_name,
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
		var schedules []response.DoctorHospitalSchedule
		if err := r.db.WithContext(ctx).Raw(`
			SELECT id, day_of_week, TO_CHAR(start_time, 'HH24:MI') AS start_time,
			       TO_CHAR(end_time, 'HH24:MI') AS end_time, timezone,
			       booking_mode, slot_duration_minutes, capacity
			FROM doctor_hospital_schedules
			WHERE affiliation_id = ? AND is_active = TRUE
			ORDER BY day_of_week, start_time`, rows[i].AffiliationID).Scan(&schedules).Error; err != nil {
			return nil, err
		}
		rows[i].Schedules = schedules
	}
	return rows, nil
}

func (r *Repository) UpdateAffiliationStatus(ctx context.Context, hospitalID, doctorID, status, actorID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var affiliations []entity.DoctorHospitalAffiliation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("hospital_id = ? AND doctor_id = ?", hospitalID, doctorID).
			Find(&affiliations).Error; err != nil {
			return err
		}
		if len(affiliations) == 0 {
			return ErrAffiliationNotFound
		}
		if err := tx.Model(&entity.DoctorHospitalAffiliation{}).
			Where("hospital_id = ? AND doctor_id = ? AND status <> ?", hospitalID, doctorID, status).
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
		affiliationIDs := make([]string, 0, len(affiliations))
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
}

func (r *Repository) ListNotifications(ctx context.Context, userID string, unreadOnly bool) ([]response.Notification, error) {
	where := "user_id = ?"
	if unreadOnly {
		where += " AND read_at IS NULL"
	}
	var rows []response.Notification
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, type, title, body, data, read_at, created_at
		FROM notifications WHERE `+where+`
		ORDER BY created_at DESC LIMIT 100`, userID).Scan(&rows).Error
	return rows, err
}

func (r *Repository) MarkNotificationRead(ctx context.Context, userID, notificationID string, now time.Time) error {
	result := r.db.WithContext(ctx).Exec(`
		UPDATE notifications SET read_at = COALESCE(read_at, ?)
		WHERE id = ? AND user_id = ?`, now, notificationID, userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}
	return nil
}
