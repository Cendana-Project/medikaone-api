package hospital

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

var ErrResourceInUse = errors.New("hospital has live appointments")

const publicHospitalColumns = `id, code, name, address, city, province, country,
    latitude, longitude, phone, description, facilities, is_active, created_at, updated_at,
    email, website, established_year, timezone, opening_hours`

func (r *Repository) ListPublic(ctx context.Context, search, city string, limit, offset int) ([]response.Hospital, error) {
	return r.ListDirectory(ctx, request.HospitalDirectoryQuery{Search: search, City: city, Limit: limit, Offset: offset, Sort: "name"})
}

func (r *Repository) GetPublic(ctx context.Context, hospitalID string) (*response.Hospital, error) {
	return r.GetDirectory(ctx, hospitalID, nil, nil)
}

func (r *Repository) UpdateHospital(ctx context.Context, hospitalID string, fields map[string]any) (*response.Hospital, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.Hospital
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND is_active = TRUE AND deleted_at IS NULL", hospitalID).First(&row).Error; err != nil {
			return err
		}
		return tx.Model(&row).Updates(fields).Error
	})
	if err != nil {
		return nil, err
	}
	return r.GetPublic(ctx, hospitalID)
}

// DeleteHospital retains all clinical and contract records. The advisory locks
// match booking's lock order; the hospital row lock stops newly validated work.
func (r *Repository) DeleteHospital(ctx context.Context, hospitalID, actorID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var affiliationIDs []string
		if err := tx.Table("doctor_hospital_affiliations").Where("hospital_id = ? AND deleted_at IS NULL", hospitalID).
			Order("id").Pluck("id", &affiliationIDs).Error; err != nil {
			return err
		}
		for _, id := range affiliationIDs {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "appointment:affiliation:"+id).Error; err != nil {
				return err
			}
		}
		var row entity.Hospital
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", hospitalID).First(&row).Error; err != nil {
			return err
		}
		var busy bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM appointments WHERE hospital_id = ?
			AND status IN ('CONFIRMED','CHECKED_IN','WAITING_VITALS','WAITING_DOCTOR','IN_CONSULTATION'))`, hospitalID).Scan(&busy).Error; err != nil {
			return err
		}
		if busy {
			return ErrResourceInUse
		}

		if err := tx.Exec(`INSERT INTO doctor_hospital_affiliation_events
			(id, affiliation_id, actor_id, event_type, from_status, to_status, metadata, created_at)
			SELECT gen_random_uuid(), id, ?, 'DELETED', status, 'SUSPENDED', '{"reason":"hospital_deleted"}'::jsonb, ?
			FROM doctor_hospital_affiliations WHERE hospital_id = ? AND deleted_at IS NULL`, actorID, now, hospitalID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE doctor_hospital_schedules SET is_active = FALSE, updated_at = ?
			WHERE affiliation_id IN (SELECT id FROM doctor_hospital_affiliations WHERE hospital_id = ?)`, now, hospitalID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE doctor_hospital_affiliations SET status = 'SUSPENDED', deleted_at = ?, updated_at = ? WHERE hospital_id = ? AND deleted_at IS NULL`, now, now, hospitalID).Error; err != nil {
			return err
		}
		var invitations []struct{ ID string }
		if err := tx.Raw(`SELECT id FROM doctor_hospital_invitations WHERE hospital_id = ? AND status = 'PENDING' AND deleted_at IS NULL FOR UPDATE`, hospitalID).Scan(&invitations).Error; err != nil {
			return err
		}
		for _, invitation := range invitations {
			if err := tx.Exec(`INSERT INTO doctor_hospital_invitation_events (id, invitation_id, actor_id, event_type, metadata, created_at)
				VALUES (?, ?, ?, 'CANCELLED', '{"reason":"hospital_deleted"}'::jsonb, ?)`, uuid.NewString(), invitation.ID, actorID, now).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`UPDATE doctor_hospital_invitations SET status = 'CANCELLED', cancelled_at = ?, updated_at = ?
			WHERE hospital_id = ? AND status = 'PENDING' AND deleted_at IS NULL`, now, now, hospitalID).Error; err != nil {
			return err
		}
		for _, table := range []string{"hospital_rooms", "hospital_departments", "user_hospitals"} {
			if err := tx.Table(table).Where("hospital_id = ?", hospitalID).Updates(map[string]any{"is_active": false, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&row).Updates(map[string]any{"is_active": false, "deleted_at": now, "updated_at": now}).Error
	})
}
