package storage

import (
	"context"
	"time"
)

type UploadedObject struct {
	Bucket     string
	ObjectPath string
	FileSize   int64
}

type Client interface {
	Upload(ctx context.Context, objectPath, contentType string, content []byte) (*UploadedObject, error)
	Delete(ctx context.Context, objectPath string) error
	CreateSignedURL(ctx context.Context, objectPath string, ttl time.Duration, downloadName string) (string, error)
}
