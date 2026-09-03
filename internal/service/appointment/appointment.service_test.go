package appointment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
)

type fakeRepository struct {
	schedules        []repository.Schedule
	counts           []repository.ReservedCount
	appointment      *response.Appointment
	bookInput        repository.BookInput
	checkInCalled    bool
	checkInReason    *string
	transitionTarget string
}

func (f *fakeRepository) ListActiveSchedules(context.Context, repository.AvailabilityFilter) ([]repository.Schedule, error) {
	return f.schedules, nil
}
func (f *fakeRepository) GetActiveSchedule(_ context.Context, id string) (*repository.Schedule, error) {
	for i := range f.schedules {
		if f.schedules[i].ID == id {
			return &f.schedules[i], nil
		}
	}
	return nil, repository.ErrScheduleNotFound
}
func (f *fakeRepository) ReservedCounts(context.Context, string, string, repository.AvailabilityFilter) ([]repository.ReservedCount, error) {
	return f.counts, nil
}
func (f *fakeRepository) Book(_ context.Context, input repository.BookInput) (*response.Appointment, bool, error) {
	f.bookInput = input
	return f.appointment, false, nil
}
func (f *fakeRepository) GetAppointment(context.Context, string) (*response.Appointment, error) {
	if f.appointment == nil {
		return nil, repository.ErrAppointmentNotFound
	}
	copy := *f.appointment
	return &copy, nil
}
func (f *fakeRepository) GetAppointmentByNumber(context.Context, string, string) (*response.Appointment, error) {
	return f.GetAppointment(context.Background(), "")
}
func (f *fakeRepository) ListAppointments(context.Context, repository.AppointmentFilter) ([]response.Appointment, error) {
	if f.appointment == nil {
		return nil, nil
	}
	return []response.Appointment{*f.appointment}, nil
}
func (f *fakeRepository) Cancel(context.Context, string, string, string, time.Time) error { return nil }
func (f *fakeRepository) Reschedule(_ context.Context, _ string, _ string, _ string, input repository.BookInput) (*response.Appointment, bool, error) {
	f.bookInput = input
	return f.appointment, false, nil
}
func (f *fakeRepository) CheckIn(_ context.Context, _, _ string, reason *string, _ time.Time) error {
	f.checkInCalled = true
	f.checkInReason = reason
	return nil
}
func (f *fakeRepository) Transition(_ context.Context, _ string, _ string, target string, _ *string, _ time.Time) error {
	f.transitionTarget = target
	return nil
}
func (f *fakeRepository) MarkNoShows(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeRepository) ClaimDueReminders(context.Context, time.Time, int) ([]response.AppointmentReminder, error) {
	return nil, nil
}
func (f *fakeRepository) CompleteReminder(context.Context, response.AppointmentReminder, time.Time) error {
	return nil
}
func (f *fakeRepository) FailReminder(context.Context, string, string, time.Time) error { return nil }
func (f *fakeRepository) ExpireScheduleChanges(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeRepository) GetAffiliation(context.Context, string) (*repository.Schedule, error) {
	if len(f.schedules) == 0 {
		return nil, repository.ErrAffiliationNotFound
	}
	return &f.schedules[0], nil
}
func (f *fakeRepository) CreateScheduleChange(context.Context, repository.ScheduleChangeInput) (*response.ScheduleChangeRequest, error) {
	return &response.ScheduleChangeRequest{ID: uuid.NewString()}, nil
}
func (f *fakeRepository) GetScheduleChange(context.Context, string) (*response.ScheduleChangeRequest, error) {
	return nil, repository.ErrScheduleChangeNotFound
}
func (f *fakeRepository) ListScheduleChanges(context.Context, string, string, string) ([]response.ScheduleChangeRequest, error) {
	return nil, nil
}
func (f *fakeRepository) ReviewScheduleChange(context.Context, string, string, string, string, *string, time.Time) error {
	return nil
}

