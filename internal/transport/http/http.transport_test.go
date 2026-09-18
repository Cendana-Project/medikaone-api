package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/gin-gonic/gin"
)

// Register the complete router, then inspect the chain selected by Gin before
// invoking dependencies. This detects conflicting wildcard paths, missing
// routes, and accidental loss of authentication/tenant guards without a DB.
func TestLifecycleRoutesRegisterAndRetainGuards(t *testing.T) {
	previousConfig, previousMode := config.Env, gin.Mode()
	config.Env = &config.EnvConfig{}
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { config.Env = previousConfig; gin.SetMode(previousMode) })
	engine := gin.New()
	var matchedPath string
	var handlers []string
	engine.Use(func(c *gin.Context) {
		matchedPath, handlers = c.FullPath(), c.HandlerNames()
		c.AbortWithStatus(stdhttp.StatusNoContent)
	})
	NewTransport().WithGinEngine(engine).InitRoute()

	cases := []struct {
		method string
		path   string
		guard  string
		tenant bool
	}{
		{stdhttp.MethodGet, "/v1/doctors", "", false},
		{stdhttp.MethodGet, "/v1/doctors/:doctor_id", "", false},
		{stdhttp.MethodGet, "/v1/hospitals", "", false},
		{stdhttp.MethodGet, "/v1/hospitals/:hospital_id", "", false},
		{stdhttp.MethodGet, "/v1/hospitals/:hospital_id/images", "", false},
		{stdhttp.MethodGet, "/v1/hospitals/:hospital_id/reviews", "", false},
		{stdhttp.MethodGet, "/v1/hospitals/:hospital_id/reviews/me", "AuthRequired", false},
		{stdhttp.MethodPut, "/v1/hospitals/:hospital_id/reviews/me", "AuthRequired", false},
		{stdhttp.MethodDelete, "/v1/hospitals/:hospital_id/reviews/me", "AuthRequired", false},
		{stdhttp.MethodPost, "/v1/hospitals/:hospital_id/images", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodPatch, "/v1/hospitals/:hospital_id/images/:image_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodDelete, "/v1/hospitals/:hospital_id/images/:image_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodPatch, "/v1/hospitals/:hospital_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodDelete, "/v1/hospitals/:hospital_id", "RequireSuperAdmin", true},
		{stdhttp.MethodPatch, "/v1/hospitals/:hospital_id/departments/:department_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodDelete, "/v1/hospitals/:hospital_id/departments/:department_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodPatch, "/v1/hospitals/:hospital_id/rooms/:room_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodDelete, "/v1/hospitals/:hospital_id/rooms/:room_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodPatch, "/v1/hospitals/:hospital_id/doctor-invitations/:invitation_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodDelete, "/v1/hospitals/:hospital_id/doctor-invitations/:invitation_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodGet, "/v1/hospitals/:hospital_id/doctor-invitations/:invitation_id/contract", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodGet, "/v1/hospitals/:hospital_id/doctors/search", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodDelete, "/v1/hospitals/:hospital_id/doctors/:doctor_id", "RequireHospitalAdminOrSuper", true},
		{stdhttp.MethodPost, "/v1/doctor/specific-schedules", "RequirePermissions", false},
		{stdhttp.MethodDelete, "/v1/doctor/schedules/:schedule_id", "RequirePermissions", false},
		{stdhttp.MethodPost, "/v1/hospitals/:hospital_id/specific-schedules", "RequireHospitalPermissions", true},
		{stdhttp.MethodDelete, "/v1/hospitals/:hospital_id/schedules/:schedule_id", "RequireHospitalPermissions", true},
		{stdhttp.MethodDelete, "/v1/notifications/:notification_id", "AuthRequired", false},
		{stdhttp.MethodDelete, "/v1/account", "AuthRequired", false},
	}
	for _, test := range cases {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			parts := strings.Split(test.path, "/")
			for i := range parts {
				if strings.HasPrefix(parts[i], ":") {
					parts[i] = "11111111-1111-4111-8111-111111111111"
				}
			}
			matchedPath, handlers = "", nil
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(test.method, strings.Join(parts, "/"), nil))
			if matchedPath != test.path || recorder.Code != stdhttp.StatusNoContent {
				t.Fatalf("route matched %q with HTTP %d", matchedPath, recorder.Code)
			}
			chain := strings.Join(handlers, " ")
			if got := strings.Contains(chain, "AuthRequired"); got != (test.guard != "") {
				t.Fatalf("authentication middleware mismatch: %s", chain)
			}
			if test.guard != "" && !strings.Contains(chain, test.guard) {
				t.Fatalf("missing %s guard: %s", test.guard, chain)
			}
			if got := strings.Contains(chain, "TenantContext"); got != test.tenant {
				t.Fatalf("tenant middleware mismatch: %s", chain)
			}
		})
	}
}
