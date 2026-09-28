package appointment

import (
	"time"

	"gorm.io/gorm"
)

func validateDeactivationInput(input ScheduleChangeInput) error {
	if input.Operation != "DEACTIVATE" {
		if input.DeactivationScope != "" || input.DeactivationDayOfWeek != nil {
			return ErrInvalidScheduleChangeState
		}
		for _, item := range input.Schedules {
			if item.TargetScheduleID != nil {
				return ErrInvalidScheduleChangeState
			}
		}
		return nil
	}
	if input.TargetScheduleID != nil {
		return ErrInvalidScheduleChangeState
	}
	switch input.DeactivationScope {
	case "ALL":
		if input.DeactivationDayOfWeek != nil {
			return ErrInvalidScheduleChangeState
		}
	case "RECURRING_DAY":
		if input.DeactivationDayOfWeek == nil || *input.DeactivationDayOfWeek < 0 || *input.DeactivationDayOfWeek > 6 {
			return ErrInvalidScheduleChangeState
		}
	default:
		return ErrInvalidScheduleChangeState
	}
	return nil
}

// The affiliation and doctor advisory locks are already held. Freeze IDs and
// display values at request time so approval cannot silently affect later rows.
func deactivationSnapshot(tx *gorm.DB, input ScheduleChangeInput) ([]ScheduleItem, error) {
	items := make([]ScheduleItem, 0)
	err := tx.Raw(`SELECT id AS target_schedule_id, day_of_week, schedule_date::text AS schedule_date,
		TO_CHAR(start_time, 'HH24:MI') AS start_time, TO_CHAR(end_time, 'HH24:MI') AS end_time,
		timezone, booking_mode, slot_duration_minutes, capacity
		FROM doctor_hospital_schedules
		WHERE affiliation_id = ? AND is_active = TRUE
		AND (? = 'ALL' OR (schedule_date IS NULL AND day_of_week = ?))
		ORDER BY id FOR SHARE`, input.AffiliationID, input.DeactivationScope, input.DeactivationDayOfWeek).Scan(&items).Error
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrScheduleNotFound
	}
	return items, nil
}

// Pending deactivation freezes only its scope. Unrelated specific additions and
// changes to other weekdays may coexist with a recurring-day deactivation.
func deactivationPendingConflict(tx *gorm.DB, input ScheduleChangeInput, excludeID string, now time.Time) (bool, error) {
	type pendingMutation struct {
		Operation             string
		DeactivationScope     string
		DeactivationDayOfWeek *int
		TargetRecurringDay    *int
	}
	var rows []pendingMutation
	where := "change.affiliation_id = ? AND change.status = 'PENDING' AND change.id::text <> ?"
	args := []any{input.AffiliationID, excludeID}
	if excludeID != "" {
		where += " AND change.expires_at > ?"
		args = append(args, now)
	}
	// Creation has already expired every unlocked pending row. A remaining
	// expired row was skipped because another transaction owns it; report the
	// transient pending conflict instead of racing its pending unique index.
	if err := tx.Raw(`SELECT change.operation, change.deactivation_scope, change.deactivation_day_of_week,
		CASE WHEN target.schedule_date IS NULL THEN target.day_of_week END AS target_recurring_day
		FROM doctor_schedule_change_requests change
		LEFT JOIN doctor_hospital_schedules target ON target.id = change.target_schedule_id
		WHERE `+where, args...).Scan(&rows).Error; err != nil {
		return false, err
	}
	var targetDay struct{ DayOfWeek *int }
	if input.Operation == "REMOVE" && input.TargetScheduleID != nil {
		if err := tx.Raw(`SELECT day_of_week FROM doctor_hospital_schedules
			WHERE id = ? AND affiliation_id = ? AND schedule_date IS NULL`, input.TargetScheduleID, input.AffiliationID).Scan(&targetDay).Error; err != nil {
			return false, err
		}
	}
	for _, row := range rows {
		if input.Operation == "DEACTIVATE" && input.DeactivationScope == "ALL" || row.Operation == "DEACTIVATE" && row.DeactivationScope == "ALL" {
			return true, nil
		}
		if input.Operation == "DEACTIVATE" {
			switch row.Operation {
			case "REPLACE":
				return true, nil
			case "DEACTIVATE":
				if sameDeactivationDay(input.DeactivationDayOfWeek, row.DeactivationDayOfWeek) {
					return true, nil
				}
			case "REMOVE":
				if sameDeactivationDay(input.DeactivationDayOfWeek, row.TargetRecurringDay) {
					return true, nil
				}
			}
		} else if row.Operation == "DEACTIVATE" {
			if input.Operation == "REPLACE" || input.Operation == "REMOVE" && sameDeactivationDay(targetDay.DayOfWeek, row.DeactivationDayOfWeek) {
				return true, nil
			}
		}
	}
	return false, nil
}

func sameDeactivationDay(left, right *int) bool {
	return left != nil && right != nil && *left == *right
}

