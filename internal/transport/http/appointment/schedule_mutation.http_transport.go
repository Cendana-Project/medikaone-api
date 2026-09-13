package appointment

import (
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (ctl *Controller) CreateDoctorSpecificSchedule(c *gin.Context) {
	var req request.CreateSpecificScheduleRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateDoctorSpecificSchedule(c.Request.Context(), util.GetUserID(c), req)
	respond(c, constant.MsgDoctorScheduleChangeCreated, http.StatusCreated, result, err)
}

func (ctl *Controller) CreateHospitalSpecificSchedule(c *gin.Context) {
	var req request.CreateSpecificScheduleRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CreateHospitalSpecificSchedule(c.Request.Context(), hospitalID(c), util.GetUserID(c), req)
	respond(c, constant.MsgHospitalScheduleChangeCreated, http.StatusCreated, result, err)
}

func (ctl *Controller) DeleteDoctorSchedule(c *gin.Context) {
	result, err := ctl.service.DeleteDoctorSchedule(c.Request.Context(), util.GetUserID(c), c.Param("schedule_id"))
	respond(c, constant.MsgDoctorScheduleChangeCreated, http.StatusAccepted, result, err)
}

func (ctl *Controller) DeleteHospitalSchedule(c *gin.Context) {
	result, err := ctl.service.DeleteHospitalSchedule(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("schedule_id"))
	respond(c, constant.MsgHospitalScheduleChangeCreated, http.StatusAccepted, result, err)
}
