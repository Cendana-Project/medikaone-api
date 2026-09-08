package doctor_hospital

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	storageclient "github.com/Cendana-Project/medikaone-api/internal/storage"
)

type fakeRepository struct {
	Repository
	eligible            *response.DoctorSearchResult
	searchResults       []response.DoctorSearchResult
	searchIdentity      string
	searchLimit         int
	searchErr           error
	departmentHospital  string
	roomHospital        string
	departmentExists    bool
	roomMatches         bool
	scheduleConflict    bool
	scheduleConflictErr error
	conflictChecked     bool
	createInvitationErr error
	createInput         repository.CreateInvitationInput
	invitation          *response.DoctorHospitalInvitation
	acceptedInvitation  string
	acceptErr           error
	rejectedInvitation  string
	updatedStatus       string
	updateStatusErr     error
}

func (f *fakeRepository) SearchEligibleDoctors(_ context.Context, identity string, limit int) ([]response.DoctorSearchResult, error) {
	f.searchIdentity = identity
	f.searchLimit = limit
	return f.searchResults, f.searchErr
}

func (f *fakeRepository) FindEligibleDoctorByID(context.Context, string) (*response.DoctorSearchResult, error) {
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

func (f *fakeRepository) HasActiveScheduleConflict(_ context.Context, _ string, _ []repository.Schedule) (bool, error) {
	f.conflictChecked = true
	return f.scheduleConflict, f.scheduleConflictErr
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

func (f *fakeRepository) GetInvitationForDoctor(_ context.Context, _, _ string, _ time.Time) (*response.DoctorHospitalInvitation, error) {
	if f.invitation == nil {
		return nil, repository.ErrInvitationNotFound
	}
	copy := *f.invitation
	return &copy, nil
}

func (f *fakeRepository) AcceptInvitation(_ context.Context, invitationID, _ string, now time.Time) error {
	if f.acceptErr != nil {
		return f.acceptErr
	}
	f.acceptedInvitation = invitationID
	f.invitation.Status = "ACCEPTED"
	f.invitation.RespondedAt = &now
	return nil
}

func (f *fakeRepository) UpdateAffiliationStatus(_ context.Context, _, _, status, _ string, _ time.Time) error {
	f.updatedStatus = status
	return f.updateStatusErr
}

func (f *fakeRepository) RejectInvitation(_ context.Context, invitationID, _ string, _ time.Time) error {
	f.rejectedInvitation = invitationID
	return nil
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

func TestSearchDoctorUsesUnifiedIdentity(t *testing.T) {
	repo := &fakeRepository{searchResults: []response.DoctorSearchResult{
		{ID: uuid.NewString(), Email: "doctor@example.com", Username: "doctor_example"},
	}}
	service := NewService(repo, &fakeStorage{}, nil, MaxContractBytes, time.Minute)

	results, err := service.SearchDoctor(context.Background(), request.DoctorSearchQuery{Identity: "  Doctor Example  "})
	if err != nil {
		t.Fatalf("search doctor returned error: %v", err)
	}
	if repo.searchIdentity != "Doctor Example" {
		t.Fatalf("identity was not normalized: %q", repo.searchIdentity)
	}
	if repo.searchLimit != MaxDoctorSearchResults {
		t.Fatalf("search limit = %d, want %d", repo.searchLimit, MaxDoctorSearchResults)
	}
	if len(results) != 1 || results[0].Email != "doctor@example.com" {
		t.Fatalf("unexpected search results: %#v", results)
	}
}

func TestSearchDoctorValidatesIdentity(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeStorage{}, nil, MaxContractBytes, time.Minute)
	for _, identity := range []string{"", "a", strings.Repeat("a", 191)} {
		if _, err := service.SearchDoctor(context.Background(), request.DoctorSearchQuery{Identity: identity}); err == nil {
			t.Fatalf("identity %q should be rejected", identity)
		}
	}
}

func TestSearchDoctorReturnsEmptyArray(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeStorage{}, nil, MaxContractBytes, time.Minute)
	results, err := service.SearchDoctor(context.Background(), request.DoctorSearchQuery{Identity: "unknown"})
	if err != nil {
		t.Fatalf("empty search returned error: %v", err)
	}
	if results == nil || len(results) != 0 {
		t.Fatalf("empty search must return a non-nil empty array, got %#v", results)
	}
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

func TestCreateInvitationAllowsOmittedInitialSchedule(t *testing.T) {
	doctorID := uuid.NewString()
	repo := &fakeRepository{
		eligible: &response.DoctorSearchResult{ID: doctorID}, departmentExists: true,
	}
	service := NewService(repo, &fakeStorage{}, nil, MaxContractBytes, time.Minute)

	created, err := service.CreateInvitation(context.Background(), uuid.NewString(), uuid.NewString(), request.CreateDoctorHospitalInvitationRequest{
		DoctorID: doctorID, DepartmentID: uuid.NewString(),
	}, UploadedFile{Filename: "contract.pdf", MIMEType: "application/pdf", Content: []byte("%PDF-test")})
	if err != nil {
		t.Fatalf("invitation without schedule returned error: %v", err)
	}
	if created == nil || len(repo.createInput.Schedules) != 0 {
		t.Fatalf("unexpected invitation schedules: %#v", repo.createInput.Schedules)
	}
	if repo.conflictChecked {
		t.Fatal("empty schedule should not execute a schedule conflict query")
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

func TestCreateInvitationRejectsExistingCrossHospitalScheduleBeforeUpload(t *testing.T) {
	repo := &fakeRepository{
		eligible:         &response.DoctorSearchResult{ID: uuid.NewString()},
		departmentExists: true,
		scheduleConflict: true,
	}
	objectStorage := &fakeStorage{}
	service := NewService(repo, objectStorage, nil, MaxContractBytes, time.Minute)

	_, err := service.CreateInvitation(context.Background(), uuid.NewString(), uuid.NewString(), request.CreateDoctorHospitalInvitationRequest{
		DoctorID: repo.eligible.ID, DepartmentID: uuid.NewString(),
		Schedules: []request.DoctorInvitationScheduleRequest{{DayOfWeek: 1, StartTime: "08:00", EndTime: "12:00"}},
	}, UploadedFile{Filename: "contract.pdf", MIMEType: "application/pdf", Content: []byte("%PDF-test")})
	if !errors.Is(err, constant.ErrDoctorScheduleConflict) {
		t.Fatalf("expected schedule conflict, got %v", err)
	}
	if objectStorage.uploadedPath != "" {
		t.Fatalf("contract was uploaded before conflict rejection: %s", objectStorage.uploadedPath)
	}
}

func TestInvitationResponseRequiresNoPayloadOrSignedContract(t *testing.T) {
	doctorID := uuid.NewString()
	invitationID := uuid.NewString()
	repo := &fakeRepository{invitation: &response.DoctorHospitalInvitation{
		ID: invitationID, DoctorID: doctorID, Status: "PENDING",
	}}
	objectStorage := &fakeStorage{}
	service := NewService(repo, objectStorage, nil, MaxContractBytes, time.Minute)

	accepted, err := service.AcceptInvitation(context.Background(), doctorID, invitationID)
	if err != nil {
		t.Fatalf("accept without payload failed: %v", err)
	}
	if repo.acceptedInvitation != invitationID || accepted.Status != "ACCEPTED" {
		t.Fatalf("invitation was not accepted: %#v", accepted)
	}
	if objectStorage.uploadedPath != "" {
		t.Fatalf("accept unexpectedly uploaded a signed contract: %s", objectStorage.uploadedPath)
	}

	repo.invitation.Status = "PENDING"
	if err := service.RejectInvitation(context.Background(), doctorID, invitationID); err != nil {
		t.Fatalf("reject without payload failed: %v", err)
	}
	if repo.rejectedInvitation != invitationID {
		t.Fatalf("invitation %s was not rejected", invitationID)
	}
}

func TestAcceptInvitationMapsHospitalWorkerDOBFailures(t *testing.T) {
	doctorID := uuid.NewString()
	invitationID := uuid.NewString()
	tests := []struct {
		name    string
		repoErr error
		want    error
	}{
		{
			name: "missing dob", repoErr: repository.ErrHospitalWorkerDOBRequired,
			want: constant.NewFieldRequiredError("dob"),
		},
		{
			name: "underage", repoErr: repository.ErrHospitalWorkerUnderage,
			want: constant.ErrHospitalWorkerMinimumAge,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepository{
				invitation: &response.DoctorHospitalInvitation{
					ID: invitationID, DoctorID: doctorID, Status: entity.DoctorHospitalInvitationPending,
				},
				acceptErr: test.repoErr,
			}
			service := NewService(repo, &fakeStorage{}, nil, MaxContractBytes, time.Minute)
			if _, err := service.AcceptInvitation(context.Background(), doctorID, invitationID); !errors.Is(err, test.want) {
				t.Fatalf("AcceptInvitation() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestReactivationMapsHospitalWorkerAgeFailureButSuspensionRemainsAllowed(t *testing.T) {
	doctorID := uuid.NewString()
	hospitalID := uuid.NewString()
	repo := &fakeRepository{updateStatusErr: repository.ErrHospitalWorkerUnderage}
	service := NewService(repo, &fakeStorage{}, nil, MaxContractBytes, time.Minute)

	if err := service.UpdateAffiliationStatus(context.Background(), hospitalID, doctorID, entity.DoctorHospitalAffiliationActive, uuid.NewString()); !errors.Is(err, constant.ErrHospitalWorkerMinimumAge) {
		t.Fatalf("reactivation error = %v, want %v", err, constant.ErrHospitalWorkerMinimumAge)
	}

	repo.updateStatusErr = nil
	if err := service.UpdateAffiliationStatus(context.Background(), hospitalID, doctorID, entity.DoctorHospitalAffiliationSuspended, uuid.NewString()); err != nil {
		t.Fatalf("suspension should not require an eligible DOB: %v", err)
	}
	if repo.updatedStatus != entity.DoctorHospitalAffiliationSuspended {
		t.Fatalf("updated status = %q, want %q", repo.updatedStatus, entity.DoctorHospitalAffiliationSuspended)
	}
}
