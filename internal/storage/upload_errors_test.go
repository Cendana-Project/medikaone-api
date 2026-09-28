package storage

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSupabaseFileLimitErrors(t *testing.T) {
	for _, tc := range []struct {
		status    int
		body      string
		oversized bool
	}{
		{413, "entity too large", true},
		{400, `{"statusCode":"413","error":"Payload too large"}`, true},
		{400, `{"statusCode":413}`, true},
		{400, `{"code":"EntityTooLarge"}`, true},
		{403, `{"code":"AccessDenied"}`, false},
		{500, `{"error":"internal error"}`, false},
	} {
		err := responseError("upload object", &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))})
		if errors.Is(err, ErrFileTooLarge) != tc.oversized {
			t.Fatalf("status=%d body=%s error=%v", tc.status, tc.body, err)
		}
	}
}
