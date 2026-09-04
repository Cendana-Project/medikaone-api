package prescription

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	service "github.com/Cendana-Project/medikaone-api/internal/service/prescription"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type Controller struct{ service *service.Service }

func NewController(service *service.Service) *Controller { return &Controller{service: service} }

func (ctl *Controller) CreateMedication(c *gin.Context) {
	var req request.MedicationCatalogRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateMedication(c.Request.Context(), hospitalID(c), util.GetUserID(c), req)
	respond(c, constant.MsgMedicationCatalogCreated, http.StatusCreated, result, err)
}

func (ctl *Controller) UpdateMedication(c *gin.Context) {
	var req request.MedicationCatalogRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.UpdateMedication(c.Request.Context(), hospitalID(c), c.Param("medication_id"), util.GetUserID(c), req)
	respond(c, constant.MsgMedicationCatalogUpdated, http.StatusOK, result, err)
}

func (ctl *Controller) ListHospitalMedications(c *gin.Context) {
	includeInactive, err := strconv.ParseBool(defaultValue(c.Query("include_inactive"), "false"))
	if err != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("include_inactive", "true or false", "true atau false"))
		return
	}
	result, serviceErr := ctl.service.ListHospitalMedications(c.Request.Context(), hospitalID(c), c.Query("q"), includeInactive)
	respond(c, constant.MsgMedicationCatalogListed, http.StatusOK, result, serviceErr)
}

func (ctl *Controller) ListDoctorMedications(c *gin.Context) {
	result, err := ctl.service.ListDoctorMedications(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), c.Query("q"))
	respond(c, constant.MsgMedicationCatalogListed, http.StatusOK, result, err)
}

func (ctl *Controller) SaveDraft(c *gin.Context) {
	var req request.PrescriptionDraftRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.SaveDraft(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgPrescriptionDraftSaved, http.StatusOK, result, err)
}

func (ctl *Controller) MarkNoMedication(c *gin.Context) {
	var req request.NoMedicationRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.MarkNoMedication(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgNoMedicationRecorded, http.StatusOK, result, err)
}

func (ctl *Controller) Issue(c *gin.Context) {
	var req request.IssuePrescriptionRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.Issue(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgPrescriptionIssued, http.StatusOK, result, err)
}

func (ctl *Controller) Correct(c *gin.Context) {
	var req request.CorrectPrescriptionRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.Correct(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgPrescriptionCorrected, http.StatusOK, result, err)
}

func (ctl *Controller) Cancel(c *gin.Context) {
	var req request.CancelPrescriptionRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.Cancel(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgPrescriptionCancelled, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorPrescription(c *gin.Context) {
	result, err := ctl.service.GetDoctorPrescription(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgPrescriptionRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorDocumentURL(c *gin.Context) {
	result, err := ctl.service.GetDoctorDocumentURL(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgPrescriptionDocumentURLRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) GetHospitalPrescription(c *gin.Context) {
	result, err := ctl.service.GetHospitalPrescription(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgPrescriptionRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) GetHospitalDocumentURL(c *gin.Context) {
	result, err := ctl.service.GetHospitalDocumentURL(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgPrescriptionDocumentURLRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) ListPatientPrescriptions(c *gin.Context) {
	result, err := ctl.service.ListPatientPrescriptions(c.Request.Context(), util.GetUserID(c))
	respond(c, constant.MsgPrescriptionsListed, http.StatusOK, result, err)
}

func (ctl *Controller) GetPatientPrescription(c *gin.Context) {
	result, err := ctl.service.GetPatientPrescription(c.Request.Context(), util.GetUserID(c), c.Param("prescription_id"))
	respond(c, constant.MsgPrescriptionRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) GetPatientDocumentURL(c *gin.Context) {
	result, err := ctl.service.GetPatientDocumentURL(c.Request.Context(), util.GetUserID(c), c.Param("prescription_id"))
	respond(c, constant.MsgPrescriptionDocumentURLRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) Verify(c *gin.Context) {
	result, err := ctl.service.Verify(c.Request.Context(), c.Param("token"))
	respond(c, constant.MsgPrescriptionVerificationCompleted, http.StatusOK, result, err)
}

func hospitalID(c *gin.Context) string { return c.GetString("hospital_id") }

func defaultValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func respond(c *gin.Context, code constant.MessageCode, status int, data any, err error) {
	if err != nil {
		util.HandleError(c, err)
		return
	}
	result := constant.NewSuccessResponse(code)
	result.StatusCode = status
	result.Data = data
	util.HandleResponse(c, result, nil)
}
