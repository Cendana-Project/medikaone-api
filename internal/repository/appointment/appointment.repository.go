package appointment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

var (
	ErrScheduleNotFound           = errors.New("active doctor schedule not found")
	ErrAffiliationNotFound        = errors.New("active doctor affiliation not found")
	ErrSlotUnavailable            = errors.New("appointment slot unavailable")
	ErrPatientTimeConflict        = errors.New("patient already has an overlapping appointment")
	ErrAppointmentNotFound        = errors.New("appointment not found")
	ErrInvalidAppointmentState    = errors.New("invalid appointment state")
	ErrInvalidVerification        = errors.New("invalid or used appointment verification")
	ErrIdempotencyConflict        = errors.New("idempotency key reused with another request")
	ErrScheduleChangeNotFound     = errors.New("schedule change request not found")
	ErrScheduleChangeExists       = errors.New("pending schedule change exists")
	ErrInvalidScheduleChangeState = errors.New("invalid schedule change state")
	ErrScheduleChangeOwnApproval  = errors.New("schedule change requires counterpart review")
	ErrScheduleChangeAppointments = errors.New("schedule change has active appointments")
	ErrDoctorScheduleConflict     = errors.New("doctor schedule conflicts with another affiliation")
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type Schedule struct {
	ID                  string
	AffiliationID       string
	HospitalID          string
	HospitalCode        string
	HospitalName        string
	DoctorID            string
	DoctorName          string
	DepartmentID        string
	DepartmentName      string
	RoomID              *string
	RoomName            *string
	DayOfWeek           int
	StartTime           string
	EndTime             string
	Timezone            string
	BookingMode         string
	SlotDurationMinutes int
	Capacity            int
}

type AvailabilityFilter struct {
	HospitalID string
	DoctorID   string
	ScheduleID string
}

type ReservedCount struct {
	ScheduleID      string
	AppointmentDate string
	ScheduledStart  time.Time
	Reserved        int
}

type BookInput struct {
	PatientID              string
	Schedule               Schedule
	AppointmentDate        string
	ScheduledStartAt       time.Time
	ScheduledEndAt         time.Time
	ReasonForVisit         string
	Note                   *string
	ConsentVersion         string
	ConsentIP              string
	ConsentUserAgent       string
	IdempotencyKey         string
	IdempotencyRequestHash string
	RescheduledFromID      *string
	Now                    time.Time
}

type AppointmentFilter struct {
	PatientID  string
	DoctorID   string
	HospitalID string
	Status     string
	Date       string
	Limit      int
}

type ScheduleChangeInput struct {
	AffiliationID string
	ActorID       string
	ActorParty    string
	HospitalID    string
	Reason        *string
	Schedules     []ScheduleItem
	Now           time.Time
	ExpiresAt     time.Time
}

type ScheduleItem struct {
	DayOfWeek           int
	StartTime           string
	EndTime             string
	Timezone            string
	BookingMode         string
	SlotDurationMinutes int
	Capacity            int
}

const appointmentSelect = `
	SELECT appointment.id, appointment.appointment_number, appointment.patient_id,
	       TRIM(CONCAT_WS(' ', patient.first_name, patient.last_name)) AS patient_name,
	       appointment.affiliation_id, appointment.schedule_id, appointment.hospital_id,
	       hospital.name AS hospital_name, appointment.doctor_id,
	       TRIM(CONCAT_WS(' ', doctor.first_name, doctor.last_name)) AS doctor_name,
	       appointment.department_id, department.name AS department_name,
	       appointment.room_id, room.name AS room_name,
	       appointment.appointment_date::text AS appointment_date,
	       appointment.scheduled_start_at, appointment.scheduled_end_at, schedule.timezone,
	       appointment.booking_mode, appointment.queue_number,
	       appointment.queue_activated_at IS NOT NULL AS queue_active,
	       appointment.status, appointment.attendance_status,
	       appointment.reason_for_visit, appointment.note,
	       appointment.consent_version, appointment.consented_at,
	       appointment.checked_in_at, appointment.cancelled_at,
	       appointment.cancellation_reason, appointment.completed_at,
	       appointment.rescheduled_from_id, appointment.rescheduled_to_id,
	       appointment.created_at, appointment.updated_at
	FROM appointments appointment
	JOIN users patient ON patient.id = appointment.patient_id
	JOIN users doctor ON doctor.id = appointment.doctor_id
	JOIN hospitals hospital ON hospital.id = appointment.hospital_id
	JOIN hospital_departments department ON department.id = appointment.department_id
	LEFT JOIN hospital_rooms room ON room.id = appointment.room_id
	JOIN doctor_hospital_schedules schedule ON schedule.id = appointment.schedule_id`

func (r *Repository) ListActiveSchedules(ctx context.Context, filter AvailabilityFilter) ([]Schedule, error) {
	where := `affiliation.status = 'ACTIVE' AND schedule.is_active = TRUE
		AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL
		AND doctor.status = 'active' AND doctor.deleted_at IS NULL
		AND department.is_active = TRUE`
	args := []any{}
	if filter.HospitalID != "" {
		where += " AND affiliation.hospital_id = ?"
		args = append(args, filter.HospitalID)
	}
	if filter.DoctorID != "" {
		where += " AND affiliation.doctor_id = ?"
		args = append(args, filter.DoctorID)
	}
	if filter.ScheduleID != "" {
		where += " AND schedule.id = ?"
		args = append(args, filter.ScheduleID)
	}
	var rows []Schedule
	err := r.db.WithContext(ctx).Raw(`
		SELECT schedule.id, schedule.affiliation_id, affiliation.hospital_id,
		       COALESCE(hospital.code, 'HOSPITAL') AS hospital_code, hospital.name AS hospital_name,
		       affiliation.doctor_id,
		       TRIM(CONCAT_WS(' ', doctor.first_name, doctor.last_name)) AS doctor_name,
		       affiliation.department_id, department.name AS department_name,
		       affiliation.room_id, room.name AS room_name,
		       schedule.day_of_week,
		       TO_CHAR(schedule.start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(schedule.end_time, 'HH24:MI') AS end_time,
		       schedule.timezone, schedule.booking_mode,
		       schedule.slot_duration_minutes, schedule.capacity
		FROM doctor_hospital_schedules schedule
		JOIN doctor_hospital_affiliations affiliation ON affiliation.id = schedule.affiliation_id
		JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
		JOIN users doctor ON doctor.id = affiliation.doctor_id
		JOIN hospital_departments department ON department.id = affiliation.department_id
		LEFT JOIN hospital_rooms room ON room.id = affiliation.room_id
		WHERE `+where+`
		ORDER BY hospital.name, doctor.first_name, schedule.day_of_week, schedule.start_time`, args...).Scan(&rows).Error
	return rows, err
}

func (r *Repository) GetActiveSchedule(ctx context.Context, scheduleID string) (*Schedule, error) {
	rows, err := r.ListActiveSchedules(ctx, AvailabilityFilter{ScheduleID: scheduleID})
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return &rows[0], nil
	}
	return nil, ErrScheduleNotFound
}

