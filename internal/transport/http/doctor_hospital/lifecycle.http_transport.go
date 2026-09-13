package doctorhospital

import (
	"net/http"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
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
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	row, err := ctl.service.UpdateInvitation(c.Request.Context(), hospitalID(c), c.Param("invitation_id"), util.GetUserID(c), req)
	respond(c, constant.MsgDoctorInvitationUpdated, http.StatusOK, row, err)
}

func (ctl *Controller) DeleteInvitation(c *gin.Context) {
	err := ctl.service.DeleteInvitation(c.Request.Context(), hospitalID(c), c.Param("invitation_id"), util.GetUserID(c))
	respond(c, constant.MsgDoctorInvitationDeleted, http.StatusOK, gin.H{"deleted": err == nil}, err)
}

func (ctl *Controller) DeleteAffiliation(c *gin.Context) {
	err := ctl.service.DeleteAffiliation(c.Request.Context(), hospitalID(c), c.Param("doctor_id"), util.GetUserID(c))
	respond(c, constant.MsgDoctorAffiliationDeleted, http.StatusOK, gin.H{"deleted": err == nil}, err)
}
