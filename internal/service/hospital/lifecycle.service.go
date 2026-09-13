package hospital

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
)

func (s *Service) ListHospitals(ctx context.Context, search, city string, limit, offset int) ([]response.Hospital, error) {
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return nil, constant.NewInvalidFieldValueError("pagination", "limit 1–100 and offset 0–100000", "limit 1–100 dan offset 0–100000")
	}
	search, city = strings.TrimSpace(search), strings.TrimSpace(city)
	if len(search) > 160 || len(city) > 100 {
		return nil, constant.NewInvalidFieldValueError("search", "search at most 160 and city at most 100 characters", "search maksimal 160 dan city maksimal 100 karakter")
	}
	rows, err := s.hospitalRepo.ListPublic(ctx, search, city, limit, offset)
	return rows, mapLifecycleError(err)
}

func (s *Service) GetHospital(ctx context.Context, hospitalID string) (*response.Hospital, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	row, err := s.hospitalRepo.GetPublic(ctx, hospitalID)
	return row, mapLifecycleError(err)
}

func hospitalUpdateFields(req request.UpdateHospitalRequest, now time.Time) (map[string]any, error) {
	fields := make(map[string]any)
	for name, field := range map[string]*string{"code": req.Code, "name": req.Name, "address": req.Address, "city": req.City, "province": req.Province, "country": req.Country, "phone": req.Phone, "description": req.Description} {
		if field == nil {
			continue
		}
		value := strings.TrimSpace(*field)
		if name == "code" {
			value = strings.ToUpper(value)
		}
		if value == "" && name != "description" {
			return nil, constant.NewFieldRequiredError(name)
		}
		if name == "description" {
			fields[name] = sp(value)
		} else {
			fields[name] = value
		}
	}
	if req.Latitude != nil {
		if *req.Latitude < -90 || *req.Latitude > 90 {
			return nil, constant.ErrInvalidHospitalCoordinates
		}
		fields["latitude"] = *req.Latitude
	}
	if req.Longitude != nil {
		if *req.Longitude < -180 || *req.Longitude > 180 {
			return nil, constant.ErrInvalidHospitalCoordinates
		}
		fields["longitude"] = *req.Longitude
	}
	if len(req.Facilities) > 0 {
		var value any
		if err := json.Unmarshal(req.Facilities, &value); err != nil {
			return nil, constant.ErrInvalidHospitalFacilities
		}
		switch value.(type) {
		case nil, map[string]any, []any:
			fields["facilities"] = datatypes.JSON(req.Facilities)
		default:
			return nil, constant.ErrInvalidHospitalFacilities
		}
	}
	if len(fields) == 0 {
		return nil, constant.NewFieldRequiredError("at least one update field")
	}
	fields["updated_at"] = now
	return fields, nil
}

func (s *Service) UpdateHospital(ctx context.Context, hospitalID string, req request.UpdateHospitalRequest) (*response.Hospital, error) {
	if _, err := uuid.Parse(hospitalID); err != nil {
		return nil, constant.ErrInvalidUUIDFormat
	}
	fields, err := hospitalUpdateFields(req, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	row, err := s.hospitalRepo.UpdateHospital(ctx, hospitalID, fields)
	return row, mapLifecycleError(err)
}

func (s *Service) DeleteHospital(ctx context.Context, hospitalID, actorID string) error {
	if _, err := uuid.Parse(actorID); err != nil {
		return constant.ErrInvalidUUIDFormat
	}
	resolvedID, err := s.hospitalRepo.ResolveHospitalID(ctx, strings.TrimSpace(hospitalID))
	if err != nil {
		return mapLifecycleError(err)
	}
	return mapLifecycleError(s.hospitalRepo.DeleteHospital(ctx, resolvedID, actorID, time.Now().UTC()))
}

func mapLifecycleError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return constant.ErrHospitalNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return constant.ErrHospitalAlreadyExists
	case errors.Is(err, repository.ErrResourceInUse):
		return constant.ErrResourceInUse
	default:
		return constant.ErrInternalServerError
	}
}
