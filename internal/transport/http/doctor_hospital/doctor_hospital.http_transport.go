package doctorhospital

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	service "github.com/Cendana-Project/medikaone-api/internal/service/doctor_hospital"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

const maxMultipartRequestBytes = int64(10 * 1024 * 1024)

type Controller struct {
	service *service.Service
}

func NewController(service *service.Service) *Controller { return &Controller{service: service} }

func (ctl *Controller) SearchDoctor(c *gin.Context) {
	result, err := ctl.service.SearchDoctor(c.Request.Context(), request.DoctorSearchQuery{
		Email: c.Query("email"), SIPNumber: c.Query("sip_number"), MedikaOneID: c.Query("medikaone_id"),
	})
	respond(c, constant.MsgDoctorSearchCompleted, http.StatusOK, result, err)
}

func (ctl *Controller) CreateDepartment(c *gin.Context) {
	var req request.CreateHospitalDepartmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateDepartment(c.Request.Context(), hospitalID(c), req)
	respond(c, constant.MsgHospitalDepartmentCreated, http.StatusCreated, result, err)
}

func (ctl *Controller) ListDepartments(c *gin.Context) {
	result, err := ctl.service.ListDepartments(c.Request.Context(), hospitalID(c))
	respond(c, constant.MsgHospitalDepartmentsListed, http.StatusOK, result, err)
}

func (ctl *Controller) CreateRoom(c *gin.Context) {
	var req request.CreateHospitalRoomRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateRoom(c.Request.Context(), hospitalID(c), req)
	respond(c, constant.MsgHospitalRoomCreated, http.StatusCreated, result, err)
}

func (ctl *Controller) ListRooms(c *gin.Context) {
	result, err := ctl.service.ListRooms(c.Request.Context(), hospitalID(c), strings.TrimSpace(c.Query("department_id")))
	respond(c, constant.MsgHospitalRoomsListed, http.StatusOK, result, err)
}

func (ctl *Controller) CreateInvitation(c *gin.Context) {
	prepareMultipart(c)
	if err := c.Request.ParseMultipartForm(maxMultipartRequestBytes); err != nil {
		util.HandleError(c, constant.ErrInvalidContractPDF)
		return
	}
	schedules := []request.DoctorInvitationScheduleRequest{}
	if raw := strings.TrimSpace(c.PostForm("schedules")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &schedules); err != nil {
			util.HandleError(c, constant.NewInvalidFieldValueError("schedules", "a valid JSON array", "berupa array JSON yang valid"))
			return
		}
	}
	var roomID *string
	if value := strings.TrimSpace(c.PostForm("room_id")); value != "" {
		roomID = &value
	}
	var message *string
	if value := strings.TrimSpace(c.PostForm("message")); value != "" {
		message = &value
	}
	file, err := readMultipartPDF(c, "contract")
	if err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateInvitation(c.Request.Context(), hospitalID(c), util.GetUserID(c), request.CreateDoctorHospitalInvitationRequest{
		DoctorID: strings.TrimSpace(c.PostForm("doctor_id")), DepartmentID: strings.TrimSpace(c.PostForm("department_id")),
		RoomID: roomID, Message: message, Schedules: schedules,
	}, file)
	respond(c, constant.MsgDoctorInvitationCreated, http.StatusCreated, result, err)
}

func (ctl *Controller) ListHospitalInvitations(c *gin.Context) {
	result, err := ctl.service.ListHospitalInvitations(c.Request.Context(), hospitalID(c), c.Query("status"))
	respond(c, constant.MsgHospitalDoctorInvitationsListed, http.StatusOK, result, err)
}

