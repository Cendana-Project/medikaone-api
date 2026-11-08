package appointment

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
)

// Build independent wire fixtures without exposing the production codec.
func mustSlotID(t *testing.T, scheduleID string, start, end time.Time) string {
	t.Helper()
	id, err := uuid.Parse(scheduleID)
	if err != nil {
		t.Fatal(err)
	}
	var payload [32]byte
	copy(payload[:16], id[:])
	binary.BigEndian.PutUint64(payload[16:24], uint64(start.Unix()))
	binary.BigEndian.PutUint64(payload[24:], uint64(end.Unix()))
	return "slt1_" + base64.RawURLEncoding.EncodeToString(payload[:])
}

func assertSlotReference(t *testing.T, value, scheduleID string, start, end time.Time) {
	t.Helper()
	if want := mustSlotID(t, scheduleID, start, end); value != want || len(value) != 48 {
		t.Fatalf("slot_id = %q, want canonical reference %q", value, want)
	}
}

func slotTestSchedule() repository.Schedule {
	return repository.Schedule{ID: "11111111-2222-4333-8444-555555555555", HospitalID: "22222222-2222-4333-8444-555555555555", DoctorID: "33333333-2222-4333-8444-555555555555", DayOfWeek: 4, StartTime: "00:30", EndTime: "01:30", Timezone: "Asia/Jayapura", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 2}
}

func slotTestRequest(value string) request.CreateAppointmentRequest {
	return request.CreateAppointmentRequest{SlotID: value, ReasonForVisit: "Consultation", ConsentAccepted: true, ConsentVersion: "v1"}
}

func TestAvailabilitySlotReferencesRoundTripBothModesAndLocalDate(t *testing.T) {
	ctx := context.Background()
	const date = "2026-10-08"
	for _, mode := range []string{"FIXED_SLOT", "SESSION_QUEUE"} {
		t.Run(mode, func(t *testing.T) {
			schedule := slotTestSchedule()
			schedule.BookingMode = mode
			repo := &fakeRepository{schedules: []repository.Schedule{schedule}, appointment: &response.Appointment{ID: uuid.NewString()}}
			svc := NewService(repo, nil, "first-secret")
			svc.now = func() time.Time { return time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC) }
			flat, err := svc.ListAvailability(ctx, schedule.HospitalID, schedule.DoctorID, date, date)
			if err != nil || len(flat) != 1 || len(flat[0].Slots) == 0 {
				t.Fatalf("flat availability: %#v %v", flat, err)
			}
			grouped, err := svc.ListGroupedAvailability(ctx, request.GroupedAvailabilityQuery{DoctorID: schedule.DoctorID, DateFrom: date, DateTo: date})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "SESSION_QUEUE" && len(flat[0].Slots) != 1 {
				t.Fatal("session queue must expose one reference, not one per queue position")
			}
			for i, slot := range flat[0].Slots {
				assertSlotReference(t, slot.SlotID, schedule.ID, slot.StartAt, slot.EndAt)
				if slot.SlotID != grouped.Dates[0].Hospitals[0].Slots[i].SlotID {
					t.Fatal("flat/grouped slot reference mismatch")
				}
				if _, _, err := svc.CreateAppointment(ctx, uuid.NewString(), uuid.NewString(), "", "", slotTestRequest(slot.SlotID)); err != nil {
					t.Fatal(err)
				}
				if repo.bookInput.AppointmentDate != date || !repo.bookInput.ScheduledStartAt.Equal(slot.StartAt) || !repo.bookInput.ScheduledEndAt.Equal(slot.EndAt) || slot.StartAt.Format("2006-01-02") != "2026-10-07" {
					t.Fatalf("reference lost local date or interval: %#v", repo.bookInput)
				}
			}
			// Capacity and verification secrets do not alter the identity of an interval.
			repo.counts = []repository.ReservedCount{{ScheduleID: schedule.ID, AppointmentDate: date, ScheduledStart: flat[0].Slots[0].StartAt, Reserved: 1}}
			restarted := NewService(repo, nil, "rotated-secret")
			restarted.now = svc.now
			again, err := restarted.ListAvailability(ctx, schedule.HospitalID, schedule.DoctorID, date, date)
			if err != nil || again[0].Slots[0].SlotID != flat[0].Slots[0].SlotID || again[0].Slots[0].AvailableCapacity != 1 {
				t.Fatalf("unstable reference after restart/capacity change: %#v %v", again, err)
			}
		})
	}
}

