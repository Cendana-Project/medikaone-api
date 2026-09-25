package doctor_hospital

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

var ErrResourceInUse = errors.New("resource still has active references")

type UpdateInvitationInput struct {
	HospitalID   string
	InvitationID string
	ActorID      string
	DepartmentID *string
	RoomID       *string
	Message      *string
	Schedules    *[]Schedule
	Now          time.Time
}

// lockActivePlacement also prevents deletion between validation and a write.
func lockActivePlacement(tx *gorm.DB, hospitalID, departmentID string, roomID *string) error {
	var found string
	if err := tx.Raw(`SELECT id FROM hospitals WHERE id = ? AND is_active = TRUE AND deleted_at IS NULL FOR SHARE`, hospitalID).Scan(&found).Error; err != nil {
		return err
	}
	if found == "" {
		return ErrPlacementNotFound
	}
	found = ""
	if err := tx.Raw(`SELECT id FROM hospital_departments WHERE id = ? AND hospital_id = ? AND is_active = TRUE FOR SHARE`, departmentID, hospitalID).Scan(&found).Error; err != nil {
		return err
	}
	if found == "" {
		return ErrPlacementNotFound
	}
	if roomID != nil {
		found = ""
		if err := tx.Raw(`SELECT id FROM hospital_rooms WHERE id = ? AND hospital_id = ? AND department_id = ? AND is_active = TRUE FOR SHARE`, *roomID, hospitalID, departmentID).Scan(&found).Error; err != nil {
			return err
		}
		if found == "" {
			return ErrPlacementNotFound
		}
	}
	return nil
}

// Lock the hospital before the invitation to match hospital deletion's order.
func lockInvitationHospital(tx *gorm.DB, invitationID, hospitalID, doctorID string) error {
	where := ""
	args := []any{invitationID}
	if hospitalID != "" {
		where += " AND i.hospital_id = ?"
		args = append(args, hospitalID)
	}
	if doctorID != "" {
		where += " AND i.doctor_id = ?"
		args = append(args, doctorID)
	}
	if where == "" {
		return ErrInvitationNotFound
	}
	var found string
	if err := tx.Raw(`SELECT h.id FROM hospitals h JOIN doctor_hospital_invitations i ON i.hospital_id = h.id
		WHERE i.id = ? AND i.deleted_at IS NULL AND h.is_active = TRUE AND h.deleted_at IS NULL
		`+where+` FOR SHARE OF h`, args...).Scan(&found).Error; err != nil {
		return err
	}
	if found == "" {
		return ErrInvitationNotFound
	}
	return nil
}

func (r *Repository) UpdateDepartment(ctx context.Context, hospitalID, departmentID, masterDepartmentID string, now time.Time) (*entity.HospitalDepartment, error) {
	var row entity.HospitalDepartment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		master, err := activeMasterDepartment(tx, masterDepartmentID)
		if err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND hospital_id = ? AND is_active = TRUE", departmentID, hospitalID).First(&row).Error; err != nil {
			return placementError(err)
		}
		if row.MasterDepartmentID == nil || *row.MasterDepartmentID != master.ID {
			var referenced bool
			if err := tx.Raw(`SELECT EXISTS(
				SELECT 1 FROM doctor_hospital_invitations WHERE hospital_id = ? AND department_id = ?
				UNION ALL SELECT 1 FROM doctor_hospital_affiliations WHERE hospital_id = ? AND department_id = ?
				UNION ALL SELECT 1 FROM appointments WHERE hospital_id = ? AND department_id = ?
			)`, hospitalID, departmentID, hospitalID, departmentID, hospitalID, departmentID).Scan(&referenced).Error; err != nil {
				return err
			}
			if referenced {
				return ErrResourceInUse
			}
		}
		if err := tx.Model(&row).Updates(map[string]any{
			"master_department_id": master.ID,
			"code":                 master.Code,
			"name":                 master.Name,
			"updated_at":           now,
		}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND hospital_id = ?", departmentID, hospitalID).First(&row).Error
	})
	return &row, err
}

func placementError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrPlacementNotFound
	}
	return err
}

func (r *Repository) DeleteDepartment(ctx context.Context, hospitalID, departmentID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.HospitalDepartment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND hospital_id = ? AND is_active = TRUE", departmentID, hospitalID).First(&row).Error; err != nil {
			return placementError(err)
		}
		if err := ensurePlacementUnused(tx, hospitalID, "department_id", departmentID); err != nil {
			return err
		}
		if err := tx.Model(&entity.HospitalRoom{}).Where("hospital_id = ? AND department_id = ?", hospitalID, departmentID).
			Updates(map[string]any{"is_active": false, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{"is_active": false, "updated_at": now}).Error
	})
}

