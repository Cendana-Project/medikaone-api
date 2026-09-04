package user

import (
	"context"
	"errors"
	"io"
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
	usersvc "github.com/Cendana-Project/medikaone-api/internal/service/user"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

type Controller struct {
	svc        *auth.Service
	userRepo   *userrepo.Repository
	profileSvc *usersvc.Service
}

func NewController(svc *auth.Service, ur *userrepo.Repository, profileServices ...*usersvc.Service) *Controller {
	controller := &Controller{svc: svc, userRepo: ur}
	if len(profileServices) > 0 {
		controller.profileSvc = profileServices[0]
	}
	return controller
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
	resp := constant.NewSuccessResponse(constant.MsgPatientProfileUpdated)
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
	resp := constant.NewSuccessResponse(constant.MsgDoctorProfileUpdated)
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

	dto, err := ctl.globalProfile(ctx, userID)
	if err != nil {
		util.HandleError(c, err)
		return
	}

	resp := constant.NewSuccessResponse(constant.MsgUserProfileRetrieved)
	resp.StatusCode = http.StatusOK
	resp.Data = dto
	util.HandleResponse(c, resp, nil)
}

// GET /v1/profile is the explicit profile resource. GET /v1/me remains as a
// backwards-compatible alias used by existing clients.
func (ctl *Controller) Profile(c *gin.Context) { ctl.Me(c) }

func (ctl *Controller) UpdateProfile(c *gin.Context) {
	if ctl.profileSvc == nil {
		util.HandleError(c, constant.ErrInternalServerError)
		return
	}
	var req request.UpdateUserProfileRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	userID := util.GetUserID(c)
	if err := ctl.profileSvc.Update(c.Request.Context(), userID, req); err != nil {
		util.HandleError(c, err)
		return
	}
	dto, err := ctl.globalProfile(c.Request.Context(), userID)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgUserProfileUpdated)
	resp.StatusCode = http.StatusOK
	resp.Data = dto
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) UploadProfilePhoto(c *gin.Context) {
	if ctl.profileSvc == nil {
		util.HandleError(c, constant.ErrInternalServerError)
		return
	}
	maxFileSize := ctl.profileSvc.MaxFileSize()
	// Multipart boundaries and headers need a small allowance beyond the file
	// limit. The file content itself is independently capped below.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxFileSize+(1<<20))
	if err := c.Request.ParseMultipartForm(maxFileSize + (1 << 20)); err != nil {
		util.HandleError(c, constant.ErrProfilePhotoInvalid)
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		util.HandleError(c, constant.ErrProfilePhotoInvalid)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil || int64(len(content)) > maxFileSize {
		util.HandleError(c, constant.ErrProfilePhotoInvalid)
		return
	}
	result, err := ctl.profileSvc.UploadPhoto(c.Request.Context(), util.GetUserID(c), usersvc.UploadedPhoto{Content: content})
	if err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgProfilePhotoUploaded)
	resp.StatusCode = http.StatusOK
	resp.Data = result
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) GetProfilePhotoURL(c *gin.Context) {
	if ctl.profileSvc == nil {
		util.HandleError(c, constant.ErrInternalServerError)
		return
	}
	result, err := ctl.profileSvc.GetPhotoURL(c.Request.Context(), util.GetUserID(c))
	if err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgProfilePhotoURLRetrieved)
	resp.StatusCode = http.StatusOK
	resp.Data = result
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) DeleteProfilePhoto(c *gin.Context) {
	if ctl.profileSvc == nil {
		util.HandleError(c, constant.ErrInternalServerError)
		return
	}
	if err := ctl.profileSvc.DeletePhoto(c.Request.Context(), util.GetUserID(c)); err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgProfilePhotoDeleted)
	resp.StatusCode = http.StatusOK
	resp.Data = gin.H{"deleted": true}
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) globalProfile(ctx context.Context, userID string) (*response.MeResponse, error) {
	u, err := ctl.userRepo.GetByID(ctx, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, constant.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, constant.ErrUserNotFound
	}

	// role global (boleh kosong), normalisasi ke UPPER
	roleSlug, err := ctl.userRepo.GetUserRoleSlug(ctx, userID)
	if err != nil {
		return nil, err
	}
	roleSlug = strings.ToUpper(roleSlug)
	dto := toMeDTO(u, roleSlug)

	h, w, a, m, err := ctl.userRepo.GetPatientProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if h != nil || w != nil || a != nil || m != nil {
		dto.PatientProfile = &response.PatientProfile{HeightCM: h, WeightKG: w, Allergies: a, MedicalHist: m}
	}
	sip, spec, err := ctl.userRepo.GetDoctorProfileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if sip != nil || spec != nil {
		dto.DoctorProfile = &response.DoctorProfile{SIPNumber: sip, Specialty: spec}
	}
	hospitals, err := ctl.userRepo.ListHospitalsByUserID(ctx, userID)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	for _, hospital := range hospitals {
		dto.Hospitals = append(dto.Hospitals, response.HospitalBrief{
			ID: hospital.ID, Code: hospital.Code, Name: hospital.Name,
		})
	}

	return &dto, nil
}

// GET /v1/tenant/me (tenant-scoped; wajib hint dan tenant role, kecuali global SUPER_ADMIN)
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

	globalRoleSlug, err := ctl.userRepo.GetUserRoleSlug(ctx, userID)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	isSuper := strings.EqualFold(globalRoleSlug, constant.RoleSuperAdmin)

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

	isMember := false
	hospitalRole := ""
	if !isSuper {
		// Non-super users need both an active membership and an active role in
		// the selected hospital.
		var memberErr error
		isMember, memberErr = ctl.userRepo.IsMemberOfHospital(ctx, userID, hosp.ID)
		if memberErr != nil {
			util.HandleError(c, memberErr)
			return
		}
		if isMember {
			hospitalRole, err = ctl.userRepo.GetHospitalRoleSlug(ctx, userID, hosp.ID)
			if err != nil {
				util.HandleError(c, err)
				return
			}
		}
	}
	roleSlug, err := auth.ResolveHospitalSessionRole(isSuper, isMember, hospitalRole)
	if err != nil {
		util.HandleError(c, err)
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

	resp := constant.NewSuccessResponse(constant.MsgTenantProfileRetrieved)
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
	dto := response.MeResponse{
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
	if u.AvatarContentType != nil && u.AvatarFileSize != nil && u.AvatarUpdatedAt != nil && u.AvatarObjectPath != nil {
		dto.ProfilePhoto = &response.ProfilePhotoMetadata{
			ContentType: *u.AvatarContentType, FileSize: *u.AvatarFileSize, UpdatedAt: u.AvatarUpdatedAt.UTC(),
		}
	}
	return dto
}
