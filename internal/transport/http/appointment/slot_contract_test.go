package appointment

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

func TestScheduledAppointmentHTTPRejectsOldSelectionFields(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	// A real canonical reference ensures unknown-field rejection is not hidden
	// behind malformed selection data. The controller must not reach a service.
	var payload [32]byte
	id := uuid.MustParse("11111111-2222-4333-8444-555555555555")
	copy(payload[:16], id[:])
	binary.BigEndian.PutUint64(payload[16:24], 1791387000)
	binary.BigEndian.PutUint64(payload[24:], 1791388800)
	slotID := "slt1_" + base64.RawURLEncoding.EncodeToString(payload[:])
	controller := &Controller{}
	for _, route := range []struct {
		name    string
		handler gin.HandlerFunc
		base    map[string]any
	}{
		{"create", controller.CreateAppointment, map[string]any{"slot_id": slotID, "reason_for_visit": "Consultation", "consent_accepted": true, "consent_version": "v1"}},
		{"patient reschedule", controller.ReschedulePatientAppointment, map[string]any{"slot_id": slotID, "reason": "Move visit"}},
		{"hospital reschedule", controller.RescheduleHospitalAppointment, map[string]any{"slot_id": slotID, "reason": "Move visit"}},
	} {
		t.Run(route.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/appointments", route.handler)
			for _, field := range []string{"schedule_id", "appointment_date", "start_time"} {
				for _, withSlot := range []bool{false, true} {
					body := make(map[string]any, len(route.base)+1)
					for key, value := range route.base {
						body[key] = value
					}
					if !withSlot {
						delete(body, "slot_id")
					}
					body[field] = map[string]string{"schedule_id": id.String(), "appointment_date": "2026-10-08", "start_time": "00:30"}[field]
					assertScheduledHTTPError(t, router, body, "UNKNOWN_FIELD", field)
				}
			}
			missing := make(map[string]any, len(route.base))
			for key, value := range route.base {
				if key != "slot_id" {
					missing[key] = value
				}
			}
			assertScheduledHTTPError(t, router, missing, "FIELD_REQUIRED", "slot_id")
			missing["slot_id"] = strings.Repeat("x", 129)
			assertScheduledHTTPError(t, router, missing, "INVALID_FIELD_LENGTH", "slot_id")
		})
	}
}

func assertScheduledHTTPError(t *testing.T, router http.Handler, body map[string]any, code, field string) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/appointments", strings.NewReader(string(encoded)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	var envelope response.BaseResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || envelope.Message != code || !strings.Contains(envelope.MessageDetail.DescEng, field) {
		t.Fatalf("field=%s status=%d body=%s", field, recorder.Code, recorder.Body.String())
	}
}