func TestCreateRejectsMalformedSlotReferencesBeforeBooking(t *testing.T) {
	schedule := slotTestSchedule()
	start := time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC)
	valid := mustSlotID(t, schedule.ID, start, start.Add(30*time.Minute))
	for name, value := range map[string]string{
		"missing": "", "uuid is not slot": schedule.ID, "bad prefix": "slt2_" + valid[5:], "padded": valid + "=",
		"invalid alphabet": valid[:47] + "+", "whitespace": " " + valid, "short": valid[:47],
		"zero uuid":                  mustSlotID(t, uuid.Nil.String(), start, start.Add(time.Minute)),
		"reversed":                   mustSlotID(t, schedule.ID, start, start.Add(-time.Minute)),
		"empty interval":             mustSlotID(t, schedule.ID, start, start),
		"negative timestamp":         mustSlotID(t, schedule.ID, time.Unix(-1, 0), start),
		"unsupported timestamp":      mustSlotID(t, schedule.ID, start, time.Unix(253402300800, 0)),
		"noncanonical trailing bits": valid[:47] + string("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"[strings.IndexByte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", valid[47])+1]),
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepository{schedules: []repository.Schedule{schedule}}
			svc := NewService(repo, nil, "test")
			_, _, err := svc.CreateAppointment(context.Background(), uuid.NewString(), uuid.NewString(), "", "", slotTestRequest(value))
			var custom response.CustomError
			want := "INVALID_FIELD_VALUE"
			if value == "" {
				want = "FIELD_REQUIRED"
			}
			if !errors.As(err, &custom) || custom.Code != want || !strings.Contains(custom.Detail.DescEng, "slot_id") || repo.bookInput.Schedule.ID != "" {
				t.Fatalf("malformed reference error=%v input=%#v", err, repo.bookInput)
			}
		})
	}
}

func TestCreateRejectsStaleOrForgedCurrentIntervals(t *testing.T) {
	start := time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		change     func(*repository.Schedule)
		start, end time.Time
		want       error
	}{
		{"second offset", nil, start.Add(time.Second), start.Add(30*time.Minute + time.Second), constant.ErrAppointmentSlotUnavailable},
		{"off grid", nil, start.Add(time.Minute), start.Add(31 * time.Minute), constant.ErrAppointmentSlotUnavailable},
		{"wrong duration", nil, start, start.Add(time.Hour), constant.ErrAppointmentSlotUnavailable},
		{"outside window", nil, start.Add(-30 * time.Minute), start, constant.ErrAppointmentSlotUnavailable},
		{"duration changed", func(s *repository.Schedule) { s.SlotDurationMinutes = 15 }, start, start.Add(30 * time.Minute), constant.ErrAppointmentSlotUnavailable},
		{"window changed", func(s *repository.Schedule) { s.StartTime = "01:00" }, start, start.Add(30 * time.Minute), constant.ErrAppointmentSlotUnavailable},
		{"timezone changed", func(s *repository.Schedule) { s.Timezone = "Asia/Jakarta" }, start, start.Add(30 * time.Minute), constant.ErrScheduleNotFound},
		{"weekday changed", func(s *repository.Schedule) { s.DayOfWeek = 5 }, start, start.Add(30 * time.Minute), constant.ErrScheduleNotFound},
		{"specific date changed", func(s *repository.Schedule) { date := "2026-10-15"; s.ScheduleDate = &date }, start, start.Add(30 * time.Minute), constant.ErrScheduleNotFound},
		{"session partial interval", func(s *repository.Schedule) { s.BookingMode = "SESSION_QUEUE" }, start, start.Add(30 * time.Minute), constant.ErrAppointmentSlotUnavailable},
		{"unknown booking mode", func(s *repository.Schedule) { s.BookingMode = "UNKNOWN" }, start, start.Add(30 * time.Minute), constant.ErrAppointmentSlotUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schedule := slotTestSchedule()
			if tc.change != nil {
				tc.change(&schedule)
			}
			repo := &fakeRepository{schedules: []repository.Schedule{schedule}}
			svc := NewService(repo, nil, "test")
			svc.now = func() time.Time { return start.Add(-48 * time.Hour) }
			_, _, err := svc.CreateAppointment(context.Background(), uuid.NewString(), uuid.NewString(), "", "", slotTestRequest(mustSlotID(t, schedule.ID, tc.start, tc.end)))
			if !errors.Is(err, tc.want) || repo.bookInput.Schedule.ID != "" {
				t.Fatalf("stale reference error=%v want=%v input=%#v", err, tc.want, repo.bookInput)
			}
		})
	}
	// An otherwise valid public reference cannot reactivate a deleted schedule.
	svc := NewService(&fakeRepository{}, nil, "test")
	_, _, err := svc.CreateAppointment(context.Background(), uuid.NewString(), uuid.NewString(), "", "", slotTestRequest(mustSlotID(t, slotTestSchedule().ID, start, start.Add(30*time.Minute))))
	if !errors.Is(err, constant.ErrScheduleNotFound) {
		t.Fatalf("deleted schedule error=%v", err)
	}
}

