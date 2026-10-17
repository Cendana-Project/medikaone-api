package specialty

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

type Repository interface {
	ListSpecialties(context.Context, request.SpecialtyQuery) ([]response.Specialty, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

var specialtyCodePattern = regexp.MustCompile(`^SP-[A-Z0-9]+(-[A-Z0-9]+)*$`)

func (s *Service) ListSpecialties(ctx context.Context, q request.SpecialtyQuery) ([]response.Specialty, error) {
	q.Search = strings.TrimSpace(q.Search)
	if utf8.RuneCountInString(q.Search) > 190 {
		return nil, constant.NewInvalidFieldValueError("q", "at most 190 characters", "maksimal 190 karakter")
	}
	if q.Code != "" && (len(q.Code) > 64 || !specialtyCodePattern.MatchString(q.Code)) {
		return nil, constant.NewInvalidFieldValueError("code", "uppercase local specialty code", "kode spesialisasi lokal huruf besar")
	}
	switch q.Category {
	case "", "GENERAL", "MEDICAL_SPECIALIST", "DENTAL_SPECIALIST":
	default:
		return nil, constant.NewInvalidFieldValueError("category", "GENERAL, MEDICAL_SPECIALIST or DENTAL_SPECIALIST", "GENERAL, MEDICAL_SPECIALIST atau DENTAL_SPECIALIST")
	}
	if q.Limit == 0 {
		q.Limit = 100
	}
	if q.Limit < 1 || q.Limit > 100 {
		return nil, constant.NewInvalidFieldValueError("limit", "integer 1-100", "angka bulat 1-100")
	}
	if q.Offset < 0 {
		return nil, constant.NewInvalidFieldValueError("offset", "nonnegative integer", "angka bulat tidak negatif")
	}
	rows, err := s.repo.ListSpecialties(ctx, q)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if rows == nil {
		rows = []response.Specialty{}
	}
	return rows, nil
}
