package scheduleconflict

import (
	"fmt"
	"sort"
	"time"
)

const (
	dateLayout       = "2006-01-02"
	timeLayout       = "15:04"
	secondsPerWeek   = 7 * 24 * 60 * 60
	offsetCheckYears = 5
)

// Schedule is the minimum projection required to compare practice windows.
// DayOfWeek follows PostgreSQL/Go: 0=Sunday through 6=Saturday.
type Schedule struct {
	DayOfWeek    int
	ScheduleDate *string
	StartTime    string
	EndTime      string
	Timezone     string
}

type parsedSchedule struct {
	day          int
	date         *time.Time
	startSeconds int
	endSeconds   int
	location     *time.Location
	timezone     string
}

type instantInterval struct{ start, end time.Time }
type weekInterval struct{ start, end int }
type offsetProfile struct {
	offset int
	fixed  bool
}

// AnyOverlap checks schedules in the same proposal. It is useful as a
// repository-level guard even when the transport/service already validated the
// request.
func AnyOverlap(schedules []Schedule, now time.Time) (bool, error) {
	parsed, err := parseSchedules(schedules)
	if err != nil {
		return false, err
	}
	profiles := map[string]offsetProfile{}
	for i := range parsed {
		for j := i + 1; j < len(parsed); j++ {
			conflict, conflictErr := overlaps(parsed[i], parsed[j], now, profiles)
			if conflictErr != nil {
				return false, conflictErr
			}
			if conflict {
				return true, nil
			}
		}
	}
	return false, nil
}

// AnyConflict compares proposed schedules with all active schedules owned by a
// doctor, regardless of hospital or affiliation.
func AnyConflict(proposed, active []Schedule, now time.Time) (bool, error) {
	left, err := parseSchedules(proposed)
	if err != nil {
		return false, err
	}
	right, err := parseSchedules(active)
	if err != nil {
		return false, err
	}
	profiles := map[string]offsetProfile{}
	for _, candidate := range left {
		for _, existing := range right {
			conflict, conflictErr := overlaps(candidate, existing, now, profiles)
			if conflictErr != nil {
				return false, conflictErr
			}
			if conflict {
				return true, nil
			}
		}
	}
	return false, nil
}

func parseSchedules(input []Schedule) ([]parsedSchedule, error) {
	result := make([]parsedSchedule, 0, len(input))
	for _, schedule := range input {
		if schedule.DayOfWeek < 0 || schedule.DayOfWeek > 6 {
			return nil, fmt.Errorf("invalid schedule weekday %d", schedule.DayOfWeek)
		}
		location, err := time.LoadLocation(schedule.Timezone)
		if err != nil {
			return nil, fmt.Errorf("load schedule timezone %q: %w", schedule.Timezone, err)
		}
		start, err := time.Parse(timeLayout, schedule.StartTime)
		if err != nil {
			return nil, fmt.Errorf("parse schedule start time %q: %w", schedule.StartTime, err)
		}
		end, err := time.Parse(timeLayout, schedule.EndTime)
		if err != nil || !end.After(start) {
			return nil, fmt.Errorf("invalid schedule end time %q", schedule.EndTime)
		}
		parsed := parsedSchedule{
			day: schedule.DayOfWeek, startSeconds: secondsSinceMidnight(start),
			endSeconds: secondsSinceMidnight(end), location: location, timezone: schedule.Timezone,
		}
		if schedule.ScheduleDate != nil {
			date, parseErr := time.Parse(dateLayout, *schedule.ScheduleDate)
			if parseErr != nil || int(date.Weekday()) != schedule.DayOfWeek {
				return nil, fmt.Errorf("invalid specific schedule date %q", *schedule.ScheduleDate)
			}
			parsed.date = &date
		}
		result = append(result, parsed)
	}
	return result, nil
}

func overlaps(a, b parsedSchedule, now time.Time, profiles map[string]offsetProfile) (bool, error) {
	switch {
	case a.date != nil && b.date != nil:
		return intervalsOverlap(specificInterval(a), specificInterval(b)), nil
	case a.date != nil:
		return specificRecurringOverlap(a, b), nil
	case b.date != nil:
		return specificRecurringOverlap(b, a), nil
	case a.timezone == b.timezone:
		return a.day == b.day && a.startSeconds < b.endSeconds && a.endSeconds > b.startSeconds, nil
	}

	// Recurring schedules in fixed-offset zones have an exact weekly UTC
	// representation. This covers Indonesia's WIB/WITA/WIT zones without
	// assuming their local clock values are directly comparable.
	profileA := timezoneOffsetProfile(a, now, profiles)
	profileB := timezoneOffsetProfile(b, now, profiles)
	if profileA.fixed && profileB.fixed {
		return weeklyOverlap(a, profileA.offset, b, profileB.offset), nil
	}

	// When either zone changes offset (DST or a similar rule), approving two
	// recurring schedules across different zones would require an unbounded
	// future comparison. Reject conservatively: one-off schedules remain fully
	// supported across every valid IANA timezone.
	return true, nil
}

