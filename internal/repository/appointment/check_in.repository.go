package appointment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

func (r *Repository) GetPatientRecordForAppointment(ctx context.Context, appointmentID string) (*PatientRecord, error) {
	var row PatientRecord
	if err := r.db.WithContext(ctx).Raw(`
		SELECT record.id, record.user_id, record.created_at_hospital_id,
		       record.first_name, record.last_name, record.email, record.phone,
		       record.dob::text AS date_of_birth, record.gender,
		       record.identity_type, record.identity_number,
		       record.identity_number_normalized, record.claimed_at, record.created_at
		FROM appointments appointment
		JOIN patient_records record ON record.id = appointment.patient_record_id
		WHERE appointment.id = ?`, appointmentID).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, ErrPatientRecordNotFound
	}
	return &row, nil
}

func (r *Repository) FindCheckInAppointments(ctx context.Context, hospitalID, date string, filter CheckInIdentityFilter) ([]response.Appointment, error) {
	where := []string{"appointment.hospital_id = ?", "appointment.appointment_date = CAST(? AS date)"}
	args := []any{hospitalID, date}
	if filter.MedikaOneID != "" {
		where = append(where, "COALESCE(patient_record.user_id, appointment.patient_id)::text = ?")
		args = append(args, filter.MedikaOneID)
	}
	if filter.NIK != "" {
		where = append(where, "((patient_record.identity_type = 'NIK' AND patient_record.identity_number_normalized = ?) OR REGEXP_REPLACE(COALESCE(patient.nik, ''), '[^[:alnum:]]', '', 'g') = ?)")
		normalized := normalizeIdentityNumber(filter.NIK)
		args = append(args, normalized, normalized)
	}
	if filter.IdentityNumber != "" {
		where = append(where, "patient_record.identity_type = ? AND patient_record.identity_number_normalized = ?")
		args = append(args, filter.IdentityType, normalizeIdentityNumber(filter.IdentityNumber))
	}
	if filter.Email != "" {
		where = append(where, "LOWER(COALESCE(patient_record.email, patient.email, '')) = LOWER(?)")
		args = append(args, strings.TrimSpace(filter.Email))
	}
	if filter.Phone != "" {
		where = append(where, "REGEXP_REPLACE(COALESCE(patient_record.phone, patient.phone, ''), '[^0-9]', '', 'g') = ?")
		args = append(args, normalizePhone(filter.Phone))
	}
	if filter.Name != "" {
		where = append(where, "LOWER(TRIM(CONCAT_WS(' ', patient_record.first_name, patient_record.last_name))) LIKE ? ESCAPE E'\\\\'")
		args = append(args, "%"+escapeLike(strings.ToLower(strings.TrimSpace(filter.Name)))+"%")
	}
	if filter.DateOfBirth != "" {
		where = append(where, "patient_record.dob = CAST(? AS date)")
		args = append(args, filter.DateOfBirth)
	}
	args = append(args, 20)
	var rows []response.Appointment
	if err := r.db.WithContext(ctx).Raw(appointmentSelect+` WHERE `+strings.Join(where, " AND ")+`
		ORDER BY appointment.scheduled_start_at, appointment.queue_number
		LIMIT ?`, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) ListQueue(ctx context.Context, filter QueueFilter) (*response.AppointmentPage, error) {
	where := []string{
		"appointment.hospital_id = ?",
		"appointment.appointment_date = CAST(? AS date)",
		"appointment.queue_activated_at IS NOT NULL",
		"appointment.status IN ('WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')",
	}
	args := []any{filter.HospitalID, filter.Date}
	if filter.DoctorID != "" {
		where = append(where, "appointment.doctor_id = ?")
		args = append(args, filter.DoctorID)
	}
	if filter.DepartmentID != "" {
		where = append(where, "appointment.department_id = ?")
		args = append(args, filter.DepartmentID)
	}
	if filter.Status != "" {
		where = append(where, "appointment.status = ?")
		args = append(args, filter.Status)
	}
	if filter.BookingMode != "" {
		where = append(where, "appointment.booking_mode = ?")
		args = append(args, filter.BookingMode)
	}
	if filter.Reference != "" {
		where = append(where, "(UPPER(appointment.appointment_number) = UPPER(?) OR UPPER(appointment.queue_number) = UPPER(?))")
		args = append(args, strings.TrimSpace(filter.Reference), strings.TrimSpace(filter.Reference))
	}

	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM appointments appointment WHERE `+whereSQL, args...).Scan(&total).Error; err != nil {
		return nil, err
	}
	page, limit := filter.Page, filter.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	queryArgs := append(append([]any{}, args...), limit, (page-1)*limit)
	var rows []response.Appointment
	if err := r.db.WithContext(ctx).Raw(appointmentSelect+` WHERE `+whereSQL+`
		ORDER BY appointment.queue_number, appointment.queue_activated_at
		LIMIT ? OFFSET ?`, queryArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []response.Appointment{}
	}
	return &response.AppointmentPage{Items: rows, Page: page, Limit: limit, Total: total}, nil
}

func (r *Repository) CanOverrideWalkInCapacity(ctx context.Context, hospitalID, actorID string) (bool, error) {
	return canOverrideWalkInCapacity(r.db.WithContext(ctx), hospitalID, actorID)
}

func canOverrideWalkInCapacity(tx *gorm.DB, hospitalID, actorID string) (bool, error) {
	var allowed bool
	err := tx.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM user_roles user_role
			JOIN roles role ON role.id = user_role.role_id
			WHERE user_role.user_id = ? AND UPPER(role.slug) = 'SUPER_ADMIN'
			  AND role.active = TRUE AND role.deleted_at IS NULL
			UNION ALL
			SELECT 1
			FROM hospital_user_roles hospital_role
			JOIN roles role ON role.id = hospital_role.role_id
			JOIN user_hospitals membership
			  ON membership.user_id = hospital_role.user_id
			 AND membership.hospital_id = hospital_role.hospital_id
			WHERE hospital_role.user_id = ? AND hospital_role.hospital_id = ?
			  AND UPPER(role.slug) = 'ADMIN' AND role.active = TRUE AND role.deleted_at IS NULL
			  AND membership.is_active = TRUE AND membership.deleted_at IS NULL
		)`, actorID, actorID, hospitalID).Scan(&allowed).Error
	return allowed, err
}

