package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/config"
)

const (
	defaultMaxFileSize = int64(10 * 1024 * 1024)
	defaultSignedTTL   = 5 * time.Minute
)

type SupabaseClient struct {
	baseURL   string
	secretKey string
	bucket    string
	http      *http.Client
}

func NewSupabaseClient(cfg config.Storage) (*SupabaseClient, error) {
	if !cfg.Enabled {
		return nil, errors.New("storage.enabled must be true")
	}
	if !strings.EqualFold(strings.TrimSpace(cfg.Provider), "supabase") {
		return nil, errors.New("storage.provider must be supabase")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.Supabase.URL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("storage.supabase.url must be a valid HTTPS URL")
	}
	if !strings.HasPrefix(strings.TrimSpace(cfg.Supabase.SecretKey), "sb_secret_") {
		return nil, errors.New("storage.supabase.secret_key must use a server-only sb_secret_ key")
	}
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, errors.New("storage.bucket is required")
	}
	return &SupabaseClient{
		baseURL: baseURL, secretKey: strings.TrimSpace(cfg.Supabase.SecretKey), bucket: bucket,
		http: &http.Client{Timeout: 45 * time.Second},
	}, nil
}

func (c *SupabaseClient) Upload(ctx context.Context, objectPath, contentType string, content []byte) (*UploadedObject, error) {
	if len(content) == 0 {
		return nil, errors.New("cannot upload an empty object")
	}
	endpoint := c.objectEndpoint(objectPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "false")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload object: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, responseError("upload object", resp)
	}
	return &UploadedObject{Bucket: c.bucket, ObjectPath: cleanObjectPath(objectPath), FileSize: int64(len(content))}, nil
}

func (c *SupabaseClient) Delete(ctx context.Context, objectPath string) error {
	body, _ := json.Marshal(map[string]any{"prefixes": []string{cleanObjectPath(objectPath)}})
	endpoint := c.baseURL + "/storage/v1/object/" + url.PathEscape(c.bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError("delete object", resp)
	}
	return nil
}

func (c *SupabaseClient) CreateSignedURL(ctx context.Context, objectPath string, ttl time.Duration, downloadName string) (string, error) {
	if ttl <= 0 {
		ttl = defaultSignedTTL
	}
	payload := map[string]any{"expiresIn": int64(ttl.Seconds()), "download": true}
	if strings.TrimSpace(downloadName) != "" {
		payload["download"] = safeDownloadName(downloadName)
	}
	body, _ := json.Marshal(payload)
	endpoint := c.baseURL + "/storage/v1/object/sign/" + url.PathEscape(c.bucket) + "/" + escapeObjectPath(objectPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("create signed URL: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", responseError("create signed URL", resp)
	}
	var decoded struct {
		SignedURL string `json:"signedURL"`
		SignedUrl string `json:"signedUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode signed URL response: %w", err)
	}
	signed := decoded.SignedURL
	if signed == "" {
		signed = decoded.SignedUrl
	}
	if signed == "" {
		return "", errors.New("Supabase returned an empty signed URL")
	}
	if strings.HasPrefix(signed, "/") {
		signed = c.baseURL + signed
	}
	return signed, nil
}

func (c *SupabaseClient) objectEndpoint(objectPath string) string {
	return c.baseURL + "/storage/v1/object/" + url.PathEscape(c.bucket) + "/" + escapeObjectPath(objectPath)
}

func (c *SupabaseClient) authorize(req *http.Request) {
	req.Header.Set("apikey", c.secretKey)
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
}

func escapeObjectPath(value string) string {
	parts := strings.Split(cleanObjectPath(value), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func cleanObjectPath(value string) string {
	return strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(value, "\\", "/")), "/")
}

func safeDownloadName(value string) string {
	value = path.Base(strings.ReplaceAll(value, "\\", "/"))
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	if value == "." || value == "" {
		return "contract.pdf"
	}
	return value
}

func responseError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("%s failed with status %d: %s", operation, resp.StatusCode, message)
}

func MaxFileSize(cfg config.Storage) int64 {
	if cfg.MaxFileSizeBytes <= 0 || cfg.MaxFileSizeBytes > defaultMaxFileSize {
		return defaultMaxFileSize
	}
	return cfg.MaxFileSizeBytes
}

func SignedURLTTL(cfg config.Storage) time.Duration {
	if cfg.SignedURLTTL <= 0 {
		return defaultSignedTTL
	}
	return cfg.SignedURLTTL
}
