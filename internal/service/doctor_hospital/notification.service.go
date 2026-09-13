package doctor_hospital

import (
	"context"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/google/uuid"
)

type notificationArchiver interface {
	ArchiveNotification(context.Context, string, string, time.Time) error
}

func (s *Service) DeleteNotification(ctx context.Context, userID, notificationID string) error {
	if userID == "" {
		return constant.ErrUnauthorized
	}
	if _, err := uuid.Parse(notificationID); err != nil {
		return constant.ErrInvalidUUIDFormat
	}
	repo, ok := s.repo.(notificationArchiver)
	if !ok {
		return constant.ErrInternalServerError
	}
	return mapRepositoryError(repo.ArchiveNotification(ctx, userID, notificationID, s.now()))
}
