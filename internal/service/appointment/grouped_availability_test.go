package appointment

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
)

func TestGroupedAvailabilityCombinesHospitalsRetainsFullSlotsAndEmptyDates(t *testing.T) {
	ctx := context.Background()
	doctorID, hospitalA, hospitalB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	date := "2026-10-08"
	queue := repository.Schedule{ID: uuid.NewString(), AffiliationID: uuid.NewString(), DoctorID: doctorID, DoctorMedikaOneID: "MDO-0123456789ABCDEF", HospitalID: hospitalA, HospitalName: "RS Alpha", DepartmentID: uuid.NewString(), DepartmentName: "Poli A", DayOfWeek: 4, StartTime: "09:00", EndTime: "10:00", Timezone: "Asia/Jakarta", BookingMode: "SESSION_QUEUE", SlotDurationMinutes: 30, Capacity: 10}
	fixed := queue
	fixed.ID, fixed.AffiliationID, fixed.DepartmentID = uuid.NewString(), uuid.NewString(), uuid.NewString()
	fixed.StartTime, fixed.EndTime, fixed.BookingMode, fixed.Capacity = "13:00", "14:00", "FIXED_SLOT", 2
	specific := fixed
	specific.ID, specific.HospitalID, specific.HospitalName, specific.ScheduleDate = uuid.NewString(), hospitalB, "RS Zeta", &date
	specific.StartTime, specific.EndTime, specific.Timezone, specific.Capacity = "15:00", "15:30", "Asia/Makassar", 1
	parse := func(value string) time.Time {
		result, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	repo := &fakeRepository{schedules: []repository.Schedule{specific, fixed, queue}, counts: []repository.ReservedCount{
		{ScheduleID: queue.ID, AppointmentDate: date, ScheduledStart: parse(date + "T02:00:00Z"), Reserved: 4},
		{ScheduleID: fixed.ID, AppointmentDate: date, ScheduledStart: parse(date + "T06:00:00Z"), Reserved: 2},
		{ScheduleID: fixed.ID, AppointmentDate: date, ScheduledStart: parse(date + "T06:30:00Z"), Reserved: 1},
		{ScheduleID: specific.ID, AppointmentDate: date, ScheduledStart: parse(date + "T07:00:00Z"), Reserved: 3},
	}}
	svc := NewService(repo, nil, "test")
	svc.now = func() time.Time { return parse("2026-10-07T00:00:00Z") }
	result, err := svc.ListGroupedAvailability(ctx, request.GroupedAvailabilityQuery{DoctorID: doctorID, DateFrom: date, DateTo: "2026-10-15"})
	if err != nil {
		t.Fatal(err)
	}
	if result.DoctorID != doctorID || result.DoctorMedikaOneID == "" || len(result.Dates) != 8 {
		t.Fatalf("invalid calendar: %#v", result)
	}
	day := result.Dates[0]
	if !day.HasAvailableSlots || len(day.Hospitals) != 2 || day.Hospitals[0].HospitalID != hospitalA || day.Hospitals[1].HospitalID != hospitalB {
		t.Fatalf("grouping: %#v", day)
	}
	a, b := day.Hospitals[0], day.Hospitals[1]
	if len(a.Slots) != 3 || !a.HasAvailableSlots || b.HasAvailableSlots || len(b.Slots) != 1 {
		t.Fatalf("hospital choices: %#v", day)
	}
	if a.Slots[0].ScheduleID != queue.ID || a.Slots[0].AvailableCapacity != 6 || a.Slots[0].BookingMode != "SESSION_QUEUE" || a.Slots[0].EndTime != "10:00" {
		t.Fatalf("session choice: %#v", a.Slots[0])
	}
	if a.Slots[1].Status != "FULL" || a.Slots[1].IsBookable || a.Slots[1].AvailableCapacity != 0 || *a.Slots[1].UnavailableReason != "CAPACITY_FULL" {
		t.Fatalf("full choice: %#v", a.Slots[1])
	}
	if a.Slots[2].Status != "AVAILABLE" || a.Slots[2].AvailableCapacity != 1 || a.Slots[2].UnavailableReason != nil || a.Slots[2].DepartmentID != fixed.DepartmentID {
		t.Fatalf("fixed choice: %#v", a.Slots[2])
	}
	if b.Slots[0].ScheduleType != "SPECIFIC" || b.Slots[0].StartTime != "15:00" || b.Slots[0].StartAt.Hour() != 7 || b.Slots[0].AvailableCapacity != 0 {
		t.Fatalf("specific timezone/overcapacity: %#v", b.Slots[0])
	}
	if result.Dates[1].HasAvailableSlots || result.Dates[1].Hospitals == nil || len(result.Dates[1].Hospitals) != 0 {
		t.Fatalf("empty date: %#v", result.Dates[1])
	}
	if len(result.Dates[7].Hospitals) != 1 || result.Dates[7].Hospitals[0].HospitalID != hospitalA {
		t.Fatal("specific schedule repeated on the following week")
	}
	legacy, err := svc.ListAvailability(ctx, "", doctorID, date, "2026-10-15")
	if err != nil {
		t.Fatal(err)
	}
	oldBookable, newBookable := map[string]int{}, map[string]int{}
	for _, row := range legacy {
		for _, slot := range row.Slots {
			oldBookable[availabilityKey(row.ScheduleID, row.Date, slot.StartAt)] = slot.AvailableCapacity
		}
	}
	for _, day := range result.Dates {
		for _, hospital := range day.Hospitals {
			for _, slot := range hospital.Slots {
				if slot.IsBookable {
					newBookable[availabilityKey(slot.ScheduleID, day.Date, slot.StartAt)] = slot.AvailableCapacity
				}
			}
		}
	}
	if !reflect.DeepEqual(oldBookable, newBookable) {
		t.Fatalf("legacy/grouped capacity mismatch: %v != %v", oldBookable, newBookable)
	}
}

func TestGroupedAvailabilityStatusBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name           string
		offset         time.Duration
		capacity       int
		status, reason string
	}{
		{"past", -time.Minute, 1, "CLOSED", "PAST"},
		{"started", 0, 1, "CLOSED", "PAST"},
		{"closed even when full", time.Hour, 0, "CLOSED", "MINIMUM_LEAD_TIME"},
		{"lead boundary", MinimumBookingLeadTime, 1, "CLOSED", "MINIMUM_LEAD_TIME"},
		{"after lead", MinimumBookingLeadTime + time.Nanosecond, 1, "AVAILABLE", ""},
		{"full", 3 * time.Hour, 0, "FULL", "CAPACITY_FULL"},
		{"last bookable instant", BookingHorizon, 1, "AVAILABLE", ""},
		{"outside horizon", BookingHorizon + time.Nanosecond, 1, "CLOSED", "OUTSIDE_BOOKING_HORIZON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := availabilityStatus(response.AvailabilitySlot{StartAt: now.Add(tc.offset), AvailableCapacity: tc.capacity}, now)
			if status != tc.status || (tc.reason == "" && reason != nil) || (tc.reason != "" && (reason == nil || *reason != tc.reason)) {
				t.Fatalf("status=%s reason=%v", status, reason)
			}
		})
	}
}