func (r *Repository) ReservedCounts(ctx context.Context, from, to string, filter AvailabilityFilter) ([]ReservedCount, error) {
	where := `appointment.appointment_date BETWEEN CAST(? AS date) AND CAST(? AS date)
		AND appointment.status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')`
	args := []any{from, to}
	if filter.HospitalID != "" {
		where += " AND appointment.hospital_id = ?"
		args = append(args, filter.HospitalID)
	}
	if filter.DoctorID != "" {
		where += " AND appointment.doctor_id = ?"
		args = append(args, filter.DoctorID)
	}
	var rows []ReservedCount
	err := r.db.WithContext(ctx).Raw(`
		SELECT appointment.schedule_id, appointment.appointment_date::text AS appointment_date,
		       appointment.scheduled_start_at AS scheduled_start, COUNT(*)::int AS reserved
		FROM appointments appointment
		WHERE `+where+`
		GROUP BY appointment.schedule_id, appointment.appointment_date, appointment.scheduled_start_at`, args...).Scan(&rows).Error
	return rows, err
}

func (r *Repository) Book(ctx context.Context, input BookInput) (*response.Appointment, bool, error) {
	var appointmentID string
	var replay bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		id, wasReplay, err := r.bookTx(ctx, tx, input, "")
		appointmentID, replay = id, wasReplay
		return err
	})
	if err != nil {
		return nil, false, err
	}
	row, err := r.GetAppointment(ctx, appointmentID)
	return row, replay, err
}

