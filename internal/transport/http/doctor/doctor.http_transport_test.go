package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor"
	service "github.com/Cendana-Project/medikaone-api/internal/service/doctor"
	"github.com/gin-gonic/gin"
)

type locationRepository struct {
	filter repository.Filter
	calls  int
}

func (r *locationRepository) ListDoctors(_ context.Context, f repository.Filter) (*response.PublicDoctorPage, error) {
	r.filter, r.calls = f, r.calls+1
	return &response.PublicDoctorPage{Items: []response.PublicDoctor{}, Page: f.Page, Limit: f.Limit}, nil
}
func (*locationRepository) GetDoctor(context.Context, string) (*response.PublicDoctorDetail, error) {
	return nil, nil
}

func TestDoctorLocationQueryParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/doctors", "/recommendations/doctors"} {
		for _, test := range []struct {
			query  string
			status int
		}{
			{"latitude=-6.2&longitude=106.8&radius_km=10", http.StatusOK},
			{"latitude=0&longitude=0", http.StatusOK},
			{"", http.StatusOK},
			{"latitude=abc&longitude=0", http.StatusBadRequest},
			{"latitude=&longitude=0", http.StatusBadRequest},
			{"latitude=0&latitude=1&longitude=0", http.StatusBadRequest},
			{"latitude=0", http.StatusBadRequest},
			{"latitude=NaN&longitude=0", http.StatusBadRequest},
			{"latitude=0&longitude=Inf", http.StatusBadRequest},
			{"latitude=0&longitude=181", http.StatusBadRequest},
			{"radius_km=10", http.StatusBadRequest},
			{"latitude=0&longitude=0&radius_km=0", http.StatusBadRequest},
		} {
			t.Run(path+"?"+test.query, func(t *testing.T) {
				repo := &locationRepository{}
				ctl := NewController(service.NewService(repo))
				router := gin.New()
				router.GET("/doctors", ctl.ListDoctors)
				router.GET("/recommendations/doctors", ctl.RecommendDoctors)
				out := httptest.NewRecorder()
				router.ServeHTTP(out, httptest.NewRequest(http.MethodGet, path+"?"+test.query, nil))
				if out.Code != test.status {
					t.Fatalf("status=%d want=%d body=%s", out.Code, test.status, out.Body.String())
				}
				if test.status != http.StatusOK && repo.calls != 0 {
					t.Fatal("invalid query reached repository")
				}
				if test.status == http.StatusOK {
					if repo.calls != 1 || repo.filter.Recommended != (path == "/recommendations/doctors") {
						t.Fatalf("filter=%#v", repo.filter)
					}
					if test.query == "latitude=-6.2&longitude=106.8&radius_km=10" && (repo.filter.Latitude == nil || *repo.filter.Latitude != -6.2 || repo.filter.Longitude == nil || *repo.filter.Longitude != 106.8 || repo.filter.RadiusKM == nil || *repo.filter.RadiusKM != 10) {
						t.Fatalf("location lost: %#v", repo.filter)
					}
				}
			})
		}
	}
}