func TestNormalizeSchedulesSupportsBothBookingModes(t *testing.T) {
	items, err := normalizeSchedules([]request.DoctorInvitationScheduleRequest{
		{DayOfWeek: 1, StartTime: "08:00", EndTime: "10:00", BookingMode: "fixed_slot", SlotDurationMins: 30, Capacity: 2},
		{DayOfWeek: 2, StartTime: "13:00", EndTime: "16:00", BookingMode: "session_queue", Capacity: 20},
	})
	if err != nil {
		t.Fatalf("normalizeSchedules() error = %v", err)
	}
	if items[0].BookingMode != entity.BookingModeFixedSlot || items[0].SlotDurationMinutes != 30 || items[0].Capacity != 2 {
		t.Fatalf("fixed-slot normalization = %+v", items[0])
	}
	if items[1].BookingMode != entity.BookingModeSessionQueue || items[1].Capacity != 20 {
		t.Fatalf("queue normalization = %+v", items[1])
	}

	_, err = normalizeSchedules([]request.DoctorInvitationScheduleRequest{{DayOfWeek: 1, StartTime: "08:00", EndTime: "09:10", BookingMode: "FIXED_SLOT", SlotDurationMins: 30}})
	want := constant.NewInvalidFieldValueError("slot_duration_minutes", "an exact divisor of the practice duration for FIXED_SLOT", "dapat membagi durasi praktik secara tepat untuk FIXED_SLOT")
	if !errors.Is(err, want) {
		t.Fatalf("misaligned fixed schedule error = %v", err)
	}
	_, err = normalizeSchedules([]request.DoctorInvitationScheduleRequest{
		{DayOfWeek: 1, StartTime: "08:00", EndTime: "10:00"},
		{DayOfWeek: 1, StartTime: "09:00", EndTime: "11:00"},
	})
	if !errors.Is(err, constant.ErrDoctorScheduleConflict) {
		t.Fatalf("overlapping schedule error = %v", err)
	}
}

func TestListAvailabilityBuildsFixedAndQueueCapacity(t *testing.T) {
	fixedNow := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	hospitalID, doctorID := uuid.NewString(), uuid.NewString()
	fixed := repository.Schedule{ID: uuid.NewString(), AffiliationID: uuid.NewString(), HospitalID: hospitalID, HospitalName: "RS Test", HospitalCode: "RST", DoctorID: doctorID, DoctorName: "Doctor Test", DepartmentID: uuid.NewString(), DepartmentName: "Umum", DayOfWeek: 1, StartTime: "08:00", EndTime: "09:00", Timezone: "Asia/Jakarta", BookingMode: entity.BookingModeFixedSlot, SlotDurationMinutes: 30, Capacity: 2}
	queue := fixed
	queue.ID = uuid.NewString()
	queue.StartTime, queue.EndTime = "13:00", "16:00"
	queue.BookingMode, queue.Capacity = entity.BookingModeSessionQueue, 5
	fixedStart := time.Date(2026, 9, 7, 8, 0, 0, 0, time.FixedZone("WIB", 7*60*60)).UTC()
	queueStart := time.Date(2026, 9, 7, 13, 0, 0, 0, time.FixedZone("WIB", 7*60*60)).UTC()
	repo := &fakeRepository{schedules: []repository.Schedule{fixed, queue}, counts: []repository.ReservedCount{
		{ScheduleID: fixed.ID, AppointmentDate: "2026-09-07", ScheduledStart: fixedStart, Reserved: 1},
		{ScheduleID: queue.ID, AppointmentDate: "2026-09-07", ScheduledStart: queueStart, Reserved: 3},
	}}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return fixedNow }

	rows, err := service.ListAvailability(context.Background(), hospitalID, doctorID, "2026-09-07", "2026-09-07")
	if err != nil {
		t.Fatalf("ListAvailability() error = %v", err)
	}
	if len(rows) != 2 || len(rows[0].Slots) != 2 || rows[0].Slots[0].AvailableCapacity != 1 {
		t.Fatalf("fixed availability = %+v", rows)
	}
	if len(rows[1].Slots) != 1 || rows[1].AvailableCapacity != 2 {
		t.Fatalf("queue availability = %+v", rows[1])
	}
}

