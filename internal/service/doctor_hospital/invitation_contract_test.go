package doctor_hospital

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	"github.com/Cendana-Project/medikaone-api/internal/storage"
	"github.com/google/uuid"
)

type contractUpdateRepository struct {
	lifecycleFake
	invitation                       *response.DoctorHospitalInvitation
	readErr                          error
	readHospitalID, readInvitationID string
}

func (f *contractUpdateRepository) GetInvitationForHospital(_ context.Context, hospitalID, invitationID string, _ time.Time) (*response.DoctorHospitalInvitation, error) {
	f.readHospitalID, f.readInvitationID = hospitalID, invitationID
	return f.invitation, f.readErr
}

func TestInvitationContractReplacementAndCleanup(t *testing.T) {
	for _, failWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "replace", true: "concurrent accept"}[failWrite], func(t *testing.T) {
			now := time.Now().UTC()
			repo := &contractUpdateRepository{invitation: &response.DoctorHospitalInvitation{Status: "PENDING", ExpiresAt: now.Add(time.Hour)}}
			if failWrite {
				repo.err = repository.ErrInvalidInvitationState
			}
			objects := &fakeStorage{}
			svc := NewService(repo, objects, nil, 0, time.Minute)
			hospitalID, invitationID, actorID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			file := &UploadedFile{Filename: "../new.pdf", MIMEType: "application/pdf", Content: []byte("%PDF-new")}
			_, err := svc.UpdateInvitationWithContract(context.Background(), hospitalID, invitationID, actorID, request.UpdateDoctorHospitalInvitationRequest{}, file)
			if failWrite && !errors.Is(err, constant.ErrInvalidDoctorInvitationState) || !failWrite && err != nil {
				t.Fatal(err)
			}
			if repo.readHospitalID != hospitalID || repo.readInvitationID != invitationID || repo.input.ActorID != actorID {
				t.Fatal("lost tenant/actor scope")
			}
			if repo.input.Contract == nil || repo.input.Contract.Filename != "new.pdf" || repo.input.Contract.ObjectPath != objects.uploadedPath || len(repo.input.Contract.SHA256) != 64 {
				t.Fatalf("missing contract metadata: %#v", repo.input.Contract)
			}
			if !strings.HasPrefix(objects.uploadedPath, "hospitals/"+hospitalID+"/doctor-invitations/"+invitationID+"/original/") {
				t.Fatal("incorrect upload ownership path")
			}
			if failWrite && objects.deletedPath != objects.uploadedPath || !failWrite && objects.deletedPath != "" {
				t.Fatal("only failed replacement upload must be deleted")
			}
			if repo.input.Schedules != nil || repo.input.DepartmentID != nil {
				t.Fatal("contract-only update must preserve omitted terms")
			}
		})
	}
}

func TestInvitationContractPreflightRejectsInvalidFilesAndInaccessibleState(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name, status string
		expiry       time.Time
		readErr      error
		file         UploadedFile
		want         error
	}{
		{name: "wrong tenant", status: "PENDING", expiry: now.Add(time.Hour), readErr: repository.ErrInvitationNotFound, want: constant.ErrDoctorInvitationNotFound},
		{name: "accepted", status: "ACCEPTED", expiry: now.Add(time.Hour), want: constant.ErrInvalidDoctorInvitationState},
		{name: "expired", status: "PENDING", expiry: now.Add(-time.Hour), want: constant.ErrDoctorInvitationExpired},
		{name: "invalid PDF", status: "PENDING", expiry: now.Add(time.Hour), file: UploadedFile{Filename: "new.pdf", MIMEType: "application/pdf", Content: []byte("not-pdf")}, want: constant.ErrInvalidContractPDF},
		{name: "configured size limit", status: "PENDING", expiry: now.Add(time.Hour), file: UploadedFile{Filename: "new.pdf", MIMEType: "application/pdf", Content: []byte("%PDF-too-large")}, want: constant.NewFileTooLargeError(10)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &contractUpdateRepository{invitation: &response.DoctorHospitalInvitation{Status: tc.status, ExpiresAt: tc.expiry}, readErr: tc.readErr}
			objects := &fakeStorage{}
			svc := NewService(repo, objects, nil, 10, time.Minute)
			file := tc.file
			if file.Filename == "" {
				file = UploadedFile{Filename: "new.pdf", MIMEType: "application/pdf", Content: []byte("%PDF-new")}
			}
			_, err := svc.UpdateInvitationWithContract(context.Background(), uuid.NewString(), uuid.NewString(), uuid.NewString(), request.UpdateDoctorHospitalInvitationRequest{}, &file)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if objects.uploadedPath != "" || repo.input.Contract != nil {
				t.Fatal("invalid update reached upload or write")
			}
		})
	}
}

func TestContractStorageSizeFailureIsPublic413(t *testing.T) {
	repo := &contractUpdateRepository{invitation: &response.DoctorHospitalInvitation{Status: "PENDING", ExpiresAt: time.Now().Add(time.Hour)}}
	objects := &fakeStorage{uploadErr: storage.ErrFileTooLarge}
	svc := NewService(repo, objects, nil, 0, time.Minute)
	file := &UploadedFile{Filename: "new.pdf", MIMEType: "application/pdf", Content: []byte("%PDF-new")}
	_, err := svc.UpdateInvitationWithContract(context.Background(), uuid.NewString(), uuid.NewString(), uuid.NewString(), request.UpdateDoctorHospitalInvitationRequest{}, file)
	if !errors.Is(err, constant.ErrFileTooLarge) {
		t.Fatalf("expected FILE_TOO_LARGE, got %v", err)
	}
}
