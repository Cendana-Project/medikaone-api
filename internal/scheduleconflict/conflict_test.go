package scheduleconflict

import (
	"testing"
	"time"
)

func TestCrossTimezoneRecurringSchedulesUseAbsoluteWeeklyTime(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	jakarta := Schedule{DayOfWeek: 1, StartTime: "08:00", EndTime: "09:00", Timezone: "Asia/Jakarta"}
	makassarConflict := Schedule{DayOfWeek: 1, StartTime: "09:00", EndTime: "10:00", Timezone: "Asia/Makassar"}
	makassarAdjacent := Schedule{DayOfWeek: 1, StartTime: "10:00", EndTime: "11:00", Timezone: "Asia/Makassar"}
	jayapuraConflict := Schedule{DayOfWeek: 1, StartTime: "10:00", EndTime: "11:00", Timezone: "Asia/Jayapura"}

	for name, candidate := range map[string]Schedule{"WITA": makassarConflict, "WIT": jayapuraConflict} {
		conflict, err := AnyConflict([]Schedule{candidate}, []Schedule{jakarta}, now)
		if err != nil || !conflict {
			t.Fatalf("%s equivalent UTC interval was not detected: conflict=%v err=%v", name, conflict, err)
		}
	}
	conflict, err := AnyConflict([]Schedule{makassarAdjacent}, []Schedule{jakarta}, now)
	if err != nil || conflict {
		t.Fatalf("adjacent UTC intervals conflict=%v err=%v", conflict, err)
	}
}

func TestSpecificScheduleUsesExactInstantAcrossTimezoneAndDateBoundary(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	jakartaDate, jayapuraDate := "2026-09-27", "2026-09-28"
	jakarta := Schedule{DayOfWeek: 0, ScheduleDate: &jakartaDate, StartTime: "23:30", EndTime: "23:59", Timezone: "Asia/Jakarta"}
	jayapuraRecurring := Schedule{DayOfWeek: 1, StartTime: "01:30", EndTime: "02:00", Timezone: "Asia/Jayapura"}
	jayapuraSpecific := Schedule{DayOfWeek: 1, ScheduleDate: &jayapuraDate, StartTime: "01:30", EndTime: "02:00", Timezone: "Asia/Jayapura"}

	for name, candidate := range map[string]Schedule{"recurring": jayapuraRecurring, "specific": jayapuraSpecific} {
		conflict, err := AnyConflict([]Schedule{candidate}, []Schedule{jakarta}, now)
		if err != nil || !conflict {
			t.Fatalf("%s exact instant conflict=%v err=%v", name, conflict, err)
		}
	}
}

func TestDifferentDSTZonesAreConservativelyRejectedForRecurringSchedules(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	a := Schedule{DayOfWeek: 1, StartTime: "08:00", EndTime: "09:00", Timezone: "America/New_York"}
	b := Schedule{DayOfWeek: 3, StartTime: "18:00", EndTime: "19:00", Timezone: "Europe/London"}
	conflict, err := AnyConflict([]Schedule{a}, []Schedule{b}, now)
	if err != nil || !conflict {
		t.Fatalf("DST cross-zone recurrence must fail closed: conflict=%v err=%v", conflict, err)
	}
}

func TestSameTimezoneAndProposalOverlap(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	schedules := []Schedule{
		{DayOfWeek: 2, StartTime: "08:00", EndTime: "10:00", Timezone: "Asia/Jakarta"},
		{DayOfWeek: 2, StartTime: "09:00", EndTime: "11:00", Timezone: "Asia/Jakarta"},
	}
	conflict, err := AnyOverlap(schedules, now)
	if err != nil || !conflict {
		t.Fatalf("proposal overlap=%v err=%v", conflict, err)
	}
	schedules[1].StartTime, schedules[1].EndTime = "10:00", "11:00"
	conflict, err = AnyOverlap(schedules, now)
	if err != nil || conflict {
		t.Fatalf("adjacent proposal overlap=%v err=%v", conflict, err)
	}
}
