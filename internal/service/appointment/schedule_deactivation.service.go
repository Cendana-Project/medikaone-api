package appointment

import (
	"context"
	"unicode/utf8"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	"github.com/google/uuid"
)

func (s *Service) DeactivateDoctorSchedules(ctx context.Context, doctorID, affiliationID string, req request.DeactivateSchedulesRequest) (*response.ScheduleChangeRequest, error) {
	return s.deactivateSchedules(ctx, doctorID, entity.ScheduleChangePartyDoctor, "", affiliationID, req)
}

func (s *Service) DeactivateHospitalSchedules(ctx context.Context, hospitalID, actorID, affiliationID string, req request.DeactivateSchedulesRequest) (*response.ScheduleChangeRequest, error) {
	return s.deactivateSchedules(ctx, actorID, entity.ScheduleChangePartyHospital, hospitalID, affiliationID, req)
}

func (s *Service) deactivateSchedules(ctx context.Context, actorID, party, hospitalID, affiliationID string, req request.DeactivateSchedulesRequest) (*response.ScheduleChangeRequest, error) {
	if _, err := uuid.Parse(affiliationID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	if req.Scope != "ALL" && req.Scope != "RECURRING_DAY" {
		return nil, constant.NewInvalidFieldValueError("scope", "ALL or RECURRING_DAY", "ALL atau RECURRING_DAY")
	}
	if req.Scope == "ALL" && req.DayOfWeek != nil {
		return nil, constant.NewInvalidFieldValueError("day_of_week", "omitted when scope is ALL", "tidak dikirim ketika scope ALL")
	}
	if req.Scope == "RECURRING_DAY" && req.DayOfWeek == nil {
		return nil, constant.NewFieldRequiredError("day_of_week")
	}
	if req.DayOfWeek != nil && (*req.DayOfWeek < 0 || *req.DayOfWeek > 6) {
		return nil, constant.NewInvalidFieldValueError("day_of_week", "an integer from 0 (Sunday) to 6 (Saturday)", "angka bulat dari 0 (Minggu) sampai 6 (Sabtu)")
	}
	if req.Reason != nil && utf8.RuneCountInString(*req.Reason) > 1000 {
		return nil, constant.NewInvalidFieldValueError("reason", "at most 1000 characters", "maksimal 1000 karakter")
	}
	affiliation, err := s.repo.GetAffiliation(ctx, affiliationID)
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	if (party == entity.ScheduleChangePartyDoctor && affiliation.DoctorID != actorID) ||
		(party == entity.ScheduleChangePartyHospital && affiliation.HospitalID != hospitalID) {
		return nil, constant.ErrAffiliationNotFound
	}
	now := s.now()
	row, err := s.repo.CreateScheduleChange(ctx, repository.ScheduleChangeInput{
		AffiliationID: affiliationID, ActorID: actorID, ActorParty: party, HospitalID: hospitalID,
		Operation: "DEACTIVATE", DeactivationScope: req.Scope, DeactivationDayOfWeek: req.DayOfWeek,
		Reason: cleanOptional(req.Reason), Now: now, ExpiresAt: now.Add(ScheduleChangeTTL),
	})
	if err != nil {
		return nil, mapRepositoryError(err)
	}
	return row, nil
}
