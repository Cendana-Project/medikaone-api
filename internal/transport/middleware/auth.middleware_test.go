package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"github.com/Cendana-Project/medikaone-api/internal/config"
)

func signedTestAccessToken(t *testing.T, method jwt.SigningMethod, secret []byte) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, jwt.MapClaims{
		"sub": "user-1", "typ": "access", "jti": "token-1", "sv": "version-1", "fid": "family-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func runAuthMiddleware(t *testing.T, token string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthRequired(nil, nil))
	router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAuthRequiredRejectsHS512(t *testing.T) {
	secret := []byte("test-secret-with-enough-entropy")
	config.Env = &config.EnvConfig{JWT: config.JWTConfig{Secret: string(secret)}}
	token := signedTestAccessToken(t, jwt.SigningMethodHS512, secret)

	response := runAuthMiddleware(t, token)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for HS512, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAuthRequiredRejectsTokenWithoutExpiration(t *testing.T) {
	secret := []byte("test-secret-with-enough-entropy")
	config.Env = &config.EnvConfig{JWT: config.JWTConfig{Secret: string(secret)}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1", "typ": "access", "jti": "token-1", "sv": "version-1", "fid": "family-1",
	}).SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	response := runAuthMiddleware(t, token)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for token without exp, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAuthRequiredFailsClosedWithoutRedis(t *testing.T) {
	secret := []byte("test-secret-with-enough-entropy")
	config.Env = &config.EnvConfig{JWT: config.JWTConfig{Secret: string(secret)}}
	token := signedTestAccessToken(t, jwt.SigningMethodHS256, secret)

	response := runAuthMiddleware(t, token)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when revocation store is unavailable, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAuthRequiredRejectsEveryAccessTokenInRevokedFamily(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_REDIS_DSN"))
	if dsn == "" {
		t.Skip("TEST_REDIS_DSN is not set")
	}
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		t.Fatalf("parse TEST_REDIS_DSN: %v", err)
	}
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })

	secret := []byte("test-secret-with-enough-entropy")
	previous := config.Env
	t.Cleanup(func() { config.Env = previous })
	config.Env = &config.EnvConfig{
		Env:      "test",
		JWT:      config.JWTConfig{Secret: string(secret)},
		Database: config.Database{DSN: "postgresql://test:test@localhost:5432/test?sslmode=disable"},
	}
	userID := "user-family-test"
	familyID := "family-revoked-test"
	revocationKey := accessFamilyRevokedKey(userID, familyID)
	ctx := t.Context()
	if err := rdb.Set(ctx, revocationKey, "1", time.Minute).Err(); err != nil {
		t.Fatalf("store family revocation: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Del(context.Background(), revocationKey).Err() })

	for _, jti := range []string{"access-one", "access-two"} {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": userID, "typ": "access", "jti": jti, "sv": "version-1", "fid": familyID,
			"exp": time.Now().Add(time.Hour).Unix(),
		}).SignedString(secret)
		if err != nil {
			t.Fatalf("sign access token: %v", err)
		}

		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.Use(AuthRequired(rdb, nil))
		router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		request := httptest.NewRequest(http.MethodGet, "/protected", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("JTI %q status = %d, want 401", jti, response.Code)
		}
	}
}