func specificInterval(schedule parsedSchedule) instantInterval {
	date := *schedule.date
	return instantInterval{
		start: time.Date(date.Year(), date.Month(), date.Day(), schedule.startSeconds/3600, (schedule.startSeconds%3600)/60, schedule.startSeconds%60, 0, schedule.location),
		end:   time.Date(date.Year(), date.Month(), date.Day(), schedule.endSeconds/3600, (schedule.endSeconds%3600)/60, schedule.endSeconds%60, 0, schedule.location),
	}
}

func specificRecurringOverlap(specific, recurring parsedSchedule) bool {
	exact := specificInterval(specific)
	firstLocal := exact.start.Add(-24 * time.Hour).In(recurring.location)
	lastLocal := exact.end.Add(24 * time.Hour).In(recurring.location)
	date := time.Date(firstLocal.Year(), firstLocal.Month(), firstLocal.Day(), 0, 0, 0, 0, recurring.location)
	lastDate := time.Date(lastLocal.Year(), lastLocal.Month(), lastLocal.Day(), 0, 0, 0, 0, recurring.location)
	for !date.After(lastDate) {
		if int(date.Weekday()) == recurring.day {
			occurrence := instantInterval{
				start: time.Date(date.Year(), date.Month(), date.Day(), recurring.startSeconds/3600, (recurring.startSeconds%3600)/60, recurring.startSeconds%60, 0, recurring.location),
				end:   time.Date(date.Year(), date.Month(), date.Day(), recurring.endSeconds/3600, (recurring.endSeconds%3600)/60, recurring.endSeconds%60, 0, recurring.location),
			}
			if intervalsOverlap(exact, occurrence) {
				return true
			}
		}
		date = date.AddDate(0, 0, 1)
	}
	return false
}

func timezoneOffsetProfile(schedule parsedSchedule, now time.Time, cache map[string]offsetProfile) offsetProfile {
	if profile, ok := cache[schedule.timezone]; ok {
		return profile
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	start := now.AddDate(0, 0, -8)
	end := now.AddDate(offsetCheckYears, 0, 8)
	_, initial := start.In(schedule.location).Zone()
	profile := offsetProfile{offset: initial, fixed: true}
	for sample := start; !sample.After(end); sample = sample.Add(24 * time.Hour) {
		_, offset := sample.In(schedule.location).Zone()
		if offset != initial {
			profile.fixed = false
			break
		}
	}
	cache[schedule.timezone] = profile
	return profile
}

func weeklyOverlap(a parsedSchedule, offsetA int, b parsedSchedule, offsetB int) bool {
	aRanges := weeklyRanges(a, offsetA)
	bRanges := weeklyRanges(b, offsetB)
	for _, left := range aRanges {
		for _, right := range bRanges {
			if left.start < right.end && left.end > right.start {
				return true
			}
		}
	}
	return false
}

func weeklyRanges(schedule parsedSchedule, offset int) []weekInterval {
	start := mod(schedule.day*24*60*60+schedule.startSeconds-offset, secondsPerWeek)
	duration := schedule.endSeconds - schedule.startSeconds
	end := start + duration
	if end <= secondsPerWeek {
		return []weekInterval{{start: start, end: end}}
	}
	result := []weekInterval{{start: start, end: secondsPerWeek}, {start: 0, end: end - secondsPerWeek}}
	sort.Slice(result, func(i, j int) bool { return result[i].start < result[j].start })
	return result
}

func intervalsOverlap(a, b instantInterval) bool {
	return a.start.Before(b.end) && a.end.After(b.start)
}
func secondsSinceMidnight(value time.Time) int {
	return value.Hour()*3600 + value.Minute()*60 + value.Second()
}
func mod(value, divisor int) int {
	result := value % divisor
	if result < 0 {
		result += divisor
	}
	return result
}