func TestSlotReferencesRetainLeadTimeAndHorizon(t *testing.T) {
	start := time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		now     time.Time
		allowed bool
	}{
		{"inside minimum lead time", start.Add(-MinimumBookingLeadTime + time.Second), false},
		{"beyond booking horizon", start.Add(-BookingHorizon - time.Second), false},
		{"existing lead boundary", start.Add(-MinimumBookingLeadTime), true},
		{"existing horizon boundary", start.Add(-BookingHorizon), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schedule := slotTestSchedule()
			repo := &fakeRepository{schedules: []repository.Schedule{schedule}, appointment: &response.Appointment{ID: uuid.NewString()}}
			svc := NewService(repo, nil, "test")
			svc.now = func() time.Time { return tc.now }
			_, _, err := svc.CreateAppointment(context.Background(), uuid.NewString(), uuid.NewString(), "", "", slotTestRequest(mustSlotID(t, schedule.ID, start, start.Add(30*time.Minute))))
			if tc.allowed {
				if err != nil || !repo.bookInput.ScheduledStartAt.Equal(start) || repo.bookInput.AppointmentDate != "2026-10-08" {
					t.Fatalf("existing booking boundary changed: input=%+v error=%v", repo.bookInput, err)
				}
			} else if !errors.Is(err, constant.ErrAppointmentSlotUnavailable) || repo.bookInput.Schedule.ID != "" {
				t.Fatalf("lead/horizon bypass: %v", err)
			}
		})
	}
}

func TestRescheduleConsumesSlotReferenceAndRetainsActorPermissions(t *testing.T) {
	ctx := context.Background()
	schedule := slotTestSchedule()
	start := time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC)
	patientID, actorID := uuid.NewString(), uuid.NewString()
	for _, hospital := range []bool{false, true} {
		t.Run(map[bool]string{false: "patient", true: "hospital"}[hospital], func(t *testing.T) {
			old := &response.Appointment{ID: uuid.NewString(), PatientID: &patientID, PatientRecordID: uuid.NewString(), HospitalID: schedule.HospitalID, ScheduledStartAt: start.Add(-24 * time.Hour), ReasonForVisit: "Original reason", ConsentVersion: "v1"}
			repo := &fakeRepository{schedules: []repository.Schedule{schedule}, appointment: old}
			svc := NewService(repo, nil, "test")
			svc.now = func() time.Time { return start.Add(-72 * time.Hour) }
			req := request.RescheduleAppointmentRequest{SlotID: mustSlotID(t, schedule.ID, start, start.Add(30*time.Minute)), Reason: "Move visit"}
			invoke := func(owner string) error {
				var err error
				if hospital {
					_, _, err = svc.RescheduleHospitalAppointment(ctx, owner, actorID, old.ID, uuid.NewString(), "", "", req)
				} else {
					_, _, err = svc.ReschedulePatientAppointment(ctx, owner, old.ID, uuid.NewString(), "", "", req)
				}
				return err
			}
			if err := invoke(uuid.NewString()); !errors.Is(err, constant.ErrAppointmentNotFound) || repo.bookInput.Schedule.ID != "" {
				t.Fatalf("foreign actor allowed: %v", err)
			}
			owner, expectedActor := patientID, patientID
			if hospital {
				owner, expectedActor = schedule.HospitalID, actorID
			}
			if err := invoke(owner); err != nil {
				t.Fatal(err)
			}
			if repo.bookInput.CreatedBy != expectedActor || repo.bookInput.PatientID != patientID || repo.bookInput.PatientRecordID != old.PatientRecordID || !repo.bookInput.ScheduledStartAt.Equal(start) || repo.bookInput.ReasonForVisit != "Original reason" {
				t.Fatalf("reschedule lost metadata: %#v", repo.bookInput)
			}
			repo.bookInput = repository.BookInput{}
			req.SlotID = "invalid"
			if err := invoke(owner); err == nil || repo.bookInput.Schedule.ID != "" {
				t.Fatalf("malformed reschedule accepted: %v", err)
			}
			req.SlotID = mustSlotID(t, schedule.ID, start.Add(time.Second), start.Add(30*time.Minute+time.Second))
			if err := invoke(owner); !errors.Is(err, constant.ErrAppointmentSlotUnavailable) || repo.bookInput.Schedule.ID != "" {
				t.Fatalf("off-grid reschedule accepted: %v", err)
			}
			if !hospital {
				req.SlotID = mustSlotID(t, schedule.ID, start, start.Add(30*time.Minute))
				svc.now = func() time.Time { return old.ScheduledStartAt.Add(-PatientChangeCutoff) }
				if err := invoke(owner); !errors.Is(err, constant.ErrAppointmentCutoffPassed) {
					t.Fatalf("patient cutoff bypassed: %v", err)
				}
			}
		})
	}
}