func (r *Repository) CreateWalkIn(ctx context.Context, input WalkInInput) (*response.Appointment, bool, error) {
	var appointmentID string
	var replay bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		record, created, err := resolveWalkInPatientRecord(tx.WithContext(ctx), input)
		if err != nil {
			return err
		}
		if created {
			if err := insertPatientRecordEvent(tx, record.ID, input.ActorID, input.Schedule.HospitalID, "CREATED", map[string]any{"source": "walk_in"}, input.Now); err != nil {
				return err
			}
		}

		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "appointment:patient-record:"+record.ID).Error; err != nil {
			return err
		}
		var existing struct {
			ID          string
			RequestHash string `gorm:"column:idempotency_request_hash"`
		}
		if err := tx.Raw(`
			SELECT id, idempotency_request_hash
			FROM appointments WHERE patient_record_id = ? AND idempotency_key = ?`,
			record.ID, input.IdempotencyKey).Scan(&existing).Error; err != nil {
			return err
		}
		if existing.ID != "" {
			if existing.RequestHash != input.IdempotencyRequestHash {
				return ErrIdempotencyConflict
			}
			appointmentID, replay = existing.ID, true
			return nil
		}

		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "appointment:slot:"+input.Schedule.ID+":"+input.ScheduledStartAt.UTC().Format(time.RFC3339)).Error; err != nil {
			return err
		}
		var schedule Schedule
		if err := tx.Raw(`
			SELECT schedule.id, schedule.affiliation_id, affiliation.hospital_id,
			       COALESCE(hospital.code, 'HOSPITAL') AS hospital_code,
			       affiliation.doctor_id, affiliation.department_id, affiliation.room_id,
			       schedule.day_of_week, TO_CHAR(schedule.start_time, 'HH24:MI') AS start_time,
			       TO_CHAR(schedule.end_time, 'HH24:MI') AS end_time, schedule.timezone,
			       schedule.booking_mode, schedule.slot_duration_minutes, schedule.capacity
			FROM doctor_hospital_schedules schedule
			JOIN doctor_hospital_affiliations affiliation ON affiliation.id = schedule.affiliation_id
			JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
			WHERE schedule.id = ? AND affiliation.hospital_id = ?
			  AND schedule.is_active = TRUE AND affiliation.status = 'ACTIVE'
			  AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL
			FOR SHARE OF schedule`, input.Schedule.ID, input.Schedule.HospitalID).Scan(&schedule).Error; err != nil {
			return err
		}
		if schedule.ID == "" {
			return ErrScheduleNotFound
		}

		var overlaps bool
		if err := tx.Raw(`
			SELECT EXISTS(
				SELECT 1 FROM appointments
				WHERE patient_record_id = ? AND scheduled_start_at < ? AND scheduled_end_at > ?
				  AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')
			)`, record.ID, input.ScheduledEndAt, input.ScheduledStartAt).Scan(&overlaps).Error; err != nil {
			return err
		}
		if overlaps {
			return ErrPatientTimeConflict
		}

		var positions []int
		if err := tx.Raw(`
			SELECT slot_position FROM appointments
			WHERE schedule_id = ? AND appointment_date = CAST(? AS date)
			  AND scheduled_start_at = ?
			  AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')
			ORDER BY slot_position`, schedule.ID, input.AppointmentDate, input.ScheduledStartAt).Scan(&positions).Error; err != nil {
			return err
		}
		used := make(map[int]struct{}, len(positions))
		for _, position := range positions {
			used[position] = struct{}{}
		}
		maxPosition := schedule.Capacity
		if input.CapacityOverride {
			allowed, permissionErr := canOverrideWalkInCapacity(tx, schedule.HospitalID, input.ActorID)
			if permissionErr != nil {
				return permissionErr
			}
			if !allowed {
				return ErrWalkInCapacityForbidden
			}
			maxPosition = 500
		}
		slotPosition := 0
		for candidate := 1; candidate <= maxPosition; candidate++ {
			if _, exists := used[candidate]; !exists {
				slotPosition = candidate
				break
			}
		}
		if slotPosition == 0 || (!input.CapacityOverride && slotPosition > schedule.Capacity) {
			return ErrWalkInCapacityFull
		}

		appointmentSequence, err := nextCounter(tx, schedule.HospitalID, input.AppointmentDate, "APPOINTMENT", "GLOBAL", input.Now)
		if err != nil {
			return err
		}
		queueSequence, err := nextCounter(tx, schedule.HospitalID, input.AppointmentDate, "QUEUE", "GLOBAL", input.Now)
		if err != nil {
			return err
		}
		appointmentID = uuid.NewString()
		appointmentNumber := fmt.Sprintf("APT-%s-%s-%04d", sanitizeCode(schedule.HospitalCode), strings.ReplaceAll(input.AppointmentDate, "-", ""), appointmentSequence)
		queueNumber := fmt.Sprintf("Q-%03d", queueSequence)
		if err := tx.Exec(`
			INSERT INTO appointments (
				id, appointment_number, patient_id, patient_record_id, affiliation_id, schedule_id,
				hospital_id, doctor_id, department_id, room_id, appointment_date,
				scheduled_start_at, scheduled_end_at, booking_mode, slot_position, queue_number,
				queue_activated_at, status, attendance_status, source, created_by,
				reason_for_visit, note, consent_version, consented_at, consent_method,
				idempotency_key, idempotency_request_hash, verification_used_at,
				checked_in_at, check_in_method, capacity_overridden, capacity_override_reason,
				created_at, updated_at
			) VALUES (
				?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS date), ?, ?, ?, ?, ?,
				?, 'WAITING_VITALS', 'PRESENT', 'WALK_IN', ?, ?, ?, ?, ?, 'RECEPTIONIST_INFORMED',
				?, ?, ?, ?, 'WALK_IN', ?, ?, ?, ?
			)`, appointmentID, appointmentNumber, record.UserID, record.ID,
			schedule.AffiliationID, schedule.ID, schedule.HospitalID, schedule.DoctorID,
			schedule.DepartmentID, schedule.RoomID, input.AppointmentDate,
			input.ScheduledStartAt, input.ScheduledEndAt, schedule.BookingMode, slotPosition,
			queueNumber, input.Now, input.ActorID, input.ReasonForVisit, input.Note,
			input.ConsentVersion, input.Now, input.IdempotencyKey, input.IdempotencyRequestHash,
			input.Now, input.Now, input.CapacityOverride, input.CapacityOverrideReason, input.Now, input.Now).Error; err != nil {
			return err
		}
		if err := insertAppointmentEvent(tx, appointmentID, input.ActorID, "CREATED", nil, entity.AppointmentConfirmed, input.CapacityOverrideReason, input.Now); err != nil {
			return err
		}
		confirmed := entity.AppointmentConfirmed
		if err := insertAppointmentEvent(tx, appointmentID, input.ActorID, "CHECKED_IN", &confirmed, entity.AppointmentCheckedIn, nil, input.Now); err != nil {
			return err
		}
		checkedIn := entity.AppointmentCheckedIn
		if err := insertAppointmentEvent(tx, appointmentID, input.ActorID, "WAITING_VITALS", &checkedIn, entity.AppointmentWaitingVitals, nil, input.Now); err != nil {
			return err
		}
		data, _ := json.Marshal(map[string]any{"appointment_id": appointmentID, "appointment_number": appointmentNumber, "event": "WALK_IN_CREATED"})
		if record.UserID != nil {
			if err := insertNotification(tx, *record.UserID, "WALK_IN_CREATED", "Walk-in terdaftar", "Kunjungan walk-in Anda telah didaftarkan.", data, input.Now); err != nil {
				return err
			}
		}
		return insertNotification(tx, schedule.DoctorID, "WALK_IN_CREATED", "Pasien walk-in baru", "Pasien walk-in telah ditambahkan ke antrean Anda.", data, input.Now)
	})
	if err != nil {
		return nil, false, err
	}
	row, err := r.GetAppointment(ctx, appointmentID)
	return row, replay, err
}

