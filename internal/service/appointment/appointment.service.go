package appointment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/email"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

const (
	DefaultTimezone            = "Asia/Jakarta"
	DefaultBookingMode         = entity.BookingModeFixedSlot
	DefaultSlotDurationMinutes = 30
	DefaultCapacity            = 1
	MaximumScheduleEntries     = 50
	MaximumAvailabilityDays    = 31
	BookingHorizon             = 90 * 24 * time.Hour
	MinimumBookingLeadTime     = 2 * time.Hour
	PatientChangeCutoff        = 2 * time.Hour
	CheckInEarlyWindow         = 30 * time.Minute
	CheckInLateWindow          = 15 * time.Minute
	ScheduleChangeTTL          = 7 * 24 * time.Hour
	reminderPollInterval       = time.Minute
)

type Repository interface {
	ListActiveSchedules(context.Context, repository.AvailabilityFilter) ([]repository.Schedule, error)
	GetActiveSchedule(context.Context, string) (*repository.Schedule, error)
	ReservedCounts(context.Context, string, string, repository.AvailabilityFilter) ([]repository.ReservedCount, error)
	Book(context.Context, repository.BookInput) (*response.Appointment, bool, error)
	GetAppointment(context.Context, string) (*response.Appointment, error)
	GetAppointmentByNumber(context.Context, string, string) (*response.Appointment, error)
	ListAppointments(context.Context, repository.AppointmentFilter) ([]response.Appointment, error)
	Cancel(context.Context, string, string, string, time.Time) error
	Reschedule(context.Context, string, string, string, repository.BookInput) (*response.Appointment, bool, error)
	CheckIn(context.Context, string, string, *string, time.Time) error
	Transition(context.Context, string, string, string, *string, time.Time) error
	MarkNoShows(context.Context, time.Time, time.Time) (int64, error)
	ClaimDueReminders(context.Context, time.Time, int) ([]response.AppointmentReminder, error)
	CompleteReminder(context.Context, response.AppointmentReminder, time.Time) error
	FailReminder(context.Context, string, string, time.Time) error
	ExpireScheduleChanges(context.Context, time.Time) (int64, error)
	GetAffiliation(context.Context, string) (*repository.Schedule, error)
	CreateScheduleChange(context.Context, repository.ScheduleChangeInput) (*response.ScheduleChangeRequest, error)
	GetScheduleChange(context.Context, string) (*response.ScheduleChangeRequest, error)
	ListScheduleChanges(context.Context, string, string, string) ([]response.ScheduleChangeRequest, error)
	ReviewScheduleChange(context.Context, string, string, string, string, *string, time.Time) error
}

type Service struct {
	repo   Repository
	email  email.Sender
	secret []byte
	now    func() time.Time
}

