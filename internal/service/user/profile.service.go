package user

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	storageclient "github.com/Cendana-Project/medikaone-api/internal/storage"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

const (
	maxProfilePhotoSize   = int64(10 * 1024 * 1024)
	maxProfilePhotoWidth  = 4096
	maxProfilePhotoHeight = 4096
)

type Repository interface {
	GetByID(context.Context, string) (*entity.User, error)
	ExistsUsernameExcludingUser(context.Context, string, string) (bool, error)
	ExistsNIKExcludingUser(context.Context, string, string) (bool, error)
	UpdateByID(context.Context, string, map[string]any) error
	ReplaceProfileImage(context.Context, string, repository.ProfileImageRecord) (*repository.ProfileImageRecord, error)
	ClearProfileImage(context.Context, string, time.Time) (*repository.ProfileImageRecord, error)
}

type UploadedPhoto struct {
	Content []byte
}

type Service struct {
	repo         Repository
	storage      storageclient.Client
	maxFileSize  int64
	signedURLTTL time.Duration
	now          func() time.Time
}

func NewService(repo Repository, storage storageclient.Client, configuredMaxFileSize int64, signedURLTTL time.Duration) *Service {
	if configuredMaxFileSize <= 0 || configuredMaxFileSize > maxProfilePhotoSize {
		configuredMaxFileSize = maxProfilePhotoSize
	}
	if signedURLTTL <= 0 {
		signedURLTTL = 5 * time.Minute
	}
	return &Service{
		repo: repo, storage: storage, maxFileSize: configuredMaxFileSize,
		signedURLTTL: signedURLTTL, now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) MaxFileSize() int64 { return s.maxFileSize }

func (s *Service) Update(ctx context.Context, userID string, input request.UpdateUserProfileRequest) error {
	if err := util.ValidateStruct(&input); err != nil {
		return util.MapValidationError(err)
	}
	if !hasProfileUpdate(input) {
		return constant.ErrProfileUpdateEmpty
	}
	if _, err := s.repo.GetByID(ctx, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constant.ErrUserNotFound
		}
		return constant.ErrInternalServerError
	}

	updates := make(map[string]any)
	if input.Username != nil {
		value := strings.ToLower(strings.TrimSpace(*input.Username))
		if value == "" {
			return constant.ErrInvalidUsername
		}
		exists, err := s.repo.ExistsUsernameExcludingUser(ctx, value, userID)
		if err != nil {
			return constant.ErrInternalServerError
		}
		if exists {
			return constant.ErrUsernameAlreadyExists
		}
		updates["username"] = value
	}
	if input.FirstName != nil {
		value := strings.TrimSpace(*input.FirstName)
		if value == "" {
			return constant.NewFieldRequiredError("first_name")
		}
		updates["first_name"] = value
	}
	if input.LastName != nil {
		updates["last_name"] = strings.TrimSpace(*input.LastName)
	}
	if input.Phone != nil {
		value := strings.TrimSpace(*input.Phone)
		if value == "" {
			updates["phone"] = nil
		} else {
			if len(value) < 8 || len(value) > 32 {
				return constant.NewInvalidFieldLengthError("phone", "between 8 and 32 characters long", "memiliki 8 sampai 32 karakter")
			}
			updates["phone"] = value
		}
	}
	if input.DOB != nil {
		value := strings.TrimSpace(*input.DOB)
		if value == "" {
			updates["dob"] = nil
		} else {
			date, err := time.Parse("2006-01-02", value)
			if err != nil || date.After(s.now()) {
				return constant.ErrInvalidDateFormat
			}
			updates["dob"] = date
		}
	}
	if input.Address != nil {
		value := strings.TrimSpace(*input.Address)
		if value == "" {
			updates["address"] = nil
		} else {
			updates["address"] = value
		}
	}
	if input.Gender != nil {
		value := strings.ToUpper(strings.TrimSpace(*input.Gender))
		if value == "" {
			updates["gender"] = nil
		} else if value != "L" && value != "P" {
			return constant.NewInvalidFieldValueError("gender", "L or P", "L atau P")
		} else {
			updates["gender"] = value
		}
	}
	if input.NIK != nil {
		value := strings.TrimSpace(*input.NIK)
		if value == "" {
			updates["nik"] = nil
		} else {
			if !isNIK(value) {
				return constant.NewInvalidFieldValueError("nik", "exactly 16 numeric digits", "tepat 16 digit angka")
			}
			exists, err := s.repo.ExistsNIKExcludingUser(ctx, value, userID)
			if err != nil {
				return constant.ErrInternalServerError
			}
			if exists {
				return constant.ErrDuplicateNIK
			}
			updates["nik"] = value
		}
	}

	if err := s.repo.UpdateByID(ctx, userID, updates); err != nil {
		return mapProfileUpdateError(err)
	}
	return nil
}

