package doctor_hospital

import (
	"context"
	"time"
)

// ArchiveNotification hides an owned notification while retaining its audit data.
func (r *Repository) ArchiveNotification(ctx context.Context, userID, notificationID string, now time.Time) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE notifications SET deleted_at = COALESCE(deleted_at, ?)
		WHERE id = ? AND user_id = ?`, now, notificationID, userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}
	return nil
}
