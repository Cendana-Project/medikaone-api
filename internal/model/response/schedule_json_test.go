package response

import (
	"encoding/json"
	"testing"
)

func TestScheduleJSONAlwaysReturnsWeekdayArray(t *testing.T) {
	date := "2026-09-14"
	for _, item := range []DoctorHospitalSchedule{{DayOfWeek: 1, Status: "ACTIVE"}, {DayOfWeek: 1, ScheduleDate: &date, Status: "PENDING"}} {
		encoded, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			DayOfWeek    []int   `json:"day_of_week"`
			ScheduleDate *string `json:"schedule_date"`
			Status       string  `json:"status"`
		}
		if err := json.Unmarshal(encoded, &body); err != nil {
			t.Fatal(err)
		}
		if body.DayOfWeek == nil {
			t.Fatalf("array must not be null: %s", encoded)
		}
		if body.Status != item.Status {
			t.Fatalf("schedule status was lost: %s", encoded)
		}
		if item.ScheduleDate == nil && (len(body.DayOfWeek) != 1 || body.DayOfWeek[0] != 1) {
			t.Fatalf("recurrence: %s", encoded)
		}
		if item.ScheduleDate != nil && (len(body.DayOfWeek) != 0 || body.ScheduleDate == nil || *body.ScheduleDate != date) {
			t.Fatalf("specific date: %s", encoded)
		}
	}
}
