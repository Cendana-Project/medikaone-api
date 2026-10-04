package doctor

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

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

func TestExperienceGenderAndAvailabilityFilters(t *testing.T) {
	number := func(v int) *int { return &v }
	date := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	for _, filter := range []repository.Filter{
		{Gender: "X"}, {MinExperienceYears: number(-1)}, {MaxExperienceYears: number(101)},
		{MinExperienceYears: number(10), MaxExperienceYears: number(3)},
		{OnlyAvailable: true}, {AvailableOn: date, AvailableFrom: "09:00"},
	} {
		for _, recommended := range []bool{false, true} {
			repo := &fakeRepository{}
			svc := NewService(repo)
			method := svc.ListDoctors
			if recommended {
				method = svc.RecommendDoctors
			}
			if _, err := method(context.Background(), filter); err == nil || repo.calls != 0 {
				t.Fatalf("accepted invalid filter: %#v %v", filter, err)
			}
		}
	}
	repo := &fakeRepository{}
	_, err := NewService(repo).RecommendDoctors(context.Background(), repository.Filter{Gender: " p ", MinExperienceYears: number(0), MaxExperienceYears: number(10), AvailableOn: date, AvailableFrom: "09:00", AvailableTo: "10:00", OnlyAvailable: true})
	if err != nil || repo.filter.Gender != "P" || repo.filter.MinExperienceYears == nil || *repo.filter.MinExperienceYears != 0 || !repo.filter.OnlyAvailable || repo.filter.AvailableFrom != "09:00" {
		t.Fatalf("lost filter: %#v %v", repo.filter, err)
	}
}

func TestDoctorLocationValidation(t *testing.T) {
	number := func(v float64) *float64 { return &v }
	for _, filter := range []repository.Filter{
		{Latitude: number(0)}, {Longitude: number(0)},
		{Latitude: number(91), Longitude: number(0)},
		{Latitude: number(0), Longitude: number(-181)},
		{Latitude: number(math.NaN()), Longitude: number(0)},
		{Latitude: number(0), Longitude: number(math.Inf(1))},
		{RadiusKM: number(10)},
		{Latitude: number(0), Longitude: number(0), RadiusKM: number(0)},
		{Latitude: number(0), Longitude: number(0), RadiusKM: number(-1)},
		{Latitude: number(0), Longitude: number(0), RadiusKM: number(5001)},
		{Latitude: number(0), Longitude: number(0), RadiusKM: number(math.NaN())},
		{Latitude: number(0), Longitude: number(0), RadiusKM: number(math.Inf(-1))},
	} {
		for _, recommended := range []bool{false, true} {
			repo := &fakeRepository{}
			svc := NewService(repo)
			method := svc.ListDoctors
			if recommended {
				method = svc.RecommendDoctors
			}
			if _, err := method(context.Background(), filter); err == nil || repo.calls != 0 {
				t.Fatalf("invalid location must fail before querying: %#v recommended=%v err=%v", filter, recommended, err)
			}
		}
	}
	for _, filter := range []repository.Filter{
		{}, {Latitude: number(0), Longitude: number(0)},
		{Latitude: number(-90), Longitude: number(180), RadiusKM: number(5000)},
		{Latitude: number(90), Longitude: number(-180)},
	} {
		repo := &fakeRepository{}
		if _, err := NewService(repo).RecommendDoctors(context.Background(), filter); err != nil || repo.calls != 1 || !repo.filter.Recommended || repo.filter.Latitude != filter.Latitude || repo.filter.Longitude != filter.Longitude || repo.filter.RadiusKM != filter.RadiusKM {
			t.Fatalf("valid location not preserved: %#v err=%v", repo.filter, err)
		}
	}
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
		{HospitalID: "invalid"}, {DepartmentID: "invalid"}, {Query: strings.Repeat("a", 191)},
		{BookingMode: "unknown"}, {AvailableOn: "2020-01-01"},
	} {
		repo := &fakeRepository{}
		if _, err := NewService(repo).ListDoctors(context.Background(), filter); err == nil || repo.calls != 0 {
			t.Fatalf("invalid filter %#v must fail before querying", filter)
		}
	}
}

func TestRecommendDoctorsNormalizesMobileFilters(t *testing.T) {
	repo := &fakeRepository{}
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	_, err := NewService(repo).RecommendDoctors(context.Background(), repository.Filter{
		DepartmentCode: " poli-mata ", DepartmentID: " 22222222-2222-4222-8222-222222222222 ",
		City: " Jakarta ", AvailableOn: tomorrow, BookingMode: " fixed_slot ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !repo.filter.Recommended || repo.filter.Limit != 10 || repo.filter.DepartmentCode != "POLI-MATA" ||
		repo.filter.City != "Jakarta" || repo.filter.BookingMode != "FIXED_SLOT" {
		t.Fatalf("recommendation filter = %#v", repo.filter)
	}
}
