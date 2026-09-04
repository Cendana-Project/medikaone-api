package appointment

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	service "github.com/Cendana-Project/medikaone-api/internal/service/appointment"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type Controller struct{ service *service.Service }

func NewController(service *service.Service) *Controller { return &Controller{service: service} }

func (ctl *Controller) ListAvailability(c *gin.Context) {
	result, err := ctl.service.ListAvailability(c.Request.Context(), strings.TrimSpace(c.Query("hospital_id")), strings.TrimSpace(c.Query("doctor_id")), strings.TrimSpace(c.Query("date_from")), strings.TrimSpace(c.Query("date_to")))
	respond(c, constant.MsgAppointmentAvailabilityListed, http.StatusOK, result, err)
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
	code := constant.MsgAppointmentCreated
	if replay {
		code = constant.MsgAppointmentCreationReplayed
	}
	respond(c, code, status, result, err)
}

func (ctl *Controller) ListPatientAppointments(c *gin.Context) {
	result, err := ctl.service.ListPatientAppointments(c.Request.Context(), util.GetUserID(c), c.Query("status"))
	respond(c, constant.MsgPatientAppointmentsListed, http.StatusOK, result, err)
}

func (ctl *Controller) GetPatientAppointment(c *gin.Context) {
	result, err := ctl.service.GetPatientAppointment(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgPatientAppointmentRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) CancelPatientAppointment(c *gin.Context) {
	var req request.CancelAppointmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	err := ctl.service.CancelPatientAppointment(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgPatientAppointmentCancelled, http.StatusOK, gin.H{"cancelled": err == nil}, err)
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
	code := constant.MsgPatientAppointmentRescheduled
	if replay {
		code = constant.MsgPatientAppointmentRescheduleReplayed
	}
	respond(c, code, status, result, err)
}

func (ctl *Controller) ListDoctorAppointments(c *gin.Context) {
	result, err := ctl.service.ListDoctorAppointments(c.Request.Context(), util.GetUserID(c), c.Query("status"), c.Query("date"))
	respond(c, constant.MsgDoctorAppointmentsListed, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorAppointment(c *gin.Context) {
	result, err := ctl.service.GetDoctorAppointment(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgDoctorAppointmentRetrieved, http.StatusOK, result, err)
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
	respond(c, constant.MsgConsultationStarted, http.StatusOK, gin.H{"status": "IN_CONSULTATION"}, err)
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
	respond(c, constant.MsgAppointmentCompleted, http.StatusOK, gin.H{"status": "COMPLETED"}, err)
}

func (ctl *Controller) CreateDoctorScheduleChange(c *gin.Context) {
	var req request.CreateScheduleChangeRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateDoctorScheduleChange(c.Request.Context(), util.GetUserID(c), req)
	respond(c, constant.MsgDoctorScheduleChangeCreated, http.StatusCreated, result, err)
}

func (ctl *Controller) ListDoctorScheduleChanges(c *gin.Context) {
	result, err := ctl.service.ListDoctorScheduleChanges(c.Request.Context(), util.GetUserID(c), c.Query("status"))
	respond(c, constant.MsgDoctorScheduleChangesListed, http.StatusOK, result, err)
}

func (ctl *Controller) ApproveDoctorScheduleChange(c *gin.Context) {
	err := ctl.service.ReviewDoctorScheduleChange(c.Request.Context(), util.GetUserID(c), c.Param("change_id"), "APPROVED", nil)
	respond(c, constant.MsgDoctorScheduleChangeApproved, http.StatusOK, gin.H{"status": "APPROVED"}, err)
}

func (ctl *Controller) RejectDoctorScheduleChange(c *gin.Context) {
	var req request.ReviewScheduleChangeRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	err := ctl.service.ReviewDoctorScheduleChange(c.Request.Context(), util.GetUserID(c), c.Param("change_id"), "REJECTED", req.Reason)
	respond(c, constant.MsgDoctorScheduleChangeRejected, http.StatusOK, gin.H{"status": "REJECTED"}, err)
}

func (ctl *Controller) ListHospitalAppointments(c *gin.Context) {
	result, err := ctl.service.ListHospitalAppointments(c.Request.Context(), hospitalID(c), c.Query("status"), c.Query("date"))
	respond(c, constant.MsgHospitalAppointmentsListed, http.StatusOK, result, err)
}

func (ctl *Controller) GetHospitalAppointment(c *gin.Context) {
	result, err := ctl.service.GetHospitalAppointment(c.Request.Context(), hospitalID(c), c.Param("appointment_id"))
	respond(c, constant.MsgHospitalAppointmentRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) CancelHospitalAppointment(c *gin.Context) {
	var req request.CancelAppointmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	err := ctl.service.CancelHospitalAppointment(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgHospitalAppointmentCancelled, http.StatusOK, gin.H{"cancelled": err == nil}, err)
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
	code := constant.MsgHospitalAppointmentRescheduled
	if replay {
		code = constant.MsgHospitalAppointmentRescheduleReplayed
	}
	respond(c, code, status, result, err)
}

func (ctl *Controller) CheckIn(c *gin.Context) {
	var req request.VerifyAppointmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CheckIn(c.Request.Context(), hospitalID(c), util.GetUserID(c), req)
	respond(c, constant.MsgAppointmentCheckedIn, http.StatusOK, result, err)
}

func (ctl *Controller) ListHospitalQueue(c *gin.Context) {
	result, err := ctl.service.ListHospitalQueue(c.Request.Context(), hospitalID(c), c.Query("date"))
	respond(c, constant.MsgHospitalAppointmentQueueListed, http.StatusOK, result, err)
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
	respond(c, constant.MsgAppointmentVitalsCompleted, http.StatusOK, gin.H{"status": "WAITING_DOCTOR"}, err)
}

func (ctl *Controller) CreateHospitalScheduleChange(c *gin.Context) {
	var req request.CreateScheduleChangeRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateHospitalScheduleChange(c.Request.Context(), hospitalID(c), util.GetUserID(c), req)
	respond(c, constant.MsgHospitalScheduleChangeCreated, http.StatusCreated, result, err)
}

func (ctl *Controller) ListHospitalScheduleChanges(c *gin.Context) {
	result, err := ctl.service.ListHospitalScheduleChanges(c.Request.Context(), hospitalID(c), c.Query("status"))
	respond(c, constant.MsgHospitalScheduleChangesListed, http.StatusOK, result, err)
}

func (ctl *Controller) ApproveHospitalScheduleChange(c *gin.Context) {
	err := ctl.service.ReviewHospitalScheduleChange(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("change_id"), "APPROVED", nil)
	respond(c, constant.MsgHospitalScheduleChangeApproved, http.StatusOK, gin.H{"status": "APPROVED"}, err)
}

func (ctl *Controller) RejectHospitalScheduleChange(c *gin.Context) {
	var req request.ReviewScheduleChangeRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	err := ctl.service.ReviewHospitalScheduleChange(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("change_id"), "REJECTED", req.Reason)
	respond(c, constant.MsgHospitalScheduleChangeRejected, http.StatusOK, gin.H{"status": "REJECTED"}, err)
}

func hospitalID(c *gin.Context) string { return c.GetString("hospital_id") }

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
