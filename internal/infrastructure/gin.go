package infrastructure

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	transportmw "github.com/Cendana-Project/medikaone-api/internal/transport/middleware"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Dev frontends (Vite/CRA/Next) hop ports when the default one is taken, so a fixed
// allowlist entry per port doesn't scale. This matches any localhost/127.0.0.1 origin
// regardless of port so local dev always works without touching SERVER_CORS_ALLOWED_ORIGINS;
// non-localhost origins still go through the strict env-configured allowlist below.
var localDevOriginPattern = regexp.MustCompile(`^https?://(localhost|127\.0\.0\.1)(:\d+)?$`)

func NewGinEngine() *gin.Engine {
	if config.Env.Env == constant.ProductionEnvironment {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(securityHeaders())
	r.Use(transportmw.TraceID())
	r.Use(accessLogger())
	r.Use(gin.CustomRecovery(func(c *gin.Context, _ any) {
		logrus.WithFields(logrus.Fields{
			"request_id": transportmw.GetTraceID(c),
			"stack":      string(debug.Stack()),
		}).Error("request panic recovered")
		resp := constant.ErrInternalServerError.ToResponse()
		util.HandleResponse(c, &resp, nil)
		c.Abort()
	}))
	r.Use(limitRequestBody(10 << 20))
	r.Use(cors.New(cors.Config{
		AllowOrigins:     config.Env.Server.CORSAllowedOrigins,
		AllowOriginFunc:  func(origin string) bool { return localDevOriginPattern.MatchString(origin) },
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-Request-ID", "X-Hospital-ID", "X-Hospital-Code"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	registerHealthRoutes(r)
	return r
}

func securityHeaders() gin.HandlerFunc {
	secureTransport := config.Env.Env == "staging" || config.Env.Env == constant.ProductionEnvironment
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		if secureTransport {
			c.Header("Strict-Transport-Security", "max-age=31536000")
		}
		c.Next()
	}
}

func limitRequestBody(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			resp := constant.ErrRequestTooLarge.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

func accessLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		clientIP, clientIPErr := transportmw.ClientIP(c, config.Env.Server.ClientIPHeader)
		clientFingerprint := "invalid"
		if clientIPErr != nil {
			clientIP = ""
		} else {
			clientFingerprint = accessLogClientFingerprint(clientIP)
		}
		entry := logrus.WithFields(logrus.Fields{
			"client_fingerprint": clientFingerprint,
			"latency_ms":         time.Since(started).Milliseconds(),
			"method":             c.Request.Method,
			"path":               path,
			"request_id":         transportmw.GetTraceID(c),
			"status":             c.Writer.Status(),
		})
		if userID, ok := c.Get("user_id"); ok {
			entry = entry.WithField("user_id", userID)
		}
		if len(c.Errors) > 0 {
			entry.WithField("error_count", len(c.Errors)).Warn("request completed with errors")
			return
		}
		if c.Writer.Status() >= http.StatusInternalServerError {
			entry.Error("request completed")
			return
		}
		if c.Writer.Status() >= http.StatusBadRequest {
			entry.Warn("request completed")
			return
		}
		if strings.HasPrefix(path, "/_internal/") {
			entry.Debug("health request completed")
			return
		}
		entry.Info("request completed")
	}
}

func accessLogClientFingerprint(clientIP string) string {
	mac := hmac.New(sha256.New, []byte(config.Env.JWT.Secret))
	_, _ = mac.Write([]byte("access-log-ip"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(clientIP))
	return hex.EncodeToString(mac.Sum(nil)[:12])
}

func registerHealthRoutes(r *gin.Engine) {
	internal := r.Group("/_internal")
	internal.GET("/livez", func(c *gin.Context) {
		resp := constant.NewSuccessResponse(constant.MsgServiceAlive)
		resp.StatusCode = http.StatusOK
		resp.Data = gin.H{"status": "alive"}
		util.HandleResponse(c, resp, nil)
	})
	readiness := func(c *gin.Context) {
		checks := snapshotHealthChecks()
		names := make([]string, 0, len(checks))
		for name := range checks {
			names = append(names, name)
		}
		sort.Strings(names)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		ready := true
		for _, name := range names {
			if err := checks[name](ctx); err != nil {
				ready = false
				logrus.WithFields(logrus.Fields{
					"dependency": name,
					"request_id": transportmw.GetTraceID(c),
				}).Warn("readiness check failed")
			}
		}

		if !ready {
			resp := constant.ErrServiceNotReady.ToResponse()
			resp.Data = gin.H{"status": "not_ready"}
			util.HandleResponse(c, &resp, nil)
			return
		}
		resp := constant.NewSuccessResponse(constant.MsgServiceReady)
		resp.StatusCode = http.StatusOK
		resp.Data = gin.H{"status": "ready"}
		util.HandleResponse(c, resp, nil)
	}
	internal.GET("/readyz", readiness)
	// Backward-compatible alias for existing hosting health-check configuration.
	internal.GET("/healthz", readiness)
}
