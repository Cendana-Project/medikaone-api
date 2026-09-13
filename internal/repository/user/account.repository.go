package user

import (
	"context"
	"errors"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrAccountHasAppointments   = errors.New("account has active appointments")
	ErrLastAccountAdministrator = errors.New("account is the last active administrator")
)

// DeleteAccount preserves clinical records and the identity referenced by audit
// events. Verification and session revocation must succeed before committing.
func (r *Repository) DeleteAccount(ctx context.Context, userID string, verify func(*entity.User) error, revoke func() error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, key := range []string{"account-lifecycle", "appointment:patient:" + userID, userID} {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, key).Error; err != nil {
				return err
			}
		}
		var account entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = 'active'", userID).First(&account).Error; err != nil {
			return err
		}
		if err := verify(&account); err != nil {
			return err
		}
		var hasAppointments bool
		if err := tx.Raw(`SELECT EXISTS (
			SELECT 1 FROM appointments
			WHERE (patient_id = ? OR doctor_id = ? OR patient_record_id IN (SELECT id FROM patient_records WHERE user_id = ?))
			AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION')
		)`, userID, userID, userID).Scan(&hasAppointments).Error; err != nil {
			return err
		}
		if hasAppointments {
			return ErrAccountHasAppointments
		}
		var lastAdmin bool
		if err := tx.Raw(`SELECT
			(EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
			 WHERE ur.user_id = ? AND UPPER(r.slug) = 'SUPER_ADMIN' AND r.active AND r.deleted_at IS NULL)
			 AND NOT EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id JOIN users u ON u.id = ur.user_id
			 WHERE u.id <> ? AND u.status = 'active' AND u.deleted_at IS NULL
			 AND UPPER(r.slug) = 'SUPER_ADMIN' AND r.active AND r.deleted_at IS NULL))
			OR EXISTS (
			 SELECT 1 FROM hospital_user_roles mine JOIN roles r ON r.id = mine.role_id
			 JOIN user_hospitals membership ON membership.hospital_id = mine.hospital_id AND membership.user_id = mine.user_id
			 JOIN hospitals hospital ON hospital.id = mine.hospital_id
			 WHERE mine.user_id = ? AND UPPER(r.slug) = 'ADMIN' AND r.active AND r.deleted_at IS NULL
			 AND membership.is_active AND membership.deleted_at IS NULL AND hospital.is_active AND hospital.deleted_at IS NULL
			 AND NOT EXISTS (
			  SELECT 1 FROM hospital_user_roles other JOIN roles role ON role.id = other.role_id
			  JOIN users u ON u.id = other.user_id
			  JOIN user_hospitals m ON m.user_id = other.user_id AND m.hospital_id = other.hospital_id
			  WHERE other.hospital_id = mine.hospital_id AND other.user_id <> ?
			  AND UPPER(role.slug) = 'ADMIN' AND role.active AND role.deleted_at IS NULL
			  AND u.status = 'active' AND u.deleted_at IS NULL AND m.is_active AND m.deleted_at IS NULL
			 ))`, userID, userID, userID, userID).Scan(&lastAdmin).Error; err != nil {
			return err
		}
		if lastAdmin {
			return ErrLastAccountAdministrator
		}
		now := time.Now().UTC()
		// Booking holds schedule rows before checking the doctor account. Do not
		// update schedules while holding this account lock: the reverse lock order
		// could deadlock. Availability and booking both require an active account,
		// so the account tombstone disables them while preserving schedule history.
		if err := tx.Exec(`UPDATE user_hospitals SET is_active = FALSE, updated_at = ? WHERE user_id = ?`, now, userID).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.User{}).Where("id = ?", userID).Updates(map[string]any{
			"status": "blocked", "deleted_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return revoke()
	})
}
