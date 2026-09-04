package infrastructure

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

func TestReadinessReturns503WithoutRuntimeDetails(t *testing.T) {
	setGinTestConfig()
	resetHealthChecksForTest()
	registerHealthCheck("database", func(_ context.Context) error {
		return errors.New("connection string and other internal detail")
	})
	router := NewGinEngine()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/_internal/readyz", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"status":"not_ready"`) {
		t.Fatalf("response = %s, want not_ready", body)
	}
	for _, sensitive := range []string{"connection string", "database", "memory", "go_version", "environment"} {
		if strings.Contains(strings.ToLower(body), sensitive) {
			t.Fatalf("readiness response leaks %q: %s", sensitive, body)
		}
	}
}

func TestLivenessDoesNotDependOnReadinessChecks(t *testing.T) {
	setGinTestConfig()
	resetHealthChecksForTest()
	registerHealthCheck("redis", func(_ context.Context) error { return errors.New("down") })
	router := NewGinEngine()

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/_internal/livez", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"alive"`) {
		t.Fatalf("liveness response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestCORSUsesExactConfiguredAllowlist(t *testing.T) {
	setGinTestConfig()
	resetHealthChecksForTest()
	router := NewGinEngine()
	router.GET("/resource", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	allowed := httptest.NewRecorder()
	allowedRequest := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	allowedRequest.Header.Set("Origin", "https://app.example.com")
	allowedRequest.Header.Set("Access-Control-Request-Method", http.MethodGet)
	router.ServeHTTP(allowed, allowedRequest)
	if allowed.Code != http.StatusNoContent || allowed.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("allowed preflight = %d headers=%v", allowed.Code, allowed.Header())
	}

	denied := httptest.NewRecorder()
	deniedRequest := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	deniedRequest.Header.Set("Origin", "https://evil.example.com")
	deniedRequest.Header.Set("Access-Control-Request-Method", http.MethodGet)
	router.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusForbidden || denied.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("denied preflight = %d headers=%v", denied.Code, denied.Header())
	}
}

func TestCORSAllowsAnyLocalhostPortForDev(t *testing.T) {
	setGinTestConfig()
	resetHealthChecksForTest()
	router := NewGinEngine()
	router.GET("/resource", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for _, origin := range []string{"http://localhost:3000", "http://localhost:5173", "https://127.0.0.1:4000"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodOptions, "/resource", nil)
		request.Header.Set("Origin", origin)
		request.Header.Set("Access-Control-Request-Method", http.MethodGet)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNoContent || recorder.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatalf("preflight for %s = %d headers=%v", origin, recorder.Code, recorder.Header())
		}
	}

	// A host merely containing "localhost" must not slip through (e.g. an attacker-controlled
	// domain like notlocalhost.evil.com or localhost.evil.com).
	denied := httptest.NewRecorder()
	deniedRequest := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	deniedRequest.Header.Set("Origin", "https://localhost.evil.com")
	deniedRequest.Header.Set("Access-Control-Request-Method", http.MethodGet)
	router.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusForbidden || denied.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("denied preflight = %d headers=%v", denied.Code, denied.Header())
	}
}

func TestAccessLogClientFingerprintIsKeyed(t *testing.T) {
	setGinTestConfig()
	config.Env.JWT.Secret = "first-secret-with-at-least-32-bytes"
	first := accessLogClientFingerprint("203.0.113.10")
	if strings.Contains(first, "203.0.113.10") {
		t.Fatal("access log fingerprint exposed the raw IP")
	}
	config.Env.JWT.Secret = "second-secret-with-at-least-32-byte"
	if second := accessLogClientFingerprint("203.0.113.10"); second == first {
		t.Fatal("access log fingerprint did not depend on the environment secret")
	}
}

func TestSecurityHeadersPreventSensitiveResponseCaching(t *testing.T) {
	setGinTestConfig()
	config.Env.Env = "staging"
	resetHealthChecksForTest()
	router := NewGinEngine()
	router.GET("/private", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/private", nil))
	for header, want := range map[string]string{
		"Cache-Control":             "no-store",
		"Pragma":                    "no-cache",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "no-referrer",
		"Strict-Transport-Security": "max-age=31536000",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestRecoveryReturnsAPIErrorWithTraceID(t *testing.T) {
	setGinTestConfig()
	resetHealthChecksForTest()
	router := NewGinEngine()
	router.GET("/panic", func(*gin.Context) { panic("must-not-reach-client") })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"message":"INTERNAL_SERVER_ERROR"`) || !strings.Contains(body, `"trace_id":`) {
		t.Fatalf("unexpected recovery response: %s", body)
	}
	if strings.Contains(body, "must-not-reach-client") {
		t.Fatalf("panic value leaked to client: %s", body)
	}
}

func TestRequestBodyLimitRejectsChunkedOversizeJSON(t *testing.T) {
	setGinTestConfig()
	resetHealthChecksForTest()
	router := NewGinEngine()
	router.POST("/upload", func(c *gin.Context) {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			util.HandleError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})

	payload := `{"value":"` + strings.Repeat("a", (10<<20)+1) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.ContentLength = -1 // exercise streaming/chunked requests too
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
}

func setGinTestConfig() {
	gin.SetMode(gin.TestMode)
	config.Env = &config.EnvConfig{
		Env: "test",
		Server: config.Server{
			CORSAllowedOrigins: []string{"https://app.example.com"},
		},
	}
}

func resetHealthChecksForTest() {
	healthChecks.Lock()
	defer healthChecks.Unlock()
	healthChecks.items = make(map[string]healthCheck)
}
