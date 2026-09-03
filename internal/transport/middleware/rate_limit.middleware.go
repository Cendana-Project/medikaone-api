package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

var publicIPRateScript = redis.NewScript(`
local value = redis.call('INCR', KEYS[1])
if value == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end
return value`)

// RateLimitPublicAuthByIP adds a second line of defense to the identity-based
// limits in the auth service. Redis is used so the limit is shared by every
// application instance.
func RateLimitPublicAuthByIP(rdb *redis.Client, limit int, window time.Duration, clientIPHeader string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil || limit <= 0 || window <= 0 {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		clientIP, err := ClientIP(c, clientIPHeader)
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		clientFingerprint := clientIPFingerprint(clientIP, []byte(config.Env.JWT.Secret))
		key := config.AuthRedisKeyPrefix() + "auth:public:ip:" + clientFingerprint
		seconds := durationSecondsCeil(window)
		count, err := publicIPRateScript.Run(c.Request.Context(), rdb, []string{key}, seconds).Int64()
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		if count > int64(limit) {
			if ttl, err := rdb.TTL(c.Request.Context(), key).Result(); err == nil && ttl > 0 {
				c.Header("Retry-After", strconv.FormatInt(durationSecondsCeil(ttl), 10))
			}
			resp := constant.ErrPublicAuthRateLimitExceeded.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}

		c.Request = c.Request.WithContext(context.WithValue(
			c.Request.Context(), constant.ClientFingerprint, clientFingerprint,
		))
		c.Next()
	}
}

func durationSecondsCeil(duration time.Duration) int64 {
	seconds := int64(duration / time.Second)
	if duration%time.Second != 0 {
		seconds++
	}
	return max(int64(1), seconds)
}

func clientIPFingerprint(clientIP string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("public-auth-ip"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(clientIP))
	return hex.EncodeToString(mac.Sum(nil)[:12])
}

// ClientIP returns a caller address only from the configured trusted proxy
// header, or from the socket peer when no proxy header is configured.
func ClientIP(c *gin.Context, configuredHeader string) (string, error) {
	configuredHeader = strings.TrimSpace(configuredHeader)
	if configuredHeader != "" {
		value := strings.TrimSpace(c.GetHeader(configuredHeader))
		if strings.EqualFold(configuredHeader, "X-Forwarded-For") {
			value = strings.TrimSpace(strings.Split(value, ",")[0])
		}
		if net.ParseIP(value) == nil {
			return "", errors.New("trusted client IP header is missing or invalid")
		}
		return value, nil
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(c.Request.RemoteAddr))
	if err != nil || net.ParseIP(host) == nil {
		return "", errors.New("remote client IP is invalid")
	}
	return host, nil
}
