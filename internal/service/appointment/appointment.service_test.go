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
	checkInMethod    string
	transitionTarget string
	walkInInput      repository.WalkInInput
	overrideAllowed  bool
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
func (f *fakeRepository) ListQueue(context.Context, repository.QueueFilter) (*response.AppointmentPage, error) {
	return &response.AppointmentPage{Items: []response.Appointment{}, Page: 1, Limit: 20}, nil
}
func (f *fakeRepository) Cancel(context.Context, string, string, string, time.Time) error { return nil }
func (f *fakeRepository) Reschedule(_ context.Context, _ string, _ string, _ string, input repository.BookInput) (*response.Appointment, bool, error) {
	f.bookInput = input
	return f.appointment, false, nil
}
func (f *fakeRepository) CheckIn(_ context.Context, _, _, method string, reason *string, _ time.Time) error {
	f.checkInCalled = true
	f.checkInReason = reason
	f.checkInMethod = method
	return nil
}
func (f *fakeRepository) FindCheckInAppointments(context.Context, string, string, repository.CheckInIdentityFilter) ([]response.Appointment, error) {
	if f.appointment == nil {
		return nil, nil
	}
	return []response.Appointment{*f.appointment}, nil
}
func (f *fakeRepository) GetPatientRecordForAppointment(context.Context, string) (*repository.PatientRecord, error) {
	return &repository.PatientRecord{ID: uuid.NewString(), FirstName: "Test", LastName: "Patient", Phone: "081234567890", DateOfBirth: "1990-01-01", Gender: "L", IdentityType: "NIK", IdentityNumber: "3174000000000001"}, nil
}
func (f *fakeRepository) CreateWalkIn(_ context.Context, input repository.WalkInInput) (*response.Appointment, bool, error) {
	f.walkInInput = input
	return f.appointment, false, nil
}
func (f *fakeRepository) CanOverrideWalkInCapacity(context.Context, string, string) (bool, error) {
	return f.overrideAllowed, nil
}
func (f *fakeRepository) ClaimPatientRecord(context.Context, string, string, string, string, time.Time) (*repository.PatientRecord, error) {
	return &repository.PatientRecord{ID: uuid.NewString(), FirstName: "Test", Phone: "081234567890", DateOfBirth: "1990-01-01", Gender: "L", IdentityType: "NIK", IdentityNumber: "3174000000000001"}, nil
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
	appointment := &response.Appointment{
		ID: uuid.NewString(), AppointmentNumber: "APT-TEST-20260907-0001", HospitalID: uuid.NewString(),
		AppointmentDate: "2026-09-07", Timezone: "Asia/Jakarta", Status: entity.AppointmentConfirmed,
		ScheduledStartAt: fixedNow.Add(20 * time.Minute),
	}
	repo := &fakeRepository{appointment: appointment}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return fixedNow }

	_, err := service.CheckIn(context.Background(), appointment.HospitalID, uuid.NewString(), request.VerifyAppointmentRequest{AppointmentNumber: appointment.AppointmentNumber, VerificationCode: "WRONG"})
	if !errors.Is(err, constant.ErrAppointmentVerificationCodeInvalid) || repo.checkInCalled {
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
		AppointmentDate: "2026-09-07", Timezone: "Asia/Jakarta", Status: entity.AppointmentConfirmed,
	}
	repo := &fakeRepository{appointment: appointment}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return fixedNow }
	code := service.verificationCode(appointment.ID, appointment.AppointmentNumber)

	_, err := service.CheckIn(context.Background(), appointment.HospitalID, uuid.NewString(), request.VerifyAppointmentRequest{
		AppointmentNumber: appointment.AppointmentNumber, VerificationCode: code,
	})
	if !errors.Is(err, constant.ErrAppointmentLateOverrideReasonRequired) || repo.checkInCalled {
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

func TestNoShowCanBeCheckedInUntilEndOfSameLocalDay(t *testing.T) {
	now := time.Date(2026, 9, 7, 15, 30, 0, 0, time.UTC) // 22:30 WIB
	appointment := &response.Appointment{
		ID: uuid.NewString(), AppointmentNumber: "APT-TEST-20260907-0003",
		HospitalID: uuid.NewString(), AppointmentDate: "2026-09-07", Timezone: "Asia/Jakarta",
		Status:           entity.AppointmentNoShow,
		ScheduledStartAt: time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC),
		ScheduledEndAt:   time.Date(2026, 9, 7, 2, 0, 0, 0, time.UTC),
	}
	repo := &fakeRepository{appointment: appointment}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return now }
	code := service.verificationCode(appointment.ID, appointment.AppointmentNumber)

	reason := "Dokter masih melayani pasien overtime"
	result, err := service.CheckIn(context.Background(), appointment.HospitalID, uuid.NewString(), request.VerifyAppointmentRequest{
		AppointmentNumber: appointment.AppointmentNumber,
		VerificationCode:  code,
		ForceLateCheckIn:  true,
		OverrideReason:    &reason,
	})
	if err != nil || result == nil || !repo.checkInCalled {
		t.Fatalf("same-day NO_SHOW check-in result=%v error=%v called=%v", result, err, repo.checkInCalled)
	}
	if repo.checkInReason == nil || *repo.checkInReason != reason {
		t.Fatalf("override reason = %v, want %q", repo.checkInReason, reason)
	}

	repo.checkInCalled = false
	now = time.Date(2026, 9, 7, 17, 1, 0, 0, time.UTC) // 00:01 WIB next day
	_, err = service.CheckIn(context.Background(), appointment.HospitalID, uuid.NewString(), request.VerifyAppointmentRequest{
		AppointmentNumber: appointment.AppointmentNumber,
		VerificationCode:  code,
		ForceLateCheckIn:  true,
		OverrideReason:    &reason,
	})
	if !errors.Is(err, constant.ErrAppointmentCheckInExpired) || repo.checkInCalled {
		t.Fatalf("next-day check-in error=%v called=%v", err, repo.checkInCalled)
	}
}

