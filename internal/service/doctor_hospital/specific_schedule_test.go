package doctor_hospital

import (
	"errors"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"testing"
)

func TestInvitationSchedulesExpandWeekdaysAndPreserveSpecificDates(t *testing.T) {
	date := "2026-09-14"
	rows, err := validateSchedules([]request.DoctorInvitationScheduleRequest{
		{DayOfWeek: []int{0, 2}, StartTime: "08:00", EndTime: "10:00"},
		{ScheduleDate: &date, StartTime: "08:00", EndTime: "10:00"},
	})
	if err != nil || len(rows) != 3 || rows[0].DayOfWeek != 0 || rows[1].DayOfWeek != 2 || rows[2].DayOfWeek != 1 || rows[2].ScheduleDate == nil || *rows[2].ScheduleDate != date {
		t.Fatalf("initial schedules=%+v err=%v", rows, err)
	}
	_, err = validateSchedules([]request.DoctorInvitationScheduleRequest{
		{DayOfWeek: []int{1, 2}, StartTime: "08:00", EndTime: "10:00"},
		{ScheduleDate: &date, StartTime: "09:00", EndTime: "11:00"},
	})
	if !errors.Is(err, constant.ErrDoctorScheduleConflict) {
		t.Fatalf("recurring vs specific invitation conflict=%v", err)
	}
}
