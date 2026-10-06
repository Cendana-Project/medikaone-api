package appointment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	service "github.com/Cendana-Project/medikaone-api/internal/service/appointment"
)

type groupedAvailabilityRepository struct {
	service.Repository
	filter repository.AvailabilityFilter
}

func (r *groupedAvailabilityRepository) GetAvailabilityDoctor(_ context.Context, id string) (*response.AvailabilityDoctor, error) {
	return &response.AvailabilityDoctor{DoctorID: id, DoctorMedikaOneID: "MDO-0123456789ABCDEF", DoctorName: "Example Doctor"}, nil
}
func (r *groupedAvailabilityRepository) ListActiveSchedules(_ context.Context, filter repository.AvailabilityFilter) ([]repository.Schedule, error) {
	r.filter = filter
	return []repository.Schedule{}, nil
}
func (r *groupedAvailabilityRepository) ReservedCounts(context.Context, string, string, repository.AvailabilityFilter) ([]repository.ReservedCount, error) {
	return nil, nil
}

func TestGroupedAvailabilityHTTPContractAndValidation(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	const path = "/v1/appointments/availability/grouped"
	doctorID, hospitalID := uuid.NewString(), uuid.NewString()
	date := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	repo := &groupedAvailabilityRepository{}
	router := gin.New()
	router.GET(path, NewController(service.NewService(repo, nil, "test")).ListGroupedAvailability)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, httptest.NewRequest(http.MethodGet, path+"?doctor_id="+doctorID+"&hospital_id="+hospitalID+"&date_from="+date+"&date_to="+date, nil))
	if result.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
	var envelope struct {
		Message       string                             `json:"message"`
		MessageDetail map[string]string                  `json:"message_detail"`
		Data          response.GroupedDoctorAvailability `json:"data"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Message != "APPOINTMENT_AVAILABILITY_GROUPED" || len(envelope.MessageDetail) != 4 || envelope.Data.DoctorID != doctorID || envelope.Data.DoctorMedikaOneID == "" || len(envelope.Data.Dates) != 1 || envelope.Data.Dates[0].Hospitals == nil || repo.filter.HospitalID != hospitalID || repo.filter.DoctorID != doctorID {
		t.Fatalf("invalid response/filter: %s %#v", result.Body.String(), repo.filter)
	}
	// Malformed transport values must be rejected before invoking any dependency.
	invalidRouter := gin.New()
	invalidRouter.GET(path, (&Controller{}).ListGroupedAvailability)
	for _, query := range []string{
		"", "doctor_id=", "doctor_id=bad", "doctor_id=" + doctorID + "&doctor_id=" + doctorID,
		"doctor_id=" + doctorID + "&hospital_id=", "doctor_id=" + doctorID + "&hospital_id=bad",
		"doctor_id=" + doctorID + "&date_from=", "doctor_id=" + doctorID + "&date_to=2026-02-30",
		"doctor_id=" + doctorID + "&date_from=" + date + "&date_from=" + date,
		"doctor_id=" + doctorID + "&date_to=%zz", "doctor_id=" + doctorID + ";hospital_id=" + hospitalID,
	} {
		t.Run(query, func(t *testing.T) {
			out := httptest.NewRecorder()
			invalidRouter.ServeHTTP(out, httptest.NewRequest(http.MethodGet, path+"?"+query, nil))
			if out.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", out.Code, out.Body.String())
			}
		})
	}
}
