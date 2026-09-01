package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDurationSecondsCeil(t *testing.T) {
	for _, test := range []struct {
		duration time.Duration
		want     int64
	}{
		{duration: time.Millisecond, want: 1},
		{duration: time.Second, want: 1},
		{duration: time.Second + time.Millisecond, want: 2},
	} {
		if got := durationSecondsCeil(test.duration); got != test.want {
			t.Fatalf("durationSecondsCeil(%s) = %d, want %d", test.duration, got, test.want)
		}
	}
}

func TestRateLimitClientIPIgnoresUntrustedForwardedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/", nil)
	ctx.Request.RemoteAddr = "192.0.2.10:1234"
	ctx.Request.Header.Set("X-Forwarded-For", "203.0.113.99")

	got, err := ClientIP(ctx, "")
	if err != nil || got != "192.0.2.10" {
		t.Fatalf("rateLimitClientIP() = %q, %v; want socket peer", got, err)
	}
}

func TestRateLimitClientIPUsesConfiguredForwardedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/", nil)
	ctx.Request.RemoteAddr = "192.0.2.10:1234"
	ctx.Request.Header.Set("X-Forwarded-For", "203.0.113.7, 192.0.2.10")

	got, err := ClientIP(ctx, "X-Forwarded-For")
	if err != nil || got != "203.0.113.7" {
		t.Fatalf("rateLimitClientIP() = %q, %v; want first trusted hop", got, err)
	}
}

func TestRateLimitClientIPRejectsInvalidTrustedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/", nil)
	ctx.Request.RemoteAddr = "192.0.2.10:1234"
	ctx.Request.Header.Set("X-Forwarded-For", "not-an-ip")

	if _, err := ClientIP(ctx, "X-Forwarded-For"); err == nil {
		t.Fatal("invalid trusted proxy header must be rejected")
	}
}

func TestClientIPFingerprintIsKeyed(t *testing.T) {
	ip := "203.0.113.7"
	first := clientIPFingerprint(ip, []byte("secret-a"))
	if first == clientIPFingerprint(ip, []byte("secret-b")) {
		t.Fatal("client IP fingerprint must depend on its secret")
	}
	if first != clientIPFingerprint(ip, []byte("secret-a")) {
		t.Fatal("client IP fingerprint must be deterministic")
	}
}