func TestGroupedAvailabilityKeepsLocalDateAndClosedChoices(t *testing.T) {
	date := "2026-10-08"
	for _, mode := range []string{"FIXED_SLOT", "SESSION_QUEUE"} {
		repo := &fakeRepository{schedules: []repository.Schedule{{ID: uuid.NewString(), HospitalID: uuid.NewString(), ScheduleDate: &date, StartTime: "00:30", EndTime: "01:00", Timezone: "Asia/Jayapura", BookingMode: mode, SlotDurationMinutes: 30, Capacity: 1}}}
		svc := NewService(repo, nil, "test")
		svc.now = func() time.Time { return time.Date(2026, 10, 7, 14, 0, 0, 0, time.UTC) }
		result, err := svc.ListGroupedAvailability(context.Background(), request.GroupedAvailabilityQuery{DoctorID: uuid.NewString(), HospitalID: uuid.NewString(), DateFrom: date, DateTo: date})
		if err != nil {
			t.Fatal(err)
		}
		slot := result.Dates[0].Hospitals[0].Slots[0]
		if result.Dates[0].Date != date || slot.StartAt.Format("2006-01-02") != "2026-10-07" || slot.StartTime != "00:30" || slot.Status != "CLOSED" || result.Dates[0].HasAvailableSlots || result.Dates[0].Hospitals[0].HasAvailableSlots {
			t.Fatalf("mode=%s calendar=%#v slot=%#v", mode, result, slot)
		}
		if repo.scheduleFilter.HospitalID == "" || repo.scheduleFilter.DoctorID != result.DoctorID {
			t.Fatal("query filters were not forwarded")
		}
	}
}