func NewService(repo Repository, sender email.Sender, secret string) *Service {
	return &Service{
		repo: repo, email: sender, secret: []byte(secret),
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) ListAvailability(ctx context.Context, hospitalID, doctorID, fromRaw, toRaw string) ([]response.DoctorScheduleAvailability, error) {
	if hospitalID == "" && doctorID == "" {
		return nil, constant.NewFieldRequiredError("hospital_id or doctor_id")
	}
	for _, id := range []string{hospitalID, doctorID} {
		if id != "" {
			if _, err := uuid.Parse(id); err != nil {
				return nil, constant.ErrInvalidUUIDFormat
			}
		}
	}
	now := s.now()
	from, to, err := normalizeDateRange(fromRaw, toRaw, now)
	if err != nil {
		return nil, err
	}
	filter := repository.AvailabilityFilter{HospitalID: hospitalID, DoctorID: doctorID}
	schedules, err := s.repo.ListActiveSchedules(ctx, filter)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	counts, err := s.repo.ReservedCounts(ctx, from.Format("2006-01-02"), to.Format("2006-01-02"), filter)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	reserved := make(map[string]int, len(counts))
	for _, count := range counts {
		reserved[availabilityKey(count.ScheduleID, count.AppointmentDate, count.ScheduledStart)] = count.Reserved
	}
	result := make([]response.DoctorScheduleAvailability, 0)
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		for _, schedule := range schedules {
			if int(date.Weekday()) != schedule.DayOfWeek {
				continue
			}
			sessionStart, sessionEnd, parseErr := scheduleWindow(schedule, date)
			if parseErr != nil {
				return nil, constant.ErrInternalServerError
			}
			row := response.DoctorScheduleAvailability{
				ScheduleID: schedule.ID, AffiliationID: schedule.AffiliationID,
				HospitalID: schedule.HospitalID, HospitalName: schedule.HospitalName,
				DoctorID: schedule.DoctorID, DoctorName: schedule.DoctorName,
				DepartmentID: schedule.DepartmentID, DepartmentName: schedule.DepartmentName,
				RoomID: schedule.RoomID, RoomName: schedule.RoomName,
				Date: date.Format("2006-01-02"), Timezone: schedule.Timezone,
				BookingMode: schedule.BookingMode, SlotDurationMinutes: schedule.SlotDurationMinutes,
				Capacity: schedule.Capacity, SessionStartAt: sessionStart, SessionEndAt: sessionEnd,
				Slots: []response.AvailabilitySlot{},
			}
			if schedule.BookingMode == entity.BookingModeSessionQueue {
				used := reserved[availabilityKey(schedule.ID, row.Date, sessionStart)]
				row.AvailableCapacity = maxInt(schedule.Capacity-used, 0)
				if sessionStart.After(now.Add(MinimumBookingLeadTime)) && row.AvailableCapacity > 0 {
					row.Slots = append(row.Slots, response.AvailabilitySlot{StartAt: sessionStart, EndAt: sessionEnd, AvailableCapacity: row.AvailableCapacity, Capacity: schedule.Capacity})
				}
			} else {
				for start := sessionStart; !start.Add(time.Duration(schedule.SlotDurationMinutes) * time.Minute).After(sessionEnd); start = start.Add(time.Duration(schedule.SlotDurationMinutes) * time.Minute) {
					end := start.Add(time.Duration(schedule.SlotDurationMinutes) * time.Minute)
					used := reserved[availabilityKey(schedule.ID, row.Date, start)]
					available := maxInt(schedule.Capacity-used, 0)
					row.AvailableCapacity += available
					if start.After(now.Add(MinimumBookingLeadTime)) && available > 0 {
						row.Slots = append(row.Slots, response.AvailabilitySlot{StartAt: start, EndAt: end, AvailableCapacity: available, Capacity: schedule.Capacity})
					}
				}
			}
			result = append(result, row)
		}
	}
	return result, nil
}

func (s *Service) CreateAppointment(ctx context.Context, patientID, idempotencyKey, clientIP, userAgent string, req request.CreateAppointmentRequest) (*response.Appointment, bool, error) {
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return nil, false, err
	}
	if !req.ConsentAccepted {
		return nil, false, constant.ErrAppointmentConsentRequired
	}
	input, err := s.prepareBookInput(ctx, patientID, idempotencyKey, clientIP, userAgent, req.ScheduleID,
		req.AppointmentDate, req.StartTime, req.ReasonForVisit, req.Note, req.ConsentVersion)
	if err != nil {
		return nil, false, err
	}
	created, replay, err := s.repo.Book(ctx, input)
	if err != nil {
		return nil, false, mapRepositoryError(err)
	}
	created.VerificationCode = s.verificationCode(created.ID, created.AppointmentNumber)
	return created, replay, nil
}

func (s *Service) GetPatientAppointment(ctx context.Context, patientID, appointmentID string) (*response.Appointment, error) {
	row, err := s.getAppointment(ctx, appointmentID)
	if err != nil {
		return nil, err
	}
	if row.PatientID != patientID {
		return nil, constant.ErrAppointmentNotFound
	}
	row.VerificationCode = s.verificationCode(row.ID, row.AppointmentNumber)
	return row, nil
}

func (s *Service) GetDoctorAppointment(ctx context.Context, doctorID, appointmentID string) (*response.Appointment, error) {
	row, err := s.getAppointment(ctx, appointmentID)
	if err != nil {
		return nil, err
	}
	if row.DoctorID != doctorID {
		return nil, constant.ErrAppointmentNotFound
	}
	return row, nil
}

func (s *Service) GetHospitalAppointment(ctx context.Context, hospitalID, appointmentID string) (*response.Appointment, error) {
	row, err := s.getAppointment(ctx, appointmentID)
	if err != nil {
		return nil, err
	}
	if row.HospitalID != hospitalID {
		return nil, constant.ErrAppointmentNotFound
	}
	redactClinicalFields(row)
	return row, nil
}

func (s *Service) ListPatientAppointments(ctx context.Context, patientID, status string) ([]response.Appointment, error) {
	status, err := normalizeAppointmentStatus(status)
	if err != nil {
		return nil, err
	}
	return s.listAppointments(ctx, repository.AppointmentFilter{PatientID: patientID, Status: status})
}

