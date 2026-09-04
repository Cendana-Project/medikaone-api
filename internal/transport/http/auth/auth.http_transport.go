package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
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

// =======================================
// AUTH — PUBLIC REGISTRATION & OTP FLOW
// =======================================

func (ctl *Controller) Register(c *gin.Context) {
	var req request.RegisterLiteRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.svc.RegisterLite(c.Request.Context(), &req)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgRegistrationChallengeCreated)
	resp.StatusCode = http.StatusOK
	resp.Data = result
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) ResendPIN(c *gin.Context) {
	var req request.ResendPINRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	email := strings.TrimSpace(req.Email)
	if err := ctl.svc.ResendPIN(c.Request.Context(), email, req.ChallengeID); err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgRegistrationPINResent)
	resp.StatusCode = http.StatusOK
	resp.Data = gin.H{"challenge_id": req.ChallengeID, "email": email, "status": "pending"}
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) VerifyPIN(c *gin.Context) {
	var req request.VerifyPINRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	email := strings.TrimSpace(req.Email)
	pin := strings.TrimSpace(req.PIN)
	tokens, aexp, rexp, err := ctl.svc.VerifyPIN(c.Request.Context(), email, req.ChallengeID, pin)
	if err != nil {
		util.HandleError(c, err)
		return
	}

	// A newly verified account has not selected a public role yet.
	roleSlug := ""

	resp := constant.NewSuccessResponse(constant.MsgRegistrationVerified)
	resp.StatusCode = http.StatusOK
	resp.Data = response.LoginResponse{
		AccessToken:           tokens.AccessToken,
		RefreshToken:          tokens.RefreshToken,
		Role:                  roleSlug,
		AccessTokenExpiredAt:  aexp.UTC().Format(time.RFC3339),
		RefreshTokenExpiredAt: rexp.UTC().Format(time.RFC3339),
	}
	util.HandleResponse(c, resp, nil)
}

// =============================
// AUTH — LOGIN
// =============================