func (r *Repository) bookTx(ctx context.Context, tx *gorm.DB, input BookInput, excludeAppointmentID string) (string, bool, error) {
	// Serialize all booking mutations for a patient. Besides preventing
	// overlapping appointments across different hospitals, this makes the
	// idempotency lookup and insert atomic for concurrent retries.
	if err := tx.WithContext(ctx).Exec(
		`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`,
		"appointment:patient:"+input.PatientID,
	).Error; err != nil {
		return "", false, err
	}
	var existing struct {
		ID          string
		RequestHash string `gorm:"column:idempotency_request_hash"`
	}
	if err := tx.WithContext(ctx).Raw(`
		SELECT id, idempotency_request_hash
		FROM appointments WHERE patient_id = ? AND idempotency_key = ?`,
		input.PatientID, input.IdempotencyKey).Scan(&existing).Error; err != nil {
		return "", false, err
	}
	if existing.ID != "" {
		if existing.RequestHash != input.IdempotencyRequestHash {
			return "", false, ErrIdempotencyConflict
		}
		return existing.ID, true, nil
	}

	if err := tx.WithContext(ctx).Exec(
		`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`,
		"appointment:affiliation:"+input.Schedule.AffiliationID,
	).Error; err != nil {
		return "", false, err
	}
	if err := tx.WithContext(ctx).Exec(
		`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`,
		fmt.Sprintf("appointment:slot:%s:%s:%s", input.Schedule.ID, input.AppointmentDate, input.ScheduledStartAt.UTC().Format(time.RFC3339)),
	).Error; err != nil {
		return "", false, err
	}

	var currentSchedule struct {
		ID                  string
		AffiliationID       string
		DayOfWeek           int
		StartTime           string
		EndTime             string
		Timezone            string
		BookingMode         string
		SlotDurationMinutes int
		Capacity            int
	}
	if err := tx.WithContext(ctx).Raw(`
		SELECT schedule.id, schedule.affiliation_id, schedule.day_of_week,
		       TO_CHAR(schedule.start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(schedule.end_time, 'HH24:MI') AS end_time,
		       schedule.timezone, schedule.booking_mode,
		       schedule.slot_duration_minutes, schedule.capacity
		FROM doctor_hospital_schedules schedule
		JOIN doctor_hospital_affiliations affiliation ON affiliation.id = schedule.affiliation_id
		JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
		WHERE schedule.id = ? AND schedule.affiliation_id = ?
		  AND schedule.is_active = TRUE AND affiliation.status = 'ACTIVE'
		  AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL
		FOR SHARE OF schedule`, input.Schedule.ID, input.Schedule.AffiliationID).Scan(&currentSchedule).Error; err != nil {
		return "", false, err
	}
	if currentSchedule.ID == "" {
		return "", false, ErrScheduleNotFound
	}
	if currentSchedule.AffiliationID != input.Schedule.AffiliationID ||
		currentSchedule.DayOfWeek != input.Schedule.DayOfWeek ||
		currentSchedule.StartTime != input.Schedule.StartTime ||
		currentSchedule.EndTime != input.Schedule.EndTime ||
		currentSchedule.Timezone != input.Schedule.Timezone ||
		currentSchedule.BookingMode != input.Schedule.BookingMode ||
		currentSchedule.SlotDurationMinutes != input.Schedule.SlotDurationMinutes ||
		currentSchedule.Capacity != input.Schedule.Capacity {
		return "", false, ErrSlotUnavailable
	}

	overlapArgs := []any{input.PatientID, input.ScheduledEndAt, input.ScheduledStartAt}
	overlapWhere := ""
	if excludeAppointmentID != "" {
		overlapWhere = " AND id <> ?"
		overlapArgs = append(overlapArgs, excludeAppointmentID)
	}
	var overlaps bool
	if err := tx.WithContext(ctx).Raw(`
		SELECT EXISTS(
			SELECT 1 FROM appointments
			WHERE patient_id = ?
			  AND scheduled_start_at < ? AND scheduled_end_at > ?
			  AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')`+overlapWhere+`
		)`, overlapArgs...).Scan(&overlaps).Error; err != nil {
		return "", false, err
	}
	if overlaps {
		return "", false, ErrPatientTimeConflict
	}

	var positions []int
	if err := tx.WithContext(ctx).Raw(`
		SELECT slot_position FROM appointments
		WHERE schedule_id = ? AND appointment_date = CAST(? AS date)
		  AND scheduled_start_at = ?
		  AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')
		ORDER BY slot_position`, input.Schedule.ID, input.AppointmentDate, input.ScheduledStartAt).Scan(&positions).Error; err != nil {
		return "", false, err
	}
	used := make(map[int]struct{}, len(positions))
	for _, position := range positions {
		used[position] = struct{}{}
	}
	slotPosition := 0
	for candidate := 1; candidate <= input.Schedule.Capacity; candidate++ {
		if _, exists := used[candidate]; !exists {
			slotPosition = candidate
			break
		}
	}
	if slotPosition == 0 {
		return "", false, ErrSlotUnavailable
	}

	appointmentSequence, err := nextCounter(tx, input.Schedule.HospitalID, input.AppointmentDate, "APPOINTMENT", "GLOBAL", input.Now)
	if err != nil {
		return "", false, err
	}
	queueSequence, err := nextCounter(tx, input.Schedule.HospitalID, input.AppointmentDate, "QUEUE", "GLOBAL", input.Now)
	if err != nil {
		return "", false, err
	}
	appointmentNumber := fmt.Sprintf("APT-%s-%s-%04d", sanitizeCode(input.Schedule.HospitalCode), strings.ReplaceAll(input.AppointmentDate, "-", ""), appointmentSequence)
	queueNumber := fmt.Sprintf("Q-%03d", queueSequence)
	appointmentID := uuid.NewString()
	if err := tx.WithContext(ctx).Exec(`
		INSERT INTO appointments (
			id, appointment_number, patient_id, affiliation_id, schedule_id,
			hospital_id, doctor_id, department_id, room_id, appointment_date,
			scheduled_start_at, scheduled_end_at, booking_mode, slot_position,
			queue_number, status, attendance_status, reason_for_visit, note,
			consent_version, consented_at, consent_ip, consent_user_agent,
			idempotency_key, idempotency_request_hash, rescheduled_from_id,
			created_at, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS date), ?, ?, ?, ?, ?,
			'CONFIRMED', 'PENDING', ?, ?, ?, ?, NULLIF(?, '')::inet, NULLIF(?, ''),
			?, ?, ?, ?, ?
		)`, appointmentID, appointmentNumber, input.PatientID, input.Schedule.AffiliationID,
		input.Schedule.ID, input.Schedule.HospitalID, input.Schedule.DoctorID,
		input.Schedule.DepartmentID, input.Schedule.RoomID, input.AppointmentDate,
		input.ScheduledStartAt, input.ScheduledEndAt, input.Schedule.BookingMode,
		slotPosition, queueNumber, input.ReasonForVisit, input.Note, input.ConsentVersion,
		input.Now, input.ConsentIP, input.ConsentUserAgent, input.IdempotencyKey,
		input.IdempotencyRequestHash, input.RescheduledFromID, input.Now, input.Now).Error; err != nil {
		return "", false, err
	}
	if err := insertAppointmentEvent(tx, appointmentID, input.PatientID, "CREATED", nil, entity.AppointmentConfirmed, nil, input.Now); err != nil {
		return "", false, err
	}
	for _, reminder := range []struct {
		kind string
		due  time.Time
	}{{"24_HOURS", input.ScheduledStartAt.Add(-24 * time.Hour)}, {"2_HOURS", input.ScheduledStartAt.Add(-2 * time.Hour)}} {
		if reminder.due.After(input.Now) {
			if err := tx.WithContext(ctx).Exec(`
				INSERT INTO appointment_reminders (id, appointment_id, reminder_type, due_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)`, uuid.NewString(), appointmentID, reminder.kind, reminder.due, input.Now, input.Now).Error; err != nil {
				return "", false, err
			}
		}
	}
	data, _ := json.Marshal(map[string]any{"appointment_id": appointmentID, "appointment_number": appointmentNumber, "event": "APPOINTMENT_CONFIRMED"})
	if err := insertNotification(tx, input.PatientID, "APPOINTMENT_CONFIRMED", "Appointment terkonfirmasi", "Appointment Anda berhasil dibuat.", data, input.Now); err != nil {
		return "", false, err
	}
	if err := insertNotification(tx, input.Schedule.DoctorID, "APPOINTMENT_CREATED", "Appointment baru", "Appointment baru telah ditambahkan ke jadwal Anda.", data, input.Now); err != nil {
		return "", false, err
	}
	return appointmentID, false, nil
}

func nextCounter(tx *gorm.DB, hospitalID, date, counterType, contextKey string, now time.Time) (int, error) {
	var value int
	err := tx.Raw(`
		INSERT INTO appointment_daily_counters (
			hospital_id, counter_date, counter_type, context_key, last_value, updated_at
		) VALUES (?, CAST(? AS date), ?, ?, 1, ?)
		ON CONFLICT (hospital_id, counter_date, counter_type, context_key)
		DO UPDATE SET last_value = appointment_daily_counters.last_value + 1, updated_at = EXCLUDED.updated_at
		RETURNING last_value`, hospitalID, date, counterType, contextKey, now).Scan(&value).Error
	return value, err
}

func sanitizeCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "HOSP"
	}
	if b.Len() > 12 {
		return b.String()[:12]
	}
	return b.String()
}