func (s *Service) ListDoctorAppointments(ctx context.Context, doctorID, status, date string) ([]response.Appointment, error) {
	status, err := normalizeAppointmentStatus(status)
	if err != nil {
		return nil, err
	}
	if err := validateOptionalDate(date); err != nil {
		return nil, err
	}
	return s.listAppointments(ctx, repository.AppointmentFilter{DoctorID: doctorID, Status: status, Date: date})
}

func (s *Service) ListHospitalAppointments(ctx context.Context, hospitalID, status, date string) ([]response.Appointment, error) {
	status, err := normalizeAppointmentStatus(status)
	if err != nil {
		return nil, err
	}
	if err := validateOptionalDate(date); err != nil {
		return nil, err
	}
	rows, err := s.listAppointments(ctx, repository.AppointmentFilter{HospitalID: hospitalID, Status: status, Date: date})
	if err != nil {
		return nil, err
	}
	for i := range rows {
		redactClinicalFields(&rows[i])
	}
	return rows, nil
}

func (s *Service) ListHospitalQueue(ctx context.Context, hospitalID, date string) ([]response.Appointment, error) {
	if date == "" {
		date = s.now().Format("2006-01-02")
	}
	if err := validateOptionalDate(date); err != nil {
		return nil, err
	}
	rows, err := s.listAppointments(ctx, repository.AppointmentFilter{HospitalID: hospitalID, Date: date, Limit: 200})
	if err != nil {
		return nil, err
	}
	queue := make([]response.Appointment, 0, len(rows))
	for _, row := range rows {
		if row.QueueActive && (row.Status == entity.AppointmentWaitingVitals || row.Status == entity.AppointmentWaitingDoctor || row.Status == entity.AppointmentInConsultation) {
			redactClinicalFields(&row)
			queue = append(queue, row)
		}
	}
	sort.SliceStable(queue, func(i, j int) bool {
		if queue[i].CreatedAt.Equal(queue[j].CreatedAt) {
			return queue[i].AppointmentNumber < queue[j].AppointmentNumber
		}
		return queue[i].CreatedAt.Before(queue[j].CreatedAt)
	})
	return queue, nil
}

func (s *Service) CancelPatientAppointment(ctx context.Context, patientID, appointmentID string, req request.CancelAppointmentRequest) error {
	row, err := s.GetPatientAppointment(ctx, patientID, appointmentID)
	if err != nil {
		return err
	}
	if !s.now().Before(row.ScheduledStartAt.Add(-PatientChangeCutoff)) {
		return constant.ErrAppointmentCutoffPassed
	}
	return mapRepositoryError(s.repo.Cancel(ctx, row.ID, patientID, strings.TrimSpace(req.Reason), s.now()))
}

func (s *Service) CancelHospitalAppointment(ctx context.Context, hospitalID, actorID, appointmentID string, req request.CancelAppointmentRequest) error {
	row, err := s.getAppointment(ctx, appointmentID)
	if err != nil {
		return err
	}
	if row.HospitalID != hospitalID {
		return constant.ErrAppointmentNotFound
	}
	return mapRepositoryError(s.repo.Cancel(ctx, row.ID, actorID, strings.TrimSpace(req.Reason), s.now()))
}

func (s *Service) ReschedulePatientAppointment(ctx context.Context, patientID, appointmentID, idempotencyKey, clientIP, userAgent string, req request.RescheduleAppointmentRequest) (*response.Appointment, bool, error) {
	row, err := s.GetPatientAppointment(ctx, patientID, appointmentID)
	if err != nil {
		return nil, false, err
	}
	if !s.now().Before(row.ScheduledStartAt.Add(-PatientChangeCutoff)) {
		return nil, false, constant.ErrAppointmentCutoffPassed
	}
	return s.reschedule(ctx, row, patientID, idempotencyKey, clientIP, userAgent, req)
}

func (s *Service) RescheduleHospitalAppointment(ctx context.Context, hospitalID, actorID, appointmentID, idempotencyKey, clientIP, userAgent string, req request.RescheduleAppointmentRequest) (*response.Appointment, bool, error) {
	row, err := s.getAppointment(ctx, appointmentID)
	if err != nil {
		return nil, false, err
	}
	if row.HospitalID != hospitalID {
		return nil, false, constant.ErrAppointmentNotFound
	}
	result, replay, err := s.reschedule(ctx, row, actorID, idempotencyKey, clientIP, userAgent, req)
	if result != nil {
		redactClinicalFields(result)
	}
	return result, replay, err
}

