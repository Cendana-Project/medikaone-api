package user

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	userrepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	"github.com/Cendana-Project/medikaone-api/internal/service/auth"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type Controller struct {
	svc      *auth.Service
	userRepo *userrepo.Repository
}

func NewController(svc *auth.Service, ur *userrepo.Repository) *Controller {
	return &Controller{svc: svc, userRepo: ur}
}

// PUT /v1/profile/patient (protected)
func (ctl *Controller) UpdatePatientProfile(c *gin.Context) {
	var req request.PatientProfileRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	userID := util.GetUserID(c)
	if err := ctl.svc.CompletePatientProfile(c.Request.Context(), userID, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	resp := response.NewResponseOK()
	resp.StatusCode = http.StatusOK
	resp.Data = gin.H{"updated": true}
	util.HandleResponse(c, resp, nil)
}

// PUT /v1/profile/doctor (protected)
func (ctl *Controller) UpdateDoctorProfile(c *gin.Context) {
	var req request.DoctorProfileRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	userID := util.GetUserID(c)
	if err := ctl.svc.CompleteDoctorProfile(c.Request.Context(), userID, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	resp := response.NewResponseOK()
	resp.StatusCode = http.StatusOK
	resp.Data = gin.H{"updated": true}
	util.HandleResponse(c, resp, nil)
}

// GET /v1/me (global, non-tenant; role global opsional)
func (ctl *Controller) Me(c *gin.Context) {
	userUUID, err := util.GetUserIDFromContext(c)
	if err != nil || userUUID == nil {
		util.HandleError(c, constant.ErrUserNotAuthenticated)
		return
	}
	userID := userUUID.String()
	ctx := c.Request.Context()

	u, err := ctl.userRepo.GetByID(ctx, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		util.HandleError(c, constant.ErrUserNotFound)
		return
	}
	if err != nil {
		util.HandleError(c, err)
		return
	}
	if u == nil {
		util.HandleError(c, constant.ErrUserNotFound)
		return
	}

	// role global (boleh kosong), normalisasi ke UPPER
	roleSlug, err := ctl.userRepo.GetUserRoleSlug(ctx, userID)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	roleSlug = strings.ToUpper(roleSlug)
	dto := toMeDTO(u, roleSlug)

	h, w, a, m, err := ctl.userRepo.GetPatientProfileByUserID(ctx, userID)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	if h != nil || w != nil || a != nil || m != nil {
		dto.PatientProfile = &response.PatientProfile{HeightCM: h, WeightKG: w, Allergies: a, MedicalHist: m}
	}
	sip, spec, err := ctl.userRepo.GetDoctorProfileByUserID(ctx, userID)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	if sip != nil || spec != nil {
		dto.DoctorProfile = &response.DoctorProfile{SIPNumber: sip, Specialty: spec}
	}
	hospitals, err := ctl.userRepo.ListHospitalsByUserID(ctx, userID)
	if err != nil {
		util.HandleError(c, constant.ErrInternalServerError)
		return
	}
	for _, hospital := range hospitals {
		dto.Hospitals = append(dto.Hospitals, response.HospitalBrief{
			ID: hospital.ID, Code: hospital.Code, Name: hospital.Name,
		})
	}

	resp := response.NewResponseOK()
	resp.StatusCode = http.StatusOK
	resp.Data = dto
	util.HandleResponse(c, resp, nil)
}

// GET /v1/tenant/me (tenant-scoped; wajib hint & membership)
func (ctl *Controller) TenantMe(c *gin.Context) {
	userUUID, err := util.GetUserIDFromContext(c)
	if err != nil || userUUID == nil {
		util.HandleError(c, constant.ErrUserNotAuthenticated)
		return
	}
	userID := userUUID.String()
	ctx := c.Request.Context()

	hintVal, ok := c.Get("hospital_hint")
	if !ok {
		util.HandleError(c, constant.ErrHospitalContextRequired)
		return
	}
	hint, ok := hintVal.(string)
	if !ok {
		util.HandleError(c, constant.ErrHospitalContextRequired)
		return
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		util.HandleError(c, constant.ErrHospitalContextRequired)
		return
	}

	// resolve hospital
	hosp, err := ctl.userRepo.ResolveHospitalHint(ctx, hint)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.HandleError(c, constant.ErrHospitalNotFound)
		} else {
			util.HandleError(c, constant.ErrInternalServerError)
		}
		return
	}

	// cek membership
	isMember, err := ctl.userRepo.IsMemberOfHospital(ctx, userID, hosp.ID)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	if !isMember {
		util.HandleError(c, constant.ErrUserNotLinkedToHospital)
		return
	}

	u, err := ctl.userRepo.GetByID(ctx, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		util.HandleError(c, constant.ErrUserNotFound)
		return
	}
	if err != nil {
		util.HandleError(c, err)
		return
	}
	if u == nil {
		util.HandleError(c, constant.ErrUserNotFound)
		return
	}

	// role scoped hospital (UPPER)
	roleSlug, err := ctl.userRepo.GetHospitalRoleSlug(ctx, userID, hosp.ID)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	roleSlug = strings.ToUpper(roleSlug)
	if roleSlug == "" {
		util.HandleError(c, constant.ErrForbidden)
		return
	}

	dto := toMeDTO(u, roleSlug)
	dto.Hospitals = []response.HospitalBrief{{ID: hosp.ID, Code: hosp.Code, Name: hosp.Name}}

	// Enrich sesuai role scoped
	switch roleSlug {
	case constant.RoleDoctor:
		sip, spec, err := ctl.userRepo.GetDoctorProfileByUserID(ctx, userID)
		if err != nil {
			util.HandleError(c, constant.ErrInternalServerError)
			return
		}
		if sip != nil || spec != nil {
			dto.DoctorProfile = &response.DoctorProfile{SIPNumber: sip, Specialty: spec}
		}
	case constant.RolePatient:
		h, w, a, m, err := ctl.userRepo.GetPatientProfileByUserID(ctx, userID)
		if err != nil {
			util.HandleError(c, constant.ErrInternalServerError)
			return
		}
		if h != nil || w != nil || a != nil || m != nil {
			dto.PatientProfile = &response.PatientProfile{HeightCM: h, WeightKG: w, Allergies: a, MedicalHist: m}
		}
	}

	resp := response.NewResponseOK()
	resp.StatusCode = http.StatusOK
	resp.Data = dto
	util.HandleResponse(c, resp, nil)
}

func toMeDTO(u *entity.User, roleSlug string) response.MeResponse {
	var dobStr *string
	if u.DOB != nil {
		s := u.DOB.Format("2006-01-02")
		dobStr = &s
	}
	var verifiedStr *string
	if u.VerifiedAt != nil {
		s := u.VerifiedAt.UTC().Format(time.RFC3339)
		verifiedStr = &s
	}
	return response.MeResponse{
		ID:         u.ID,
		Email:      u.Email,
		Username:   u.Username,
		FirstName:  u.FirstName,
		LastName:   u.LastName,
		Phone:      u.Phone,
		Gender:     u.Gender,
		DOB:        dobStr,
		Address:    u.Address,
		Status:     u.Status,
		VerifiedAt: verifiedStr,
		Role:       roleSlug, // sudah dinormalisasi UPPER di pemanggil
	}
}