func (s *Service) UploadPhoto(ctx context.Context, userID string, file UploadedPhoto) (*response.ProfilePhotoMetadata, error) {
	contentType, extension, err := s.validatePhoto(file.Content)
	if err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, constant.ErrStorageUnavailable
	}
	now := s.now()
	objectPath := fmt.Sprintf("users/%s/%s%s", userID, uuid.NewString(), extension)
	uploaded, err := s.storage.Upload(ctx, objectPath, contentType, file.Content)
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	image := repository.ProfileImageRecord{
		Bucket: uploaded.Bucket, ObjectPath: uploaded.ObjectPath, ContentType: contentType,
		FileSize: uploaded.FileSize, UpdatedAt: now,
	}
	previous, err := s.repo.ReplaceProfileImage(ctx, userID, image)
	if err != nil {
		cleanupObject(s.storage, ctx, uploaded.ObjectPath)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constant.ErrUserNotFound
		}
		return nil, constant.ErrInternalServerError
	}
	if previous != nil && previous.ObjectPath != "" && previous.ObjectPath != uploaded.ObjectPath {
		cleanupObject(s.storage, ctx, previous.ObjectPath)
	}
	return &response.ProfilePhotoMetadata{ContentType: contentType, FileSize: uploaded.FileSize, UpdatedAt: now}, nil
}

func (s *Service) GetPhotoURL(ctx context.Context, userID string) (*response.ProfilePhotoURL, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) || user == nil {
		return nil, constant.ErrUserNotFound
	}
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if user.AvatarObjectPath == nil || strings.TrimSpace(*user.AvatarObjectPath) == "" {
		return nil, constant.ErrProfilePhotoNotFound
	}
	if s.storage == nil {
		return nil, constant.ErrStorageUnavailable
	}
	// Empty downloadName asks Supabase for an inline URL suitable for an <img>
	// element instead of forcing Content-Disposition: attachment.
	url, err := s.storage.CreateSignedURL(ctx, *user.AvatarObjectPath, s.signedURLTTL, "")
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	return &response.ProfilePhotoURL{URL: url, ExpiresAt: s.now().Add(s.signedURLTTL)}, nil
}

func (s *Service) DeletePhoto(ctx context.Context, userID string) error {
	previous, err := s.repo.ClearProfileImage(ctx, userID, s.now())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return constant.ErrUserNotFound
	}
	if err != nil {
		return constant.ErrInternalServerError
	}
	if previous == nil || previous.ObjectPath == "" {
		return constant.ErrProfilePhotoNotFound
	}
	if s.storage != nil {
		cleanupObject(s.storage, ctx, previous.ObjectPath)
	}
	return nil
}

func (s *Service) validatePhoto(content []byte) (string, string, error) {
	if len(content) == 0 || int64(len(content)) > s.maxFileSize {
		return "", "", constant.ErrProfilePhotoInvalid
	}
	contentType := http.DetectContentType(content)
	extension := ""
	switch contentType {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	default:
		return "", "", constant.ErrProfilePhotoInvalid
	}
	decoded, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return "", "", constant.ErrProfilePhotoInvalid
	}
	dimensions := decoded.Bounds()
	if dimensions.Dx() < 1 || dimensions.Dy() < 1 ||
		dimensions.Dx() > maxProfilePhotoWidth || dimensions.Dy() > maxProfilePhotoHeight {
		return "", "", constant.ErrProfilePhotoInvalid
	}
	return contentType, extension, nil
}

func hasProfileUpdate(input request.UpdateUserProfileRequest) bool {
	return input.Username != nil || input.FirstName != nil || input.LastName != nil || input.Phone != nil ||
		input.DOB != nil || input.Address != nil || input.Gender != nil || input.NIK != nil
}

func isNIK(value string) bool {
	if len(value) != 16 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func mapProfileUpdateError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return constant.ErrConflict
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.ConstraintName {
		case "ux_users_username_lower":
			return constant.ErrUsernameAlreadyExists
		case "ux_users_nik_active":
			return constant.ErrDuplicateNIK
		}
	}
	return constant.ErrInternalServerError
}

func cleanupObject(storage storageclient.Client, parent context.Context, objectPath string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	if err := storage.Delete(cleanupCtx, objectPath); err != nil {
		logrus.WithError(err).Warn("profile photo object cleanup failed")
	}
}
