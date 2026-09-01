package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

// TenantContext: resolve hospital ID dari:
// 1) path param :hospital_id
// 2) header X-Hospital-ID
// 3) header X-Hospital-Code  (disimpan apa adanya - resolver ada di service)
// Middleware ini hanya MENYALIN hint ke context (tanpa query DB).
func TenantContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		// prioritas path
		if v := c.Param("hospital_id"); v != "" {
			c.Set("hospital_hint", strings.TrimSpace(v))
			c.Next()
			return
		}
		// header id
		if v := strings.TrimSpace(c.GetHeader("X-Hospital-ID")); v != "" {
			c.Set("hospital_hint", v)
			c.Next()
			return
		}
		// header code
		if v := strings.TrimSpace(c.GetHeader("X-Hospital-Code")); v != "" {
			c.Set("hospital_hint", v)
			c.Next()
			return
		}
		// tidak fatal untuk semua endpoint; validasi akan dilakukan pada middleware berikutnya / handler
		c.Next()
	}
}

// RequireHospitalHint: pastikan hospital_hint tersedia
func RequireHospitalHint() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get("hospital_hint"); !ok {
			resp := constant.ErrValidationFailed.ToResponse()
			resp.Message = "hospital context required (X-Hospital-ID or X-Hospital-Code or :hospital_id)"
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
