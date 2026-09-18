package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor"
)

type fakeRepository struct {
	filter   repository.Filter
	identity string
	calls    int
	err      error
}

func (r *fakeRepository) ListDoctors(_ context.Context, filter repository.Filter) (*response.PublicDoctorPage, error) {
	r.filter, r.calls = filter, r.calls+1
	return &response.PublicDoctorPage{Items: []response.PublicDoctor{}, Page: filter.Page, Limit: filter.Limit}, r.err
}

func (r *fakeRepository) GetDoctor(_ context.Context, identity string) (*response.PublicDoctorDetail, error) {
	r.identity, r.calls = identity, r.calls+1
	return &response.PublicDoctorDetail{PublicDoctor: response.PublicDoctor{DoctorMedikaOneID: "MDO-0123456789ABCDEF"}}, r.err
}

func TestGetDoctorAcceptsBothIDsAndNormalizes(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"  mdo-0123456789abcdef  ", "MDO-0123456789ABCDEF"},
		{"22222222-2222-4222-8222-ABCDEFABCDEF", "22222222-2222-4222-8222-abcdefabcdef"},
	} {
		repo := &fakeRepository{}
		result, err := NewService(repo).GetDoctor(context.Background(), test.input)
		if err != nil || repo.identity != test.want || result.DoctorMedikaOneID == "" {
			t.Fatalf("GetDoctor(%q) = %#v, %v; repository identity = %q", test.input, result, err, repo.identity)
		}
	}
}

func TestGetDoctorRejectsMalformedIDsBeforeQuery(t *testing.T) {
	for _, identity := range []string{"", "doctor@example.test", "MDO-123", "MDO-0123456789ABCDEG"} {
		repo := &fakeRepository{}
		if _, err := NewService(repo).GetDoctor(context.Background(), identity); err == nil || repo.calls != 0 {
			t.Fatalf("GetDoctor(%q) should reject before querying", identity)
		}
	}
}

func TestGetDoctorMapsMissingAndStorageErrors(t *testing.T) {
	for _, test := range []struct{ stored, want error }{
		{gorm.ErrRecordNotFound, constant.ErrDoctorNotFound},
		{errors.New("storage failed"), constant.ErrInternalServerError},
	} {
		_, err := NewService(&fakeRepository{err: test.stored}).GetDoctor(context.Background(), "MDO-0123456789ABCDEF")
		if !errors.Is(err, test.want) {
			t.Fatalf("GetDoctor() error = %v, want %v", err, test.want)
		}
	}
}

func TestListDoctorsNormalizesAndBoundsQueries(t *testing.T) {
	repo := &fakeRepository{}
	_, err := NewService(repo).ListDoctors(context.Background(), repository.Filter{Query: "  dr. Dian ", Specialty: " Cardiology "})
	if err != nil || repo.filter.Page != 1 || repo.filter.Limit != 20 || repo.filter.Query != "dr. Dian" || repo.filter.Specialty != "Cardiology" {
		t.Fatalf("ListDoctors() filter = %#v, error = %v", repo.filter, err)
	}
	for _, filter := range []repository.Filter{
		{Page: -1}, {Page: 100001}, {Limit: -1}, {Limit: 101},
		{HospitalID: "invalid"}, {Query: strings.Repeat("a", 191)},
	} {
		repo := &fakeRepository{}
		if _, err := NewService(repo).ListDoctors(context.Background(), filter); err == nil || repo.calls != 0 {
			t.Fatalf("invalid filter %#v must fail before querying", filter)
		}
	}
}
