package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDeleteAccountRejectsMissingPasswordAndAccountTargetInBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`{}`, `{"current_password":"secret","user_id":"another-account"}`} {
		router := gin.New()
		controller := &Controller{}
		router.DELETE("/account", controller.DeleteAccount)
		request := httptest.NewRequest(http.MethodDelete, "/account", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid account deletion body accepted: status=%d", response.Code)
		}
	}
}