type groupedFailureRepository struct {
	fakeRepository
	doctorErr, schedulesErr, countsErr error
}

func (f *groupedFailureRepository) GetAvailabilityDoctor(ctx context.Context, id string) (*response.AvailabilityDoctor, error) {
	if f.doctorErr != nil {
		return nil, f.doctorErr
	}
	return f.fakeRepository.GetAvailabilityDoctor(ctx, id)
}
func (f *groupedFailureRepository) ListActiveSchedules(ctx context.Context, filter repository.AvailabilityFilter) ([]repository.Schedule, error) {
	if f.schedulesErr != nil {
		return nil, f.schedulesErr
	}
	return f.fakeRepository.ListActiveSchedules(ctx, filter)
}
func (f *groupedFailureRepository) ReservedCounts(ctx context.Context, from, to string, filter repository.AvailabilityFilter) ([]repository.ReservedCount, error) {
	if f.countsErr != nil {
		return nil, f.countsErr
	}
	return f.fakeRepository.ReservedCounts(ctx, from, to, filter)
}

func TestGroupedAvailabilityEmptyCalendarValidationAndFailures(t *testing.T) {
	query := request.GroupedAvailabilityQuery{DoctorID: uuid.NewString(), DateFrom: "2026-10-08", DateTo: "2026-10-09"}
	svc := NewService(&fakeRepository{}, nil, "test")
	svc.now = func() time.Time { return time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC) }
	result, err := svc.ListGroupedAvailability(context.Background(), query)
	if err != nil || len(result.Dates) != 2 || result.DoctorMedikaOneID == "" {
		t.Fatalf("empty doctor calendar: %#v %v", result, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	for _, day := range body["dates"].([]any) {
		if _, ok := day.(map[string]any)["hospitals"].([]any); !ok {
			t.Fatal("empty hospitals must serialize as []")
		}
	}
	for _, invalid := range []request.GroupedAvailabilityQuery{
		{}, {DoctorID: "bad"}, {DoctorID: query.DoctorID, HospitalID: "bad"},
		{DoctorID: query.DoctorID, DateFrom: "2026-02-30"},
		{DoctorID: query.DoctorID, DateFrom: "2026-10-09", DateTo: "2026-10-08"},
		{DoctorID: query.DoctorID, DateFrom: "2026-10-08", DateTo: "2026-11-08"},
		{DoctorID: query.DoctorID, DateFrom: "2027-10-08"},
	} {
		if _, err := svc.ListGroupedAvailability(context.Background(), invalid); err == nil {
			t.Fatalf("accepted invalid query: %#v", invalid)
		}
	}
	for _, tc := range []struct {
		repo *groupedFailureRepository
		want error
	}{
		{&groupedFailureRepository{doctorErr: gorm.ErrRecordNotFound}, constant.ErrDoctorNotFound},
		{&groupedFailureRepository{doctorErr: errors.New("database")}, constant.ErrInternalServerError},
		{&groupedFailureRepository{schedulesErr: errors.New("database")}, constant.ErrInternalServerError},
		{&groupedFailureRepository{countsErr: errors.New("database")}, constant.ErrInternalServerError},
	} {
		svc.repo = tc.repo
		if _, err := svc.ListGroupedAvailability(context.Background(), query); !errors.Is(err, tc.want) {
			t.Fatalf("failure=%v want=%v", err, tc.want)
		}
	}
}
