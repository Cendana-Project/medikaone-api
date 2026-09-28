package doctor_hospital

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type LifecycleRepository interface {
	UpdateDepartment(context.Context, string, string, string, time.Time) (*entity.HospitalDepartment, error)
	DeleteDepartment(context.Context, string, string, time.Time) error
	UpdateRoom(context.Context, string, string, *string, map[string]any) (*entity.HospitalRoom, error)
	DeleteRoom(context.Context, string, string, time.Time) error
	UpdateInvitation(context.Context, repository.UpdateInvitationInput) (*response.DoctorHospitalInvitation, error)
	DeleteInvitation(context.Context, string, string, string, time.Time) error
	DeleteAffiliation(context.Context, string, string, string, time.Time) error
}

func (s *Service) lifecycleRepository() (LifecycleRepository, error) {
	repo, ok := s.repo.(LifecycleRepository)
	if !ok {
		return nil, constant.ErrInternalServerError
	}
	return repo, nil
}

func validateResourceIDs(ids ...string) error {
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return constant.ErrInvalidUUIDFormat
		}
	}
	return nil
}

func placementUpdateFields(code, name *string, now time.Time) (map[string]any, error) {
	fields := map[string]any{"updated_at": now}
	for field, raw := range map[string]*string{"code": code, "name": name} {
		if raw == nil {
			continue
		}
		value := strings.TrimSpace(*raw)
		if value == "" {
			return nil, constant.NewFieldRequiredError(field)
		}
		maxLen := 120
		if field == "code" {
			value = strings.ToUpper(value)
			maxLen = 40
		}
		if len(value) > maxLen {
			return nil, constant.NewInvalidFieldLengthError(field, "within the documented maximum", "sesuai batas panjang yang didokumentasikan")
		}
		fields[field] = value
	}
	return fields, nil
}

func (s *Service) UpdateDepartment(ctx context.Context, hospitalID, departmentID string, req request.UpdateHospitalDepartmentRequest) (*entity.HospitalDepartment, error) {
	if err := validateResourceIDs(hospitalID, departmentID); err != nil {
		return nil, err
	}
	if req.MasterDepartmentID == nil || strings.TrimSpace(*req.MasterDepartmentID) == "" {
		return nil, constant.NewFieldRequiredError("master_department_id")
	}
	masterDepartmentID := strings.TrimSpace(*req.MasterDepartmentID)
	if err := validateResourceIDs(masterDepartmentID); err != nil {
		return nil, err
	}
	repo, err := s.lifecycleRepository()
	if err != nil {
		return nil, err
	}
	row, err := repo.UpdateDepartment(ctx, hospitalID, departmentID, masterDepartmentID, s.now())
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, constant.ErrDepartmentAlreadyExists
	}
	if errors.Is(err, repository.ErrMasterDepartmentNotFound) {
		return nil, constant.ErrMasterDepartmentNotFound
	}
	return row, mapLifecycleError(err)
}

func (s *Service) DeleteDepartment(ctx context.Context, hospitalID, departmentID string) error {
	if err := validateResourceIDs(hospitalID, departmentID); err != nil {
		return err
	}
	repo, err := s.lifecycleRepository()
	if err != nil {
		return err
	}
	return mapLifecycleError(repo.DeleteDepartment(ctx, hospitalID, departmentID, s.now()))
}

func (s *Service) UpdateRoom(ctx context.Context, hospitalID, roomID string, req request.UpdateHospitalRoomRequest) (*entity.HospitalRoom, error) {
	if err := validateResourceIDs(hospitalID, roomID); err != nil {
		return nil, err
	}
	if req.DepartmentID != nil {
		value := strings.TrimSpace(*req.DepartmentID)
		if value == "" {
			return nil, constant.ErrHospitalPlacementNotFound
		}
		if err := validateResourceIDs(value); err != nil {
			return nil, err
		}
		req.DepartmentID = &value
	}
	if req.Code == nil && req.Name == nil && req.DepartmentID == nil {
		return nil, constant.NewFieldRequiredError("department_id, code or name")
	}
	fields, err := placementUpdateFields(req.Code, req.Name, s.now())
	if err != nil {
		return nil, err
	}
	repo, err := s.lifecycleRepository()
	if err != nil {
		return nil, err
	}
	row, err := repo.UpdateRoom(ctx, hospitalID, roomID, req.DepartmentID, fields)
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, constant.ErrRoomAlreadyExists
	}
	return row, mapLifecycleError(err)
}

func (s *Service) DeleteRoom(ctx context.Context, hospitalID, roomID string) error {
	if err := validateResourceIDs(hospitalID, roomID); err != nil {
		return err
	}
	repo, err := s.lifecycleRepository()
	if err != nil {
		return err
	}
	return mapLifecycleError(repo.DeleteRoom(ctx, hospitalID, roomID, s.now()))
}