func resolveWalkInPatientRecord(tx *gorm.DB, input WalkInInput) (*PatientRecord, bool, error) {
	patient := input.Patient
	if patient.PatientRecordID != "" {
		var row PatientRecord
		if err := scanPatientRecord(tx.Raw(`
			SELECT id, user_id, created_at_hospital_id, first_name, last_name, email, phone,
			       dob::text AS date_of_birth, gender, identity_type, identity_number,
			       identity_number_normalized, claimed_at, created_at
			FROM patient_records WHERE id = ? FOR UPDATE`, patient.PatientRecordID), &row); err != nil {
			return nil, false, err
		}
		if row.ID == "" {
			return nil, false, ErrWalkInPatientNotFound
		}
		return &row, false, nil
	}
	if patient.MedikaOneID != "" {
		recordID, err := ensurePatientRecordForUser(tx, patient.MedikaOneID, input.Now)
		if err != nil {
			if errors.Is(err, ErrPatientRecordNotFound) {
				return nil, false, ErrWalkInPatientNotFound
			}
			return nil, false, err
		}
		var row PatientRecord
		if err := scanPatientRecord(tx.Raw(`
			SELECT id, user_id, created_at_hospital_id, first_name, last_name, email, phone,
			       dob::text AS date_of_birth, gender, identity_type, identity_number,
			       identity_number_normalized, claimed_at, created_at
			FROM patient_records WHERE id = ?`, recordID), &row); err != nil {
			return nil, false, err
		}
		return &row, false, nil
	}

	if err := tx.Exec(
		`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`,
		"patient-record:"+patient.IdentityType+":"+patient.IdentityNormalized,
	).Error; err != nil {
		return nil, false, err
	}
	var existing PatientRecord
	if err := scanPatientRecord(tx.Raw(`
		SELECT id, user_id, created_at_hospital_id, first_name, last_name, email, phone,
		       dob::text AS date_of_birth, gender, identity_type, identity_number,
		       identity_number_normalized, claimed_at, created_at
		FROM patient_records
		WHERE identity_type = ? AND identity_number_normalized = ?
		FOR UPDATE`, patient.IdentityType, patient.IdentityNormalized), &existing); err != nil {
		return nil, false, err
	}
	if existing.ID != "" {
		if existing.DateOfBirth != patient.DateOfBirth {
			return nil, false, ErrWalkInPatientIdentityConflict
		}
		return &existing, false, nil
	}

	var linkedUserID *string
	if patient.IdentityType == "NIK" || patient.IdentityType == "MEDIKAONE_ID" {
		var userID string
		where := "REGEXP_REPLACE(COALESCE(user_account.nik, ''), '[^[:alnum:]]', '', 'g') = ?"
		identifier := patient.IdentityNormalized
		if patient.IdentityType == "MEDIKAONE_ID" {
			where = "user_account.id::text = ?"
			identifier = patient.IdentityNumber
		}
		if err := tx.Raw(`
			SELECT user_account.id
			FROM users user_account
			JOIN user_roles user_role ON user_role.user_id = user_account.id
			JOIN roles role ON role.id = user_role.role_id
			WHERE `+where+` AND user_account.dob = CAST(? AS date)
			  AND user_account.status = 'active' AND user_account.deleted_at IS NULL
			  AND UPPER(role.slug) = 'PATIENT' AND role.active = TRUE AND role.deleted_at IS NULL
			LIMIT 1`, identifier, patient.DateOfBirth).Scan(&userID).Error; err != nil {
			return nil, false, err
		}
		if userID != "" {
			linkedUserID = &userID
		}
	}

	recordID := uuid.NewString()
	claimedAt, claimedBy := any(nil), any(nil)
	if linkedUserID != nil {
		claimedAt, claimedBy = input.Now, *linkedUserID
	}
	if err := tx.Exec(`
		INSERT INTO patient_records (
			id, user_id, created_at_hospital_id, first_name, last_name, email, phone,
			dob, gender, identity_type, identity_number, identity_number_normalized,
			created_by, claimed_at, claimed_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, CAST(? AS date), ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		recordID, linkedUserID, input.Schedule.HospitalID, patient.FirstName, patient.LastName,
		patient.Email, patient.Phone, patient.DateOfBirth, patient.Gender, patient.IdentityType,
		patient.IdentityNumber, patient.IdentityNormalized, input.ActorID, claimedAt, claimedBy,
		input.Now, input.Now).Error; err != nil {
		return nil, false, err
	}
	return &PatientRecord{
		ID: recordID, UserID: linkedUserID, CreatedAtHospitalID: &input.Schedule.HospitalID,
		FirstName: patient.FirstName, LastName: patient.LastName, Email: patient.Email,
		Phone: patient.Phone, DateOfBirth: patient.DateOfBirth, Gender: patient.Gender,
		IdentityType: patient.IdentityType, IdentityNumber: patient.IdentityNumber,
		IdentityNumberNormalized: patient.IdentityNormalized, CreatedAt: input.Now,
	}, true, nil
}

func (r *Repository) ClaimPatientRecord(ctx context.Context, userID, identityType, identityNumber, dateOfBirth string, now time.Time) (*PatientRecord, error) {
	normalized := normalizeIdentityNumber(identityNumber)
	var claimedID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record PatientRecord
		if err := scanPatientRecord(tx.Raw(`
			SELECT id, user_id, created_at_hospital_id, first_name, last_name, email, phone,
			       dob::text AS date_of_birth, gender, identity_type, identity_number,
			       identity_number_normalized, claimed_at, created_at
			FROM patient_records
			WHERE identity_type = ? AND identity_number_normalized = ? AND dob = CAST(? AS date)
			FOR UPDATE`, identityType, normalized, dateOfBirth), &record); err != nil {
			return err
		}
		if record.ID == "" {
			return ErrPatientRecordNotFound
		}
		if record.UserID != nil {
			if *record.UserID == userID {
				claimedID = record.ID
				return nil
			}
			return ErrPatientRecordClaimed
		}

		var user struct {
			ID        string
			Email     string
			Phone     *string
			DOB       *time.Time
			NIK       *string
			IsPatient bool
		}
		if err := tx.Raw(`
			SELECT user_account.id, user_account.email, user_account.phone,
			       user_account.dob, user_account.nik,
			       EXISTS (
			           SELECT 1 FROM user_roles user_role
			           JOIN roles role ON role.id = user_role.role_id
			           WHERE user_role.user_id = user_account.id
			             AND UPPER(role.slug) = 'PATIENT'
			             AND role.active = TRUE AND role.deleted_at IS NULL
			       ) AS is_patient
			FROM users user_account
			WHERE user_account.id = ? AND user_account.status = 'active'
			  AND user_account.deleted_at IS NULL
			FOR UPDATE`, userID).Scan(&user).Error; err != nil {
			return err
		}
		if user.ID == "" || !user.IsPatient || user.DOB == nil || user.DOB.Format("2006-01-02") != dateOfBirth {
			return ErrPatientIdentityMismatch
		}

		identityMatches := false
		switch identityType {
		case "NIK":
			identityMatches = user.NIK != nil && normalizeIdentityNumber(*user.NIK) == normalized
		case "MEDIKAONE_ID":
			identityMatches = strings.EqualFold(user.ID, strings.TrimSpace(identityNumber))
		default:
			emailMatches := record.Email != nil && strings.EqualFold(strings.TrimSpace(*record.Email), strings.TrimSpace(user.Email))
			phoneMatches := user.Phone != nil && normalizePhone(*user.Phone) == normalizePhone(record.Phone)
			identityMatches = emailMatches || phoneMatches
		}
		if !identityMatches {
			return ErrPatientIdentityMismatch
		}

		if err := tx.Exec(`
			UPDATE patient_records
			SET user_id = ?, claimed_at = ?, claimed_by = ?, updated_at = ?
			WHERE id = ?`, userID, now, userID, now, record.ID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE appointments SET patient_id = ?, updated_at = ? WHERE patient_record_id = ? AND patient_id IS NULL`, userID, now, record.ID).Error; err != nil {
			return err
		}
		if err := insertPatientRecordEvent(tx, record.ID, userID, record.CreatedAtHospitalID, "CLAIMED", map[string]any{"identity_type": identityType}, now); err != nil {
			return err
		}
		claimedID = record.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetPatientRecord(ctx, claimedID)
}

func (r *Repository) GetPatientRecord(ctx context.Context, recordID string) (*PatientRecord, error) {
	var row PatientRecord
	if err := scanPatientRecord(r.db.WithContext(ctx).Raw(`
		SELECT id, user_id, created_at_hospital_id, first_name, last_name, email, phone,
		       dob::text AS date_of_birth, gender, identity_type, identity_number,
		       identity_number_normalized, claimed_at, created_at
		FROM patient_records WHERE id = ?`, recordID), &row); err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, ErrPatientRecordNotFound
	}
	return &row, nil
}

func scanPatientRecord(query *gorm.DB, destination *PatientRecord) error {
	return query.Scan(destination).Error
}

func normalizePhone(value string) string {
	var normalized strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