func TestCheckInLookupRequiresTwoIdentityFacts(t *testing.T) {
	service := NewService(&fakeRepository{}, nil, "test-secret-at-least-thirty-two-characters")
	_, err := service.LookupCheckIn(context.Background(), uuid.NewString(), uuid.NewString(), request.CheckInLookupRequest{
		Identity: &request.CheckInIdentity{Name: "Siti"},
	})
	if !errors.Is(err, constant.ErrCheckInIdentityInsufficient) {
		t.Fatalf("single-field identity error = %v", err)
	}

	dob := "1990-01-01"
	filter, err := normalizeCheckInIdentity(request.CheckInIdentity{
		IdentityType: "passport", IdentityNumber: " A-12 34 ", DateOfBirth: &dob,
	})
	if err != nil || filter.IdentityType != "PASSPORT" || filter.IdentityNumber != "A-12 34" {
		t.Fatalf("generic identity filter = %+v, error=%v", filter, err)
	}
}

func TestCheckInGrantIsBoundToReceptionist(t *testing.T) {
	now := time.Date(2026, 9, 7, 1, 0, 0, 0, time.UTC)
	appointment := &response.Appointment{
		ID: uuid.NewString(), AppointmentNumber: "APT-TEST-20260907-0004",
		HospitalID: uuid.NewString(), AppointmentDate: "2026-09-07", Timezone: "Asia/Jakarta",
		Status: entity.AppointmentConfirmed, ScheduledStartAt: now.Add(10 * time.Minute),
	}
	repo := &fakeRepository{appointment: appointment}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return now }
	receptionistID := uuid.NewString()
	code := service.verificationCode(appointment.ID, appointment.AppointmentNumber)
	lookup, err := service.LookupCheckIn(context.Background(), appointment.HospitalID, receptionistID, request.CheckInLookupRequest{
		AppointmentNumber: appointment.AppointmentNumber, VerificationCode: code,
	})
	if err != nil || lookup.Count != 1 {
		t.Fatalf("LookupCheckIn() result=%+v error=%v", lookup, err)
	}
	_, err = service.ConfirmCheckIn(context.Background(), appointment.HospitalID, uuid.NewString(), appointment.ID, request.ConfirmCheckInRequest{
		CheckInToken: lookup.Candidates[0].CheckInToken,
	})
	if !errors.Is(err, constant.ErrCheckInTokenInvalidOrExpired) || repo.checkInCalled {
		t.Fatalf("cross-receptionist grant error=%v called=%v", err, repo.checkInCalled)
	}
	_, err = service.ConfirmCheckIn(context.Background(), appointment.HospitalID, receptionistID, appointment.ID, request.ConfirmCheckInRequest{
		CheckInToken: lookup.Candidates[0].CheckInToken,
	})
	if err != nil || !repo.checkInCalled || repo.checkInMethod != entity.CheckInMethodCode {
		t.Fatalf("bound grant error=%v called=%v method=%q", err, repo.checkInCalled, repo.checkInMethod)
	}
}

