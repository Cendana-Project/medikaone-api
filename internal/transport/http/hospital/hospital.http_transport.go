package hospital

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	hs "github.com/Cendana-Project/medikaone-api/internal/service/hospital"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type Controller struct {
	svc *hs.Service
}

func NewController(s *hs.Service) *Controller { return &Controller{svc: s} }

func (ctl *Controller) CreateHospital(c *gin.Context) {
	var req request.CreateHospitalRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}

	h, err := ctl.svc.CreateHospital(c.Request.Context(), &req)
	if err != nil {
		util.HandleError(c, err)
		return
	}

	resp := response.NewResponseOK()
	resp.Data = gin.H{
		"id":          h.ID,
		"code":        h.Code,
		"name":        h.Name,
		"address":     h.Address,
		"city":        h.City,
		"province":    h.Province,
		"country":     h.Country,
		"latitude":    h.Latitude,
		"longitude":   h.Longitude,
		"phone":       h.Phone,
		"description": h.Description,
		"facilities":  h.Facilities,
		"is_active":   h.IsActive,
		"created_at":  h.CreatedAt,
	}
	util.HandleResponse(c, resp, nil)
}

// POST /v1/hospitals/:hospital_id/admins
func (ctl *Controller) CreateHospitalAdmin(c *gin.Context) {
	var req request.CreateHospitalAdminRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	req.HospitalID = hospitalHint(c)
	if req.HospitalID == "" {
		util.HandleError(c, constant.ErrHospitalContextRequired)
		return
	}

	if req.Gender != nil {
		g := strings.ToUpper(strings.TrimSpace(*req.Gender))
		req.Gender = &g
	}

	uid, err := ctl.svc.CreateHospitalAdmin(c.Request.Context(), req)
	if err != nil {
		util.HandleError(c, err)
		return
	}

	resp := response.NewResponseOK()
	resp.StatusCode = http.StatusCreated
	resp.Data = gin.H{"user_id": uid, "role": constant.RoleAdmin}
	util.HandleResponse(c, resp, nil)
}

// POST /v1/hospitals/:hospital_id/staff
func (ctl *Controller) CreateHospitalStaff(c *gin.Context) {
	// Bind hanya URI ke struct kecil agar field JSON belum ikut divalidasi.
	var req request.CreateHospitalStaffRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	req.HospitalID = hospitalHint(c)
	if req.HospitalID == "" {
		util.HandleError(c, constant.ErrHospitalContextRequired)
		return
	}

	// Normalisasi role dan gender agar konsisten dengan validator dan database.
	req.Role = strings.ToUpper(strings.TrimSpace(req.Role))
	if req.Gender != nil {
		g := strings.ToUpper(strings.TrimSpace(*req.Gender))
		req.Gender = &g
	}

	uid, err := ctl.svc.CreateHospitalStaff(c.Request.Context(), req)
	if err != nil {
		util.HandleError(c, err)
		return
	}

	resp := response.NewResponseOK()
	resp.StatusCode = http.StatusCreated
	resp.Data = gin.H{"user_id": uid, "role": req.Role}
	util.HandleResponse(c, resp, nil)
}

func hospitalHint(c *gin.Context) string {
	if hospitalID := strings.TrimSpace(c.GetString("hospital_id")); hospitalID != "" {
		return hospitalID
	}
	if hint, ok := c.Get("hospital_hint"); ok {
		if value, ok := hint.(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return strings.TrimSpace(c.Param("hospital_id"))
}
