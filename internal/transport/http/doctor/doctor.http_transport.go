package doctor

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor"
	service "github.com/Cendana-Project/medikaone-api/internal/service/doctor"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type Controller struct{ service *service.Service }

func NewController(service *service.Service) *Controller { return &Controller{service: service} }

func (ctl *Controller) ListDoctors(c *gin.Context) {
	filter := repository.Filter{Query: c.Query("q"), Specialty: c.Query("specialty"), HospitalID: c.Query("hospital_id")}
	for key, target := range map[string]*int{"page": &filter.Page, "limit": &filter.Limit} {
		if value, exists := c.GetQuery(key); exists {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				util.HandleError(c, constant.NewInvalidFieldValueError(key, "a positive integer", "bilangan bulat positif"))
				return
			}
			*target = parsed
		}
	}
	out, err := ctl.service.ListDoctors(c.Request.Context(), filter)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgDoctorsListed)
	resp.Data = out
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) GetDoctor(c *gin.Context) {
	out, err := ctl.service.GetDoctor(c.Request.Context(), c.Param("doctor_id"))
	if err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgDoctorRetrieved)
	resp.Data = out
	util.HandleResponse(c, resp, nil)
}