func TestCreateAppointmentRequiresConsentAndBuildsSecureResult(t *testing.T) {
	fixedNow := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	schedule := repository.Schedule{ID: uuid.NewString(), AffiliationID: uuid.NewString(), HospitalID: uuid.NewString(), HospitalCode: "HSP-MO-001", DoctorID: uuid.NewString(), DepartmentID: uuid.NewString(), DayOfWeek: 1, StartTime: "08:00", EndTime: "10:00", Timezone: "Asia/Jakarta", BookingMode: entity.BookingModeFixedSlot, SlotDurationMinutes: 30, Capacity: 1}
	created := &response.Appointment{ID: uuid.NewString(), AppointmentNumber: "APT-HSPMO001-20260907-0001"}
	repo := &fakeRepository{schedules: []repository.Schedule{schedule}, appointment: created}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return fixedNow }
	start := "08:30"
	req := request.CreateAppointmentRequest{ScheduleID: schedule.ID, AppointmentDate: "2026-09-07", StartTime: &start, ReasonForVisit: "Kontrol", ConsentVersion: "2026-09", ConsentAccepted: false}
	if _, _, err := service.CreateAppointment(context.Background(), uuid.NewString(), uuid.NewString(), "127.0.0.1", "test", req); !errors.Is(err, constant.ErrAppointmentConsentRequired) {
		t.Fatalf("missing consent error = %v", err)
	}
	req.ConsentAccepted = true
	result, _, err := service.CreateAppointment(context.Background(), uuid.NewString(), uuid.NewString(), "127.0.0.1", "test", req)
	if err != nil {
		t.Fatalf("CreateAppointment() error = %v", err)
	}
	wantStart := time.Date(2026, 9, 7, 1, 30, 0, 0, time.UTC)
	if !repo.bookInput.ScheduledStartAt.Equal(wantStart) || repo.bookInput.ScheduledEndAt.Sub(repo.bookInput.ScheduledStartAt) != 30*time.Minute {
		t.Fatalf("booked interval = %s - %s", repo.bookInput.ScheduledStartAt, repo.bookInput.ScheduledEndAt)
	}
	if len(result.VerificationCode) < 12 || result.VerificationCode != service.verificationCode(result.ID, result.AppointmentNumber) {
		t.Fatalf("verification code was not derived securely: %q", result.VerificationCode)
	}
}

func TestCheckInUsesOneTimeCodeAndWindow(t *testing.T) {
	fixedNow := time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)
	appointment := &response.Appointment{ID: uuid.NewString(), AppointmentNumber: "APT-TEST-20260907-0001", HospitalID: uuid.NewString(), ScheduledStartAt: fixedNow.Add(20 * time.Minute)}
	repo := &fakeRepository{appointment: appointment}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return fixedNow }

	_, err := service.CheckIn(context.Background(), appointment.HospitalID, uuid.NewString(), request.VerifyAppointmentRequest{AppointmentNumber: appointment.AppointmentNumber, VerificationCode: "WRONG"})
	if !errors.Is(err, constant.ErrAppointmentInvalidVerification) || repo.checkInCalled {
		t.Fatalf("invalid code error=%v called=%v", err, repo.checkInCalled)
	}
	code := service.verificationCode(appointment.ID, appointment.AppointmentNumber)
	if _, err := service.CheckIn(context.Background(), appointment.HospitalID, uuid.NewString(), request.VerifyAppointmentRequest{AppointmentNumber: appointment.AppointmentNumber, VerificationCode: code}); err != nil {
		t.Fatalf("CheckIn() error = %v", err)
	}
	if !repo.checkInCalled {
		t.Fatal("repository CheckIn was not called")
	}
}

func TestLateCheckInRequiresAndAuditsOverrideReason(t *testing.T) {
	fixedNow := time.Date(2026, 9, 7, 2, 0, 0, 0, time.UTC)
	appointment := &response.Appointment{
		ID: uuid.NewString(), AppointmentNumber: "APT-TEST-20260907-0002",
		HospitalID: uuid.NewString(), ScheduledStartAt: fixedNow.Add(-20 * time.Minute),
	}
	repo := &fakeRepository{appointment: appointment}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return fixedNow }
	code := service.verificationCode(appointment.ID, appointment.AppointmentNumber)

	_, err := service.CheckIn(context.Background(), appointment.HospitalID, uuid.NewString(), request.VerifyAppointmentRequest{
		AppointmentNumber: appointment.AppointmentNumber, VerificationCode: code,
	})
	if !errors.Is(err, constant.ErrAppointmentOutsideCheckInWindow) || repo.checkInCalled {
		t.Fatalf("late check-in without override error=%v called=%v", err, repo.checkInCalled)
	}

	reason := "Pasien terlambat karena kondisi darurat"
	_, err = service.CheckIn(context.Background(), appointment.HospitalID, uuid.NewString(), request.VerifyAppointmentRequest{
		AppointmentNumber: appointment.AppointmentNumber, VerificationCode: code,
		ForceLateCheckIn: true, OverrideReason: &reason,
	})
	if err != nil {
		t.Fatalf("late CheckIn() with override error = %v", err)
	}
	if repo.checkInReason == nil || *repo.checkInReason != reason {
		t.Fatalf("audited late check-in reason = %v, want %q", repo.checkInReason, reason)
	}
}