func (s *Service) reschedule(ctx context.Context, old *response.Appointment, actorID, idempotencyKey, clientIP, userAgent string, req request.RescheduleAppointmentRequest) (*response.Appointment, bool, error) {
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return nil, false, err
	}
	input, err := s.prepareBookInput(ctx, old.PatientID, idempotencyKey, clientIP, userAgent, req.ScheduleID,
		req.AppointmentDate, req.StartTime, old.ReasonForVisit, old.Note, old.ConsentVersion)
	if err != nil {
		return nil, false, err
	}
	rescheduleHash := sha256.Sum256([]byte(input.IdempotencyRequestHash + "|reschedule:" + old.ID))
	input.IdempotencyRequestHash = hex.EncodeToString(rescheduleHash[:])
	row, replay, err := s.repo.Reschedule(ctx, old.ID, actorID, strings.TrimSpace(req.Reason), input)
	if err != nil {
		return nil, false, mapRepositoryError(err)
	}
	row.VerificationCode = s.verificationCode(row.ID, row.AppointmentNumber)
	return row, replay, nil
}

func (s *Service) CheckIn(ctx context.Context, hospitalID, actorID string, req request.VerifyAppointmentRequest) (*response.Appointment, error) {
	row, err := s.repo.GetAppointmentByNumber(ctx, hospitalID, strings.TrimSpace(req.AppointmentNumber))
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	expected := s.verificationCode(row.ID, row.AppointmentNumber)
	provided := strings.ToUpper(strings.TrimSpace(req.VerificationCode))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		return nil, constant.ErrAppointmentInvalidVerification
	}
	now := s.now()
	if now.Before(row.ScheduledStartAt.Add(-CheckInEarlyWindow)) {
		return nil, constant.ErrAppointmentOutsideCheckInWindow
	}
	if now.After(row.ScheduledStartAt.Add(CheckInLateWindow)) {
		if !req.ForceLateCheckIn || req.OverrideReason == nil || strings.TrimSpace(*req.OverrideReason) == "" {
			return nil, constant.ErrAppointmentOutsideCheckInWindow
		}
	}
	overrideReason := cleanOptional(req.OverrideReason)
	if err := s.repo.CheckIn(ctx, row.ID, actorID, overrideReason, now); err != nil {
		return nil, mapRepositoryError(err)
	}
	return s.GetHospitalAppointment(ctx, hospitalID, row.ID)
}

func (s *Service) CompleteVitals(ctx context.Context, hospitalID, actorID, appointmentID string, reason *string) error {
	if _, err := s.GetHospitalAppointment(ctx, hospitalID, appointmentID); err != nil {
		return err
	}
	return mapRepositoryError(s.repo.Transition(ctx, appointmentID, actorID, entity.AppointmentWaitingDoctor, cleanOptional(reason), s.now()))
}

func (s *Service) StartConsultation(ctx context.Context, doctorID, appointmentID string, reason *string) error {
	if _, err := s.GetDoctorAppointment(ctx, doctorID, appointmentID); err != nil {
		return err
	}
	return mapRepositoryError(s.repo.Transition(ctx, appointmentID, doctorID, entity.AppointmentInConsultation, cleanOptional(reason), s.now()))
}

func (s *Service) CompleteAppointment(ctx context.Context, doctorID, appointmentID string, reason *string) error {
	if _, err := s.GetDoctorAppointment(ctx, doctorID, appointmentID); err != nil {
		return err
	}
	return mapRepositoryError(s.repo.Transition(ctx, appointmentID, doctorID, entity.AppointmentCompleted, cleanOptional(reason), s.now()))
}

func (s *Service) CreateDoctorScheduleChange(ctx context.Context, doctorID string, req request.CreateScheduleChangeRequest) (*response.ScheduleChangeRequest, error) {
	return s.createScheduleChange(ctx, doctorID, entity.ScheduleChangePartyDoctor, "", req)
}

func (s *Service) CreateHospitalScheduleChange(ctx context.Context, hospitalID, actorID string, req request.CreateScheduleChangeRequest) (*response.ScheduleChangeRequest, error) {
	return s.createScheduleChange(ctx, actorID, entity.ScheduleChangePartyHospital, hospitalID, req)
}

func (s *Service) createScheduleChange(ctx context.Context, actorID, party, hospitalID string, req request.CreateScheduleChangeRequest) (*response.ScheduleChangeRequest, error) {
	affiliation, err := s.repo.GetAffiliation(ctx, req.AffiliationID)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if (party == entity.ScheduleChangePartyDoctor && affiliation.DoctorID != actorID) ||
		(party == entity.ScheduleChangePartyHospital && affiliation.HospitalID != hospitalID) {
		return nil, constant.ErrAffiliationNotFound
	}
	schedules, err := normalizeSchedules(req.Schedules)
	if err != nil {
		return nil, err
	}
	now := s.now()
	row, err := s.repo.CreateScheduleChange(ctx, repository.ScheduleChangeInput{
		AffiliationID: req.AffiliationID, ActorID: actorID, ActorParty: party,
		HospitalID: hospitalID, Reason: cleanOptional(req.Reason), Schedules: schedules,
		Now: now, ExpiresAt: now.Add(ScheduleChangeTTL),
	})
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return row, nil
}

