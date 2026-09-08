package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/gin-gonic/gin"
)

type fakeGlobalRoleChecker struct {
	hasRole bool
	err     error
	role    string
}

func (f *fakeGlobalRoleChecker) UserHasRole(_ context.Context, _, roleSlug string) (bool, error) {
	f.role = roleSlug
	return f.hasRole, f.err
}

func TestRequirePatientRejectsNonPatient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &fakeGlobalRoleChecker{hasRole: false}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(constant.UserID), "11111111-1111-4111-8111-111111111111")
		c.Next()
	})
	router.PUT("/v1/profile/patient", requirePatient(checker), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/v1/profile/patient", nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if checker.role != constant.RolePatient {
		t.Errorf("checked role = %q, want %q", checker.role, constant.RolePatient)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"message":"PATIENT_ROLE_REQUIRED"`) {
		t.Errorf("response = %s, want PATIENT_ROLE_REQUIRED", body)
	}
}

func TestRequirePatientAllowsGlobalPatient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &fakeGlobalRoleChecker{hasRole: true}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(constant.UserID), "11111111-1111-4111-8111-111111111111")
		c.Next()
	})
	router.PUT("/v1/profile/patient", requirePatient(checker), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/v1/profile/patient", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}
