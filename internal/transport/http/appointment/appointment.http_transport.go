package appointment

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	service "github.com/Cendana-Project/medikaone-api/internal/service/appointment"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type Controller struct{ service *service.Service }

func NewController(service *service.Service) *Controller { return &Controller{service: service} }

func (ctl *Controller) ListAvailability(c *gin.Context) {
	result, err := ctl.service.ListAvailability(c.Request.Context(), strings.TrimSpace(c.Query("hospital_id")), strings.TrimSpace(c.Query("doctor_id")), strings.TrimSpace(c.Query("date_from")), strings.TrimSpace(c.Query("date_to")))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) CreateAppointment(c *gin.Context) {
	var req request.CreateAppointmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, replay, err := ctl.service.CreateAppointment(c.Request.Context(), util.GetUserID(c), c.GetHeader("Idempotency-Key"), c.ClientIP(), c.Request.UserAgent(), req)
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	respond(c, status, result, err)
}

func (ctl *Controller) ListPatientAppointments(c *gin.Context) {
	result, err := ctl.service.ListPatientAppointments(c.Request.Context(), util.GetUserID(c), c.Query("status"))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) GetPatientAppointment(c *gin.Context) {
	result, err := ctl.service.GetPatientAppointment(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) CancelPatientAppointment(c *gin.Context) {
	var req request.CancelAppointmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	err := ctl.service.CancelPatientAppointment(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, http.StatusOK, gin.H{"cancelled": err == nil}, err)
}

func (ctl *Controller) ReschedulePatientAppointment(c *gin.Context) {
	var req request.RescheduleAppointmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, replay, err := ctl.service.ReschedulePatientAppointment(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), c.GetHeader("Idempotency-Key"), c.ClientIP(), c.Request.UserAgent(), req)
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	respond(c, status, result, err)
}

func (ctl *Controller) ListDoctorAppointments(c *gin.Context) {
	result, err := ctl.service.ListDoctorAppointments(c.Request.Context(), util.GetUserID(c), c.Query("status"), c.Query("date"))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorAppointment(c *gin.Context) {
	result, err := ctl.service.GetDoctorAppointment(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) StartConsultation(c *gin.Context) {
	var req request.AppointmentTransitionRequest
	if c.Request.ContentLength > 0 {
		if err := util.BindAndValidate(c, &req); err != nil {
			util.HandleError(c, err)
			return
		}
	}
	err := ctl.service.StartConsultation(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req.Reason)
	respond(c, http.StatusOK, gin.H{"status": "IN_CONSULTATION"}, err)
}

func (ctl *Controller) CompleteAppointment(c *gin.Context) {
	var req request.AppointmentTransitionRequest
	if c.Request.ContentLength > 0 {
		if err := util.BindAndValidate(c, &req); err != nil {
			util.HandleError(c, err)
			return
		}
	}
	err := ctl.service.CompleteAppointment(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req.Reason)
	respond(c, http.StatusOK, gin.H{"status": "COMPLETED"}, err)
}

func (ctl *Controller) CreateDoctorScheduleChange(c *gin.Context) {
	var req request.CreateScheduleChangeRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateDoctorScheduleChange(c.Request.Context(), util.GetUserID(c), req)
	respond(c, http.StatusCreated, result, err)
}

func (ctl *Controller) ListDoctorScheduleChanges(c *gin.Context) {
	result, err := ctl.service.ListDoctorScheduleChanges(c.Request.Context(), util.GetUserID(c), c.Query("status"))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) ApproveDoctorScheduleChange(c *gin.Context) {
	err := ctl.service.ReviewDoctorScheduleChange(c.Request.Context(), util.GetUserID(c), c.Param("change_id"), "APPROVED", nil)
	respond(c, http.StatusOK, gin.H{"status": "APPROVED"}, err)
}

func (ctl *Controller) RejectDoctorScheduleChange(c *gin.Context) {
	var req request.ReviewScheduleChangeRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	err := ctl.service.ReviewDoctorScheduleChange(c.Request.Context(), util.GetUserID(c), c.Param("change_id"), "REJECTED", req.Reason)
	respond(c, http.StatusOK, gin.H{"status": "REJECTED"}, err)
}

func (ctl *Controller) ListHospitalAppointments(c *gin.Context) {
	result, err := ctl.service.ListHospitalAppointments(c.Request.Context(), hospitalID(c), c.Query("status"), c.Query("date"))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) GetHospitalAppointment(c *gin.Context) {
	result, err := ctl.service.GetHospitalAppointment(c.Request.Context(), hospitalID(c), c.Param("appointment_id"))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) CancelHospitalAppointment(c *gin.Context) {
	var req request.CancelAppointmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	err := ctl.service.CancelHospitalAppointment(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, http.StatusOK, gin.H{"cancelled": err == nil}, err)
}

func (ctl *Controller) RescheduleHospitalAppointment(c *gin.Context) {
	var req request.RescheduleAppointmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, replay, err := ctl.service.RescheduleHospitalAppointment(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"), c.GetHeader("Idempotency-Key"), c.ClientIP(), c.Request.UserAgent(), req)
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	respond(c, status, result, err)
}

func (ctl *Controller) CheckIn(c *gin.Context) {
	var req request.VerifyAppointmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CheckIn(c.Request.Context(), hospitalID(c), util.GetUserID(c), req)
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) ListHospitalQueue(c *gin.Context) {
	result, err := ctl.service.ListHospitalQueue(c.Request.Context(), hospitalID(c), c.Query("date"))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) CompleteVitals(c *gin.Context) {
	var req request.AppointmentTransitionRequest
	if c.Request.ContentLength > 0 {
		if err := util.BindAndValidate(c, &req); err != nil {
			util.HandleError(c, err)
			return
		}
	}
	err := ctl.service.CompleteVitals(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"), req.Reason)
	respond(c, http.StatusOK, gin.H{"status": "WAITING_DOCTOR"}, err)
}

func (ctl *Controller) CreateHospitalScheduleChange(c *gin.Context) {
	var req request.CreateScheduleChangeRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateHospitalScheduleChange(c.Request.Context(), hospitalID(c), util.GetUserID(c), req)
	respond(c, http.StatusCreated, result, err)
}

func (ctl *Controller) ListHospitalScheduleChanges(c *gin.Context) {
	result, err := ctl.service.ListHospitalScheduleChanges(c.Request.Context(), hospitalID(c), c.Query("status"))
	respond(c, http.StatusOK, result, err)
}

func (ctl *Controller) ApproveHospitalScheduleChange(c *gin.Context) {
	err := ctl.service.ReviewHospitalScheduleChange(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("change_id"), "APPROVED", nil)
	respond(c, http.StatusOK, gin.H{"status": "APPROVED"}, err)
}

func (ctl *Controller) RejectHospitalScheduleChange(c *gin.Context) {
	var req request.ReviewScheduleChangeRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	err := ctl.service.ReviewHospitalScheduleChange(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("change_id"), "REJECTED", req.Reason)
	respond(c, http.StatusOK, gin.H{"status": "REJECTED"}, err)
}

func hospitalID(c *gin.Context) string { return c.GetString("hospital_id") }

func respond(c *gin.Context, status int, data any, err error) {
	if err != nil {
		util.HandleError(c, err)
		return
	}
	result := response.NewResponseOK()
	result.StatusCode = status
	result.Data = data
	util.HandleResponse(c, result, nil)
}