func (ctl *Controller) LoginPublic(c *gin.Context) {
	var req request.LoginRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	identity := strings.TrimSpace(req.Identity)
	password := req.Password

	tokens, roles, aexp, rexp, err := ctl.svc.Login(c.Request.Context(), identity, password)
	if err != nil {
		util.HandleError(c, err)
		return
	}

	roleSlug := ""
	if len(roles) > 0 {
		roleSlug = roles[0].Slug
	}

	resp := constant.NewSuccessResponse(constant.MsgPublicLoginSucceeded)
	resp.StatusCode = http.StatusOK
	resp.Data = response.LoginResponse{
		AccessToken:           tokens.AccessToken,
		RefreshToken:          tokens.RefreshToken,
		Role:                  roleSlug,
		AccessTokenExpiredAt:  aexp.UTC().Format(time.RFC3339),
		RefreshTokenExpiredAt: rexp.UTC().Format(time.RFC3339),
	}
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) LoginHospital(c *gin.Context) {
	var req request.LoginHospitalRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}

	hint := ""
	if req.HospitalID != nil && *req.HospitalID != "" {
		hint = *req.HospitalID
	} else if req.HospitalCode != nil && *req.HospitalCode != "" {
		hint = *req.HospitalCode
	}

	res, err := ctl.svc.LoginHospital(c.Request.Context(), req.Identifier, req.Password, hint)
	if err != nil {
		util.HandleError(c, err)
		return
	}

	roleSlug := ""
	if len(res.Roles) > 0 {
		roleSlug = res.Roles[0].Slug
	}

	resp := constant.NewSuccessResponse(constant.MsgHospitalLoginSucceeded)
	resp.StatusCode = http.StatusOK
	resp.Data = response.LoginHospitalResponse{
		AccessToken:           res.AccessToken,
		RefreshToken:          res.RefreshToken,
		ExpiresIn:             res.ExpiresIn,
		TokenType:             res.TokenType,
		HospitalID:            res.HospitalID,
		Role:                  roleSlug,
		AccessTokenExpiredAt:  res.AccessExp.UTC().Format(time.RFC3339),
		RefreshTokenExpiredAt: res.RefreshExp.UTC().Format(time.RFC3339),
	}
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) Refresh(c *gin.Context) {
	var req request.RefreshTokenRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	tokens, aexp, rexp, err := ctl.svc.Refresh(
		c.Request.Context(), strings.TrimSpace(req.RefreshToken), strings.TrimSpace(req.IdempotencyKey),
	)
	if err != nil {
		util.HandleError(c, err)
		return
	}

	// The access token was just issued by this service; use its subject to load
	// the response's current role.
	userID, _ := extractSubFromJWT(tokens.AccessToken)
	roleSlug := ""
	if userID != "" && ctl.userRepo != nil {
		if slug, err := ctl.userRepo.GetUserRoleSlug(c.Request.Context(), userID); err == nil {
			roleSlug = slug
		}
	}

	resp := constant.NewSuccessResponse(constant.MsgTokensRefreshed)
	resp.StatusCode = http.StatusOK
	resp.Data = response.LoginResponse{
		AccessToken:           tokens.AccessToken,
		RefreshToken:          tokens.RefreshToken,
		Role:                  roleSlug,
		AccessTokenExpiredAt:  aexp.UTC().Format(time.RFC3339),
		RefreshTokenExpiredAt: rexp.UTC().Format(time.RFC3339),
	}
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) ChooseRole(c *gin.Context) {
	userID := util.GetUserID(c)
	if userID == "" {
		res := constant.ErrUnauthorized.ToResponse()
		util.HandleResponse(c, &res, nil)
		return
	}
	var req request.ChooseRoleRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	role := strings.ToUpper(strings.TrimSpace(req.Role))
	if err := ctl.svc.ChooseRole(c.Request.Context(), userID, role); err != nil {
		util.HandleError(c, err)
		return
	}
	message := constant.MsgPatientRoleSelected
	if role == constant.RoleDoctor {
		message = constant.MsgDoctorRoleSelected
	}
	resp := constant.NewSuccessResponse(message)
	resp.StatusCode = http.StatusOK
	resp.Data = gin.H{"role": role}
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) PasswordForgot(c *gin.Context) {
	var req request.PasswordForgotRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.svc.PasswordForgot(c.Request.Context(), &req)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgPasswordResetPINSent)
	resp.Data = result
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) PasswordResetVerifyPIN(c *gin.Context) {
	// The response contains a short-lived credential and must not be stored by
	// browsers, proxies, or other shared caches.
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")

	var req request.PasswordResetVerifyPINRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.svc.PasswordResetVerifyPIN(c.Request.Context(), &req)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgPasswordResetPINVerified)
	resp.Data = result
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) PasswordReset(c *gin.Context) {
	var req request.PasswordResetRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	if err := ctl.svc.PasswordReset(c.Request.Context(), &req); err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgPasswordResetCompleted)
	resp.Data = gin.H{"status": "password_updated"}
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) PasswordChange(c *gin.Context) {
	var req request.PasswordChangeRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	userID := util.GetUserID(c)
	if userID == "" {
		util.HandleError(c, constant.ErrUnauthorized)
		return
	}
	if err := ctl.svc.PasswordChange(c.Request.Context(), userID, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	resp := constant.NewSuccessResponse(constant.MsgPasswordChanged)
	resp.Data = gin.H{"status": "password_changed"}
	util.HandleResponse(c, resp, nil)
}

// === SET PROFILE (gabungan choose-role + set profile) ===

func (ctl *Controller) SetProfile(c *gin.Context) {
	var req request.SetProfileRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	userID := util.GetUserID(c)
	if userID == "" {
		util.HandleError(c, constant.ErrUnauthorized)
		return
	}

	// Safety normalize role ke UPPERCASE di service juga.
	req.Role = strings.ToUpper(strings.TrimSpace(req.Role))

	// Pastikan profile non-null dan JSON well-formed
	if req.Profile == nil || len(*req.Profile) == 0 || string(*req.Profile) == "null" {
		util.HandleError(c, constant.NewFieldRequiredError("profile"))
		return
	}

	res, err := ctl.svc.SetProfile(c.Request.Context(), userID, req.Role, req.Profile)
	if err != nil {
		util.HandleError(c, err)
		return
	}

	message := constant.MsgPatientProfileCompleted
	if res.Role == constant.RoleDoctor {
		message = constant.MsgDoctorProfileCompleted
	}
	resp := constant.NewSuccessResponse(message)
	resp.StatusCode = http.StatusOK
	resp.Data = res
	util.HandleResponse(c, resp, nil)
}

// =============================
// AUTH — LOGOUT (new)
// =============================

func (ctl *Controller) Logout(c *gin.Context) {
	var body request.LogoutRequest
	if err := util.DecodeStrictJSON(c.Request.Body, &body); err != nil && !errors.Is(err, io.EOF) {
		util.HandleError(c, util.MapJSONDecodeError(err))
		return
	}

	h := c.GetHeader("Authorization")
	if !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		util.HandleError(c, constant.ErrUnauthorized)
		return
	}
	access := strings.TrimSpace(h[7:])

	if err := ctl.svc.Logout(c.Request.Context(), access, strings.TrimSpace(body.RefreshToken)); err != nil {
		util.HandleError(c, err)
		return
	}

	resp := constant.NewSuccessResponse(constant.MsgLogoutCompleted)
	resp.StatusCode = http.StatusOK
	resp.Data = gin.H{
		"status":         "logout_success",
		"revoked_family": true,
	}
	util.HandleResponse(c, resp, nil)
}

func (ctl *Controller) LogoutAll(c *gin.Context) {
	h := c.GetHeader("Authorization")
	if !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		util.HandleError(c, constant.ErrUnauthorized)
		return
	}
	access := strings.TrimSpace(h[7:])
	if err := ctl.svc.LogoutAll(c.Request.Context(), access); err != nil {
		util.HandleError(c, err)
		return
	}

	resp := constant.NewSuccessResponse(constant.MsgAllSessionsLoggedOut)
	resp.StatusCode = http.StatusOK
	resp.Data = gin.H{"status": "logout_all_success"}
	util.HandleResponse(c, resp, nil)
}

// extractSubFromJWT mengekstrak claim "sub" dari JWT tanpa verifikasi signature.
// Aman untuk use-case ini karena token baru saja diterbitkan di sisi server.
func extractSubFromJWT(tok string) (string, error) {
	parts := strings.Split(tok, ".")
	if len(parts) < 2 {
		return "", errors.New("invalid jwt")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return "", err
	}
	return strings.TrimSpace(claims.Sub), nil
}
