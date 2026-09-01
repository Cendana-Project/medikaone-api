package infrastructure

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactConnectionErrorRemovesURLAndPassword(t *testing.T) {
	rawURL := "postgresql://user:s3cr%40t@db.example.com/app?sslmode=require"
	err := errors.New("connect " + rawURL + ": password s3cr@t rejected")

	redacted := redactConnectionError(err, rawURL)
	for _, secret := range []string{rawURL, "s3cr@t", "s3cr%40t"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted error still contains %q: %s", secret, redacted)
		}
	}
}