func (s *Service) ListDoctorScheduleChanges(ctx context.Context, doctorID, status string) ([]response.ScheduleChangeRequest, error) {
	status, err := normalizeScheduleChangeStatus(status)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListScheduleChanges(ctx, "", doctorID, status)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return rows, nil
}

func (s *Service) ListHospitalScheduleChanges(ctx context.Context, hospitalID, status string) ([]response.ScheduleChangeRequest, error) {
	status, err := normalizeScheduleChangeStatus(status)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListScheduleChanges(ctx, hospitalID, "", status)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return rows, nil
}

func (s *Service) ReviewDoctorScheduleChange(ctx context.Context, doctorID, changeID, decision string, reason *string) error {
	row, err := s.repo.GetScheduleChange(ctx, changeID)
	if err != nil {
		return mapRepositoryError(err)
	}
	if row.DoctorID != doctorID {
		return constant.ErrScheduleChangeNotFound
	}
	return s.reviewScheduleChange(ctx, changeID, doctorID, entity.ScheduleChangePartyDoctor, decision, reason)
}

func (s *Service) ReviewHospitalScheduleChange(ctx context.Context, hospitalID, actorID, changeID, decision string, reason *string) error {
	row, err := s.repo.GetScheduleChange(ctx, changeID)
	if err != nil {
		return mapRepositoryError(err)
	}
	if row.HospitalID != hospitalID {
		return constant.ErrScheduleChangeNotFound
	}
	return s.reviewScheduleChange(ctx, changeID, actorID, entity.ScheduleChangePartyHospital, decision, reason)
}

func (s *Service) reviewScheduleChange(ctx context.Context, changeID, actorID, party, decision string, reason *string) error {
	decision = strings.ToUpper(strings.TrimSpace(decision))
	if decision != entity.ScheduleChangeApproved && decision != entity.ScheduleChangeRejected {
		return constant.NewInvalidFieldValueError("decision", "APPROVED or REJECTED", "APPROVED atau REJECTED")
	}
	if err := s.repo.ReviewScheduleChange(ctx, changeID, actorID, party, decision, cleanOptional(reason), s.now()); err != nil {
		return mapRepositoryError(err)
	}
	return nil
}

func (s *Service) RunBackgroundWorker(ctx context.Context) {
	s.processBackground(ctx)
	ticker := time.NewTicker(reminderPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processBackground(ctx)
		}
	}
}

func (s *Service) processBackground(ctx context.Context) {
	now := s.now()
	if _, err := s.repo.ExpireScheduleChanges(ctx, now); err != nil {
		util.Errorf(ctx, "doctor schedule change expiration failed error_type=%T", err)
	}
	if _, err := s.repo.MarkNoShows(ctx, now.Add(-CheckInLateWindow), now); err != nil {
		util.Errorf(ctx, "appointment no-show sweep failed error_type=%T", err)
	}
	reminders, err := s.repo.ClaimDueReminders(ctx, now, 20)
	if err != nil {
		util.Errorf(ctx, "appointment reminder claim failed error_type=%T", err)
		return
	}
	for _, reminder := range reminders {
		s.sendReminderEmails(ctx, reminder)
		if err := s.repo.CompleteReminder(ctx, reminder, s.now()); err != nil {
			_ = s.repo.FailReminder(ctx, reminder.ID, "notification persistence failed", s.now())
			util.Errorf(ctx, "appointment reminder completion failed reminder_id=%s error_type=%T", reminder.ID, err)
		}
	}
}

func (s *Service) sendReminderEmails(ctx context.Context, reminder response.AppointmentReminder) {
	if s.email == nil {
		return
	}
	when := reminder.ScheduledStartAt
	if location, err := time.LoadLocation(reminder.Timezone); err == nil {
		when = when.In(location)
	}
	for _, recipient := range []struct {
		email string
		name  string
		role  string
	}{{reminder.PatientEmail, reminder.PatientFirstName, "pasien"}, {reminder.DoctorEmail, reminder.DoctorFirstName, "dokter"}} {
		html := email.RenderAppointmentReminder(recipient.name, recipient.role, reminder.HospitalName, when)
		if err := s.email.SendWithContext(ctx, recipient.email, "Pengingat appointment MedikaOne", html); err != nil && !errors.Is(err, email.ErrDeliveryDisabled) {
			util.Errorf(ctx, "appointment reminder email failed reminder_id=%s recipient_role=%s error_type=%T", reminder.ID, recipient.role, err)
		}
	}
}

