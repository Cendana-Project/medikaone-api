package hospital

import (
	"context"
	"errors"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"gorm.io/gorm"
)

var ErrHospitalGalleryFull = errors.New("hospital gallery has 20 images")

func (r *Repository) AddImage(ctx context.Context, image response.HospitalImage, actorID string) (*response.HospitalImage, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectoryHospital(tx, image.HospitalID, "UPDATE"); err != nil {
			return err
		}
		var count int64
		if err := tx.Table("hospital_images").Where("hospital_id=? AND deleted_at IS NULL", image.HospitalID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 20 {
			return ErrHospitalGalleryFull
		}
		image.IsCover = image.IsCover || count == 0
		if image.IsCover {
			if err := tx.Exec("UPDATE hospital_images SET is_cover=FALSE,updated_at=? WHERE hospital_id=? AND is_cover AND deleted_at IS NULL", image.CreatedAt, image.HospitalID).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`INSERT INTO hospital_images (id,hospital_id,bucket,object_path,content_type,file_size,caption,sort_order,is_cover,uploaded_by,created_at,updated_at)
            VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, image.ID, image.HospitalID, image.Bucket, image.ObjectPath, image.ContentType, image.FileSize, image.Caption, image.SortOrder, image.IsCover, actorID, image.CreatedAt, image.CreatedAt).Error
	})
	return &image, err
}

func (r *Repository) UpdateImage(ctx context.Context, hospitalID, imageID string, fields map[string]any, now time.Time) (*response.HospitalImage, error) {
	var row response.HospitalImage
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectoryHospital(tx, hospitalID, "UPDATE"); err != nil {
			return err
		}
		if err := tx.Table("hospital_images").Where("id=? AND hospital_id=? AND deleted_at IS NULL", imageID, hospitalID).Take(&row).Error; err != nil {
			return err
		}
		if cover, ok := fields["is_cover"].(bool); ok && cover {
			if err := tx.Exec("UPDATE hospital_images SET is_cover=FALSE,updated_at=? WHERE hospital_id=? AND is_cover AND deleted_at IS NULL", now, hospitalID).Error; err != nil {
				return err
			}
		}
		fields["updated_at"] = now
		if err := tx.Table("hospital_images").Where("id=?", imageID).Updates(fields).Error; err != nil {
			return err
		}
		return tx.Table("hospital_images").Where("id=?", imageID).Take(&row).Error
	})
	return &row, err
}
func (r *Repository) DeleteImage(ctx context.Context, hospitalID, imageID string, now time.Time) (*response.HospitalImage, error) {
	var row response.HospitalImage
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectoryHospital(tx, hospitalID, "UPDATE"); err != nil {
			return err
		}
		if err := tx.Table("hospital_images").Where("id=? AND hospital_id=? AND deleted_at IS NULL", imageID, hospitalID).Take(&row).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE hospital_images SET deleted_at=?,updated_at=?,is_cover=FALSE WHERE id=?", now, now, imageID).Error; err != nil {
			return err
		}
		if row.IsCover {
			return tx.Exec(`UPDATE hospital_images SET is_cover=TRUE,updated_at=? WHERE id=(SELECT id FROM hospital_images WHERE hospital_id=? AND deleted_at IS NULL ORDER BY sort_order,created_at,id LIMIT 1)`, now, hospitalID).Error
		}
		return nil
	})
	return &row, err
}