type rejectingSlotRepository struct {
	fakeRepository
	rejection error
}

func (r *rejectingSlotRepository) Book(_ context.Context, input repository.BookInput) (*response.Appointment, bool, error) {
	r.bookInput = input
	return nil, false, r.rejection
}

func (r *rejectingSlotRepository) Reschedule(_ context.Context, _, _, _ string, input repository.BookInput) (*response.Appointment, bool, error) {
	r.bookInput = input
	return nil, false, r.rejection
}

func TestSlotReferenceDoesNotBypassTransactionalBookingRejections(t *testing.T) {
	schedule := slotTestSchedule()
	start := time.Date(2026, 10, 7, 15, 30, 0, 0, time.UTC)
	patientID := uuid.NewString()
	for _, tc := range []struct {
		name            string
		rejection, want error
	}{
		{"capacity", repository.ErrSlotUnavailable, constant.ErrAppointmentSlotUnavailable},
		{"patient conflict", repository.ErrPatientTimeConflict, constant.ErrAppointmentSlotUnavailable},
		{"idempotency conflict", repository.ErrIdempotencyConflict, constant.ErrIdempotencyConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &rejectingSlotRepository{fakeRepository: fakeRepository{schedules: []repository.Schedule{schedule}, appointment: &response.Appointment{ID: uuid.NewString(), PatientID: &patientID, HospitalID: schedule.HospitalID, ScheduledStartAt: start.Add(-24 * time.Hour), ReasonForVisit: "Consultation", ConsentVersion: "v1"}}, rejection: tc.rejection}
			svc := NewService(repo, nil, "test")
			svc.now = func() time.Time { return start.Add(-72 * time.Hour) }
			req := slotTestRequest(mustSlotID(t, schedule.ID, start, start.Add(30*time.Minute)))
			if _, _, err := svc.CreateAppointment(context.Background(), patientID, uuid.NewString(), "", "", req); !errors.Is(err, tc.want) {
				t.Fatalf("creation bypassed transactional rejection: %v", err)
			}
			move := request.RescheduleAppointmentRequest{SlotID: req.SlotID, Reason: "Move visit"}
			if _, _, err := svc.ReschedulePatientAppointment(context.Background(), patientID, repo.appointment.ID, uuid.NewString(), "", "", move); !errors.Is(err, tc.want) {
				t.Fatalf("reschedule bypassed transactional rejection: %v", err)
			}
			if repo.bookInput.Schedule.ID != schedule.ID || !repo.bookInput.ScheduledEndAt.Equal(start.Add(30*time.Minute)) {
				t.Fatalf("wrong reference reached transaction: %#v", repo.bookInput)
			}
		})
	}
}
