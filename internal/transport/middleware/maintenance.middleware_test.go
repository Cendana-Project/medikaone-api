package middleware

import (
	"testing"
	"time"
)

func TestActiveRequestHeartbeatInterval(t *testing.T) {
	if got := activeRequestHeartbeatInterval(60); got != 20*time.Second {
		t.Fatalf("activeRequestHeartbeatInterval(60) = %v, want 20s", got)
	}
	if got := activeRequestHeartbeatInterval(1); got != time.Second {
		t.Fatalf("activeRequestHeartbeatInterval(1) = %v, want 1s minimum", got)
	}
}
