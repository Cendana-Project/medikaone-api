package doctor_hospital

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/email"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	storageclient "github.com/Cendana-Project/medikaone-api/internal/storage"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

const (
	InvitationTTL    = 7 * 24 * time.Hour
	DefaultTimezone  = "Asia/Jakarta"
	MaxContractBytes = int64(10 * 1024 * 1024)
	MaxSchedules     = 50
)

type Repository interface {
	SearchEligibleDoctor(context.Context, repository.DoctorSearchCriteria) (*response.DoctorSearchResult, error)
	CreateDepartment(context.Context, string, string, string, time.Time) (*entity.HospitalDepartment, error)
	ListDepartments(context.Context, string) ([]entity.HospitalDepartment, error)
	CreateRoom(context.Context, string, string, string, string, time.Time) (*entity.HospitalRoom, error)
	ListRooms(context.Context, string, string) ([]entity.HospitalRoom, error)
	DepartmentExists(context.Context, string, string) (bool, error)
	RoomMatchesDepartment(context.Context, string, string, string) (bool, error)
	CreateInvitation(context.Context, repository.CreateInvitationInput) (*response.DoctorHospitalInvitation, error)
	ListInvitationsForDoctor(context.Context, string, string, time.Time) ([]response.DoctorHospitalInvitation, error)
	ListInvitationsForHospital(context.Context, string, string, time.Time) ([]response.DoctorHospitalInvitation, error)
	GetInvitationForDoctor(context.Context, string, string, time.Time) (*response.DoctorHospitalInvitation, error)
	GetInvitationForHospital(context.Context, string, string, time.Time) (*response.DoctorHospitalInvitation, error)
	AcceptInvitation(context.Context, string, string, repository.Document, time.Time) error
	RejectInvitation(context.Context, string, string, *string, time.Time) error
	CancelInvitation(context.Context, string, string, string, time.Time) error
	ResendInvitation(context.Context, string, string, string, time.Time, time.Time) (*response.DoctorHospitalInvitation, error)
	GetContractForDoctor(context.Context, string, string, string) (*repository.ContractDocument, error)
	GetContractForHospital(context.Context, string, string, string) (*repository.ContractDocument, error)
	ListHospitalDoctors(context.Context, string, string) ([]response.HospitalDoctor, error)
	UpdateAffiliationStatus(context.Context, string, string, string, string, time.Time) error
	ListNotifications(context.Context, string, bool) ([]response.Notification, error)
	MarkNotificationRead(context.Context, string, string, time.Time) error
}

type Service struct {
	repo         Repository
	storage      storageclient.Client
	email        email.Sender
	maxFileSize  int64
	signedURLTTL time.Duration
	now          func() time.Time
}

func NewService(repo Repository, storage storageclient.Client, sender email.Sender, maxFileSize int64, signedURLTTL time.Duration) *Service {
	if maxFileSize <= 0 || maxFileSize > MaxContractBytes {
		maxFileSize = MaxContractBytes
	}
	if signedURLTTL <= 0 {
		signedURLTTL = 5 * time.Minute
	}
	return &Service{
		repo: repo, storage: storage, email: sender, maxFileSize: maxFileSize,
		signedURLTTL: signedURLTTL, now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) SearchDoctor(ctx context.Context, query request.DoctorSearchQuery) (*response.DoctorSearchResult, error) {
	query.Email = strings.TrimSpace(query.Email)
	query.SIPNumber = strings.TrimSpace(query.SIPNumber)
	query.MedikaOneID = strings.TrimSpace(query.MedikaOneID)
	provided := 0
	for _, value := range []string{query.Email, query.SIPNumber, query.MedikaOneID} {
		if value != "" {
			provided++
		}
	}
	if provided != 1 {
		return nil, constant.ErrValidationFailed
	}
	if query.Email != "" && (!strings.Contains(query.Email, "@") || len(query.Email) > 190) {
		return nil, constant.ErrInvalidEmail
	}
	if len(query.SIPNumber) > 64 {
		return nil, constant.ErrValidationFailed
	}
	if query.MedikaOneID != "" {
		if _, err := uuid.Parse(query.MedikaOneID); err != nil {
			return nil, constant.ErrInvalidUUIDFormat
		}
	}
	result, err := s.repo.SearchEligibleDoctor(ctx, repository.DoctorSearchCriteria{
		Email: query.Email, SIPNumber: query.SIPNumber, MedikaOneID: query.MedikaOneID,
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, constant.ErrDoctorNotEligible
	}
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return result, nil
}

type UploadedFile struct {
	Filename string
	MIMEType string
	Content  []byte
}

type SignedContractURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) CreateDepartment(ctx context.Context, hospitalID string, req request.CreateHospitalDepartmentRequest) (*entity.HospitalDepartment, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	name := strings.TrimSpace(req.Name)
	if code == "" || name == "" || len(code) > 40 || len(name) > 120 {
		return nil, constant.ErrValidationFailed
	}
	department, err := s.repo.CreateDepartment(ctx, hospitalID, code, name, s.now())
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, constant.ErrConflict
	}
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return department, nil
}