func (r *Repository) GetAppointment(ctx context.Context, appointmentID string) (*response.Appointment, error) {
	var row response.Appointment
	if err := r.db.WithContext(ctx).Raw(appointmentSelect+` WHERE appointment.id = ?`, appointmentID).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, ErrAppointmentNotFound
	}
	return &row, nil
}

func (r *Repository) GetAppointmentByNumber(ctx context.Context, hospitalID, number string) (*response.Appointment, error) {
	var row response.Appointment
	if err := r.db.WithContext(ctx).Raw(appointmentSelect+` WHERE appointment.hospital_id = ? AND UPPER(appointment.appointment_number) = UPPER(?)`, hospitalID, number).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, ErrAppointmentNotFound
	}
	return &row, nil
}

func (r *Repository) ListAppointments(ctx context.Context, filter AppointmentFilter) ([]response.Appointment, error) {
	where := []string{"1=1"}
	args := []any{}
	if filter.PatientID != "" {
		where = append(where, "appointment.patient_id = ?")
		args = append(args, filter.PatientID)
	}
	if filter.DoctorID != "" {
		where = append(where, "appointment.doctor_id = ?")
		args = append(args, filter.DoctorID)
	}
	if filter.HospitalID != "" {
		where = append(where, "appointment.hospital_id = ?")
		args = append(args, filter.HospitalID)
	}
	if filter.Status != "" {
		where = append(where, "appointment.status = ?")
		args = append(args, filter.Status)
	}
	if filter.Date != "" {
		where = append(where, "appointment.appointment_date = CAST(? AS date)")
		args = append(args, filter.Date)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	args = append(args, limit)
	var rows []response.Appointment
	err := r.db.WithContext(ctx).Raw(appointmentSelect+` WHERE `+strings.Join(where, " AND ")+`
		ORDER BY appointment.scheduled_start_at DESC LIMIT ?`, args...).Scan(&rows).Error
	return rows, err
}

func (r *Repository) Cancel(ctx context.Context, appointmentID, actorID, reason string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct{ Status string }
		if err := tx.Raw(`SELECT status FROM appointments WHERE id = ? FOR UPDATE`, appointmentID).Scan(&current).Error; err != nil {
			return err
		}
		if current.Status == "" {
			return ErrAppointmentNotFound
		}
		if !isActiveAppointmentStatus(current.Status) {
			return ErrInvalidAppointmentState
		}
		if err := tx.Exec(`
			UPDATE appointments SET status = 'CANCELLED', cancelled_at = ?, cancelled_by = ?,
			       cancellation_reason = ?, updated_at = ? WHERE id = ?`, now, actorID, reason, now, appointmentID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE appointment_reminders SET status = 'CANCELLED', updated_at = ? WHERE appointment_id = ? AND status <> 'SENT'`, now, appointmentID).Error; err != nil {
			return err
		}
		if err := insertAppointmentEvent(tx, appointmentID, actorID, "CANCELLED", &current.Status, entity.AppointmentCancelled, &reason, now); err != nil {
			return err
		}
		return notifyAppointmentParticipants(tx, appointmentID, "APPOINTMENT_CANCELLED", "Appointment dibatalkan", "Appointment telah dibatalkan.", now)
	})
}

func (r *Repository) Reschedule(ctx context.Context, oldID, actorID, reason string, input BookInput) (*response.Appointment, bool, error) {
	var newID string
	var replay bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old struct {
			Status          string
			PatientID       string
			RescheduledToID *string
		}
		if err := tx.Raw(`SELECT status, patient_id, rescheduled_to_id FROM appointments WHERE id = ? FOR UPDATE`, oldID).Scan(&old).Error; err != nil {
			return err
		}
		if old.Status == "" {
			return ErrAppointmentNotFound
		}
		input.PatientID = old.PatientID
		var existing struct {
			ID          string
			RequestHash string `gorm:"column:idempotency_request_hash"`
		}
		if err := tx.Raw(`SELECT id, idempotency_request_hash FROM appointments WHERE patient_id = ? AND idempotency_key = ?`, input.PatientID, input.IdempotencyKey).Scan(&existing).Error; err != nil {
			return err
		}
		if existing.ID != "" {
			if existing.RequestHash != input.IdempotencyRequestHash {
				return ErrIdempotencyConflict
			}
			if old.RescheduledToID == nil || *old.RescheduledToID != existing.ID {
				return ErrIdempotencyConflict
			}
			newID, replay = existing.ID, true
			return nil
		}
		if !isActiveAppointmentStatus(old.Status) {
			return ErrInvalidAppointmentState
		}
		if err := tx.Exec(`
			UPDATE appointments SET status = 'RESCHEDULED', cancelled_at = ?, cancelled_by = ?,
			       cancellation_reason = ?, updated_at = ? WHERE id = ?`, input.Now, actorID, reason, input.Now, oldID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE appointment_reminders SET status = 'CANCELLED', updated_at = ? WHERE appointment_id = ? AND status <> 'SENT'`, input.Now, oldID).Error; err != nil {
			return err
		}
		input.RescheduledFromID = &oldID
		id, _, err := r.bookTx(ctx, tx, input, oldID)
		if err != nil {
			return err
		}
		newID = id
		if err := tx.Exec(`UPDATE appointments SET rescheduled_to_id = ? WHERE id = ?`, newID, oldID).Error; err != nil {
			return err
		}
		if err := insertAppointmentEvent(tx, oldID, actorID, "RESCHEDULED", &old.Status, entity.AppointmentRescheduled, &reason, input.Now); err != nil {
			return err
		}
		return notifyAppointmentParticipants(tx, oldID, "APPOINTMENT_RESCHEDULED", "Appointment dijadwalkan ulang", "Appointment telah dipindahkan ke jadwal baru.", input.Now)
	})
	if err != nil {
		return nil, false, err
	}
	row, err := r.GetAppointment(ctx, newID)
	return row, replay, err
}

