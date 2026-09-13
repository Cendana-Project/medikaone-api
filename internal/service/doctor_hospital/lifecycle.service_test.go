package doctor_hospital

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	"github.com/google/uuid"
)

type lifecycleFake struct {
	Repository
	LifecycleRepository
	input      repository.UpdateInvitationInput
	hospitalID string
	resourceID string
	err        error
}

func (f *lifecycleFake) UpdateInvitation(_ context.Context, input repository.UpdateInvitationInput) (*response.DoctorHospitalInvitation, error) {
	f.input = input
	return &response.DoctorHospitalInvitation{ID: input.InvitationID, HospitalID: input.HospitalID}, f.err
}

func (f *lifecycleFake) DeleteDepartment(_ context.Context, hospitalID, departmentID string, _ time.Time) error {
	f.hospitalID, f.resourceID = hospitalID, departmentID
	return f.err
}

func (f *lifecycleFake) UpdateDepartment(_ context.Context, hospitalID, departmentID string, fields map[string]any) (*entity.HospitalDepartment, error) {
	f.hospitalID, f.resourceID = hospitalID, departmentID
	return &entity.HospitalDepartment{ID: departmentID, HospitalID: hospitalID, Name: fields["name"].(string)}, f.err
}

func TestUpdateInvitationPreservesOmittedFieldsAndClearsExplicitSchedules(t *testing.T) {
	repo := &lifecycleFake{}
	svc := NewService(repo, nil, nil, 0, time.Minute)
	hospitalID, invitationID, actorID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	message := " Updated invitation "
	if _, err := svc.UpdateInvitation(context.Background(), hospitalID, invitationID, actorID, request.UpdateDoctorHospitalInvitationRequest{Message: &message}); err != nil {
		t.Fatal(err)
	}
	if repo.input.Schedules != nil || repo.input.DepartmentID != nil || repo.input.RoomID != nil {
		t.Fatal("omitted fields must remain unchanged")
	}
	if repo.input.HospitalID != hospitalID || repo.input.InvitationID != invitationID || repo.input.ActorID != actorID || *repo.input.Message != "Updated invitation" {
		t.Fatalf("unexpected update: %#v", repo.input)
	}
	emptySchedules := []request.DoctorInvitationScheduleRequest{}
	emptyRoom := " "
	if _, err := svc.UpdateInvitation(context.Background(), hospitalID, invitationID, actorID, request.UpdateDoctorHospitalInvitationRequest{Schedules: &emptySchedules, RoomID: &emptyRoom}); err != nil {
		t.Fatal(err)
	}
	if repo.input.Schedules == nil || len(*repo.input.Schedules) != 0 || repo.input.RoomID == nil || *repo.input.RoomID != "" {
		t.Fatal("explicit empty fields must clear values")
	}
}

func TestUpdateInvitationRejectsInvalidScheduleAndEmptyPatch(t *testing.T) {
	svc := NewService(&lifecycleFake{}, nil, nil, 0, time.Minute)
	for _, req := range []request.UpdateDoctorHospitalInvitationRequest{
		{},
		{Schedules: &[]request.DoctorInvitationScheduleRequest{{DayOfWeek: []int{1}, StartTime: "12:00", EndTime: "11:00"}}},
		{Schedules: &[]request.DoctorInvitationScheduleRequest{{DayOfWeek: []int{7}, StartTime: "10:00", EndTime: "11:00"}}},
	} {
		if _, err := svc.UpdateInvitation(context.Background(), uuid.NewString(), uuid.NewString(), uuid.NewString(), req); err == nil {
			t.Fatalf("invalid patch accepted: %#v", req)
		}
	}
}

func TestDepartmentMutationScopesTenantAndMapsConflict(t *testing.T) {
	repo := &lifecycleFake{err: repository.ErrResourceInUse}
	svc := NewService(repo, nil, nil, 0, time.Minute)
	hospitalID, departmentID := uuid.NewString(), uuid.NewString()
	if err := svc.DeleteDepartment(context.Background(), hospitalID, departmentID); !errors.Is(err, constant.ErrResourceInUse) {
		t.Fatalf("conflict should be public HTTP 409, got %v", err)
	}
	if repo.hospitalID != hospitalID || repo.resourceID != departmentID {
		t.Fatal("resource mutation lost tenant scope")
	}
	repo.err = repository.ErrPlacementNotFound
	name := "Renamed"
	if _, err := svc.UpdateDepartment(context.Background(), hospitalID, departmentID, request.UpdateHospitalDepartmentRequest{Name: &name}); !errors.Is(err, constant.ErrHospitalPlacementNotFound) {
		t.Fatalf("wrong tenant placement must return not found: %v", err)
	}
	if err := svc.DeleteDepartment(context.Background(), "invalid", departmentID); !errors.Is(err, constant.ErrInvalidUUIDFormat) {
		t.Fatal("invalid tenant accepted")
	}
}
