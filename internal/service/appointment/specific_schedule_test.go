package appointment

import (
	"context"
	"errors"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestNormalizeScheduleArrayAndSpecificDateConflicts(t *testing.T) {
	a, b := "2026-09-14", "2026-09-21"
	rows, err := normalizeSchedules([]request.DoctorInvitationScheduleRequest{{DayOfWeek: []int{1, 3, 5}, StartTime: "08:00", EndTime: "10:00"}})
	if err != nil || len(rows) != 3 || rows[2].DayOfWeek != 5 {
		t.Fatalf("expanded rows=%+v err=%v", rows, err)
	}
	rows, err = normalizeSchedules([]request.DoctorInvitationScheduleRequest{{ScheduleDate: &a, StartTime: "08:00", EndTime: "10:00"}, {ScheduleDate: &b, StartTime: "08:00", EndTime: "10:00"}})
	if err != nil || len(rows) != 2 {
		t.Fatalf("distinct dates=%+v err=%v", rows, err)
	}
	_, err = normalizeSchedules([]request.DoctorInvitationScheduleRequest{{ScheduleDate: &a, StartTime: "08:00", EndTime: "10:00"}, {DayOfWeek: []int{1}, StartTime: "09:00", EndTime: "11:00"}})
	if !errors.Is(err, constant.ErrDoctorScheduleConflict) {
		t.Fatalf("one-off vs recurring conflict = %v", err)
	}
}

func TestSpecificScheduleAvailabilityDoesNotRepeatAndReservesCapacity(t *testing.T) {
	date := "2026-09-14"
	start := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	schedule := repository.Schedule{ID: uuid.NewString(), AffiliationID: uuid.NewString(), HospitalID: uuid.NewString(), DoctorID: uuid.NewString(), DayOfWeek: 1, ScheduleDate: &date, StartTime: "08:00", EndTime: "09:00", Timezone: "Asia/Jakarta", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30, Capacity: 2}
	repo := &fakeRepository{schedules: []repository.Schedule{schedule}, counts: []repository.ReservedCount{{ScheduleID: schedule.ID, AppointmentDate: date, ScheduledStart: start, Reserved: 1}}}
	service := NewService(repo, nil, "test")
	service.now = func() time.Time { return time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC) }
	rows, err := service.ListAvailability(context.Background(), schedule.HospitalID, schedule.DoctorID, date, "2026-09-21")
	if err != nil || len(rows) != 1 || rows[0].Date != date || len(rows[0].DayOfWeek) != 0 || rows[0].Slots[0].AvailableCapacity != 1 {
		t.Fatalf("availability=%+v err=%v", rows, err)
	}
	service.now = func() time.Time { return start.AddDate(0, 0, 7) }
	today, err := service.ListDoctorTodaySchedules(context.Background(), schedule.DoctorID)
	if err != nil || len(today) != 0 {
		t.Fatalf("one-off repeated in today: %+v %v", today, err)
	}
	_, _, err = service.CreateWalkInAppointment(context.Background(), schedule.HospitalID, uuid.NewString(), uuid.NewString(), request.CreateWalkInAppointmentRequest{ScheduleID: schedule.ID})
	if !errors.Is(err, constant.ErrScheduleNotFound) {
		t.Fatalf("one-off repeated in walk-in: %v", err)
	}
	_, err = service.prepareBookInput(context.Background(), uuid.NewString(), uuid.NewString(), "", "", schedule.ID, "2026-09-21", nil, "consultation", nil, "v1")
	if !errors.Is(err, constant.ErrScheduleNotFound) {
		t.Fatalf("one-off repeated in booking: %v", err)
	}
}

type scheduleMutationRepository struct {
	fakeRepository
	change repository.ScheduleChangeInput
}

func (f *scheduleMutationRepository) CreateScheduleChange(_ context.Context, input repository.ScheduleChangeInput) (*response.ScheduleChangeRequest, error) {
	f.change = input
	return &response.ScheduleChangeRequest{ID: uuid.NewString(), Operation: input.Operation, Status: "PENDING"}, nil
}

func TestSpecificCreateAndDeleteRequireOwnerAndCounterpartReview(t *testing.T) {
	date := "2026-09-14"
	schedule := repository.Schedule{ID: uuid.NewString(), AffiliationID: uuid.NewString(), DoctorID: uuid.NewString(), HospitalID: uuid.NewString()}
	repo := &scheduleMutationRepository{fakeRepository: fakeRepository{schedules: []repository.Schedule{schedule}}}
	service := NewService(repo, nil, "test")
	service.now = func() time.Time { return time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC) }
	req := request.CreateSpecificScheduleRequest{AffiliationID: schedule.AffiliationID, Schedule: request.DoctorInvitationScheduleRequest{ScheduleDate: &date, StartTime: "08:00", EndTime: "10:00"}}
	if _, err := service.CreateDoctorSpecificSchedule(context.Background(), uuid.NewString(), req); !errors.Is(err, constant.ErrAffiliationNotFound) {
		t.Fatalf("foreign create=%v", err)
	}
	row, err := service.CreateDoctorSpecificSchedule(context.Background(), schedule.DoctorID, req)
	if err != nil || row.Status != "PENDING" || repo.change.Operation != "ADD" || len(repo.change.Schedules) != 1 {
		t.Fatalf("creation=%+v err=%v", repo.change, err)
	}
	if _, err := service.DeleteHospitalSchedule(context.Background(), uuid.NewString(), uuid.NewString(), schedule.ID); !errors.Is(err, constant.ErrScheduleNotFound) {
		t.Fatalf("foreign delete=%v", err)
	}
	row, err = service.DeleteDoctorSchedule(context.Background(), schedule.DoctorID, schedule.ID)
	if err != nil || row.Status != "PENDING" || repo.change.Operation != "REMOVE" || repo.change.TargetScheduleID == nil || *repo.change.TargetScheduleID != schedule.ID {
		t.Fatalf("deletion=%+v err=%v", repo.change, err)
	}
}