func (r *Repository) CheckIn(ctx context.Context, appointmentID, actorID string, overrideReason *string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct {
			Status           string
			VerificationUsed *time.Time
		}
		if err := tx.Raw(`SELECT status, verification_used_at FROM appointments WHERE id = ? FOR UPDATE`, appointmentID).Scan(&current).Error; err != nil {
			return err
		}
		if current.Status == "" {
			return ErrAppointmentNotFound
		}
		if current.Status != entity.AppointmentConfirmed {
			return ErrInvalidAppointmentState
		}
		if current.VerificationUsed != nil {
			return ErrInvalidVerification
		}
		if err := tx.Exec(`
			UPDATE appointments SET status = 'WAITING_VITALS', attendance_status = 'PRESENT',
			       verification_used_at = ?, checked_in_at = ?, queue_activated_at = ?, updated_at = ?
			WHERE id = ?`, now, now, now, now, appointmentID).Error; err != nil {
			return err
		}
		from := entity.AppointmentConfirmed
		if err := insertAppointmentEvent(tx, appointmentID, actorID, "CHECKED_IN", &from, entity.AppointmentCheckedIn, overrideReason, now); err != nil {
			return err
		}
		checkedIn := entity.AppointmentCheckedIn
		if err := insertAppointmentEvent(tx, appointmentID, actorID, "WAITING_VITALS", &checkedIn, entity.AppointmentWaitingVitals, nil, now); err != nil {
			return err
		}
		return notifyAppointmentParticipants(tx, appointmentID, "APPOINTMENT_CHECKED_IN", "Check-in berhasil", "Pasien telah check-in dan masuk antrean pemeriksaan awal.", now)
	})
}

func (r *Repository) Transition(ctx context.Context, appointmentID, actorID, target string, reason *string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct{ Status string }
		if err := tx.Raw(`SELECT status FROM appointments WHERE id = ? FOR UPDATE`, appointmentID).Scan(&current).Error; err != nil {
			return err
		}
		if current.Status == "" {
			return ErrAppointmentNotFound
		}
		valid := (current.Status == entity.AppointmentWaitingVitals && target == entity.AppointmentWaitingDoctor) ||
			(current.Status == entity.AppointmentWaitingDoctor && target == entity.AppointmentInConsultation) ||
			(current.Status == entity.AppointmentInConsultation && target == entity.AppointmentCompleted)
		if !valid {
			return ErrInvalidAppointmentState
		}
		completedAt := any(nil)
		if target == entity.AppointmentCompleted {
			completedAt = now
		}
		if err := tx.Exec(`UPDATE appointments SET status = ?, completed_at = ?, updated_at = ? WHERE id = ?`, target, completedAt, now, appointmentID).Error; err != nil {
			return err
		}
		if err := insertAppointmentEvent(tx, appointmentID, actorID, target, &current.Status, target, reason, now); err != nil {
			return err
		}
		return notifyAppointmentParticipants(tx, appointmentID, "APPOINTMENT_STATUS_CHANGED", "Status appointment diperbarui", "Status appointment telah diperbarui menjadi "+target+".", now)
	})
}

