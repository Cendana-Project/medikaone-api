package doctorhospital

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor_hospital"
	service "github.com/Cendana-Project/medikaone-api/internal/service/doctor_hospital"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func multipartPatchContext(t *testing.T, fields map[string][]string, files int, fileBytes int) *gin.Context {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for field, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(field, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := 0; i < files; i++ {
		part, err := writer.CreateFormFile("contract", "replacement.pdf")
		if err != nil {
			t.Fatal(err)
		}
		payload := make([]byte, fileBytes)
		copy(payload, "%PDF-test")
		if _, err := part.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPatch, "/", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request.ContentLength = -1 // Must still enforce the cap for streamed bodies.
	return c
}

func TestMultipartInvitationPatchPreservesFieldPresence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := multipartPatchContext(t, map[string][]string{"room_id": {""}, "message": {""}, "schedules": {"[]"}}, 1, 12)
	if err := parseContractMultipart(c, 100); err != nil {
		t.Fatal(err)
	}
	defer c.Request.MultipartForm.RemoveAll()
	req, file, err := decodeInvitationPatch(c, 100)
	if err != nil {
		t.Fatal(err)
	}
	if req.DepartmentID != nil || req.RoomID == nil || *req.RoomID != "" || req.Message == nil || *req.Message != "" || req.Schedules == nil || len(*req.Schedules) != 0 {
		t.Fatalf("field presence lost: %#v", req)
	}
	if file == nil || file.Filename != "replacement.pdf" || !bytes.HasPrefix(file.Content, []byte("%PDF-")) {
		t.Fatalf("file missing: %#v", file)
	}
	for _, fields := range []map[string][]string{nil, {"schedules": {"null"}}} {
		c := multipartPatchContext(t, fields, 1, 12)
		if err := parseContractMultipart(c, 100); err != nil {
			t.Fatal(err)
		}
		req, file, err := decodeInvitationPatch(c, 100)
		_ = c.Request.MultipartForm.RemoveAll()
		if err != nil || file == nil || req.Schedules != nil {
			t.Fatalf("contract-only or null schedules altered terms: %#v %v", req, err)
		}
	}
}

func TestMultipartInvitationPatchRejectsAmbiguousAndOversizedInput(t *testing.T) {
	for _, tc := range []struct {
		name        string
		fields      map[string][]string
		files, size int
		want        string
	}{
		{name: "duplicate files", files: 2, size: 12, want: "INVALID_FIELD_VALUE"},
		{name: "file too large", files: 1, size: 101, want: "FILE_TOO_LARGE"},
		{name: "duplicate fields", fields: map[string][]string{"message": {"one", "two"}}, want: "INVALID_FIELD_VALUE"},
		{name: "unknown doctor", fields: map[string][]string{"doctor_id": {uuid.NewString()}}, want: "UNKNOWN_FIELD"},
		{name: "invalid schedules", fields: map[string][]string{"schedules": {"[broken]"}}, want: "MALFORMED_JSON"},
		{name: "object schedules", fields: map[string][]string{"schedules": {"{}"}}, want: "INVALID_FIELD_VALUE"},
		{name: "unknown schedule field", fields: map[string][]string{"schedules": {"[{\"invalid\":1}]"}}, want: "UNKNOWN_FIELD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := multipartPatchContext(t, tc.fields, tc.files, tc.size)
			if err := parseContractMultipart(c, 100); err != nil {
				t.Fatal(err)
			}
			defer c.Request.MultipartForm.RemoveAll()
			_, _, err := decodeInvitationPatch(c, 100)
			var apiErr response.CustomError
			if !errors.As(err, &apiErr) || apiErr.Code != tc.want {
				t.Fatalf("got %v want %s", err, tc.want)
			}
		})
	}
}

func TestContractMultipartRequestBoundary(t *testing.T) {
	for _, streamed := range []bool{true, false} {
		c := multipartPatchContext(t, nil, 1, int(service.MaxContractBytes))
		if !streamed {
			c.Request.ContentLength = service.MaxContractBytes + 300
		}
		if err := parseContractMultipart(c, service.MaxContractBytes); err != nil {
			t.Fatalf("exact max file rejected: %v", err)
		}
		_, err := readMultipartPDF(c, "contract", service.MaxContractBytes)
		_ = c.Request.MultipartForm.RemoveAll()
		if err != nil {
			t.Fatalf("exact max file rejected: %v", err)
		}
	}
	c := multipartPatchContext(t, nil, 1, int(maxMultipartRequestBytes))
	if err := parseContractMultipart(c, service.MaxContractBytes); !errors.Is(err, constant.ErrRequestTooLarge) {
		t.Fatalf("streamed oversized body must return 413: %v", err)
	}
}

type invitationPatchRepository struct {
	service.Repository
	service.LifecycleRepository
	input repository.UpdateInvitationInput
}

func (r *invitationPatchRepository) UpdateInvitation(_ context.Context, input repository.UpdateInvitationInput) (*response.DoctorHospitalInvitation, error) {
	r.input = input
	return &response.DoctorHospitalInvitation{ID: input.InvitationID}, nil
}

func TestInvitationJSONPatchCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &invitationPatchRepository{}
	controller := NewController(service.NewService(repo, nil, nil, 0, time.Minute))
	hospitalID, invitationID, actorID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	router := gin.New()
	router.PATCH("/:invitation_id", func(c *gin.Context) {
		c.Set("hospital_id", hospitalID)
		c.Set(string(constant.UserID), actorID)
		controller.UpdateInvitation(c)
	})
	req := httptest.NewRequest(http.MethodPatch, "/"+invitationID, strings.NewReader(`{"message":"updated","schedules":null}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	var body struct{ Message string }
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if recorder.Code != 200 || body.Message != "DOCTOR_INVITATION_UPDATED" || repo.input.Contract != nil || repo.input.Schedules != nil || repo.input.HospitalID != hospitalID || repo.input.ActorID != actorID {
		t.Fatalf("legacy JSON PATCH changed: %s", recorder.Body.String())
	}
}
