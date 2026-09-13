package appointment

import (
	"context"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/google/uuid"
)

func (s *Service) CreateDoctorSpecificSchedule(ctx context.Context, doctorID string, req request.CreateSpecificScheduleRequest) (*response.ScheduleChangeRequest, error) {
	return s.createSpecificSchedule(ctx, doctorID, entity.ScheduleChangePartyDoctor, "", req)
}

func (s *Service) CreateHospitalSpecificSchedule(ctx context.Context, hospitalID, actorID string, req request.CreateSpecificScheduleRequest) (*response.ScheduleChangeRequest, error) {
	return s.createSpecificSchedule(ctx, actorID, entity.ScheduleChangePartyHospital, hospitalID, req)
}

func (s *Service) createSpecificSchedule(ctx context.Context, actorID, party, hospitalID string, req request.CreateSpecificScheduleRequest) (*response.ScheduleChangeRequest, error) {
	if req.Schedule.ScheduleDate == nil {
		return nil, constant.NewFieldRequiredError("schedule.schedule_date")
	}
	return s.createScheduleMutation(ctx, actorID, party, hospitalID, request.CreateScheduleChangeRequest{AffiliationID: req.AffiliationID, Reason: req.Reason, Schedules: []request.DoctorInvitationScheduleRequest{req.Schedule}}, "ADD", nil)
}

func (s *Service) DeleteDoctorSchedule(ctx context.Context, doctorID, scheduleID string) (*response.ScheduleChangeRequest, error) {
	return s.deleteSchedule(ctx, doctorID, entity.ScheduleChangePartyDoctor, "", scheduleID)
}

func (s *Service) DeleteHospitalSchedule(ctx context.Context, hospitalID, actorID, scheduleID string) (*response.ScheduleChangeRequest, error) {
	return s.deleteSchedule(ctx, actorID, entity.ScheduleChangePartyHospital, hospitalID, scheduleID)
}

// Deletion is a counterpart-reviewed change, preserving the established approval contract.
func (s *Service) deleteSchedule(ctx context.Context, actorID, party, hospitalID, scheduleID string) (*response.ScheduleChangeRequest, error) {
	if _, err := uuid.Parse(scheduleID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	schedule, err := s.repo.GetActiveSchedule(ctx, scheduleID)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if (party == entity.ScheduleChangePartyDoctor && schedule.DoctorID != actorID) || (party == entity.ScheduleChangePartyHospital && schedule.HospitalID != hospitalID) {
		return nil, constant.ErrScheduleNotFound
	}
	return s.createScheduleMutation(ctx, actorID, party, hospitalID, request.CreateScheduleChangeRequest{AffiliationID: schedule.AffiliationID}, "REMOVE", &scheduleID)
}