func (r *Repository) MarkNoShows(ctx context.Context, cutoff time.Time, now time.Time) (int64, error) {
	var affected int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Raw(`
			SELECT id FROM appointments
			WHERE status = 'CONFIRMED' AND attendance_status = 'PENDING'
			  AND scheduled_start_at < ?
			FOR UPDATE SKIP LOCKED`, cutoff).Scan(&ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			if err := tx.Exec(`UPDATE appointments SET status = 'NO_SHOW', attendance_status = 'NO_SHOW', updated_at = ? WHERE id = ?`, now, id).Error; err != nil {
				return err
			}
			from := entity.AppointmentConfirmed
			if err := insertAppointmentEvent(tx, id, nil, "NO_SHOW", &from, entity.AppointmentNoShow, nil, now); err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE appointment_reminders SET status = 'CANCELLED', updated_at = ? WHERE appointment_id = ? AND status <> 'SENT'`, now, id).Error; err != nil {
				return err
			}
			if err := notifyAppointmentParticipants(tx, id, "APPOINTMENT_NO_SHOW", "Appointment terlewat", "Appointment ditandai tidak hadir.", now); err != nil {
				return err
			}
			affected++
		}
		return nil
	})
	return affected, err
}

func (r *Repository) ClaimDueReminders(ctx context.Context, now time.Time, limit int) ([]response.AppointmentReminder, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var rows []response.AppointmentReminder
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Raw(`
			SELECT reminder.id FROM appointment_reminders reminder
			JOIN appointments appointment ON appointment.id = reminder.appointment_id
			WHERE reminder.due_at <= ?
			  AND (reminder.status IN ('PENDING','FAILED') OR (reminder.status = 'PROCESSING' AND reminder.updated_at < ?))
			  AND reminder.attempts < 5
			  AND appointment.status = 'CONFIRMED'
			ORDER BY reminder.due_at
			FOR UPDATE OF reminder SKIP LOCKED LIMIT ?`, now, now.Add(-5*time.Minute), limit).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Exec(`UPDATE appointment_reminders SET status = 'PROCESSING', attempts = attempts + 1, updated_at = ? WHERE id IN ?`, now, ids).Error; err != nil {
			return err
		}
		return tx.Raw(`
			SELECT reminder.id, reminder.appointment_id, reminder.reminder_type, reminder.due_at,
			       appointment.patient_id, patient.email AS patient_email, patient.first_name AS patient_first_name,
			       appointment.doctor_id, doctor.email AS doctor_email, doctor.first_name AS doctor_first_name,
			       hospital.name AS hospital_name, appointment.scheduled_start_at, schedule.timezone
			FROM appointment_reminders reminder
			JOIN appointments appointment ON appointment.id = reminder.appointment_id
			JOIN users patient ON patient.id = appointment.patient_id
			JOIN users doctor ON doctor.id = appointment.doctor_id
			JOIN hospitals hospital ON hospital.id = appointment.hospital_id
			JOIN doctor_hospital_schedules schedule ON schedule.id = appointment.schedule_id
			WHERE reminder.id IN ?`, ids).Scan(&rows).Error
	})
	return rows, err
}

func (r *Repository) CompleteReminder(ctx context.Context, reminder response.AppointmentReminder, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`UPDATE appointment_reminders SET status = 'SENT', sent_at = ?, last_error = NULL, updated_at = ? WHERE id = ? AND status = 'PROCESSING'`, now, now, reminder.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrInvalidAppointmentState
		}
		data, _ := json.Marshal(map[string]any{"appointment_id": reminder.AppointmentID, "reminder_type": reminder.ReminderType})
		body := "Pengingat appointment Anda di " + reminder.HospitalName + "."
		if err := insertNotification(tx, reminder.PatientID, "APPOINTMENT_REMINDER", "Pengingat appointment", body, data, now); err != nil {
			return err
		}
		return insertNotification(tx, reminder.DoctorID, "APPOINTMENT_REMINDER", "Pengingat jadwal pasien", "Anda memiliki appointment pasien yang akan datang.", data, now)
	})
}

func (r *Repository) FailReminder(ctx context.Context, reminderID, message string, now time.Time) error {
	if len(message) > 255 {
		message = message[:255]
	}
	return r.db.WithContext(ctx).Exec(`UPDATE appointment_reminders SET status = 'FAILED', last_error = ?, updated_at = ? WHERE id = ? AND status = 'PROCESSING'`, message, now, reminderID).Error
}

func (r *Repository) ExpireScheduleChanges(ctx context.Context, now time.Time) (int64, error) {
	var affected int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []struct {
			ID          string
			RequestedBy string
		}
		if err := tx.Raw(`
			SELECT id, requested_by
			FROM doctor_schedule_change_requests
			WHERE status = 'PENDING' AND expires_at <= ?
			FOR UPDATE SKIP LOCKED`, now).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := tx.Exec(`
				UPDATE doctor_schedule_change_requests
				SET status = 'EXPIRED', updated_at = ?
				WHERE id = ? AND status = 'PENDING'`, now, row.ID).Error; err != nil {
				return err
			}
			if err := insertScheduleChangeEvent(tx, row.ID, nil, "EXPIRED", now); err != nil {
				return err
			}
			data, _ := json.Marshal(map[string]any{"schedule_change_id": row.ID, "event": "SCHEDULE_CHANGE_EXPIRED"})
			if err := insertNotification(tx, row.RequestedBy, "SCHEDULE_CHANGE_EXPIRED", "Pengajuan jadwal kedaluwarsa", "Pengajuan perubahan jadwal tidak ditinjau dalam tujuh hari.", data, now); err != nil {
				return err
			}
			affected++
		}
		return nil
	})
	return affected, err
}

func (r *Repository) GetAffiliation(ctx context.Context, affiliationID string) (*Schedule, error) {
	var row Schedule
	if err := r.db.WithContext(ctx).Raw(`
		SELECT affiliation.id AS affiliation_id, affiliation.hospital_id,
		       COALESCE(hospital.code, 'HOSPITAL') AS hospital_code, hospital.name AS hospital_name,
		       affiliation.doctor_id, TRIM(CONCAT_WS(' ', doctor.first_name, doctor.last_name)) AS doctor_name,
		       affiliation.department_id, department.name AS department_name,
		       affiliation.room_id, room.name AS room_name
		FROM doctor_hospital_affiliations affiliation
		JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
		JOIN users doctor ON doctor.id = affiliation.doctor_id
		JOIN hospital_departments department ON department.id = affiliation.department_id
		LEFT JOIN hospital_rooms room ON room.id = affiliation.room_id
		WHERE affiliation.id = ? AND affiliation.status = 'ACTIVE'
		  AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL`, affiliationID).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.AffiliationID == "" {
		return nil, ErrAffiliationNotFound
	}
	return &row, nil
}

func (r *Repository) CreateScheduleChange(ctx context.Context, input ScheduleChangeInput) (*response.ScheduleChangeRequest, error) {
	changeID := uuid.NewString()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "schedule-change:"+input.AffiliationID).Error; err != nil {
			return err
		}
		var pending bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM doctor_schedule_change_requests WHERE affiliation_id = ? AND status = 'PENDING')`, input.AffiliationID).Scan(&pending).Error; err != nil {
			return err
		}
		if pending {
			return ErrScheduleChangeExists
		}
		if err := tx.Exec(`
			INSERT INTO doctor_schedule_change_requests (
				id, affiliation_id, requested_by, requested_by_party, status, reason,
				expires_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, 'PENDING', ?, ?, ?, ?)`, changeID, input.AffiliationID,
			input.ActorID, input.ActorParty, input.Reason, input.ExpiresAt, input.Now, input.Now).Error; err != nil {
			return err
		}
		for _, schedule := range input.Schedules {
			if err := tx.Exec(`
				INSERT INTO doctor_schedule_change_items (
					id, change_request_id, day_of_week, start_time, end_time, timezone,
					booking_mode, slot_duration_minutes, capacity, created_at
				) VALUES (?, ?, ?, ?::time, ?::time, ?, ?, ?, ?, ?)`, uuid.NewString(), changeID,
				schedule.DayOfWeek, schedule.StartTime, schedule.EndTime, schedule.Timezone,
				schedule.BookingMode, schedule.SlotDurationMinutes, schedule.Capacity, input.Now).Error; err != nil {
				return err
			}
		}
		if err := insertScheduleChangeEvent(tx, changeID, input.ActorID, "CREATED", input.Now); err != nil {
			return err
		}
		return notifyScheduleCounterpart(tx, input.AffiliationID, input.ActorParty, "SCHEDULE_CHANGE_REQUESTED", "Perubahan jadwal diajukan", "Terdapat pengajuan perubahan jadwal yang memerlukan persetujuan Anda.", changeID, input.Now)
	})
	if err != nil {
		return nil, err
	}
	return r.GetScheduleChange(ctx, changeID)
}