func approveScheduleDeactivation(tx *gorm.DB, changeID string, input ScheduleChangeInput, now time.Time) error {
	if err := validateDeactivationInput(input); err != nil {
		return err
	}
	if conflict, err := deactivationPendingConflict(tx, input, changeID, now); err != nil {
		return err
	} else if conflict {
		return ErrScheduleChangeExists
	}
	var snapshot []struct{ TargetScheduleID *string }
	if err := tx.Raw(`SELECT target_schedule_id FROM doctor_schedule_change_items WHERE change_request_id = ?`, changeID).Scan(&snapshot).Error; err != nil {
		return err
	}
	if len(snapshot) == 0 {
		return ErrInvalidScheduleChangeState
	}
	ids := make([]string, 0, len(snapshot))
	seen := make(map[string]bool, len(snapshot))
	for _, item := range snapshot {
		if item.TargetScheduleID == nil || seen[*item.TargetScheduleID] {
			return ErrInvalidScheduleChangeState
		}
		seen[*item.TargetScheduleID] = true
		ids = append(ids, *item.TargetScheduleID)
	}
	var activeIDs []string
	if err := tx.Raw(`SELECT id FROM doctor_hospital_schedules
		WHERE id IN ? AND affiliation_id = ? AND is_active = TRUE
		AND (? = 'ALL' OR (schedule_date IS NULL AND day_of_week = ?))
		ORDER BY id FOR UPDATE`, ids, input.AffiliationID, input.DeactivationScope, input.DeactivationDayOfWeek).Scan(&activeIDs).Error; err != nil {
		return err
	}
	if len(activeIDs) != len(ids) {
		return ErrInvalidScheduleChangeState
	}
	var activeAppointments bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM appointments WHERE affiliation_id = ? AND schedule_id IN ?
		AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION'))`, input.AffiliationID, ids).Scan(&activeAppointments).Error; err != nil {
		return err
	}
	if activeAppointments {
		return ErrScheduleChangeAppointments
	}
	result := tx.Exec(`UPDATE doctor_hospital_schedules SET is_active = FALSE, updated_at = ?
		WHERE id IN ? AND affiliation_id = ? AND is_active = TRUE`, now, ids, input.AffiliationID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != int64(len(ids)) {
		return ErrInvalidScheduleChangeState
	}
	return nil
}

func finishScheduleChangeApproval(tx *gorm.DB, changeID, affiliationID, actorID, actorParty, body string, now time.Time) error {
	if err := tx.Exec(`UPDATE doctor_schedule_change_requests
		SET status = 'APPROVED', reviewed_by = ?, reviewed_at = ?, updated_at = ? WHERE id = ?`, actorID, now, now, changeID).Error; err != nil {
		return err
	}
	if err := insertScheduleChangeEvent(tx, changeID, actorID, "APPROVED", now); err != nil {
		return err
	}
	return notifyScheduleCounterpart(tx, affiliationID, actorParty, "SCHEDULE_CHANGE_APPROVED", "Perubahan jadwal disetujui", body, changeID, now)
}

// Transport supplies tenant permission checks; repeat their active scope inside
// the write transaction so a stale service read cannot authorize deactivation.
func authorizeDeactivationActor(tx *gorm.DB, affiliationID, actorID, actorParty, permission string) (bool, error) {
	var allowed bool
	err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM doctor_hospital_affiliations affiliation
		JOIN hospitals hospital ON hospital.id = affiliation.hospital_id
		JOIN users actor ON actor.id = CAST(? AS uuid)
		WHERE affiliation.id = ? AND affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL
		AND hospital.is_active = TRUE AND hospital.deleted_at IS NULL
		AND actor.status = 'active' AND actor.deleted_at IS NULL
		AND ((? = 'DOCTOR' AND affiliation.doctor_id = actor.id)
		OR (? = 'HOSPITAL' AND (
			EXISTS(SELECT 1 FROM user_roles assignment JOIN roles role ON role.id = assignment.role_id
				WHERE assignment.user_id = actor.id AND UPPER(role.slug) = 'SUPER_ADMIN'
				AND role.active = TRUE AND role.deleted_at IS NULL)
			OR EXISTS(SELECT 1 FROM hospital_user_roles assignment
				JOIN user_hospitals membership ON membership.user_id = assignment.user_id AND membership.hospital_id = assignment.hospital_id
				JOIN roles role ON role.id = assignment.role_id
				JOIN role_permissions grant_permission ON grant_permission.role_id = role.id
				JOIN permissions permission ON permission.id = grant_permission.permission_id
				WHERE assignment.user_id = actor.id AND assignment.hospital_id = affiliation.hospital_id
				AND membership.is_active = TRUE AND membership.deleted_at IS NULL
				AND role.active = TRUE AND role.deleted_at IS NULL
				AND permission.slug = ? AND permission.is_active = TRUE AND permission.deleted_at IS NULL)
		))))`, actorID, affiliationID, actorParty, actorParty, permission).Scan(&allowed).Error
	return allowed, err
}