func (s *Service) prepareBookInput(ctx context.Context, patientID, idempotencyKey, clientIP, userAgent, scheduleID, appointmentDate string, startTime *string, reason string, note *string, consentVersion string) (repository.BookInput, error) {
	if _, err := uuid.Parse(scheduleID); err != nil {
		return repository.BookInput{}, constant.ErrInvalidUUIDFormat
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(appointmentDate))
	if err != nil {
		return repository.BookInput{}, constant.ErrInvalidDateFormat
	}
	schedule, err := s.repo.GetActiveSchedule(ctx, scheduleID)
	if err != nil {
		return repository.BookInput{}, mapRepositoryError(err)
	}
	sessionStart, sessionEnd, err := scheduleWindow(*schedule, date)
	if err != nil || int(date.Weekday()) != schedule.DayOfWeek {
		return repository.BookInput{}, constant.ErrScheduleNotFound
	}
	start, end := sessionStart, sessionEnd
	if schedule.BookingMode == entity.BookingModeFixedSlot {
		if startTime == nil {
			return repository.BookInput{}, constant.NewFieldRequiredError("start_time")
		}
		parsed, parseErr := time.Parse("15:04", strings.TrimSpace(*startTime))
		if parseErr != nil {
			return repository.BookInput{}, constant.NewInvalidFieldValueError("start_time", "a valid HH:mm time", "berupa waktu HH:mm yang valid")
		}
		location, _ := time.LoadLocation(schedule.Timezone)
		start = time.Date(date.Year(), date.Month(), date.Day(), parsed.Hour(), parsed.Minute(), 0, 0, location).UTC()
		end = start.Add(time.Duration(schedule.SlotDurationMinutes) * time.Minute)
		if start.Before(sessionStart) || end.After(sessionEnd) || int(start.Sub(sessionStart).Minutes())%schedule.SlotDurationMinutes != 0 {
			return repository.BookInput{}, constant.ErrAppointmentSlotUnavailable
		}
	} else if startTime != nil && strings.TrimSpace(*startTime) != schedule.StartTime {
		return repository.BookInput{}, constant.NewInvalidFieldValueError("start_time", "empty or equal to the session start for SESSION_QUEUE", "kosong atau sama dengan awal sesi untuk SESSION_QUEUE")
	}
	now := s.now()
	if start.Before(now.Add(MinimumBookingLeadTime)) || start.After(now.Add(BookingHorizon)) {
		return repository.BookInput{}, constant.ErrAppointmentSlotUnavailable
	}
	reason = strings.TrimSpace(reason)
	consentVersion = strings.TrimSpace(consentVersion)
	note = cleanOptional(note)
	if reason == "" {
		return repository.BookInput{}, constant.NewFieldRequiredError("reason")
	}
	if len(reason) > 2000 {
		return repository.BookInput{}, constant.NewInvalidFieldLengthError("reason", "at most 2000 characters long", "memiliki maksimal 2000 karakter")
	}
	if consentVersion == "" {
		return repository.BookInput{}, constant.NewFieldRequiredError("consent_version")
	}
	if len(consentVersion) > 64 {
		return repository.BookInput{}, constant.NewInvalidFieldLengthError("consent_version", "at most 64 characters long", "memiliki maksimal 64 karakter")
	}
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	hashPayload, _ := json.Marshal(struct {
		PatientID, ScheduleID, AppointmentDate, StartAt, Reason, ConsentVersion string
		Note                                                                    *string
	}{patientID, scheduleID, appointmentDate, start.UTC().Format(time.RFC3339Nano), reason, consentVersion, note})
	hash := sha256.Sum256(hashPayload)
	return repository.BookInput{
		PatientID: patientID, Schedule: *schedule, AppointmentDate: date.Format("2006-01-02"),
		ScheduledStartAt: start, ScheduledEndAt: end, ReasonForVisit: reason, Note: note,
		ConsentVersion: consentVersion, ConsentIP: strings.TrimSpace(clientIP),
		ConsentUserAgent: strings.TrimSpace(userAgent), IdempotencyKey: idempotencyKey,
		IdempotencyRequestHash: hex.EncodeToString(hash[:]), Now: now,
	}, nil
}

