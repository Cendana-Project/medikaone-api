package hospital

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"gorm.io/gorm"
)

var ErrReviewVisitRequired = errors.New("completed hospital visit required")

const reviewSelect = `SELECT review.id,review.hospital_id,review.rating,review.comment,
    CASE WHEN u.status='active' AND u.deleted_at IS NULL THEN COALESCE(NULLIF(TRIM(u.first_name),''),'Pasien') ELSE 'Pasien' END AS reviewer_name,
    TRUE AS verified_visit,review.created_at,review.updated_at
    FROM hospital_reviews review LEFT JOIN users u ON u.id=review.user_id`

func (r *Repository) ListReviews(ctx context.Context, hospitalID string, limit, offset int, sort string) (*response.HospitalReviewPage, error) {
	page := &response.HospitalReviewPage{Items: []response.HospitalReview{}, Limit: limit, Offset: offset, RatingDistribution: map[string]int64{"1": 0, "2": 0, "3": 0, "4": 0, "5": 0}}
	// A snapshot keeps the summary and page consistent during concurrent edits.
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.WithTx(tx).FindByID(ctx, hospitalID); err != nil {
			return err
		}
		var counts []struct {
			Rating int
			Count  int64
		}
		if err := tx.Table("hospital_reviews").Select("rating, COUNT(*) AS count").Where("hospital_id=? AND deleted_at IS NULL", hospitalID).Group("rating").Scan(&counts).Error; err != nil {
			return err
		}
		var sum int64
		for _, row := range counts {
			page.Total += row.Count
			sum += int64(row.Rating) * row.Count
			page.RatingDistribution[strconv.Itoa(row.Rating)] = row.Count
		}
		if page.Total > 0 {
			avg := float64(sum) / float64(page.Total)
			page.RatingAverage = &avg
		}
		order := "review.created_at DESC, review.id DESC"
		if sort == "highest" {
			order = "review.rating DESC, " + order
		}
		if sort == "lowest" {
			order = "review.rating ASC, " + order
		}
		return tx.Raw(reviewSelect+" WHERE review.hospital_id=? AND review.deleted_at IS NULL ORDER BY "+order+" LIMIT ? OFFSET ?", hospitalID, limit, offset).Scan(&page.Items).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return page, err
}

func (r *Repository) GetOwnReview(ctx context.Context, hospitalID, userID string) (*response.HospitalReview, error) {
	if _, err := r.FindByID(ctx, hospitalID); err != nil {
		return nil, err
	}
	return ownReview(r.db.WithContext(ctx), hospitalID, userID)
}
func ownReview(tx *gorm.DB, hospitalID, userID string) (*response.HospitalReview, error) {
	var row response.HospitalReview
	if err := tx.Raw(reviewSelect+" WHERE review.hospital_id=? AND review.user_id=? AND review.deleted_at IS NULL", hospitalID, userID).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}
func (r *Repository) PutReview(ctx context.Context, hospitalID, userID string, rating int, comment string, now time.Time) (*response.HospitalReview, error) {
	var result *response.HospitalReview
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectoryHospital(tx, hospitalID, "SHARE"); err != nil {
			return err
		}
		var id string
		if err := tx.Raw(`SELECT id FROM users WHERE id=? AND status='active' AND deleted_at IS NULL AND verified_at IS NOT NULL FOR SHARE`, userID).Scan(&id).Error; err != nil {
			return err
		}
		if id == "" {
			return ErrReviewVisitRequired
		}
		var eligible bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM appointments appointment
            LEFT JOIN patient_records patient ON patient.id=appointment.patient_record_id
            WHERE appointment.hospital_id=? AND appointment.status='COMPLETED'
            AND COALESCE(patient.user_id,appointment.patient_id)=?)`, hospitalID, userID).Scan(&eligible).Error; err != nil {
			return err
		}
		if !eligible {
			return ErrReviewVisitRequired
		}
		if err := tx.Exec(`INSERT INTO hospital_reviews (hospital_id,user_id,rating,comment,created_at,updated_at)
            VALUES (?,?,?,?,?,?) ON CONFLICT(hospital_id,user_id) DO UPDATE SET rating=EXCLUDED.rating,comment=EXCLUDED.comment,
            updated_at=EXCLUDED.updated_at,deleted_at=NULL`, hospitalID, userID, rating, comment, now, now).Error; err != nil {
			return err
		}
		var err error
		result, err = ownReview(tx, hospitalID, userID)
		return err
	})
	return result, err
}
func (r *Repository) DeleteOwnReview(ctx context.Context, hospitalID, userID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectoryHospital(tx, hospitalID, "SHARE"); err != nil {
			return err
		}
		result := tx.Exec("UPDATE hospital_reviews SET deleted_at=?,updated_at=? WHERE hospital_id=? AND user_id=? AND deleted_at IS NULL", now, now, hospitalID, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}
func lockDirectoryHospital(tx *gorm.DB, id, strength string) error {
	clause := "FOR SHARE"
	if strength == "UPDATE" {
		clause = "FOR UPDATE"
	}
	var found string
	if err := tx.Raw("SELECT id FROM hospitals WHERE id=? AND is_active AND deleted_at IS NULL "+clause, id).Scan(&found).Error; err != nil {
		return err
	}
	if found == "" {
		return gorm.ErrRecordNotFound
	}
	return nil
}
