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
	maxProfilePhotoSize           = int64(10 * 1024 * 1024)
	maxProfilePhotoWidth          = 4096
	maxProfilePhotoHeight         = 4096
	minimumHospitalWorkerAgeYears = 15
)

type Repository interface {
	GetByID(context.Context, string) (*entity.User, error)
	GetDoctorProfileByUserID(context.Context, string) (*string, *string, error)
	ExistsUsernameExcludingUser(context.Context, string, string) (bool, error)
	ExistsNIKExcludingUser(context.Context, string, string) (bool, error)
	ExistsSIPExcludingUser(context.Context, string, string) (bool, error)
	UserHasActiveGlobalRole(context.Context, string, string) (bool, error)
	UserHasAnyActiveHospitalRole(context.Context, string, string) (bool, error)
	UserIsActiveHospitalWorker(context.Context, string) (bool, error)
	ApplyUnifiedProfileUpdate(context.Context, string, repository.UnifiedProfileUpdate) error
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
	if err := validateUnifiedProfileInput(input); err != nil {
		return err
	}
	if !hasProfileUpdate(input) {
		return constant.ErrProfileUpdateEmpty
	}
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return constant.ErrUserNotFound
		}
		return constant.ErrInternalServerError
	}
	if user == nil {
		return constant.ErrUserNotFound
	}

	patientUpdateRequested := hasPatientProfileUpdate(input.PatientProfile)
	doctorUpdateRequested := hasDoctorProfileUpdate(input.DoctorProfile)

	hasDoctorRole := false
	if doctorUpdateRequested {
		var err error
		hasDoctorRole, err = s.repo.UserHasActiveGlobalRole(ctx, userID, constant.RoleDoctor)
		if err != nil {
			return constant.ErrInternalServerError
		}
		if !hasDoctorRole {
			hasDoctorRole, err = s.repo.UserHasAnyActiveHospitalRole(ctx, userID, constant.RoleDoctor)
			if err != nil {
				return constant.ErrInternalServerError
			}
		}
	}
	if doctorUpdateRequested && !hasDoctorRole {
		return constant.ErrDoctorRoleRequired
	}
	if doctorUpdateRequested && !input.DoctorProfile.SIPNumber.IsSet() {
		currentSIP, _, err := s.repo.GetDoctorProfileByUserID(ctx, userID)
		if err != nil {
			return constant.ErrInternalServerError
		}
		if currentSIP == nil || strings.TrimSpace(*currentSIP) == "" {
			return constant.NewFieldRequiredError("doctor_profile.sip_number")
		}
	}
	isHospitalWorker, err := s.repo.UserIsActiveHospitalWorker(ctx, userID)
	if err != nil {
		return constant.ErrInternalServerError
	}
	if patientUpdateRequested {
		hasPatientRole, err := s.repo.UserHasActiveGlobalRole(ctx, userID, constant.RolePatient)
		if err != nil {
			return constant.ErrInternalServerError
		}
		if !hasPatientRole {
			return constant.ErrPatientRoleRequired
		}
	}

	updates := make(map[string]any)
	if raw, present := input.Username.Get(); present {
		if raw == nil {
			return constant.NewFieldRequiredError("username")
		}
		value := strings.ToLower(strings.TrimSpace(*raw))
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
	if raw, present := input.FirstName.Get(); present {
		if raw == nil {
			return constant.NewFieldRequiredError("first_name")
		}
		value := strings.TrimSpace(*raw)
		if value == "" {
			return constant.NewFieldRequiredError("first_name")
		}
		updates["first_name"] = value
	}
	if raw, present := input.LastName.Get(); present {
		if raw == nil || strings.TrimSpace(*raw) == "" {
			return constant.NewFieldRequiredError("last_name")
		}
		updates["last_name"] = strings.TrimSpace(*raw)
	}
	if raw, present := input.Phone.Get(); present {
		if raw == nil {
			updates["phone"] = nil
		} else {
			value := strings.TrimSpace(*raw)
			if value == "" {
				updates["phone"] = nil
			} else {
				if len(value) < 8 || len(value) > 32 {
					return constant.NewInvalidFieldLengthError("phone", "between 8 and 32 characters long", "memiliki 8 sampai 32 karakter")
				}
				updates["phone"] = value
			}
		}
	}
	if raw, present := input.DOB.Get(); present {
		if raw == nil {
			if isHospitalWorker {
				return constant.NewFieldRequiredError("dob")
			}
			updates["dob"] = nil
		} else {
			value := strings.TrimSpace(*raw)
			if value == "" {
				if isHospitalWorker {
					return constant.NewFieldRequiredError("dob")
				}
				updates["dob"] = nil
			} else {
				date, err := time.Parse("2006-01-02", value)
				today := dateOnlyUTC(s.now())
				if err != nil || date.After(today) {
					return constant.ErrInvalidDateFormat
				}
				if isHospitalWorker && !date.AddDate(minimumHospitalWorkerAgeYears, 0, 0).Before(today) {
					return constant.ErrHospitalWorkerMinimumAge
				}
				updates["dob"] = date
			}
		}
	}
	if isHospitalWorker && !input.DOB.IsSet() {
		if user.DOB == nil {
			return constant.NewFieldRequiredError("dob")
		}
		if !user.DOB.AddDate(minimumHospitalWorkerAgeYears, 0, 0).Before(dateOnlyUTC(s.now())) {
			return constant.ErrHospitalWorkerMinimumAge
		}
	}

	if raw, present := input.Address.Get(); present {
		if raw == nil || strings.TrimSpace(*raw) == "" {
			updates["address"] = nil
		} else {
			updates["address"] = strings.TrimSpace(*raw)
		}
	}
	if raw, present := input.Gender.Get(); present {
		if raw == nil || strings.TrimSpace(*raw) == "" {
			updates["gender"] = nil
		} else {
			value := strings.ToUpper(strings.TrimSpace(*raw))
			if value != "L" && value != "P" {
				return constant.NewInvalidFieldValueError("gender", "L or P", "L atau P")
			}
			updates["gender"] = value
		}
	}
	if raw, present := input.NIK.Get(); present {
		if raw == nil || strings.TrimSpace(*raw) == "" {
			updates["nik"] = nil
		} else {
			value := strings.TrimSpace(*raw)
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

	patientUpdates := make(map[string]any)
	if input.PatientProfile != nil {
		if value, present := input.PatientProfile.HeightCM.Get(); present {
			if value == nil {
				patientUpdates["height_cm"] = nil
			} else {
				patientUpdates["height_cm"] = *value
			}
		}
		if value, present := input.PatientProfile.WeightKG.Get(); present {
			if value == nil {
				patientUpdates["weight_kg"] = nil
			} else {
				patientUpdates["weight_kg"] = *value
			}
		}
		if value, present := input.PatientProfile.Allergies.Get(); present {
			if value == nil {
				patientUpdates["allergies"] = nil
			} else {
				patientUpdates["allergies"] = nullableTrimmed(*value)
			}
		}
		if value, present := input.PatientProfile.MedicalHistory.Get(); present {
			if value == nil {
				patientUpdates["medical_hist"] = nil
			} else {
				patientUpdates["medical_hist"] = nullableTrimmed(*value)
			}
		}
	}

	doctorUpdates := make(map[string]any)
	if input.DoctorProfile != nil {
		if raw, present := input.DoctorProfile.SIPNumber.Get(); present {
			if raw == nil || strings.TrimSpace(*raw) == "" {
				return constant.NewFieldRequiredError("doctor_profile.sip_number")
			}
			value := strings.TrimSpace(*raw)
			exists, err := s.repo.ExistsSIPExcludingUser(ctx, value, userID)
			if err != nil {
				return constant.ErrInternalServerError
			}
			if exists {
				return constant.ErrDoctorSIPAlreadyExists
			}
			doctorUpdates["sip_number"] = value
		}
		if value, present := input.DoctorProfile.Specialty.Get(); present {
			if value == nil {
				doctorUpdates["specialty"] = nil
			} else {
				doctorUpdates["specialty"] = nullableTrimmed(*value)
			}
		}
	}

	profileUpdate := repository.UnifiedProfileUpdate{
		UserFields: updates, PatientFields: patientUpdates, DoctorFields: doctorUpdates,
	}
	if err := s.repo.ApplyUnifiedProfileUpdate(ctx, userID, profileUpdate); err != nil {
		return s.resolveProfileUpdateError(ctx, userID, profileUpdate, err)
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

func validateUnifiedProfileInput(input request.UpdateUserProfileRequest) error {
	if input.PatientProfileIsSet() && input.PatientProfile == nil {
		return constant.NewInvalidFieldTypeError("patient_profile", "object")
	}
	if input.DoctorProfileIsSet() && input.DoctorProfile == nil {
		return constant.NewInvalidFieldTypeError("doctor_profile", "object")
	}

	common := struct {
		Username  *string `json:"username,omitempty" validate:"omitempty,min=3,max=64,username"`
		FirstName *string `json:"first_name,omitempty" validate:"omitempty,max=100"`
		LastName  *string `json:"last_name,omitempty" validate:"omitempty,max=100"`
		Phone     *string `json:"phone,omitempty" validate:"omitempty,max=32"`
		Address   *string `json:"address,omitempty" validate:"omitempty,max=1000"`
	}{
		Username: patchValue(input.Username), FirstName: patchValue(input.FirstName),
		LastName: patchValue(input.LastName), Phone: patchValue(input.Phone),
		Address: patchValue(input.Address),
	}
	if err := util.ValidateStruct(&common); err != nil {
		return util.MapValidationError(err)
	}
	if input.PatientProfile != nil {
		patient := struct {
			HeightCM       *int    `json:"height_cm,omitempty" validate:"omitempty,gte=1,lte=300"`
			WeightKG       *int    `json:"weight_kg,omitempty" validate:"omitempty,gte=1,lte=1000"`
			Allergies      *string `json:"allergies,omitempty" validate:"omitempty,max=2000"`
			MedicalHistory *string `json:"medical_history,omitempty" validate:"omitempty,max=10000"`
		}{
			HeightCM:       patchValue(input.PatientProfile.HeightCM),
			WeightKG:       patchValue(input.PatientProfile.WeightKG),
			Allergies:      patchValue(input.PatientProfile.Allergies),
			MedicalHistory: patchValue(input.PatientProfile.MedicalHistory),
		}
		if err := util.ValidateStruct(&patient); err != nil {
			return util.MapValidationError(err)
		}
	}
	if input.DoctorProfile != nil {
		doctor := struct {
			SIPNumber *string `json:"sip_number,omitempty" validate:"omitempty,max=64"`
			Specialty *string `json:"specialty,omitempty" validate:"omitempty,max=100"`
		}{
			SIPNumber: patchValue(input.DoctorProfile.SIPNumber),
			Specialty: patchValue(input.DoctorProfile.Specialty),
		}
		if err := util.ValidateStruct(&doctor); err != nil {
			return util.MapValidationError(err)
		}
	}
	return nil
}

func patchValue[T any](field request.PatchField[T]) *T {
	value, _ := field.Get()
	return value
}

func hasProfileUpdate(input request.UpdateUserProfileRequest) bool {
	return input.Username.IsSet() || input.FirstName.IsSet() || input.LastName.IsSet() || input.Phone.IsSet() ||
		input.DOB.IsSet() || input.Address.IsSet() || input.Gender.IsSet() || input.NIK.IsSet() ||
		hasPatientProfileUpdate(input.PatientProfile) || hasDoctorProfileUpdate(input.DoctorProfile)
}

func hasPatientProfileUpdate(profile *request.UpdatePatientProfileRequest) bool {
	return profile != nil && (profile.HeightCM.IsSet() || profile.WeightKG.IsSet() ||
		profile.Allergies.IsSet() || profile.MedicalHistory.IsSet())
}

func hasDoctorProfileUpdate(profile *request.UpdateDoctorProfileRequest) bool {
	return profile != nil && (profile.SIPNumber.IsSet() || profile.Specialty.IsSet())
}

func nullableTrimmed(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func dateOnlyUTC(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
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
	if errors.Is(err, repository.ErrHospitalWorkerDOBRequired) {
		return constant.NewFieldRequiredError("dob")
	}
	if errors.Is(err, repository.ErrHospitalWorkerUnderage) {
		return constant.ErrHospitalWorkerMinimumAge
	}
	if errors.Is(err, repository.ErrDoctorSIPConflict) {
		return constant.ErrDoctorSIPAlreadyExists
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.ConstraintName {
		case "ux_users_username_lower":
			return constant.ErrUsernameAlreadyExists
		case "ux_users_nik_active":
			return constant.ErrDuplicateNIK
		case "doctor_profiles_sip_number_key", "ux_doctor_profiles_sip_number_normalized":
			return constant.ErrDoctorSIPAlreadyExists
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return constant.ErrUserNotFound
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return constant.ErrConflict
	}
	return constant.ErrInternalServerError
}

// resolveProfileUpdateError handles uniqueness races that happen after the
// optimistic prechecks above. With GORM TranslateError enabled, PostgreSQL's
// constraint name is replaced by gorm.ErrDuplicatedKey, so the only safe way
// to retain a field-specific API error is to re-check the values that were
// actually part of the failed transaction after it has rolled back.
func (s *Service) resolveProfileUpdateError(
	ctx context.Context,
	userID string,
	update repository.UnifiedProfileUpdate,
	err error,
) error {
	mapped := mapProfileUpdateError(err)
	if !errors.Is(err, gorm.ErrDuplicatedKey) || !errors.Is(mapped, constant.ErrConflict) {
		return mapped
	}

	if username, ok := update.UserFields["username"].(string); ok && username != "" {
		exists, lookupErr := s.repo.ExistsUsernameExcludingUser(ctx, username, userID)
		if lookupErr != nil {
			return constant.ErrConflict
		}
		if exists {
			return constant.ErrUsernameAlreadyExists
		}
	}

	if nik, ok := update.UserFields["nik"].(string); ok && nik != "" {
		exists, lookupErr := s.repo.ExistsNIKExcludingUser(ctx, nik, userID)
		if lookupErr != nil {
			return constant.ErrConflict
		}
		if exists {
			return constant.ErrDuplicateNIK
		}
	}

	if sipNumber, ok := update.DoctorFields["sip_number"].(string); ok && sipNumber != "" {
		exists, lookupErr := s.repo.ExistsSIPExcludingUser(ctx, sipNumber, userID)
		if lookupErr != nil {
			return constant.ErrConflict
		}
		if exists {
			return constant.ErrDoctorSIPAlreadyExists
		}
	}

	return constant.ErrConflict
}

func cleanupObject(storage storageclient.Client, parent context.Context, objectPath string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	if err := storage.Delete(cleanupCtx, objectPath); err != nil {
		logrus.WithError(err).Warn("profile photo object cleanup failed")
	}
}
