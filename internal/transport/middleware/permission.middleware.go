package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	rolerepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

// RequirePermissions memastikan user (dari JWT) memiliki minimal
// salah satu dari permission 'required'.
func RequirePermissions(roleRepo *rolerepo.Repository, required ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if roleRepo == nil {
			// repo belum diinject: balas 500 yang rapi, jangan panic
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		userID := c.GetString("user_id")
		if userID == "" {
			resp := constant.ErrUnauthorized.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		perms, err := roleRepo.ListPermissionsByUser(c.Request.Context(), userID)
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		have := make(map[string]struct{}, len(perms))
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
			resp := constant.ErrForbidden.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireSuperAdmin restricts a global operation to an active SUPER_ADMIN.
// Tenant administrators must use the hospital-scoped middleware instead.
func RequireSuperAdmin(roleRepo *rolerepo.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if roleRepo == nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		userID := c.GetString("user_id")
		if userID == "" {
			resp := constant.ErrUnauthorized.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		isSuperAdmin, err := roleRepo.IsUserSuperAdmin(c.Request.Context(), userID)
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		if !isSuperAdmin {
			resp := constant.ErrOnlySuperAdmin.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireDoctor allows a doctor profile to be edited by its owner when the
// user has either the global DOCTOR role or a DOCTOR role in any active
// hospital membership. It does not promote a tenant role to a global role.
func RequireDoctor(roleRepo *rolerepo.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if roleRepo == nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		userID := c.GetString(string(constant.UserID))
		if userID == "" {
			resp := constant.ErrUnauthorized.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		globalDoctor, err := roleRepo.UserHasRole(c.Request.Context(), userID, constant.RoleDoctor)
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		tenantDoctor := false
		if !globalDoctor {
			tenantDoctor, err = roleRepo.UserHasAnyActiveHospitalRole(
				c.Request.Context(), userID, constant.RoleDoctor,
			)
			if err != nil {
				resp := constant.ErrInternalServerError.ToResponse()
				util.HandleResponse(c, &resp, nil)
				c.Abort()
				return
			}
		}
		if !globalDoctor && !tenantDoctor {
			resp := constant.ErrForbidden.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		c.Next()
	}
}