func (r *Repository) GetScheduleChange(ctx context.Context, changeID string) (*response.ScheduleChangeRequest, error) {
	var row response.ScheduleChangeRequest
	if err := r.db.WithContext(ctx).Raw(`
		SELECT change.id, change.affiliation_id, affiliation.hospital_id, hospital.name AS hospital_name,
		       affiliation.doctor_id, TRIM(CONCAT_WS(' ', doctor.first_name, doctor.last_name)) AS doctor_name,
		       change.requested_by, change.requested_by_party, change.status, change.reason,
		       change.reviewed_by, change.reviewed_at, change.rejection_reason,
		       change.expires_at, change.created_at, change.updated_at
		FROM doctor_schedule_change_requests change
		JOIN doctor_hospital_affiliations affiliation ON affiliation.id = change.affiliation_id
		JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
		JOIN users doctor ON doctor.id = affiliation.doctor_id
		WHERE change.id = ?`, changeID).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, ErrScheduleChangeNotFound
	}
	if err := r.db.WithContext(ctx).Raw(`
		SELECT id, day_of_week, TO_CHAR(start_time, 'HH24:MI') AS start_time,
		       TO_CHAR(end_time, 'HH24:MI') AS end_time, timezone, booking_mode,
		       slot_duration_minutes, capacity
		FROM doctor_schedule_change_items WHERE change_request_id = ?
		ORDER BY day_of_week, start_time`, changeID).Scan(&row.Schedules).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) ListScheduleChanges(ctx context.Context, hospitalID, doctorID, status string) ([]response.ScheduleChangeRequest, error) {
	where := []string{"1=1"}
	args := []any{}
	if hospitalID != "" {
		where = append(where, "affiliation.hospital_id = ?")
		args = append(args, hospitalID)
	}
	if doctorID != "" {
		where = append(where, "affiliation.doctor_id = ?")
		args = append(args, doctorID)
	}
	if status != "" {
		where = append(where, "change.status = ?")
		args = append(args, status)
	}
	var rows []response.ScheduleChangeRequest
	if err := r.db.WithContext(ctx).Raw(`
		SELECT change.id, change.affiliation_id, affiliation.hospital_id, hospital.name AS hospital_name,
		       affiliation.doctor_id, TRIM(CONCAT_WS(' ', doctor.first_name, doctor.last_name)) AS doctor_name,
		       change.requested_by, change.requested_by_party, change.status, change.reason,
		       change.reviewed_by, change.reviewed_at, change.rejection_reason,
		       change.expires_at, change.created_at, change.updated_at
		FROM doctor_schedule_change_requests change
		JOIN doctor_hospital_affiliations affiliation ON affiliation.id = change.affiliation_id
		JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
		JOIN users doctor ON doctor.id = affiliation.doctor_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY change.created_at DESC LIMIT 100`, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		if err := r.db.WithContext(ctx).Raw(`
			SELECT id, day_of_week, TO_CHAR(start_time, 'HH24:MI') AS start_time,
			       TO_CHAR(end_time, 'HH24:MI') AS end_time, timezone, booking_mode,
			       slot_duration_minutes, capacity
			FROM doctor_schedule_change_items WHERE change_request_id = ?
			ORDER BY day_of_week, start_time`, rows[i].ID).Scan(&rows[i].Schedules).Error; err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (r *Repository) ReviewScheduleChange(ctx context.Context, changeID, actorID, actorParty, decision string, reason *string, now time.Time) error {
	expired := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct {
			Status           string
			AffiliationID    string
			DoctorID         string
			RequestedByParty string
			ExpiresAt        time.Time
		}
		if err := tx.Raw(`
			SELECT change.status, change.affiliation_id, affiliation.doctor_id,
			       change.requested_by_party, change.expires_at
			FROM doctor_schedule_change_requests change
			JOIN doctor_hospital_affiliations affiliation ON affiliation.id = change.affiliation_id
			WHERE change.id = ? FOR UPDATE OF change`, changeID).Scan(&current).Error; err != nil {
			return err
		}
		if current.Status == "" {
			return ErrScheduleChangeNotFound
		}
		if current.Status != entity.ScheduleChangePending {
			return ErrInvalidScheduleChangeState
		}
		if !current.ExpiresAt.After(now) {
			if err := tx.Exec(`UPDATE doctor_schedule_change_requests SET status = 'EXPIRED', updated_at = ? WHERE id = ?`, now, changeID).Error; err != nil {
				return err
			}
			if err := insertScheduleChangeEvent(tx, changeID, actorID, "EXPIRED", now); err != nil {
				return err
			}
			expired = true
			return nil
		}
		if current.RequestedByParty == actorParty {
			return ErrScheduleChangeOwnApproval
		}
		if decision == entity.ScheduleChangeRejected {
			if reason == nil || strings.TrimSpace(*reason) == "" {
				return ErrInvalidScheduleChangeState
			}
			if err := tx.Exec(`
				UPDATE doctor_schedule_change_requests
				SET status = 'REJECTED', reviewed_by = ?, reviewed_at = ?, rejection_reason = ?, updated_at = ?
				WHERE id = ?`, actorID, now, reason, now, changeID).Error; err != nil {
				return err
			}
			if err := insertScheduleChangeEvent(tx, changeID, actorID, "REJECTED", now); err != nil {
				return err
			}
			return notifyScheduleCounterpart(tx, current.AffiliationID, actorParty, "SCHEDULE_CHANGE_REJECTED", "Perubahan jadwal ditolak", "Pengajuan perubahan jadwal telah ditolak.", changeID, now)
		}

		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "appointment:affiliation:"+current.AffiliationID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(CAST(? AS text), 0))`, current.DoctorID).Error; err != nil {
			return err
		}
		var activeAppointments bool
		if err := tx.Raw(`
			SELECT EXISTS(
				SELECT 1 FROM appointments
				WHERE affiliation_id = ? AND scheduled_start_at >= ?
				  AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')
			)`, current.AffiliationID, now).Scan(&activeAppointments).Error; err != nil {
			return err
		}
		if activeAppointments {
			return ErrScheduleChangeAppointments
		}
		var scheduleConflict bool
		if err := tx.Raw(`
			SELECT EXISTS(
				SELECT 1
				FROM doctor_schedule_change_items proposed
				JOIN doctor_hospital_affiliations other_affiliation
				  ON other_affiliation.doctor_id = ?
				 AND other_affiliation.id <> ?
				 AND other_affiliation.status = 'ACTIVE'
				JOIN doctor_hospital_schedules existing
				  ON existing.affiliation_id = other_affiliation.id
				 AND existing.is_active = TRUE
				WHERE proposed.change_request_id = ?
				  AND proposed.day_of_week = existing.day_of_week
				  AND proposed.start_time < existing.end_time
				  AND proposed.end_time > existing.start_time
			)`, current.DoctorID, current.AffiliationID, changeID).Scan(&scheduleConflict).Error; err != nil {
			return err
		}
		if scheduleConflict {
			return ErrDoctorScheduleConflict
		}
		if err := tx.Exec(`UPDATE doctor_hospital_schedules SET is_active = FALSE, updated_at = ? WHERE affiliation_id = ? AND is_active = TRUE`, now, current.AffiliationID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO doctor_hospital_schedules (
				id, affiliation_id, day_of_week, start_time, end_time, timezone,
				booking_mode, slot_duration_minutes, capacity, is_active, created_at, updated_at
			)
			SELECT gen_random_uuid(), ?, day_of_week, start_time, end_time, timezone,
			       booking_mode, slot_duration_minutes, capacity, TRUE, ?, ?
			FROM doctor_schedule_change_items WHERE change_request_id = ?
			ON CONFLICT (affiliation_id, day_of_week, start_time, end_time)
			DO UPDATE SET timezone = EXCLUDED.timezone, booking_mode = EXCLUDED.booking_mode,
			              slot_duration_minutes = EXCLUDED.slot_duration_minutes,
			              capacity = EXCLUDED.capacity, is_active = TRUE, updated_at = EXCLUDED.updated_at`,
			current.AffiliationID, now, now, changeID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			UPDATE doctor_schedule_change_requests
			SET status = 'APPROVED', reviewed_by = ?, reviewed_at = ?, updated_at = ? WHERE id = ?`,
			actorID, now, now, changeID).Error; err != nil {
			return err
		}
		if err := insertScheduleChangeEvent(tx, changeID, actorID, "APPROVED", now); err != nil {
			return err
		}
		return notifyScheduleCounterpart(tx, current.AffiliationID, actorParty, "SCHEDULE_CHANGE_APPROVED", "Perubahan jadwal disetujui", "Jadwal baru telah aktif.", changeID, now)
	})
	if err != nil {
		return err
	}
	if expired {
		return ErrInvalidScheduleChangeState
	}
	return nil
}

func insertAppointmentEvent(tx *gorm.DB, appointmentID string, actorID any, eventType string, from *string, to string, reason *string, now time.Time) error {
	return tx.Exec(`
		INSERT INTO appointment_status_events (
			id, appointment_id, actor_id, event_type, from_status, to_status, reason, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), appointmentID, actorID, eventType, from, to, reason, now).Error
}

