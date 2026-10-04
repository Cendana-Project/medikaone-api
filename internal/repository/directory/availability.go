// Package directory contains predicates shared by doctor and hospital discovery.
package directory

import (
	"github.com/Cendana-Project/medikaone-api/internal/bookingpolicy"
	"github.com/Cendana-Project/medikaone-api/internal/directorycriteria"
)

// SchedulePredicate uses the caller's schedule alias. A bookable candidate is a
// complete slot/session within the requested local interval, with spare capacity.
// Keeping this predicate in SQL makes pagination and doctor totals accurate.
func SchedulePredicate(q directorycriteria.Availability) (string, []any) {
	sql := `schedule.is_active = TRUE
		AND (schedule.schedule_date IS NULL OR ((schedule.schedule_date + schedule.end_time) AT TIME ZONE schedule.timezone) > CURRENT_TIMESTAMP)`
	args := []any{}
	if q.Date != "" {
		sql += ` AND (schedule.schedule_date = ?::date OR
			(schedule.schedule_date IS NULL AND schedule.day_of_week = EXTRACT(DOW FROM ?::date)::integer))
			AND ((?::date + schedule.end_time) AT TIME ZONE schedule.timezone) > CURRENT_TIMESTAMP`
		args = append(args, q.Date, q.Date, q.Date)
	}
	if q.BookingMode != "" {
		sql += " AND schedule.booking_mode = ?"
		args = append(args, q.BookingMode)
	}
	if !q.OnlyAvailable {
		if q.From != "" {
			// Schedule-only search uses interval overlap; it makes no capacity promise.
			sql += " AND schedule.start_time < ?::time AND schedule.end_time > ?::time"
			args = append(args, q.To, q.From)
		}
		return sql, args
	}
	sql += ` AND EXISTS (
		SELECT 1 FROM LATERAL (
			SELECT session_window.*,
			       CASE WHEN schedule.booking_mode = 'SESSION_QUEUE'
			            THEN ends_at - starts_at
			            ELSE make_interval(mins => schedule.slot_duration_minutes) END AS duration
			FROM (SELECT (?::date + schedule.start_time) AT TIME ZONE schedule.timezone AS starts_at,
			             (?::date + schedule.end_time) AT TIME ZONE schedule.timezone AS ends_at) session_window
		) practice_window
		CROSS JOIN LATERAL generate_series(practice_window.starts_at, practice_window.ends_at - practice_window.duration, NULLIF(practice_window.duration, INTERVAL '0')) slot(starts_at)
		WHERE practice_window.duration > INTERVAL '0'
		  AND slot.starts_at > CURRENT_TIMESTAMP + (? * INTERVAL '1 second')
		  AND slot.starts_at <= CURRENT_TIMESTAMP + (? * INTERVAL '1 second')`
	args = append(args, q.Date, q.Date, bookingpolicy.MinimumLeadTime.Seconds(), bookingpolicy.Horizon.Seconds())
	if q.From != "" {
		sql += ` AND slot.starts_at >= ((?::date + ?::time) AT TIME ZONE schedule.timezone)
			AND slot.starts_at + practice_window.duration <= ((?::date + ?::time) AT TIME ZONE schedule.timezone)`
		args = append(args, q.Date, q.From, q.Date, q.To)
	}
	sql += ` AND (SELECT COUNT(*) FROM appointments appointment
		WHERE appointment.schedule_id = schedule.id AND appointment.appointment_date = ?::date
		  AND appointment.scheduled_start_at = slot.starts_at
		  AND appointment.status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')) < schedule.capacity
	)`
	args = append(args, q.Date)
	return sql, args
}