func (s *Service) ListDepartments(ctx context.Context, hospitalID string) ([]entity.HospitalDepartment, error) {
	rows, err := s.repo.ListDepartments(ctx, hospitalID)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return rows, nil
}

func (s *Service) CreateRoom(ctx context.Context, hospitalID string, req request.CreateHospitalRoomRequest) (*entity.HospitalRoom, error) {
	if _, err := uuid.Parse(req.DepartmentID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	name := strings.TrimSpace(req.Name)
	if code == "" || name == "" || len(code) > 40 || len(name) > 120 {
		return nil, constant.ErrValidationFailed
	}
	room, err := s.repo.CreateRoom(ctx, hospitalID, req.DepartmentID, code, name, s.now())
	if errors.Is(err, repository.ErrPlacementNotFound) {
		return nil, constant.ErrHospitalPlacementNotFound
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, constant.ErrConflict
	}
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return room, nil
}

func (s *Service) ListRooms(ctx context.Context, hospitalID, departmentID string) ([]entity.HospitalRoom, error) {
	if departmentID != "" {
		if _, err := uuid.Parse(departmentID); err != nil {
			return nil, constant.ErrInvalidUUIDFormat
		}
	}
	rows, err := s.repo.ListRooms(ctx, hospitalID, departmentID)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return rows, nil
}

func (s *Service) CreateInvitation(ctx context.Context, hospitalID, invitedBy string, req request.CreateDoctorHospitalInvitationRequest, file UploadedFile) (*response.DoctorHospitalInvitation, error) {
	if _, err := uuid.Parse(req.DoctorID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if _, err := uuid.Parse(req.DepartmentID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if req.RoomID != nil && strings.TrimSpace(*req.RoomID) != "" {
		trimmed := strings.TrimSpace(*req.RoomID)
		if _, err := uuid.Parse(trimmed); err != nil {
			return nil, constant.ErrInvalidUUIDFormat
		}
		req.RoomID = &trimmed
	} else {
		req.RoomID = nil
	}
	if req.Message != nil {
		trimmed := strings.TrimSpace(*req.Message)
		if len(trimmed) > 1000 {
			return nil, constant.ErrValidationFailed
		}
		if trimmed == "" {
			req.Message = nil
		} else {
			req.Message = &trimmed
		}
	}

	eligible, err := s.repo.SearchEligibleDoctor(ctx, repository.DoctorSearchCriteria{MedikaOneID: req.DoctorID})
	if errors.Is(err, gorm.ErrRecordNotFound) || eligible == nil {
		return nil, constant.ErrDoctorNotEligible
	}
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	ok, err := s.repo.DepartmentExists(ctx, hospitalID, req.DepartmentID)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if !ok {
		return nil, constant.ErrHospitalPlacementNotFound
	}
	if req.RoomID != nil {
		ok, err = s.repo.RoomMatchesDepartment(ctx, hospitalID, req.DepartmentID, *req.RoomID)
		if err != nil {
			return nil, constant.ErrInternalServerError
		}
		if !ok {
			return nil, constant.ErrHospitalPlacementNotFound
		}
	}
	schedules, err := validateSchedules(req.Schedules)
	if err != nil {
		return nil, err
	}
	document, err := s.validatePDF(file)
	if err != nil {
		return nil, err
	}

	invitationID := uuid.NewString()
	objectPath := fmt.Sprintf("hospitals/%s/doctor-invitations/%s/original/%s.pdf", hospitalID, invitationID, uuid.NewString())
	uploaded, err := s.storage.Upload(ctx, objectPath, "application/pdf", file.Content)
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	document.Bucket = uploaded.Bucket
	document.ObjectPath = uploaded.ObjectPath
	document.FileSize = uploaded.FileSize

	now := s.now()
	created, err := s.repo.CreateInvitation(ctx, repository.CreateInvitationInput{
		InvitationID: invitationID,
		HospitalID:   hospitalID, DoctorID: req.DoctorID, DepartmentID: req.DepartmentID,
		RoomID: req.RoomID, InvitedBy: invitedBy, Message: req.Message,
		ExpiresAt: now.Add(InvitationTTL), Contract: document, Schedules: schedules, Now: now,
	})
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cleanupErr := s.storage.Delete(cleanupCtx, uploaded.ObjectPath); cleanupErr != nil {
			util.Errorf(ctx, "storage cleanup failed operation=cleanup_invitation_contract error_type=%T", cleanupErr)
		}
		return nil, mapRepositoryError(err)
	}

	if s.email != nil {
		html := email.RenderDoctorHospitalInvitation(created.DoctorFirstName, created.HospitalName, created.DepartmentName, created.ExpiresAt)
		if sendErr := s.email.SendWithContext(ctx, created.DoctorEmail, "Undangan bergabung dengan "+created.HospitalName, html); sendErr != nil {
			util.Errorf(ctx, "doctor invitation email failed invitation_id=%s error_type=%T", created.ID, sendErr)
		}
	}
	return created, nil
}

func (s *Service) ListDoctorInvitations(ctx context.Context, doctorID, status string) ([]response.DoctorHospitalInvitation, error) {
	status, err := normalizeInvitationStatusFilter(status)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListInvitationsForDoctor(ctx, doctorID, status, s.now())
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return rows, nil
}

func (s *Service) ListHospitalInvitations(ctx context.Context, hospitalID, status string) ([]response.DoctorHospitalInvitation, error) {
	status, err := normalizeInvitationStatusFilter(status)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListInvitationsForHospital(ctx, hospitalID, status, s.now())
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return rows, nil
}

func (s *Service) GetDoctorInvitation(ctx context.Context, doctorID, invitationID string) (*response.DoctorHospitalInvitation, error) {
	if _, err := uuid.Parse(invitationID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetInvitationForDoctor(ctx, doctorID, invitationID, s.now())
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return row, nil
}

func (s *Service) GetHospitalInvitation(ctx context.Context, hospitalID, invitationID string) (*response.DoctorHospitalInvitation, error) {
	if _, err := uuid.Parse(invitationID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.repo.GetInvitationForHospital(ctx, hospitalID, invitationID, s.now())
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return row, nil
}

func (s *Service) AcceptInvitation(ctx context.Context, doctorID, invitationID string, file UploadedFile) (*response.DoctorHospitalInvitation, error) {
	if _, err := uuid.Parse(invitationID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	document, err := s.validatePDF(file)
	if err != nil {
		return nil, err
	}
	invitation, err := s.repo.GetInvitationForDoctor(ctx, doctorID, invitationID, s.now())
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if invitation.Status == entity.DoctorHospitalInvitationExpired {
		return nil, constant.ErrDoctorInvitationExpired
	}
	if invitation.Status != entity.DoctorHospitalInvitationPending {
		return nil, constant.ErrInvalidDoctorInvitationState
	}
	objectPath := fmt.Sprintf("hospitals/%s/doctor-invitations/%s/signed/%s.pdf", invitation.HospitalID, invitationID, uuid.NewString())
	uploaded, err := s.storage.Upload(ctx, objectPath, "application/pdf", file.Content)
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	document.Bucket = uploaded.Bucket
	document.ObjectPath = uploaded.ObjectPath
	document.FileSize = uploaded.FileSize

	if err := s.repo.AcceptInvitation(ctx, invitationID, doctorID, document, s.now()); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cleanupErr := s.storage.Delete(cleanupCtx, uploaded.ObjectPath); cleanupErr != nil {
			util.Errorf(ctx, "storage cleanup failed operation=cleanup_signed_contract error_type=%T", cleanupErr)
		}
		return nil, mapRepositoryError(err)
	}
	return s.repo.GetInvitationForDoctor(ctx, doctorID, invitationID, s.now())
}

func (s *Service) RejectInvitation(ctx context.Context, doctorID, invitationID string, reason *string) error {
	if _, err := uuid.Parse(invitationID); err != nil {
		return constant.ErrInvalidUUIDFormat
	}
	if reason != nil {
		trimmed := strings.TrimSpace(*reason)
		if len(trimmed) > 500 {
			return constant.ErrValidationFailed
		}
		if trimmed == "" {
			reason = nil
		} else {
			reason = &trimmed
		}
	}
	return mapRepositoryError(s.repo.RejectInvitation(ctx, invitationID, doctorID, reason, s.now()))
}

func (s *Service) CancelInvitation(ctx context.Context, hospitalID, invitationID, actorID string) error {
	if _, err := uuid.Parse(invitationID); err != nil {
		return constant.ErrInvalidUUIDFormat
	}
	return mapRepositoryError(s.repo.CancelInvitation(ctx, invitationID, hospitalID, actorID, s.now()))
}

func (s *Service) ResendInvitation(ctx context.Context, hospitalID, invitationID, invitedBy string) (*response.DoctorHospitalInvitation, error) {
	if _, err := uuid.Parse(invitationID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	now := s.now()
	row, err := s.repo.ResendInvitation(ctx, invitationID, hospitalID, invitedBy, now.Add(InvitationTTL), now)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if s.email != nil {
		html := email.RenderDoctorHospitalInvitation(row.DoctorFirstName, row.HospitalName, row.DepartmentName, row.ExpiresAt)
		if sendErr := s.email.SendWithContext(ctx, row.DoctorEmail, "Undangan bergabung dengan "+row.HospitalName, html); sendErr != nil {
			util.Errorf(ctx, "doctor invitation email failed invitation_id=%s error_type=%T", row.ID, sendErr)
		}
	}
	return row, nil
}

func (s *Service) GetDoctorContractURL(ctx context.Context, doctorID, invitationID, version string) (*SignedContractURL, error) {
	if _, err := uuid.Parse(invitationID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	version, err := normalizeContractVersion(version)
	if err != nil {
		return nil, err
	}
	document, err := s.repo.GetContractForDoctor(ctx, invitationID, doctorID, version)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return s.signDocument(ctx, document)
}

func (s *Service) GetHospitalContractURL(ctx context.Context, hospitalID, invitationID, version string) (*SignedContractURL, error) {
	if _, err := uuid.Parse(invitationID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	version, err := normalizeContractVersion(version)
	if err != nil {
		return nil, err
	}
	document, err := s.repo.GetContractForHospital(ctx, invitationID, hospitalID, version)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return s.signDocument(ctx, document)
}

func (s *Service) signDocument(ctx context.Context, document *repository.ContractDocument) (*SignedContractURL, error) {
	url, err := s.storage.CreateSignedURL(ctx, document.ObjectPath, s.signedURLTTL, document.Filename)
	if err != nil {
		return nil, constant.ErrStorageUnavailable
	}
	return &SignedContractURL{URL: url, ExpiresAt: s.now().Add(s.signedURLTTL)}, nil
}

func (s *Service) ListHospitalDoctors(ctx context.Context, hospitalID, status string) ([]response.HospitalDoctor, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "" && status != entity.DoctorHospitalAffiliationActive && status != entity.DoctorHospitalAffiliationSuspended {
		return nil, constant.ErrValidationFailed
	}
	rows, err := s.repo.ListHospitalDoctors(ctx, hospitalID, status)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return rows, nil
}

func (s *Service) UpdateAffiliationStatus(ctx context.Context, hospitalID, doctorID, status, actorID string) error {
	if _, err := uuid.Parse(doctorID); err != nil {
		return constant.ErrInvalidUUIDFormat
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != entity.DoctorHospitalAffiliationActive && status != entity.DoctorHospitalAffiliationSuspended {
		return constant.ErrValidationFailed
	}
	return mapRepositoryError(s.repo.UpdateAffiliationStatus(ctx, hospitalID, doctorID, status, actorID, s.now()))
}

func (s *Service) ListNotifications(ctx context.Context, userID string, unreadOnly bool) ([]response.Notification, error) {
	rows, err := s.repo.ListNotifications(ctx, userID, unreadOnly)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return rows, nil
}

func (s *Service) MarkNotificationRead(ctx context.Context, userID, notificationID string) error {
	if _, err := uuid.Parse(notificationID); err != nil {
		return constant.ErrInvalidUUIDFormat
	}
	return mapRepositoryError(s.repo.MarkNotificationRead(ctx, userID, notificationID, s.now()))
}

func (s *Service) validatePDF(file UploadedFile) (repository.Document, error) {
	filename := filepath.Base(strings.ReplaceAll(strings.TrimSpace(file.Filename), "\\", "/"))
	if filename == "." || filename == "" || !strings.EqualFold(filepath.Ext(filename), ".pdf") {
		return repository.Document{}, constant.ErrInvalidContractPDF
	}
	if len(file.Content) == 0 || int64(len(file.Content)) > s.maxFileSize {
		return repository.Document{}, constant.ErrInvalidContractPDF
	}
	if !bytes.HasPrefix(file.Content, []byte("%PDF-")) {
		return repository.Document{}, constant.ErrInvalidContractPDF
	}
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(file.MIMEType, ";")[0]))
	if mimeType != "application/pdf" && mimeType != "application/octet-stream" {
		return repository.Document{}, constant.ErrInvalidContractPDF
	}
	sum := sha256.Sum256(file.Content)
	return repository.Document{
		Filename: filename, MIMEType: "application/pdf", FileSize: int64(len(file.Content)),
		SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func validateSchedules(input []request.DoctorInvitationScheduleRequest) ([]repository.Schedule, error) {
	if len(input) > MaxSchedules {
		return nil, constant.ErrValidationFailed
	}
	output := make([]repository.Schedule, 0, len(input))
	type interval struct{ start, end int }
	byDay := make(map[int][]interval)
	commonTimezone := ""
	for _, value := range input {
		if value.DayOfWeek < 0 || value.DayOfWeek > 6 {
			return nil, constant.ErrValidationFailed
		}
		start, err := time.Parse("15:04", strings.TrimSpace(value.StartTime))
		if err != nil {
			return nil, constant.ErrValidationFailed
		}
		end, err := time.Parse("15:04", strings.TrimSpace(value.EndTime))
		if err != nil || !end.After(start) {
			return nil, constant.ErrValidationFailed
		}
		timezone := strings.TrimSpace(value.Timezone)
		if timezone == "" {
			timezone = DefaultTimezone
		}
		if _, err := time.LoadLocation(timezone); err != nil {
			return nil, constant.ErrValidationFailed
		}
		if commonTimezone == "" {
			commonTimezone = timezone
		} else if commonTimezone != timezone {
			return nil, constant.ErrValidationFailed
		}
		startMinute := start.Hour()*60 + start.Minute()
		endMinute := end.Hour()*60 + end.Minute()
		for _, existing := range byDay[value.DayOfWeek] {
			if startMinute < existing.end && endMinute > existing.start {
				return nil, constant.ErrDoctorScheduleConflict
			}
		}
		byDay[value.DayOfWeek] = append(byDay[value.DayOfWeek], interval{start: startMinute, end: endMinute})
		output = append(output, repository.Schedule{
			DayOfWeek: value.DayOfWeek, StartTime: start.Format("15:04"),
			EndTime: end.Format("15:04"), Timezone: timezone,
		})
	}
	return output, nil
}

func normalizeInvitationStatusFilter(status string) (string, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status == "" {
		return "", nil
	}
	switch status {
	case entity.DoctorHospitalInvitationPending, entity.DoctorHospitalInvitationAccepted,
		entity.DoctorHospitalInvitationRejected, entity.DoctorHospitalInvitationCancelled,
		entity.DoctorHospitalInvitationExpired:
		return status, nil
	default:
		return "", constant.ErrValidationFailed
	}
}

func normalizeContractVersion(version string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "", "original":
		return "original", nil
	case "signed":
		return "signed", nil
	default:
		return "", constant.ErrValidationFailed
	}
}

func mapRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrInvitationNotFound):
		return constant.ErrDoctorInvitationNotFound
	case errors.Is(err, repository.ErrInvitationExists), errors.Is(err, gorm.ErrDuplicatedKey):
		return constant.ErrDoctorInvitationExists
	case errors.Is(err, repository.ErrInvitationExpired):
		return constant.ErrDoctorInvitationExpired
	case errors.Is(err, repository.ErrInvalidInvitationState):
		return constant.ErrInvalidDoctorInvitationState
	case errors.Is(err, repository.ErrPlacementNotFound):
		return constant.ErrHospitalPlacementNotFound
	case errors.Is(err, repository.ErrScheduleConflict):
		return constant.ErrDoctorScheduleConflict
	case errors.Is(err, repository.ErrAffiliationNotFound):
		return constant.ErrAffiliationNotFound
	case errors.Is(err, repository.ErrNotificationNotFound):
		return constant.ErrNotificationNotFound
	default:
		return constant.ErrInternalServerError
	}
}
