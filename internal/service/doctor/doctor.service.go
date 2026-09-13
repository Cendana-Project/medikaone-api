package doctor

import (
	"context"
	"errors"
	"regexp"
	"strings"

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
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Specialty = strings.TrimSpace(filter.Specialty)
	filter.HospitalID = strings.TrimSpace(filter.HospitalID)
	if len(filter.Query) > 190 || len(filter.Specialty) > 190 {
		return nil, constant.NewInvalidFieldLengthError("q or specialty", "at most 190 characters long", "memiliki maksimal 190 karakter")
	}
	if filter.HospitalID != "" {
		id, err := uuid.Parse(filter.HospitalID)
		if err != nil {
			return nil, constant.ErrInvalidUUIDFormat
		}
		filter.HospitalID = id.String()
	}
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.Limit == 0 {
		filter.Limit = 20
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
