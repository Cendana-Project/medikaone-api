package doctor

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	repository "github.com/Cendana-Project/medikaone-api/internal/repository/doctor"
	service "github.com/Cendana-Project/medikaone-api/internal/service/doctor"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type Controller struct{ service *service.Service }

func NewController(service *service.Service) *Controller { return &Controller{service: service} }

func (ctl *Controller) ListDoctors(c *gin.Context) {
	ctl.listDoctors(c, false)
}

func (ctl *Controller) RecommendDoctors(c *gin.Context) {
	ctl.listDoctors(c, true)
}

func (ctl *Controller) listDoctors(c *gin.Context, recommended bool) {
	filter := repository.Filter{
		Query: c.Query("q"), Specialty: c.Query("specialty"), HospitalID: c.Query("hospital_id"),
		DepartmentCode: c.Query("department_code"), DepartmentID: c.Query("department_id"), City: c.Query("city"),
		AvailableOn: c.Query("available_on"), BookingMode: c.Query("booking_mode"),
	}
	for _, field := range []struct {
		name   string
		target **float64
	}{{"latitude", &filter.Latitude}, {"longitude", &filter.Longitude}, {"radius_km", &filter.RadiusKM}} {
		if values, exists := c.Request.URL.Query()[field.name]; exists {
			if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
				util.HandleError(c, constant.NewInvalidFieldValueError(field.name, "a single numeric value", "satu nilai angka"))
				return
			}
			value, err := strconv.ParseFloat(values[0], 64)
			if err != nil {
				util.HandleError(c, constant.NewInvalidFieldValueError(field.name, "a numeric value", "nilai angka"))
				return
			}
			*field.target = &value
		}
	}
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
	var out any
	var err error
	if recommended {
		out, err = ctl.service.RecommendDoctors(c.Request.Context(), filter)
	} else {
		out, err = ctl.service.ListDoctors(c.Request.Context(), filter)
	}
	if err != nil {
		util.HandleError(c, err)
		return
	}
	message := constant.MsgDoctorsListed
	if recommended {
		message = constant.MsgDoctorRecommendationsListed
	}
	resp := constant.NewSuccessResponse(message)
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