func scheduleWindow(schedule repository.Schedule, date time.Time) (time.Time, time.Time, error) {
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	startClock, err := time.Parse("15:04", schedule.StartTime)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endClock, err := time.Parse("15:04", schedule.EndTime)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), startClock.Hour(), startClock.Minute(), 0, 0, location)
	end := time.Date(date.Year(), date.Month(), date.Day(), endClock.Hour(), endClock.Minute(), 0, 0, location)
	return start.UTC(), end.UTC(), nil
}

func normalizeDateRange(fromRaw, toRaw string, now time.Time) (time.Time, time.Time, error) {
	if strings.TrimSpace(fromRaw) == "" {
		fromRaw = now.Format("2006-01-02")
	}
	if strings.TrimSpace(toRaw) == "" {
		toRaw = fromRaw
	}
	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		return time.Time{}, time.Time{}, constant.ErrInvalidDateFormat
	}
	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		return time.Time{}, time.Time{}, constant.ErrInvalidDateFormat
	}
	if to.Before(from) || to.Sub(from) > (MaximumAvailabilityDays-1)*24*time.Hour || from.After(now.Add(BookingHorizon)) {
		return time.Time{}, time.Time{}, constant.NewInvalidFieldValueError("date_range", "ordered, no longer than 31 days, and within the 90-day booking horizon", "berurutan, maksimal 31 hari, dan berada dalam rentang pemesanan 90 hari")
	}
	return from, to, nil
}

func normalizeSchedules(input []request.DoctorInvitationScheduleRequest) ([]repository.ScheduleItem, error) {
	if len(input) == 0 || len(input) > MaximumScheduleEntries {
		return nil, constant.NewInvalidFieldLengthError("schedules", "between 1 and 50 items long", "memiliki 1 sampai 50 item")
	}
	type interval struct{ start, end int }
	byDay := map[int][]interval{}
	timezone := ""
	result := make([]repository.ScheduleItem, 0, len(input))
	for _, value := range input {
		if value.DayOfWeek < 0 || value.DayOfWeek > 6 {
			return nil, constant.NewInvalidFieldValueError("day_of_week", "an integer from 0 through 6", "berupa angka bulat dari 0 sampai 6")
		}
		start, err := time.Parse("15:04", strings.TrimSpace(value.StartTime))
		if err != nil {
			return nil, constant.NewInvalidFieldValueError("start_time", "a valid HH:mm time", "berupa waktu HH:mm yang valid")
		}
		end, err := time.Parse("15:04", strings.TrimSpace(value.EndTime))
		if err != nil || !end.After(start) {
			return nil, constant.NewInvalidFieldValueError("end_time", "a valid HH:mm time later than start_time", "berupa waktu HH:mm yang valid dan setelah start_time")
		}
		zone := strings.TrimSpace(value.Timezone)
		if zone == "" {
			zone = DefaultTimezone
		}
		if _, err := time.LoadLocation(zone); err != nil || (timezone != "" && timezone != zone) {
			return nil, constant.NewInvalidFieldValueError("timezone", "a valid and consistent IANA timezone", "berupa zona waktu IANA yang valid dan konsisten")
		}
		timezone = zone
		mode := strings.ToUpper(strings.TrimSpace(value.BookingMode))
		if mode == "" {
			mode = DefaultBookingMode
		}
		if mode != entity.BookingModeFixedSlot && mode != entity.BookingModeSessionQueue {
			return nil, constant.NewInvalidFieldValueError("booking_mode", "FIXED_SLOT or SESSION_QUEUE", "FIXED_SLOT atau SESSION_QUEUE")
		}
		duration := value.SlotDurationMins
		if duration == 0 {
			duration = DefaultSlotDurationMinutes
		}
		capacity := value.Capacity
		if capacity == 0 {
			capacity = DefaultCapacity
		}
		startMinute, endMinute := start.Hour()*60+start.Minute(), end.Hour()*60+end.Minute()
		if duration < 5 || duration > 240 {
			return nil, constant.NewInvalidFieldValueError("slot_duration_minutes", "an integer from 5 through 240", "berupa angka bulat dari 5 sampai 240")
		}
		if capacity < 1 || capacity > 500 {
			return nil, constant.NewInvalidFieldValueError("capacity", "an integer from 1 through 500", "berupa angka bulat dari 1 sampai 500")
		}
		if mode == entity.BookingModeFixedSlot && (endMinute-startMinute)%duration != 0 {
			return nil, constant.NewInvalidFieldValueError("slot_duration_minutes", "an exact divisor of the practice duration for FIXED_SLOT", "dapat membagi durasi praktik secara tepat untuk FIXED_SLOT")
		}
		for _, existing := range byDay[value.DayOfWeek] {
			if startMinute < existing.end && endMinute > existing.start {
				return nil, constant.ErrDoctorScheduleConflict
			}
		}
		byDay[value.DayOfWeek] = append(byDay[value.DayOfWeek], interval{startMinute, endMinute})
		result = append(result, repository.ScheduleItem{DayOfWeek: value.DayOfWeek, StartTime: start.Format("15:04"), EndTime: end.Format("15:04"), Timezone: zone, BookingMode: mode, SlotDurationMinutes: duration, Capacity: capacity})
	}
	return result, nil
}

