package middleware

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	hospRepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	roleRepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

func RequireHospitalPermissions(hRepo *hospRepo.Repository, rRepo *roleRepo.Repository, required ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			resp := constant.ErrUnauthorized.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		if hRepo == nil || rRepo == nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		hintVal, ok := c.Get("hospital_hint")
		if !ok {
			util.HandleError(c, constant.ErrHospitalContextRequired)
			c.Abort()
			return
		}
		hint, ok := hintVal.(string)
		if !ok || hint == "" {
			util.HandleError(c, constant.ErrHospitalContextRequired)
			c.Abort()
			return
		}

		hospitalID, err := hRepo.ResolveHospitalID(c.Request.Context(), hint)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				resp := constant.ErrInternalServerError.ToResponse()
				util.HandleResponse(c, &resp, nil)
				c.Abort()
				return
			}
		}
		if hospitalID == "" {
			util.HandleError(c, constant.ErrHospitalNotFound)
			c.Abort()
			return
		}
		c.Set("hospital_id", hospitalID)

		isSuper, err := rRepo.IsUserSuperAdmin(c.Request.Context(), userID)
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		if isSuper {
			c.Next()
			return
		}

		perms, err := rRepo.ListHospitalPermissionsByUser(c.Request.Context(), hospitalID, userID)
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		have := map[string]struct{}{}
		for _, p := range perms {
			have[p.Slug] = struct{}{}
		}

		allowed := false
		for _, need := range required {
			if _, ok := have[need]; ok {
				allowed = true
				break
			}
		}
		if !allowed {
			resp := constant.NewRequiredPermissionError(required...).ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

// Hanya izinkan SUPER_ADMIN global atau HOSPITAL_ADMIN pada hospital terkait.
func RequireHospitalAdminOrSuper(hRepo *hospRepo.Repository, rRepo *roleRepo.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			resp := constant.ErrUnauthorized.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		if hRepo == nil || rRepo == nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		hintVal, ok := c.Get("hospital_hint")
		if !ok {
			util.HandleError(c, constant.ErrHospitalContextRequired)
			c.Abort()
			return
		}
		hint, ok := hintVal.(string)
		if !ok || hint == "" {
			util.HandleError(c, constant.ErrHospitalContextRequired)
			c.Abort()
			return
		}

		hospitalID, err := hRepo.ResolveHospitalID(c.Request.Context(), hint)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				resp := constant.ErrInternalServerError.ToResponse()
				util.HandleResponse(c, &resp, nil)
				c.Abort()
				return
			}
		}
		if hospitalID == "" {
			util.HandleError(c, constant.ErrHospitalNotFound)
			c.Abort()
			return
		}
		c.Set("hospital_id", hospitalID)

		// SUPER_ADMIN global langsung lolos
		isSuper, err := rRepo.IsUserSuperAdmin(c.Request.Context(), userID)
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		if isSuper {
			c.Next()
			return
		}

		// Jika bukan super, wajib HOSPITAL_ADMIN di hospital ini
		isHospAdmin, err := rRepo.IsUserHospitalAdmin(c.Request.Context(), hospitalID, userID)
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		if !isHospAdmin {
			util.HandleError(c, constant.ErrHospitalAdminRequired)
			c.Abort()
			return
		}

		c.Next()
	}
}