func TestCreateWalkInUsesReceptionistConsentAndCurrentDate(t *testing.T) {
	now := time.Date(2026, 9, 7, 1, 15, 0, 0, time.UTC) // Monday 08:15 WIB
	schedule := repository.Schedule{
		ID: uuid.NewString(), AffiliationID: uuid.NewString(), HospitalID: uuid.NewString(),
		HospitalCode: "HSP-MO-001", DoctorID: uuid.NewString(), DepartmentID: uuid.NewString(),
		DayOfWeek: 1, StartTime: "08:00", EndTime: "10:00", Timezone: "Asia/Jakarta",
		BookingMode: entity.BookingModeFixedSlot, SlotDurationMinutes: 30, Capacity: 1,
	}
	appointment := &response.Appointment{ID: uuid.NewString(), AppointmentNumber: "APT-HSPMO001-20260907-0005"}
	repo := &fakeRepository{schedules: []repository.Schedule{schedule}, appointment: appointment}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return now }
	start := "08:00"
	result, _, err := service.CreateWalkInAppointment(context.Background(), schedule.HospitalID, uuid.NewString(), uuid.NewString(), request.CreateWalkInAppointmentRequest{
		ScheduleID: schedule.ID, StartTime: &start, ReasonForVisit: "Demam",
		ConsentVersion: "2026-09", Patient: request.WalkInPatientRequest{
			FirstName: "Siti", LastName: "Aminah", Phone: "081234567890",
			DateOfBirth: "1990-01-01", Gender: "P", IdentityType: "NIK",
			IdentityNumber: "3174000000000001",
		},
	})
	if err != nil || result == nil {
		t.Fatalf("CreateWalkInAppointment() result=%v error=%v", result, err)
	}
	if repo.walkInInput.AppointmentDate != "2026-09-07" || repo.walkInInput.Patient.IdentityNormalized != "3174000000000001" {
		t.Fatalf("walk-in input = %+v", repo.walkInInput)
	}
}

func TestWalkInPatientSelectionMustUseExactlyOneMode(t *testing.T) {
	recordID := uuid.NewString()
	medikaOneID := uuid.NewString()
	_, err := normalizeWalkInPatient(request.WalkInPatientRequest{
		PatientRecordID: &recordID,
		MedikaOneID:     &medikaOneID,
	})
	if !errors.Is(err, constant.ErrWalkInPatientModeInvalid) {
		t.Fatalf("ambiguous patient selection error = %v", err)
	}

	_, err = normalizeWalkInPatient(request.WalkInPatientRequest{})
	if !errors.Is(err, constant.ErrWalkInPatientDataRequired) {
		t.Fatalf("missing patient selection error = %v", err)
	}
}

func TestWalkInCapacityOverrideRequiresAdminAndReason(t *testing.T) {
	now := time.Date(2026, 9, 7, 1, 15, 0, 0, time.UTC)
	schedule := repository.Schedule{
		ID: uuid.NewString(), AffiliationID: uuid.NewString(), HospitalID: uuid.NewString(),
		DoctorID: uuid.NewString(), DepartmentID: uuid.NewString(), DayOfWeek: 1,
		StartTime: "08:00", EndTime: "10:00", Timezone: "Asia/Jakarta",
		BookingMode: entity.BookingModeSessionQueue, SlotDurationMinutes: 30, Capacity: 1,
	}
	patientRecordID := uuid.NewString()
	repo := &fakeRepository{schedules: []repository.Schedule{schedule}, appointment: &response.Appointment{ID: uuid.NewString()}}
	service := NewService(repo, nil, "test-secret-at-least-thirty-two-characters")
	service.now = func() time.Time { return now }
	req := request.CreateWalkInAppointmentRequest{
		ScheduleID: schedule.ID, ReasonForVisit: "Kontrol", ConsentVersion: "2026-09",
		Patient: request.WalkInPatientRequest{PatientRecordID: &patientRecordID}, CapacityOverride: true,
	}
	_, _, err := service.CreateWalkInAppointment(context.Background(), schedule.HospitalID, uuid.NewString(), uuid.NewString(), req)
	if !errors.Is(err, constant.ErrWalkInCapacityOverrideReasonRequired) {
		t.Fatalf("missing override reason error = %v", err)
	}
	reason := "Dokter menyetujui pasien tambahan"
	req.CapacityOverrideReason = &reason
	_, _, err = service.CreateWalkInAppointment(context.Background(), schedule.HospitalID, uuid.NewString(), uuid.NewString(), req)
	if !errors.Is(err, constant.ErrWalkInCapacityOverrideForbidden) {
		t.Fatalf("receptionist override error = %v", err)
	}
	repo.overrideAllowed = true
	if _, _, err := service.CreateWalkInAppointment(context.Background(), schedule.HospitalID, uuid.NewString(), uuid.NewString(), req); err != nil {
		t.Fatalf("admin override error = %v", err)
	}
}
