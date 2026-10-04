package hospital

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHospitalDiscoveryRejectsAmbiguousQueryBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctl := &Controller{}
	router := gin.New()
	router.GET("/hospitals", ctl.ListHospitals)
	router.GET("/recommendations/hospitals", ctl.RecommendHospitals)
	for _, path := range []string{"/hospitals", "/recommendations/hospitals"} {
		for _, query := range []string{"open_now=", "open_now=yes", "open_now=true&open_now=false", "only_available=1", "available_on=", "available_from=09:00&available_from=10:00"} {
			out := httptest.NewRecorder()
			router.ServeHTTP(out, httptest.NewRequest(http.MethodGet, path+"?"+query, nil))
			if out.Code != http.StatusBadRequest {
				t.Fatalf("query %s status %d", query, out.Code)
			}
		}
	}
}
