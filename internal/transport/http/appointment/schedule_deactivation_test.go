package appointment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/appointment"
	service "github.com/Cendana-Project/medikaone-api/internal/service/appointment"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type deactivationRepository struct {
	service.Repository
	affiliation repository.Schedule
	input       *repository.ScheduleChangeInput
	createError error
}

func (r *deactivationRepository) GetAffiliation(context.Context, string) (*repository.Schedule, error) {
	return &r.affiliation, nil
}

func (r *deactivationRepository) CreateScheduleChange(_ context.Context, input repository.ScheduleChangeInput) (*response.ScheduleChangeRequest, error) {
	r.input = &input
	if r.createError != nil {
		return nil, r.createError
	}
	return &response.ScheduleChangeRequest{
		ID: uuid.NewString(), AffiliationID: input.AffiliationID, Operation: input.Operation,
		Status: "PENDING", DeactivationScope: input.DeactivationScope, DeactivationDayOfWeek: input.DeactivationDayOfWeek,
		Schedules: []response.DoctorHospitalSchedule{},
	}, nil
}

func TestDeactivateSchedulesHTTPValidationAndActorScope(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	for _, hospital := range []bool{false, true} {
		for _, test := range []struct {
			name, body string
			status     int
			foreign    bool
			badID      bool
		}{
			{name: "all", body: `{"scope":"ALL","reason":"  Cuti praktik  "}`, status: 202},
			{name: "Sunday zero is a day", body: `{"scope":"RECURRING_DAY","day_of_week":0}`, status: 202},
			{name: "Saturday", body: `{"scope":"RECURRING_DAY","day_of_week":6}`, status: 202},
			{name: "missing scope", body: `{}`, status: 400},
			{name: "unknown scope", body: `{"scope":"ALL_HOSPITALS"}`, status: 400},
			{name: "missing weekday", body: `{"scope":"RECURRING_DAY"}`, status: 400},
			{name: "null weekday", body: `{"scope":"RECURRING_DAY","day_of_week":null}`, status: 400},
			{name: "negative weekday", body: `{"scope":"RECURRING_DAY","day_of_week":-1}`, status: 400},
			{name: "weekday too high", body: `{"scope":"RECURRING_DAY","day_of_week":7}`, status: 400},
			{name: "weekday array", body: `{"scope":"RECURRING_DAY","day_of_week":[1]}`, status: 400},
			{name: "weekday forbidden for all", body: `{"scope":"ALL","day_of_week":0}`, status: 400},
			{name: "reason too long", body: `{"scope":"ALL","reason":"` + strings.Repeat("界", 1001) + `"}`, status: 400},
			{name: "cannot override affiliation", body: `{"scope":"ALL","affiliation_id":"` + uuid.NewString() + `"}`, status: 400},
			{name: "cannot pick actor", body: `{"scope":"ALL","doctor_id":"` + uuid.NewString() + `"}`, status: 400},
			{name: "malformed", body: `{"scope":`, status: 400},
			{name: "trailing object", body: `{"scope":"ALL"}{}`, status: 400},
			{name: "foreign affiliation", body: `{"scope":"ALL"}`, status: 404, foreign: true},
			{name: "invalid affiliation UUID", body: `{"scope":"ALL"}`, status: 400, badID: true},
		} {
			party := "doctor"
			if hospital {
				party = "hospital"
			}
			t.Run(party+"/"+test.name, func(t *testing.T) {
				affiliation := repository.Schedule{AffiliationID: uuid.NewString(), HospitalID: uuid.NewString(), DoctorID: uuid.NewString()}
				actorID, tenantID := affiliation.DoctorID, affiliation.HospitalID
				if hospital {
					actorID = uuid.NewString()
				}
				if test.foreign {
					actorID, tenantID = uuid.NewString(), uuid.NewString()
				}
				repo := &deactivationRepository{affiliation: affiliation}
				controller := NewController(service.NewService(repo, nil, "test-only-secret"))
				router := gin.New()
				router.POST("/:affiliation_id/deactivate", func(c *gin.Context) {
					c.Set(string(constant.UserID), actorID)
					c.Set("hospital_id", tenantID)
					if hospital {
						controller.DeactivateHospitalSchedules(c)
					} else {
						controller.DeactivateDoctorSchedules(c)
					}
				})
				pathID := affiliation.AffiliationID
				if test.badID {
					pathID = "invalid"
				}
				req := httptest.NewRequest(http.MethodPost, "/"+pathID+"/deactivate", strings.NewReader(test.body))
				req.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				if recorder.Code != test.status {
					t.Fatalf("status=%d want=%d response=%s", recorder.Code, test.status, recorder.Body.String())
				}
				if test.status != 202 {
					if repo.input != nil {
						t.Fatal("invalid/foreign request reached mutation")
					}
					return
				}
				if repo.input == nil || repo.input.AffiliationID != affiliation.AffiliationID || repo.input.ActorID != actorID || repo.input.Operation != "DEACTIVATE" || repo.input.ActorParty != strings.ToUpper(party) {
					t.Fatalf("lost authoritative actor/path: %+v", repo.input)
				}
				if hospital && repo.input.HospitalID != tenantID {
					t.Fatal("tenant scope lost")
				}
				if test.name == "all" && (repo.input.Reason == nil || *repo.input.Reason != "Cuti praktik") {
					t.Fatal("reason was not preserved/trimmed")
				}
				var body struct {
					Message string
					Data    struct{ Operation, Status string }
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Message != strings.ToUpper(party)+"_SCHEDULE_DEACTIVATION_REQUESTED" || body.Data.Operation != "DEACTIVATE" || body.Data.Status != "PENDING" {
					t.Fatalf("invalid proposal response: %s", recorder.Body.String())
				}
			})
		}
	}
}
