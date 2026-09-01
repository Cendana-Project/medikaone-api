package middleware

import (
	"context"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const traceKey = "trace_id"

// TraceID men-setup trace id per request.
// Sumber: header X-Request-Id (jika ada dan valid UUID), jika tidak generate UUID v4.
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader("X-Request-Id")
		if _, err := uuid.Parse(tid); err != nil || tid == "" {
			tid = uuid.NewString()
		}
		c.Set(traceKey, tid)
		c.Set(string(constant.RequestID), tid)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), constant.RequestID, tid))
		c.Writer.Header().Set("X-Request-Id", tid) // echo ke client
		c.Next()
	}
}

// GetTraceID mengambil trace id dari context.
func GetTraceID(c *gin.Context) string {
	if v, ok := c.Get(traceKey); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}