func (ctl *Controller) GetHospitalInvitation(c *gin.Context) {
	result, err := ctl.service.GetHospitalInvitation(c.Request.Context(), hospitalID(c), c.Param("invitation_id"))
	respond(c, constant.MsgHospitalDoctorInvitationRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) CancelInvitation(c *gin.Context) {
	err := ctl.service.CancelInvitation(c.Request.Context(), hospitalID(c), c.Param("invitation_id"), util.GetUserID(c))
	respond(c, constant.MsgDoctorInvitationCancelled, http.StatusOK, gin.H{"cancelled": err == nil}, err)
}

func (ctl *Controller) ResendInvitation(c *gin.Context) {
	result, err := ctl.service.ResendInvitation(c.Request.Context(), hospitalID(c), c.Param("invitation_id"), util.GetUserID(c))
	respond(c, constant.MsgDoctorInvitationResent, http.StatusCreated, result, err)
}

func (ctl *Controller) GetHospitalContractURL(c *gin.Context) {
	result, err := ctl.service.GetHospitalContractURL(c.Request.Context(), hospitalID(c), c.Param("invitation_id"), c.DefaultQuery("version", "original"))
	respond(c, constant.MsgHospitalDoctorContractURLRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) ListHospitalDoctors(c *gin.Context) {
	result, err := ctl.service.ListHospitalDoctors(c.Request.Context(), hospitalID(c), c.Query("status"))
	respond(c, constant.MsgHospitalDoctorsListed, http.StatusOK, result, err)
}

func (ctl *Controller) ListDoctorAffiliations(c *gin.Context) {
	result, err := ctl.service.ListDoctorAffiliations(c.Request.Context(), util.GetUserID(c), c.Query("status"))
	respond(c, constant.MsgDoctorAffiliationsListed, http.StatusOK, result, err)
}

func (ctl *Controller) UpdateAffiliationStatus(c *gin.Context) {
	var req request.UpdateDoctorHospitalAffiliationStatusRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	err := ctl.service.UpdateAffiliationStatus(c.Request.Context(), hospitalID(c), c.Param("doctor_id"), req.Status, util.GetUserID(c))
	respond(c, constant.MsgDoctorAffiliationStatusUpdated, http.StatusOK, gin.H{"doctor_id": c.Param("doctor_id"), "status": strings.ToUpper(req.Status)}, err)
}

func (ctl *Controller) ListDoctorInvitations(c *gin.Context) {
	result, err := ctl.service.ListDoctorInvitations(c.Request.Context(), util.GetUserID(c), c.Query("status"))
	respond(c, constant.MsgDoctorInvitationsListed, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorInvitation(c *gin.Context) {
	result, err := ctl.service.GetDoctorInvitation(c.Request.Context(), util.GetUserID(c), c.Param("invitation_id"))
	respond(c, constant.MsgDoctorInvitationRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorContractURL(c *gin.Context) {
	result, err := ctl.service.GetDoctorContractURL(c.Request.Context(), util.GetUserID(c), c.Param("invitation_id"), c.DefaultQuery("version", "original"))
	respond(c, constant.MsgDoctorContractURLRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) AcceptInvitation(c *gin.Context) {
	prepareMultipart(c)
	if err := c.Request.ParseMultipartForm(maxMultipartRequestBytes); err != nil {
		util.HandleError(c, constant.ErrInvalidContractPDF)
		return
	}
	file, err := readMultipartPDF(c, "signed_contract")
	if err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.AcceptInvitation(c.Request.Context(), util.GetUserID(c), c.Param("invitation_id"), file)
	respond(c, constant.MsgDoctorInvitationAccepted, http.StatusOK, result, err)
}

func (ctl *Controller) RejectInvitation(c *gin.Context) {
	var req request.RejectDoctorHospitalInvitationRequest
	if c.Request.ContentLength > 0 {
		if err := util.BindAndValidate(c, &req); err != nil {
			util.HandleError(c, err)
			return
		}
	}
	err := ctl.service.RejectInvitation(c.Request.Context(), util.GetUserID(c), c.Param("invitation_id"), req.Reason)
	respond(c, constant.MsgDoctorInvitationRejected, http.StatusOK, gin.H{"rejected": err == nil}, err)
}

func (ctl *Controller) ListNotifications(c *gin.Context) {
	unreadOnly, err := strconv.ParseBool(c.DefaultQuery("unread_only", "false"))
	if err != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("unread_only", "true or false", "true atau false"))
		return
	}
	result, err := ctl.service.ListNotifications(c.Request.Context(), util.GetUserID(c), unreadOnly)
	respond(c, constant.MsgNotificationsListed, http.StatusOK, result, err)
}

func (ctl *Controller) MarkNotificationRead(c *gin.Context) {
	err := ctl.service.MarkNotificationRead(c.Request.Context(), util.GetUserID(c), c.Param("notification_id"))
	respond(c, constant.MsgNotificationMarkedRead, http.StatusOK, gin.H{"read": err == nil}, err)
}

func prepareMultipart(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMultipartRequestBytes)
}

func readMultipartPDF(c *gin.Context, field string) (service.UploadedFile, error) {
	file, header, err := c.Request.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return service.UploadedFile{}, constant.ErrInvalidContractPDF
		}
		return service.UploadedFile{}, constant.ErrInvalidContractPDF
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxMultipartRequestBytes+1))
	if err != nil || int64(len(content)) > maxMultipartRequestBytes {
		return service.UploadedFile{}, constant.ErrInvalidContractPDF
	}
	return service.UploadedFile{Filename: header.Filename, MIMEType: multipartContentType(header), Content: content}, nil
}

func multipartContentType(header *multipart.FileHeader) string {
	if header == nil {
		return ""
	}
	return header.Header.Get("Content-Type")
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
