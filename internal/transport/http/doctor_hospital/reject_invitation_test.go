package doctorhospital

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	service "github.com/Cendana-Project/medikaone-api/internal/service/doctor_hospital"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type rejectionRepository struct {
	service.Repository
	called                 bool
	invitationID, doctorID string
	message                *string
}

func (r *rejectionRepository) RejectInvitation(_ context.Context, invitationID, doctorID string, message *string, _ time.Time) error {
	r.called, r.invitationID, r.doctorID, r.message = true, invitationID, doctorID, message
	return nil
}

func TestRejectInvitationOptionalMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jsonMessage := func(message string) string {
		body, err := json.Marshal(map[string]string{"message": message})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	for _, tc := range []struct {
		name, body, wantMessage string
		status                  int
	}{
		{name: "legacy no body", status: 200},
		{name: "empty object", body: `{}`, status: 200},
		{name: "null message", body: `{"message":null}`, status: 200},
		{name: "blank message", body: `{"message":"  \n "}`, status: 200},
		{name: "trimmed message", body: `{"message":"  Jadwal berbenturan dengan praktik lain.  "}`, wantMessage: "Jadwal berbenturan dengan praktik lain.", status: 200},
		{name: "unicode limit", body: jsonMessage(strings.Repeat("界", 1000)), wantMessage: strings.Repeat("界", 1000), status: 200},
		{name: "too long", body: jsonMessage(strings.Repeat("a", 1001)), status: 400},
		{name: "unicode too long", body: jsonMessage(strings.Repeat("界", 1001)), status: 400},
		{name: "wrong field", body: `{"reason":"wrong field"}`, status: 400},
		{name: "wrong type", body: `{"message":123}`, status: 400},
		{name: "malformed", body: `{"message":`, status: 400},
		{name: "trailing JSON", body: `{"message":"first"} {}`, status: 400},
		{name: "trailing garbage", body: `{"message":"first"} broken`, status: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &rejectionRepository{}
			controller := NewController(service.NewService(repo, nil, nil, 0, time.Minute))
			doctorID, invitationID := uuid.NewString(), uuid.NewString()
			router := gin.New()
			router.POST("/invitations/:invitation_id/reject", func(c *gin.Context) {
				c.Set(string(constant.UserID), doctorID)
				controller.RejectInvitation(c)
			})
			req := httptest.NewRequest(http.MethodPost, "/invitations/"+invitationID+"/reject", strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
				req.ContentLength = -1 // Also support streamed/chunked request bodies.
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)
			if recorder.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.status, recorder.Body.String())
			}
			if tc.status != 200 {
				if repo.called {
					t.Fatal("invalid request reached repository")
				}
				return
			}
			if !repo.called || repo.invitationID != invitationID || repo.doctorID != doctorID {
				t.Fatal("request lost doctor identity or invitation path")
			}
			if tc.wantMessage == "" {
				if repo.message != nil {
					t.Fatalf("empty message should become nil: %q", *repo.message)
				}
			} else if repo.message == nil || *repo.message != tc.wantMessage {
				t.Fatalf("message was not preserved: %v", repo.message)
			}
			var payload struct {
				Message string
				Data    struct{ Rejected bool }
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil || payload.Message != "DOCTOR_INVITATION_REJECTED" || !payload.Data.Rejected {
				t.Fatalf("success response contract changed: %s", recorder.Body.String())
			}
		})
	}
}
