package doctorhospital

import (
	"net/http"
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	service "github.com/Cendana-Project/medikaone-api/internal/service/doctor_hospital"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

func (ctl *Controller) UpdateDepartment(c *gin.Context) {
	var req request.UpdateHospitalDepartmentRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	row, err := ctl.service.UpdateDepartment(c.Request.Context(), hospitalID(c), c.Param("department_id"), req)
	respond(c, constant.MsgHospitalDepartmentUpdated, http.StatusOK, row, err)
}

func (ctl *Controller) DeleteDepartment(c *gin.Context) {
	err := ctl.service.DeleteDepartment(c.Request.Context(), hospitalID(c), c.Param("department_id"))
	respond(c, constant.MsgHospitalDepartmentDeleted, http.StatusOK, gin.H{"deleted": err == nil}, err)
}

func (ctl *Controller) UpdateRoom(c *gin.Context) {
	var req request.UpdateHospitalRoomRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	row, err := ctl.service.UpdateRoom(c.Request.Context(), hospitalID(c), c.Param("room_id"), req)
	respond(c, constant.MsgHospitalRoomUpdated, http.StatusOK, row, err)
}

func (ctl *Controller) DeleteRoom(c *gin.Context) {
	err := ctl.service.DeleteRoom(c.Request.Context(), hospitalID(c), c.Param("room_id"))
	respond(c, constant.MsgHospitalRoomDeleted, http.StatusOK, gin.H{"deleted": err == nil}, err)
}

func (ctl *Controller) UpdateInvitation(c *gin.Context) {
	var req request.UpdateDoctorHospitalInvitationRequest
	var contract *service.UploadedFile
	if c.ContentType() == "multipart/form-data" {
		if err := parseContractMultipart(c, ctl.maxContractSize()); err != nil {
			util.HandleError(c, err)
			return
		}
		defer c.Request.MultipartForm.RemoveAll()
		var err error
		req, contract, err = decodeInvitationPatch(c, ctl.maxContractSize())
		if err != nil {
			util.HandleError(c, err)
			return
		}
	} else {
		if err := util.BindAndValidate(c, &req); err != nil {
			util.HandleError(c, err)
			return
		}
	}
	row, err := ctl.service.UpdateInvitationWithContract(c.Request.Context(), hospitalID(c), c.Param("invitation_id"), util.GetUserID(c), req, contract)
	respond(c, constant.MsgDoctorInvitationUpdated, http.StatusOK, row, err)
}

func decodeInvitationPatch(c *gin.Context, maxFileSize int64) (request.UpdateDoctorHospitalInvitationRequest, *service.UploadedFile, error) {
	var req request.UpdateDoctorHospitalInvitationRequest
	for field, values := range c.Request.MultipartForm.Value {
		if len(values) != 1 {
			return req, nil, constant.NewInvalidFieldValueError(field, "a single value", "satu nilai")
		}
		value := values[0]
		switch field {
		case "department_id":
			req.DepartmentID = &value
		case "room_id":
			req.RoomID = &value
		case "message":
			req.Message = &value
		case "schedules":
			if strings.TrimSpace(value) == "null" {
				continue
			}
			if !strings.HasPrefix(strings.TrimSpace(value), "[") {
				return req, nil, constant.NewInvalidFieldValueError(field, "a valid JSON array", "berupa array JSON yang valid")
			}
			var schedules []request.DoctorInvitationScheduleRequest
			if err := util.DecodeStrictJSON(strings.NewReader(value), &schedules); err != nil {
				return req, nil, util.MapJSONDecodeError(err)
			}
			req.Schedules = &schedules
		default:
			return req, nil, constant.NewUnknownFieldError(field)
		}
	}
	for field := range c.Request.MultipartForm.File {
		if field != "contract" {
			return req, nil, constant.NewUnknownFieldError(field)
		}
	}
	if _, present := c.Request.MultipartForm.File["contract"]; present {
		file, err := readMultipartPDF(c, "contract", maxFileSize)
		return req, &file, err
	}
	return req, nil, nil
}

func (ctl *Controller) DeleteInvitation(c *gin.Context) {
	err := ctl.service.DeleteInvitation(c.Request.Context(), hospitalID(c), c.Param("invitation_id"), util.GetUserID(c))
	respond(c, constant.MsgDoctorInvitationDeleted, http.StatusOK, gin.H{"deleted": err == nil}, err)
}

func (ctl *Controller) DeleteAffiliation(c *gin.Context) {
	err := ctl.service.DeleteAffiliation(c.Request.Context(), hospitalID(c), c.Param("doctor_id"), util.GetUserID(c))
	respond(c, constant.MsgDoctorAffiliationDeleted, http.StatusOK, gin.H{"deleted": err == nil}, err)
}
