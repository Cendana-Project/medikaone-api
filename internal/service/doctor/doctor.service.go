package doctor

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor"
)

type Repository interface {
	ListDoctors(context.Context, repository.Filter) (*response.PublicDoctorPage, error)
	GetDoctor(context.Context, string) (*response.PublicDoctorDetail, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

var medikaOneIDPattern = regexp.MustCompile(`^MDO-[0-9A-F]{16}$`)

func (s *Service) ListDoctors(ctx context.Context, filter repository.Filter) (*response.PublicDoctorPage, error) {
	return s.listDoctors(ctx, filter, 20)
}

func (s *Service) RecommendDoctors(ctx context.Context, filter repository.Filter) (*response.PublicDoctorPage, error) {
	filter.Recommended = true
	return s.listDoctors(ctx, filter, 10)
}

func (s *Service) listDoctors(ctx context.Context, filter repository.Filter, defaultLimit int) (*response.PublicDoctorPage, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Specialty = strings.TrimSpace(filter.Specialty)
	filter.HospitalID = strings.TrimSpace(filter.HospitalID)
	filter.DepartmentCode = strings.ToUpper(strings.TrimSpace(filter.DepartmentCode))
	filter.DepartmentID = strings.TrimSpace(filter.DepartmentID)
	filter.City = strings.TrimSpace(filter.City)
	filter.AvailableOn = strings.TrimSpace(filter.AvailableOn)
	filter.BookingMode = strings.ToUpper(strings.TrimSpace(filter.BookingMode))
	if len(filter.Query) > 190 || len(filter.Specialty) > 190 || len(filter.DepartmentCode) > 40 || len(filter.City) > 100 {
		return nil, constant.NewInvalidFieldLengthError("directory filter", "q/specialty <=190, department_code <=40, and city <=100 characters", "q/specialty <=190, department_code <=40, dan city <=100 karakter")
	}
	if filter.HospitalID != "" {
		id, err := uuid.Parse(filter.HospitalID)
		if err != nil {
			return nil, constant.ErrInvalidUUIDFormat
		}
		filter.HospitalID = id.String()
	}
	if filter.DepartmentID != "" {
		id, err := uuid.Parse(filter.DepartmentID)
		if err != nil {
			return nil, constant.ErrInvalidUUIDFormat
		}
		filter.DepartmentID = id.String()
	}
	if filter.AvailableOn != "" {
		date, err := time.Parse("2006-01-02", filter.AvailableOn)
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if err != nil || date.Before(today) || date.After(today.AddDate(1, 0, 0)) {
			return nil, constant.NewInvalidFieldValueError("available_on", "a date from today through one year ahead in YYYY-MM-DD format", "tanggal hari ini hingga satu tahun ke depan dengan format YYYY-MM-DD")
		}
	}
	if filter.BookingMode != "" && filter.BookingMode != "FIXED_SLOT" && filter.BookingMode != "SESSION_QUEUE" {
		return nil, constant.NewInvalidFieldValueError("booking_mode", "FIXED_SLOT or SESSION_QUEUE", "FIXED_SLOT atau SESSION_QUEUE")
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.Limit == 0 {
		filter.Limit = defaultLimit
	}
	if filter.Page < 1 || filter.Page > 100000 || filter.Limit < 1 || filter.Limit > 100 {
		return nil, constant.NewInvalidFieldValueError("page/limit", "page between 1 and 100000 and limit between 1 and 100", "page antara 1 dan 100000 dan limit antara 1 dan 100")
	}
	out, err := s.repo.ListDoctors(ctx, filter)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return out, nil
}

func (s *Service) GetDoctor(ctx context.Context, identity string) (*response.PublicDoctorDetail, error) {
	identity = strings.TrimSpace(identity)
	if id, err := uuid.Parse(identity); err == nil {
		identity = id.String()
	} else {
		identity = strings.ToUpper(identity)
		if !medikaOneIDPattern.MatchString(identity) {
			return nil, constant.NewInvalidFieldValueError("doctor_id", "a UUID or MDO- followed by 16 hexadecimal characters", "UUID atau MDO- diikuti 16 karakter heksadesimal")
		}
	}
	out, err := s.repo.GetDoctor(ctx, identity)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, constant.ErrDoctorNotFound
	}
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	return out, nil
}
