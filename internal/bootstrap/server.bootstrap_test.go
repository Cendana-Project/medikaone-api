package bootstrap

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/config"
)

func TestNewHTTPServerAppliesResourceTimeouts(t *testing.T) {
	config.Env = &config.EnvConfig{Server: config.Server{
		Port:              "8081",
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       4 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       6 * time.Second,
	}}

	server := newHTTPServer(context.Background(), http.NewServeMux())
	if server.Addr != ":8081" {
		t.Fatalf("Addr = %q, want :8081", server.Addr)
	}
	if server.ReadHeaderTimeout != 3*time.Second || server.ReadTimeout != 4*time.Second || server.WriteTimeout != 5*time.Second || server.IdleTimeout != 6*time.Second {
		t.Fatalf("HTTP timeouts were not applied: %+v", server)
	}
	if server.BaseContext == nil || server.MaxHeaderBytes != 1<<20 {
		t.Fatalf("server lifecycle/resource limits not configured: %+v", server)
	}
}