func (s *Service) getAppointment(ctx context.Context, appointmentID string) (*response.Appointment, error) {
	if _, err := uuid.Parse(appointmentID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetAppointment(ctx, appointmentID)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return row, nil
}

func (s *Service) listAppointments(ctx context.Context, filter repository.AppointmentFilter) ([]response.Appointment, error) {
	rows, err := s.repo.ListAppointments(ctx, filter)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if rows == nil {
		rows = []response.Appointment{}
	}
	return rows, nil
}

func (s *Service) verificationCode(appointmentID, number string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("appointment-verification:" + appointmentID + ":" + number))
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(mac.Sum(nil)[:10])
}

func availabilityKey(scheduleID, date string, start time.Time) string {
	return scheduleID + "|" + date + "|" + start.UTC().Format(time.RFC3339)
}

func validateIdempotencyKey(value string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return constant.ErrIdempotencyKeyRequired
	}
	return nil
}

func validateOptionalDate(value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return constant.ErrInvalidDateFormat
	}
	return nil
}

func normalizeAppointmentStatus(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	switch value {
	case entity.AppointmentConfirmed, entity.AppointmentCheckedIn, entity.AppointmentWaitingVitals,
		entity.AppointmentWaitingDoctor, entity.AppointmentInConsultation, entity.AppointmentCompleted,
		entity.AppointmentCancelled, entity.AppointmentNoShow, entity.AppointmentRescheduled:
		return value, nil
	default:
		return "", constant.NewInvalidFieldValueError("status", "a supported appointment status", "berupa status appointment yang didukung")
	}
}

func normalizeScheduleChangeStatus(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	switch value {
	case entity.ScheduleChangePending, entity.ScheduleChangeApproved, entity.ScheduleChangeRejected,
		entity.ScheduleChangeCancelled, entity.ScheduleChangeExpired:
		return value, nil
	default:
		return "", constant.NewInvalidFieldValueError("status", "a supported schedule-change status", "berupa status perubahan jadwal yang didukung")
	}
}

func cleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}

func redactClinicalFields(row *response.Appointment) {
	row.ReasonForVisit = ""
	row.Note = nil
	row.ConsentVersion = ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mapRepositoryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrScheduleNotFound):
		return constant.ErrScheduleNotFound
	case errors.Is(err, repository.ErrAffiliationNotFound):
		return constant.ErrAffiliationNotFound
	case errors.Is(err, repository.ErrSlotUnavailable), errors.Is(err, repository.ErrPatientTimeConflict):
		return constant.ErrAppointmentSlotUnavailable
	case errors.Is(err, repository.ErrAppointmentNotFound):
		return constant.ErrAppointmentNotFound
	case errors.Is(err, repository.ErrInvalidAppointmentState):
		return constant.ErrAppointmentInvalidState
	case errors.Is(err, repository.ErrInvalidVerification):
		return constant.ErrAppointmentInvalidVerification
	case errors.Is(err, repository.ErrIdempotencyConflict):
		return constant.ErrIdempotencyConflict
	case errors.Is(err, repository.ErrScheduleChangeNotFound):
		return constant.ErrScheduleChangeNotFound
	case errors.Is(err, repository.ErrScheduleChangeExists):
		return constant.ErrScheduleChangeExists
	case errors.Is(err, repository.ErrInvalidScheduleChangeState):
		return constant.ErrInvalidScheduleChangeState
	case errors.Is(err, repository.ErrScheduleChangeOwnApproval):
		return constant.ErrScheduleChangeOwnApproval
	case errors.Is(err, repository.ErrScheduleChangeAppointments):
		return constant.ErrScheduleChangeHasAppointments
	case errors.Is(err, repository.ErrDoctorScheduleConflict):
		return constant.ErrDoctorScheduleConflict
	default:
		util.Errorf(context.Background(), "appointment repository error error_type=%T", err)
		return constant.ErrInternalServerError
	}
}
