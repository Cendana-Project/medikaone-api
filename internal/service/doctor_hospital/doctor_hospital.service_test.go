package doctor_hospital

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	storageclient "github.com/Cendana-Project/medikaone-api/internal/storage"
)

type fakeRepository struct {
	Repository
	eligible            *response.DoctorSearchResult
	departmentHospital  string
	roomHospital        string
	departmentExists    bool
	roomMatches         bool
	createInvitationErr error
	createInput         repository.CreateInvitationInput
}

func (f *fakeRepository) SearchEligibleDoctor(context.Context, repository.DoctorSearchCriteria) (*response.DoctorSearchResult, error) {
	return f.eligible, nil
}

func (f *fakeRepository) DepartmentExists(_ context.Context, hospitalID, _ string) (bool, error) {
	f.departmentHospital = hospitalID
	return f.departmentExists, nil
}

func (f *fakeRepository) RoomMatchesDepartment(_ context.Context, hospitalID, _, _ string) (bool, error) {
	f.roomHospital = hospitalID
	return f.roomMatches, nil
}

func (f *fakeRepository) CreateInvitation(_ context.Context, input repository.CreateInvitationInput) (*response.DoctorHospitalInvitation, error) {
	f.createInput = input
	if f.createInvitationErr != nil {
		return nil, f.createInvitationErr
	}
	return &response.DoctorHospitalInvitation{
		ID: input.InvitationID, HospitalID: input.HospitalID, DoctorID: input.DoctorID,
		DoctorEmail: "doctor@example.com", HospitalName: "RS Test", DepartmentName: "Poli Umum",
	}, nil
}

type fakeStorage struct {
	uploadedPath string
	deletedPath  string
	uploadErr    error
}

func (f *fakeStorage) Upload(_ context.Context, objectPath, _ string, content []byte) (*storageclient.UploadedObject, error) {
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	f.uploadedPath = objectPath
	return &storageclient.UploadedObject{Bucket: "doctor-contracts", ObjectPath: objectPath, FileSize: int64(len(content))}, nil
}

func (f *fakeStorage) Delete(_ context.Context, objectPath string) error {
	f.deletedPath = objectPath
	return nil
}

func (f *fakeStorage) CreateSignedURL(context.Context, string, time.Duration, string) (string, error) {
	return "", nil
}

func TestValidatePDFEnforcesTenMegabyteCeiling(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeStorage{}, nil, 50*1024*1024, time.Minute)
	valid := UploadedFile{Filename: "contract.pdf", MIMEType: "application/pdf", Content: []byte("%PDF-test")}
	if _, err := service.validatePDF(valid); err != nil {
		t.Fatalf("valid PDF rejected: %v", err)
	}

	invalidFiles := []UploadedFile{
		{Filename: "contract.txt", MIMEType: "application/pdf", Content: []byte("%PDF-test")},
		{Filename: "contract.pdf", MIMEType: "application/pdf", Content: []byte("not-a-pdf")},
		{Filename: "contract.pdf", MIMEType: "text/plain", Content: []byte("%PDF-test")},
		{Filename: "contract.pdf", MIMEType: "application/pdf", Content: make([]byte, MaxContractBytes+1)},
	}
	for i := range invalidFiles {
		if _, err := service.validatePDF(invalidFiles[i]); !errors.Is(err, constant.ErrInvalidContractPDF) {
			t.Fatalf("invalid file %d should be rejected, got %v", i, err)
		}
	}
}

func TestValidateSchedulesRejectsOverlap(t *testing.T) {
	_, err := validateSchedules([]request.DoctorInvitationScheduleRequest{
		{DayOfWeek: 1, StartTime: "08:00", EndTime: "12:00", Timezone: "Asia/Jakarta"},
		{DayOfWeek: 1, StartTime: "11:00", EndTime: "13:00", Timezone: "Asia/Jakarta"},
	})
	if !errors.Is(err, constant.ErrDoctorScheduleConflict) {
		t.Fatalf("expected overlap error, got %v", err)
	}

	schedules, err := validateSchedules([]request.DoctorInvitationScheduleRequest{
		{DayOfWeek: 1, StartTime: "08:00", EndTime: "12:00"},
		{DayOfWeek: 1, StartTime: "12:00", EndTime: "16:00"},
	})
	if err != nil {
		t.Fatalf("adjacent schedules should be valid: %v", err)
	}
	if schedules[0].Timezone != DefaultTimezone || schedules[1].Timezone != DefaultTimezone {
		t.Fatalf("expected default timezone %s", DefaultTimezone)
	}
}

func TestValidateSchedulesEnforcesEntryLimit(t *testing.T) {
	schedules := make([]request.DoctorInvitationScheduleRequest, MaxSchedules+1)
	want := constant.NewInvalidFieldLengthError("schedules", "at most 50 items long", "memiliki maksimal 50 item")
	if _, err := validateSchedules(schedules); !errors.Is(err, want) {
		t.Fatalf("expected schedule count validation error, got %v", err)
	}
}

func TestNormalizeContractVersionRejectsUnknownVersion(t *testing.T) {
	if version, err := normalizeContractVersion("SIGNED"); err != nil || version != "signed" {
		t.Fatalf("expected signed version, got version=%q err=%v", version, err)
	}
	want := constant.NewInvalidFieldValueError("version", "original or signed", "original atau signed")
	if _, err := normalizeContractVersion("preview"); !errors.Is(err, want) {
		t.Fatalf("expected invalid version error, got %v", err)
	}
}

func TestCreateInvitationScopesPlacementAndCleansFailedUpload(t *testing.T) {
	hospitalID := uuid.NewString()
	doctorID := uuid.NewString()
	departmentID := uuid.NewString()
	roomID := uuid.NewString()
	repo := &fakeRepository{
		eligible: &response.DoctorSearchResult{ID: doctorID}, departmentExists: true,
		roomMatches: true, createInvitationErr: repository.ErrInvitationExists,
	}
	objectStorage := &fakeStorage{}
	service := NewService(repo, objectStorage, nil, MaxContractBytes, 5*time.Minute)
	fixedNow := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }

	_, err := service.CreateInvitation(context.Background(), hospitalID, uuid.NewString(), request.CreateDoctorHospitalInvitationRequest{
		DoctorID: doctorID, DepartmentID: departmentID, RoomID: &roomID,
		Schedules: []request.DoctorInvitationScheduleRequest{{DayOfWeek: 1, StartTime: "08:00", EndTime: "12:00"}},
	}, UploadedFile{Filename: "contract.pdf", MIMEType: "application/pdf", Content: []byte("%PDF-test")})
	if !errors.Is(err, constant.ErrDoctorInvitationExists) {
		t.Fatalf("expected mapped duplicate error, got %v", err)
	}
	if repo.departmentHospital != hospitalID || repo.roomHospital != hospitalID || repo.createInput.HospitalID != hospitalID {
		t.Fatal("hospital scope was not preserved through placement and invitation operations")
	}
	if !strings.HasPrefix(objectStorage.uploadedPath, "hospitals/"+hospitalID+"/doctor-invitations/") {
		t.Fatalf("unexpected tenant object path: %s", objectStorage.uploadedPath)
	}
	if objectStorage.deletedPath != objectStorage.uploadedPath {
		t.Fatalf("failed transaction object was not cleaned up: uploaded=%s deleted=%s", objectStorage.uploadedPath, objectStorage.deletedPath)
	}
	if !repo.createInput.ExpiresAt.Equal(fixedNow.Add(InvitationTTL)) {
		t.Fatalf("expected seven-day expiry, got %s", repo.createInput.ExpiresAt)
	}
}
