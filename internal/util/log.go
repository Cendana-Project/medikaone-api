package util

import (
	"context"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/sirupsen/logrus"
)

// NewDefaultLogger membuat logger dengan field requestID & userID diambil dari context.
func NewDefaultLogger(ctx context.Context) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"requestID": ctx.Value(constant.RequestID),
		"userID":    ctx.Value(constant.UserID),
	})
}

// Convenience wrappers
func Infof(ctx context.Context, format string, args ...any) {
	NewDefaultLogger(ctx).Infof(format, args...)
}
func Errorf(ctx context.Context, format string, args ...any) {
	NewDefaultLogger(ctx).Errorf(format, args...)
}