func (r *Repository) UpdateRoom(ctx context.Context, hospitalID, roomID string, departmentID *string, fields map[string]any) (*entity.HospitalRoom, error) {
	var row entity.HospitalRoom
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The new department is locked first, following invitation placement order.
		if departmentID != nil {
			if err := lockActivePlacement(tx, hospitalID, *departmentID, nil); err != nil {
				return err
			}
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND hospital_id = ? AND is_active = TRUE", roomID, hospitalID).First(&row).Error; err != nil {
			return placementError(err)
		}
		if departmentID != nil && *departmentID != row.DepartmentID {
			var referenced bool
			if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM doctor_hospital_invitations WHERE hospital_id = ? AND room_id = ?
				UNION ALL SELECT 1 FROM doctor_hospital_affiliations WHERE hospital_id = ? AND room_id = ?
				UNION ALL SELECT 1 FROM appointments WHERE hospital_id = ? AND room_id = ?)`, hospitalID, roomID, hospitalID, roomID, hospitalID, roomID).Scan(&referenced).Error; err != nil {
				return err
			}
			if referenced {
				return ErrResourceInUse
			}
			fields["department_id"] = *departmentID
		}
		if err := tx.Model(&row).Updates(fields).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND hospital_id = ?", roomID, hospitalID).First(&row).Error
	})
	return &row, err
}

func (r *Repository) DeleteRoom(ctx context.Context, hospitalID, roomID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.HospitalRoom
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND hospital_id = ? AND is_active = TRUE", roomID, hospitalID).First(&row).Error; err != nil {
			return placementError(err)
		}
		if err := ensurePlacementUnused(tx, hospitalID, "room_id", roomID); err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{"is_active": false, "updated_at": now}).Error
	})
}

// column comes only from the fixed call sites above, never from request input.
func ensurePlacementUnused(tx *gorm.DB, hospitalID, column, id string) error {
	if column != "department_id" && column != "room_id" {
		return ErrPlacementNotFound
	}
	var busy bool
	if err := tx.Raw(`SELECT EXISTS(
		SELECT 1 FROM doctor_hospital_affiliations affiliation JOIN users doctor ON doctor.id = affiliation.doctor_id
		WHERE affiliation.hospital_id = ? AND affiliation.`+column+` = ? AND affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL
		  AND doctor.status = 'active' AND doctor.deleted_at IS NULL
		UNION ALL SELECT 1 FROM doctor_hospital_invitations invitation JOIN users doctor ON doctor.id = invitation.doctor_id
		WHERE invitation.hospital_id = ? AND invitation.`+column+` = ? AND invitation.status = 'PENDING' AND invitation.deleted_at IS NULL AND invitation.expires_at > NOW()
		  AND doctor.status = 'active' AND doctor.deleted_at IS NULL
		UNION ALL SELECT 1 FROM appointments WHERE hospital_id = ? AND `+column+` = ? AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')
	)`, hospitalID, id, hospitalID, id, hospitalID, id).Scan(&busy).Error; err != nil {
		return err
	}
	if busy {
		return ErrResourceInUse
	}
	return nil
}

func (r *Repository) UpdateInvitation(ctx context.Context, input UpdateInvitationInput) (*response.DoctorHospitalInvitation, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockInvitationHospital(tx, input.InvitationID, input.HospitalID, ""); err != nil {
			return err
		}
		var row entity.DoctorHospitalInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND hospital_id = ? AND deleted_at IS NULL", input.InvitationID, input.HospitalID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvitationNotFound
			}
			return err
		}
		if row.Status != entity.DoctorHospitalInvitationPending {
			return ErrInvalidInvitationState
		}
		if !row.ExpiresAt.After(input.Now) {
			return ErrInvitationExpired
		}
		departmentID, roomID := row.DepartmentID, row.RoomID
		previousTerms := map[string]any{"department_id": row.DepartmentID, "room_id": row.RoomID, "message": row.Message}
		if input.Schedules != nil {
			var schedulesJSON string
			if err := tx.Raw(`SELECT COALESCE(jsonb_agg(to_jsonb(schedule)), '[]'::jsonb)::text FROM doctor_hospital_invitation_schedules schedule WHERE invitation_id = ?`, input.InvitationID).Scan(&schedulesJSON).Error; err != nil {
				return err
			}
			previousTerms["schedules"] = json.RawMessage(schedulesJSON)
		}
		fields := map[string]any{"updated_at": input.Now}
		if input.DepartmentID != nil {
			departmentID = *input.DepartmentID
			fields["department_id"] = departmentID
		}
		if input.RoomID != nil {
			if *input.RoomID == "" {
				roomID = nil
			} else {
				roomID = input.RoomID
			}
			fields["room_id"] = roomID
		}
		if input.Message != nil {
			if *input.Message == "" {
				fields["message"] = nil
			} else {
				fields["message"] = *input.Message
			}
		}
		if err := lockActivePlacement(tx, input.HospitalID, departmentID, roomID); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(CAST(? AS text), 0))`, row.DoctorID).Error; err != nil {
			return err
		}
		if err := lockAndValidateHospitalWorkerDOB(tx, row.DoctorID, input.Now); err != nil {
			return err
		}
		var affiliated bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM doctor_hospital_affiliations WHERE hospital_id = ? AND doctor_id = ? AND department_id = ?
			AND COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE(?::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
			AND deleted_at IS NULL)`, input.HospitalID, row.DoctorID, departmentID, roomID).Scan(&affiliated).Error; err != nil {
			return err
		}
		if affiliated {
			return ErrInvitationExists
		}
		if input.Schedules != nil {
			conflict, err := hasActiveScheduleConflict(tx, row.DoctorID, *input.Schedules, input.Now)
			if err != nil {
				return err
			}
			if conflict {
				return ErrScheduleConflict
			}
			if err := tx.Exec(`DELETE FROM doctor_hospital_invitation_schedules WHERE invitation_id = ?`, input.InvitationID).Error; err != nil {
				return err
			}
			for _, schedule := range *input.Schedules {
				if err := tx.Exec(`INSERT INTO doctor_hospital_invitation_schedules
					(id, invitation_id, day_of_week, schedule_date, start_time, end_time, timezone, booking_mode, slot_duration_minutes, capacity, created_at)
					VALUES (?, ?, ?, ?::date, ?::time, ?::time, ?, ?, ?, ?, ?)`, uuid.NewString(), input.InvitationID, schedule.DayOfWeek, schedule.ScheduleDate,
					schedule.StartTime, schedule.EndTime, schedule.Timezone, schedule.BookingMode, schedule.SlotDurationMinutes, schedule.Capacity, input.Now).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&row).Updates(fields).Error; err != nil {
			return err
		}
		// Keep the previous proposed terms in the audit trail when editing a draft.
		metadata, _ := json.Marshal(map[string]any{"previous_terms": previousTerms, "schedules_replaced": input.Schedules != nil})
		if err := tx.Exec(`INSERT INTO doctor_hospital_invitation_events (id, invitation_id, actor_id, event_type, metadata, created_at)
			VALUES (?, ?, ?, 'UPDATED', ?::jsonb, ?)`, uuid.NewString(), input.InvitationID, input.ActorID, string(metadata), input.Now).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetInvitationForHospital(ctx, input.HospitalID, input.InvitationID, input.Now)
}

func (r *Repository) DeleteInvitation(ctx context.Context, hospitalID, invitationID, actorID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.DoctorHospitalInvitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND hospital_id = ? AND deleted_at IS NULL", invitationID, hospitalID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvitationNotFound
			}
			return err
		}
		if row.Status == entity.DoctorHospitalInvitationAccepted {
			return ErrInvalidInvitationState
		}
		fields := map[string]any{"deleted_at": now, "updated_at": now}
		if row.Status == entity.DoctorHospitalInvitationPending {
			fields["status"], fields["cancelled_at"] = entity.DoctorHospitalInvitationCancelled, now
			if err := insertInvitationEvent(tx, invitationID, actorID, "CANCELLED", now); err != nil {
				return err
			}
		}
		if err := tx.Model(&row).Updates(fields).Error; err != nil {
			return err
		}
		return insertInvitationEvent(tx, invitationID, actorID, "DELETED", now)
	})
}

func (r *Repository) DeleteAffiliation(ctx context.Context, hospitalID, doctorID, actorID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []entity.DoctorHospitalAffiliation
		if err := tx.Where("hospital_id = ? AND doctor_id = ? AND deleted_at IS NULL", hospitalID, doctorID).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrAffiliationNotFound
		}
		for _, row := range rows {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "appointment:affiliation:"+row.ID).Error; err != nil {
				return err
			}
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("hospital_id = ? AND doctor_id = ? AND deleted_at IS NULL", hospitalID, doctorID).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrAffiliationNotFound
		}
		var busy bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM appointments WHERE hospital_id = ? AND doctor_id = ?
			AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION'))`, hospitalID, doctorID).Scan(&busy).Error; err != nil {
			return err
		}
		if busy {
			return ErrResourceInUse
		}
		for _, row := range rows {
			if err := tx.Exec(`UPDATE doctor_hospital_schedules SET is_active = FALSE, updated_at = ? WHERE affiliation_id = ?`, now, row.ID).Error; err != nil {
				return err
			}
			if err := insertAffiliationEvent(tx, row.ID, actorID, "DELETED", &row.Status, entity.DoctorHospitalAffiliationSuspended, now); err != nil {
				return err
			}
		}
		if err := tx.Model(&entity.DoctorHospitalAffiliation{}).Where("hospital_id = ? AND doctor_id = ? AND deleted_at IS NULL", hospitalID, doctorID).
			Updates(map[string]any{"status": "SUSPENDED", "deleted_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM hospital_user_roles WHERE hospital_id = ? AND user_id = ? AND role_id IN (SELECT id FROM roles WHERE UPPER(slug) = 'DOCTOR')`, hospitalID, doctorID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE user_hospitals SET is_active = FALSE, updated_at = ? WHERE hospital_id = ? AND user_id = ?
			AND NOT EXISTS (SELECT 1 FROM hospital_user_roles WHERE hospital_id = ? AND user_id = ?)`, now, hospitalID, doctorID, hospitalID, doctorID).Error
	})
}
