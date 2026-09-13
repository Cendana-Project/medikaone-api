package request

import (
	"encoding/json"
	"testing"
)

func TestScheduleRequestRequiresArrayWeekdays(t *testing.T) {
	var schedule DoctorInvitationScheduleRequest
	if err := json.Unmarshal([]byte(`{"day_of_week":1,"start_time":"08:00","end_time":"10:00"}`), &schedule); err == nil {
		t.Fatal("scalar weekday must be rejected")
	}
	if err := json.Unmarshal([]byte(`{"day_of_week":[1,3],"start_time":"08:00","end_time":"10:00"}`), &schedule); err != nil || len(schedule.DayOfWeek) != 2 {
		t.Fatalf("array request=%+v err=%v", schedule, err)
	}
	if err := json.Unmarshal([]byte(`{"day_of_week":[],"schedule_date":"2026-09-14","start_time":"08:00","end_time":"10:00"}`), &schedule); err != nil || schedule.ScheduleDate == nil || len(schedule.DayOfWeek) != 0 {
		t.Fatalf("specific request=%+v err=%v", schedule, err)
	}
}
