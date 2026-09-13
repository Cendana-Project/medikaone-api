package hospital

import (
	"net/http"
	"strconv"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

func (ctl *Controller) ListHospitals(c *gin.Context) {
	limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, offsetErr := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limitErr != nil || offsetErr != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("pagination", "integer limit and offset", "limit dan offset berupa angka bulat"))
		return
	}
	rows, err := ctl.svc.ListHospitals(c.Request.Context(), c.Query("search"), c.Query("city"), limit, offset)
	hospitalRespond(c, constant.MsgHospitalsListed, rows, err)
}

func (ctl *Controller) GetHospital(c *gin.Context) {
	row, err := ctl.svc.GetHospital(c.Request.Context(), c.Param("hospital_id"))
	hospitalRespond(c, constant.MsgHospitalRetrieved, row, err)
}

func (ctl *Controller) UpdateHospital(c *gin.Context) {
	var req request.UpdateHospitalRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	row, err := ctl.svc.UpdateHospital(c.Request.Context(), hospitalHint(c), req)
	hospitalRespond(c, constant.MsgHospitalUpdated, row, err)
}

func (ctl *Controller) DeleteHospital(c *gin.Context) {
	err := ctl.svc.DeleteHospital(c.Request.Context(), hospitalHint(c), util.GetUserID(c))
	hospitalRespond(c, constant.MsgHospitalDeleted, gin.H{"deleted": err == nil}, err)
}

func hospitalRespond(c *gin.Context, code constant.MessageCode, data any, err error) {
	if err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(code)
	resp.StatusCode = http.StatusOK
	resp.Data = data
	util.HandleResponse(c, resp, nil)
}
