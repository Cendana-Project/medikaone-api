package appointment

import (
	"gorm.io/gorm"
	"time"
)

func sameScheduleDate(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func validScheduleOccurrence(schedule Schedule, dateRaw string, start, end time.Time) bool {
	date, err := time.Parse("2006-01-02", dateRaw)
	if err != nil || int(date.Weekday()) != schedule.DayOfWeek {
		return false
	}
	if schedule.ScheduleDate != nil && *schedule.ScheduleDate != dateRaw {
		return false
	}
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return false
	}
	windowStart, err := time.ParseInLocation("2006-01-02 15:04", dateRaw+" "+schedule.StartTime, loc)
	if err != nil {
		return false
	}
	windowEnd, err := time.ParseInLocation("2006-01-02 15:04", dateRaw+" "+schedule.EndTime, loc)
	if err != nil || start.Before(windowStart) || end.After(windowEnd) || !end.After(start) {
		return false
	}
	if schedule.BookingMode == "SESSION_QUEUE" {
		return start.Equal(windowStart) && end.Equal(windowEnd)
	}
	duration := time.Duration(schedule.SlotDurationMinutes) * time.Minute
	return duration > 0 && end.Sub(start) == duration && start.Sub(windowStart)%duration == 0
}

func requireActivePatient(tx *gorm.DB, userID string) error {
	var activeID string
	if err := tx.Raw(`SELECT id FROM users WHERE id = ? AND status = 'active' AND deleted_at IS NULL FOR SHARE`, userID).Scan(&activeID).Error; err != nil {
		return err
	}
	if activeID == "" {
		return ErrPatientRecordNotFound
	}
	return nil
}
