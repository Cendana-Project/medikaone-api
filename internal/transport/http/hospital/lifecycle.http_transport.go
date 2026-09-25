package hospital

import (
	"net/http"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

func (ctl *Controller) ListHospitals(c *gin.Context) {
	q := request.HospitalDirectoryQuery{Limit: 20}
	if err := c.ShouldBindQuery(&q); err != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("query", "valid directory query parameters", "parameter query direktori yang valid"))
		return
	}
	rows, err := ctl.svc.ListDirectory(c.Request.Context(), q)
	hospitalRespond(c, constant.MsgHospitalsListed, rows, err)
}

func (ctl *Controller) RecommendHospitals(c *gin.Context) {
	q := request.HospitalDirectoryQuery{Limit: 10, Sort: "rating"}
	if err := c.ShouldBindQuery(&q); err != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("query", "valid recommendation query parameters", "parameter query rekomendasi yang valid"))
		return
	}
	rows, err := ctl.svc.RecommendHospitals(c.Request.Context(), q)
	hospitalRespond(c, constant.MsgHospitalRecommendationsListed, rows, err)
}

func (ctl *Controller) ListDepartmentOptions(c *gin.Context) {
	q := request.DepartmentDirectoryQuery{Limit: 100}
	if err := c.ShouldBindQuery(&q); err != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("query", "valid department query parameters", "parameter query departemen yang valid"))
		return
	}
	rows, err := ctl.svc.ListDepartmentOptions(c.Request.Context(), q)
	hospitalRespond(c, constant.MsgDepartmentsListed, rows, err)
}

func (ctl *Controller) GetHospital(c *gin.Context) {
	var q request.HospitalDirectoryQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		util.HandleError(c, constant.ErrInvalidHospitalCoordinates)
		return
	}
	row, err := ctl.svc.GetDirectory(c.Request.Context(), c.Param("hospital_id"), q.Latitude, q.Longitude)
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