func insertScheduleChangeEvent(tx *gorm.DB, changeID string, actorID any, eventType string, now time.Time) error {
	return tx.Exec(`
		INSERT INTO doctor_schedule_change_events (id, change_request_id, actor_id, event_type, created_at)
		VALUES (?, ?, ?, ?, ?)`, uuid.NewString(), changeID, actorID, eventType, now).Error
}

func insertNotification(tx *gorm.DB, userID, kind, title, body string, data []byte, now time.Time) error {
	return tx.Exec(`
		INSERT INTO notifications (id, user_id, type, title, body, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?::jsonb, ?)`, uuid.NewString(), userID, kind, title, body, string(data), now).Error
}

func notifyAppointmentParticipants(tx *gorm.DB, appointmentID, kind, title, body string, now time.Time) error {
	var row struct {
		PatientID         string
		DoctorID          string
		AppointmentNumber string
	}
	if err := tx.Raw(`SELECT patient_id, doctor_id, appointment_number FROM appointments WHERE id = ?`, appointmentID).Scan(&row).Error; err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"appointment_id": appointmentID, "appointment_number": row.AppointmentNumber, "event": kind})
	if err := insertNotification(tx, row.PatientID, kind, title, body, data, now); err != nil {
		return err
	}
	return insertNotification(tx, row.DoctorID, kind, title, body, data, now)
}

func notifyScheduleCounterpart(tx *gorm.DB, affiliationID, actorParty, kind, title, body, changeID string, now time.Time) error {
	var affiliation struct {
		HospitalID string
		DoctorID   string
	}
	if err := tx.Raw(`SELECT hospital_id, doctor_id FROM doctor_hospital_affiliations WHERE id = ?`, affiliationID).Scan(&affiliation).Error; err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"schedule_change_id": changeID, "affiliation_id": affiliationID, "event": kind})
	if actorParty == entity.ScheduleChangePartyHospital {
		return insertNotification(tx, affiliation.DoctorID, kind, title, body, data, now)
	}
	return tx.Exec(`
		INSERT INTO notifications (id, user_id, type, title, body, data, created_at)
		SELECT gen_random_uuid(), hur.user_id, ?, ?, ?, ?::jsonb, ?
		FROM hospital_user_roles hur
		JOIN roles role ON role.id = hur.role_id AND UPPER(role.slug) = 'ADMIN'
		JOIN user_hospitals membership ON membership.user_id = hur.user_id AND membership.hospital_id = hur.hospital_id
		WHERE hur.hospital_id = ? AND membership.is_active = TRUE AND membership.deleted_at IS NULL`,
		kind, title, body, string(data), now, affiliation.HospitalID).Error
}

func isActiveAppointmentStatus(status string) bool {
	switch status {
	case entity.AppointmentConfirmed, entity.AppointmentCheckedIn, entity.AppointmentWaitingVitals,
		entity.AppointmentWaitingDoctor, entity.AppointmentInConsultation:
		return true
	default:
		return false
	}
}
