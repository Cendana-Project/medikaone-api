package appointment

import (
	"testing"
	"time"
)

func TestValidScheduleOccurrenceRejectsDifferentDateAndStaleWindow(t *testing.T) {
	date := "2026-09-14"
	schedule := Schedule{DayOfWeek: 1, ScheduleDate: &date, StartTime: "08:00", EndTime: "10:00", Timezone: "Asia/Jakarta", BookingMode: "FIXED_SLOT", SlotDurationMinutes: 30}
	start := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	if !validScheduleOccurrence(schedule, date, start, start.Add(30*time.Minute)) {
		t.Fatal("valid occurrence rejected")
	}
	if validScheduleOccurrence(schedule, "2026-09-21", start.AddDate(0, 0, 7), start.AddDate(0, 0, 7).Add(30*time.Minute)) {
		t.Fatal("specific schedule repeated")
	}
	if validScheduleOccurrence(schedule, date, start.Add(5*time.Minute), start.Add(35*time.Minute)) {
		t.Fatal("misaligned slot accepted")
	}
	if validScheduleOccurrence(schedule, date, start, start.Add(time.Hour)) {
		t.Fatal("stale duration accepted")
	}
	schedule.BookingMode = "SESSION_QUEUE"
	if !validScheduleOccurrence(schedule, date, start, start.Add(2*time.Hour)) {
		t.Fatal("valid queue session rejected")
	}
	if validScheduleOccurrence(schedule, date, start, start.Add(time.Hour)) {
		t.Fatal("partial queue session accepted")
	}
}
