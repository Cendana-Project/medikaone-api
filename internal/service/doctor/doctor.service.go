package doctor

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/directorycriteria"
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
	filter.Gender = strings.ToUpper(strings.TrimSpace(filter.Gender))
	if filter.Gender != "" && filter.Gender != "L" && filter.Gender != "P" {
		return nil, constant.NewInvalidFieldValueError("gender", "L or P", "L (laki-laki) atau P (perempuan)")
	}
	for _, value := range []*int{filter.MinExperienceYears, filter.MaxExperienceYears} {
		if value != nil && (*value < 0 || *value > 100) {
			return nil, constant.NewInvalidFieldValueError("experience_years", "an integer between 0 and 100", "bilangan bulat antara 0 dan 100")
		}
	}
	if filter.MinExperienceYears != nil && filter.MaxExperienceYears != nil && *filter.MinExperienceYears > *filter.MaxExperienceYears {
		return nil, constant.NewInvalidFieldValueError("min_experience_years", "less than or equal to max_experience_years", "lebih kecil atau sama dengan max_experience_years")
	}
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Specialty = strings.TrimSpace(filter.Specialty)
	filter.HospitalID = strings.TrimSpace(filter.HospitalID)
	filter.DepartmentCode = strings.ToUpper(strings.TrimSpace(filter.DepartmentCode))
	filter.DepartmentID = strings.TrimSpace(filter.DepartmentID)
	filter.City = strings.TrimSpace(filter.City)
	filter.AvailableOn = strings.TrimSpace(filter.AvailableOn)
	filter.BookingMode = strings.ToUpper(strings.TrimSpace(filter.BookingMode))
	if (filter.Latitude == nil) != (filter.Longitude == nil) {
		return nil, constant.NewInvalidFieldValueError("coordinates", "latitude and longitude supplied together", "latitude dan longitude dikirim berpasangan")
	}
	for _, coordinate := range []struct {
		name  string
		value *float64
		bound float64
	}{{"latitude", filter.Latitude, 90}, {"longitude", filter.Longitude, 180}} {
		if coordinate.value != nil && (math.IsNaN(*coordinate.value) || math.IsInf(*coordinate.value, 0) || math.Abs(*coordinate.value) > coordinate.bound) {
			return nil, constant.NewInvalidFieldValueError(coordinate.name, "a finite coordinate within latitude -90..90 and longitude -180..180", "koordinat angka berhingga dalam latitude -90..90 dan longitude -180..180")
		}
	}
	if filter.RadiusKM != nil && (filter.Latitude == nil || math.IsNaN(*filter.RadiusKM) || math.IsInf(*filter.RadiusKM, 0) || *filter.RadiusKM <= 0 || *filter.RadiusKM > 5000) {
		return nil, constant.NewInvalidFieldValueError("radius_km", "greater than 0 through 5000, with latitude and longitude", "lebih dari 0 hingga 5000, dengan latitude dan longitude")
	}
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
	availability := directorycriteria.Availability{Date: filter.AvailableOn, From: filter.AvailableFrom, To: filter.AvailableTo, OnlyAvailable: filter.OnlyAvailable, BookingMode: filter.BookingMode}
	if err := availability.Validate(time.Now()); err != nil {
		return nil, err
	}
	filter.AvailableOn, filter.AvailableFrom, filter.AvailableTo, filter.BookingMode = availability.Date, availability.From, availability.To, availability.BookingMode
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
