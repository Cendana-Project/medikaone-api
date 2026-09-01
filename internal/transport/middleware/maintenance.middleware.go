package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

var enterRequestScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
  return 0
end
local active = redis.call('INCR', KEYS[2])
redis.call('EXPIRE', KEYS[2], ARGV[1])
return active`)

var leaveRequestScript = redis.NewScript(`
local active = tonumber(redis.call('GET', KEYS[1])) or 0
if active <= 1 then
  redis.call('DEL', KEYS[1])
  return 0
end
return redis.call('DECR', KEYS[1])`)

var refreshActiveRequestScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
  return 1
end
return 0`)

func activeRequestHeartbeatInterval(activeTTLSeconds int64) time.Duration {
	interval := time.Duration(activeTTLSeconds) * time.Second / 3
	if interval < time.Second {
		return time.Second
	}
	return interval
}

// RejectDuringMaintenance drains application traffic while a guarded
// migration/reset owns the environment maintenance lease. Ping remains
// available so the platform can distinguish maintenance from a crashed app.
func RejectDuringMaintenance(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/ping" {
			c.Next()
			return
		}
		if rdb == nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		activeTTL := durationSecondsCeil(2*config.Env.Server.WriteTimeout + time.Minute)
		if activeTTL < 60 {
			activeTTL = 60
		}
		active, err := enterRequestScript.Run(
			c.Request.Context(),
			rdb,
			[]string{config.MaintenanceRedisKey(), config.ActiveRequestsRedisKey()},
			activeTTL,
		).Int64()
		if err != nil {
			resp := constant.ErrInternalServerError.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		if active == 0 {
			resp := constant.ErrServiceUnavailable.ToResponse()
			util.HandleResponse(c, &resp, nil)
			c.Abort()
			return
		}
		trackedRequestCtx, cancelTrackedRequest := context.WithCancel(c.Request.Context())
		c.Request = c.Request.WithContext(trackedRequestCtx)
		heartbeatCtx, stopHeartbeat := context.WithCancel(context.Background())
		heartbeatDone := make(chan struct{})
		go func() {
			defer close(heartbeatDone)
			ticker := time.NewTicker(activeRequestHeartbeatInterval(activeTTL))
			defer ticker.Stop()
			for {
				select {
				case <-heartbeatCtx.Done():
					return
				case <-ticker.C:
					refreshCtx, cancelRefresh := context.WithTimeout(heartbeatCtx, 2*time.Second)
					refreshed, err := refreshActiveRequestScript.Run(
						refreshCtx, rdb, []string{config.ActiveRequestsRedisKey()}, activeTTL,
					).Int64()
					cancelRefresh()
					if err != nil || refreshed != 1 {
						// If tracking cannot be kept alive, cancel database work rather
						// than let a reset mistake this handler for a completed request.
						cancelTrackedRequest()
						return
					}
				}
			}
		}()
		defer func() {
			// Stop and join the TTL heartbeat before decrementing; otherwise a late
			// refresh could recreate the appearance of an in-flight request.
			stopHeartbeat()
			<-heartbeatDone
			cancelTrackedRequest()
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Second)
			defer cancel()
			_ = leaveRequestScript.Run(cleanupCtx, rdb, []string{config.ActiveRequestsRedisKey()}).Err()
		}()
		c.Next()
	}
}
