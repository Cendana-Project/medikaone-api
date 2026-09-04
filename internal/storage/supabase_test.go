package storage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/config"
)

func TestNewSupabaseClientRejectsUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Storage
	}{
		{name: "disabled", cfg: config.Storage{}},
		{name: "wrong provider", cfg: config.Storage{Enabled: true, Provider: "s3"}},
		{name: "non https URL", cfg: config.Storage{Enabled: true, Provider: "supabase", Bucket: "contracts", Supabase: config.SupabaseStorage{URL: "http://example.com", SecretKey: "sb_secret_test"}}},
		{name: "public key", cfg: config.Storage{Enabled: true, Provider: "supabase", Bucket: "contracts", Supabase: config.SupabaseStorage{URL: "https://example.com", SecretKey: "sb_publishable_test"}}},
		{name: "empty bucket", cfg: config.Storage{Enabled: true, Provider: "supabase", Supabase: config.SupabaseStorage{URL: "https://example.com", SecretKey: "sb_secret_test"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewSupabaseClient(tt.cfg); err == nil {
				t.Fatal("expected invalid storage configuration to be rejected")
			}
		})
	}
}

func TestSupabaseClientObjectLifecycle(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "sb_secret_test" || r.Header.Get("Authorization") != "Bearer sb_secret_test" {
			http.Error(w, "missing backend authorization", http.StatusUnauthorized)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/contracts/"):
			content, _ := io.ReadAll(r.Body)
			if string(content) != "%PDF-test" || r.Header.Get("x-upsert") != "false" {
				http.Error(w, "unexpected upload", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/sign/contracts/"):
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["expiresIn"] != float64(300) || payload["download"] != "contract.pdf" {
				http.Error(w, "unexpected signed URL payload", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"signedURL": "/storage/v1/object/sign/contracts/token"})
		case r.Method == http.MethodDelete && r.URL.Path == "/storage/v1/object/contracts":
			var payload struct {
				Prefixes []string `json:"prefixes"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if len(payload.Prefixes) != 1 || payload.Prefixes[0] != "hospitals/h-1/contract.pdf" {
				http.Error(w, "unexpected delete payload", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &SupabaseClient{
		baseURL: server.URL, secretKey: "sb_secret_test", bucket: "contracts",
		http: server.Client(),
	}
	objectPath := "hospitals/h-1/contract.pdf"
	uploaded, err := client.Upload(context.Background(), objectPath, "application/pdf", []byte("%PDF-test"))
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if uploaded.Bucket != "contracts" || uploaded.ObjectPath != objectPath || uploaded.FileSize != 9 {
		t.Fatalf("unexpected uploaded object: %#v", uploaded)
	}
	signedURL, err := client.CreateSignedURL(context.Background(), objectPath, 5*time.Minute, "../contract.pdf")
	if err != nil {
		t.Fatalf("create signed URL failed: %v", err)
	}
	if signedURL != server.URL+"/storage/v1/object/sign/contracts/token" {
		t.Fatalf("unexpected signed URL: %s", signedURL)
	}
	if err := client.Delete(context.Background(), objectPath); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 Supabase calls, got %d: %v", len(calls), calls)
	}
}

func TestCreateSignedURLWithoutNameIsInline(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_ = json.NewEncoder(w).Encode(map[string]string{"signedURL": "/signed/profile"})
	}))
	defer server.Close()
	client := &SupabaseClient{
		baseURL: server.URL, secretKey: "sb_secret_test", bucket: "profile-images", http: server.Client(),
	}
	if _, err := client.CreateSignedURL(context.Background(), "users/user-1/photo.png", time.Minute, ""); err != nil {
		t.Fatalf("CreateSignedURL() error = %v", err)
	}
	if _, exists := payload["download"]; exists {
		t.Fatalf("inline signed URL unexpectedly contains download option: %#v", payload)
	}
}

func TestMaxFileSizeNeverExceedsTenMegabytes(t *testing.T) {
	if got := MaxFileSize(config.Storage{MaxFileSizeBytes: 20 * 1024 * 1024}); got != defaultMaxFileSize {
		t.Fatalf("expected hard limit %d, got %d", defaultMaxFileSize, got)
	}
	if got := MaxFileSize(config.Storage{MaxFileSizeBytes: 2 * 1024 * 1024}); got != 2*1024*1024 {
		t.Fatalf("expected lower configured limit, got %d", got)
	}
}