func (s *Service) UpdateInvitation(ctx context.Context, hospitalID, invitationID, actorID string, req request.UpdateDoctorHospitalInvitationRequest) (*response.DoctorHospitalInvitation, error) {
	return s.UpdateInvitationWithContract(ctx, hospitalID, invitationID, actorID, req, nil)
}

func (s *Service) UpdateInvitationWithContract(ctx context.Context, hospitalID, invitationID, actorID string, req request.UpdateDoctorHospitalInvitationRequest, file *UploadedFile) (*response.DoctorHospitalInvitation, error) {
	if err := validateResourceIDs(hospitalID, invitationID, actorID); err != nil {
		return nil, err
	}
	if req.DepartmentID == nil && req.RoomID == nil && req.Message == nil && req.Schedules == nil && file == nil {
		return nil, constant.NewFieldRequiredError("at least one update field")
	}
	if req.DepartmentID != nil {
		value := strings.TrimSpace(*req.DepartmentID)
		if value == "" {
			return nil, constant.ErrHospitalPlacementNotFound
		}
		if err := validateResourceIDs(value); err != nil {
			return nil, err
		}
		req.DepartmentID = &value
	}
	if req.RoomID != nil {
		value := strings.TrimSpace(*req.RoomID)
		if value != "" {
			if err := validateResourceIDs(value); err != nil {
				return nil, err
			}
		}
		req.RoomID = &value
	}
	if req.Message != nil {
		value := strings.TrimSpace(*req.Message)
		if len(value) > 1000 {
			return nil, constant.NewInvalidFieldLengthError("message", "at most 1000 characters long", "memiliki maksimal 1000 karakter")
		}
		req.Message = &value
	}
	var schedules *[]repository.Schedule
	if req.Schedules != nil {
		validated, err := validateSchedules(*req.Schedules)
		if err != nil {
			return nil, err
		}
		schedules = &validated
	}
	repo, err := s.lifecycleRepository()
	if err != nil {
		return nil, err
	}
	var contract *repository.Document
	if file != nil {
		document, err := s.validatePDF(*file)
		if err != nil {
			return nil, err
		}
		// Check tenant ownership and lifecycle before uploading. The write transaction
		// locks and repeats these checks in case accept/cancel races the upload.
		invitation, err := s.repo.GetInvitationForHospital(ctx, hospitalID, invitationID, s.now())
		if err != nil {
			return nil, mapLifecycleError(err)
		}
		if invitation.Status == entity.DoctorHospitalInvitationExpired {
			return nil, constant.ErrDoctorInvitationExpired
		}
		if invitation.Status != entity.DoctorHospitalInvitationPending {
			return nil, constant.ErrInvalidDoctorInvitationState
		}
		if !invitation.ExpiresAt.After(s.now()) {
			return nil, constant.ErrDoctorInvitationExpired
		}
		if s.storage == nil {
			return nil, constant.ErrStorageUnavailable
		}
		objectPath := fmt.Sprintf("hospitals/%s/doctor-invitations/%s/original/%s.pdf", hospitalID, invitationID, uuid.NewString())
		uploaded, err := s.storage.Upload(ctx, objectPath, "application/pdf", file.Content)
		if err != nil {
			return nil, mapContractUploadError(err)
		}
		document.Bucket, document.ObjectPath, document.FileSize = uploaded.Bucket, uploaded.ObjectPath, uploaded.FileSize
		contract = &document
	}
	row, err := repo.UpdateInvitation(ctx, repository.UpdateInvitationInput{
		HospitalID: hospitalID, InvitationID: invitationID, ActorID: actorID, DepartmentID: req.DepartmentID,
		RoomID: req.RoomID, Message: req.Message, Schedules: schedules, Contract: contract, Now: s.now(),
	})
	if err != nil && contract != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if cleanupErr := s.storage.Delete(cleanupCtx, contract.ObjectPath); cleanupErr != nil {
			util.Errorf(ctx, "storage cleanup failed operation=cleanup_replacement_contract error_type=%T", cleanupErr)
		}
	}
	return row, mapLifecycleError(err)
}

func (s *Service) DeleteInvitation(ctx context.Context, hospitalID, invitationID, actorID string) error {
	if err := validateResourceIDs(hospitalID, invitationID, actorID); err != nil {
		return err
	}
	repo, err := s.lifecycleRepository()
	if err != nil {
		return err
	}
	return mapLifecycleError(repo.DeleteInvitation(ctx, hospitalID, invitationID, actorID, s.now()))
}

func (s *Service) DeleteAffiliation(ctx context.Context, hospitalID, doctorID, actorID string) error {
	if err := validateResourceIDs(hospitalID, doctorID, actorID); err != nil {
		return err
	}
	repo, err := s.lifecycleRepository()
	if err != nil {
		return err
	}
	return mapLifecycleError(repo.DeleteAffiliation(ctx, hospitalID, doctorID, actorID, s.now()))
}

func mapLifecycleError(err error) error {
	if errors.Is(err, repository.ErrResourceInUse) {
		return constant.ErrResourceInUse
	}
	return mapRepositoryError(err)
}
