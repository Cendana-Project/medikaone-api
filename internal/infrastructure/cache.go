package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

func OpenRedisClient() (*redis.Client, error) {
	opts, err := redis.ParseURL(config.Env.Redis.CacheDSN)
	if err != nil {
		return nil, errors.New("invalid Redis configuration")
	}

	opts.MaxIdleConns = config.Env.Redis.MaxIdleConns
	opts.MaxActiveConns = config.Env.Redis.MaxActiveConns
	opts.PoolSize = config.Env.Redis.MaxActiveConns
	opts.MaxRetries = config.Env.Redis.MaxRetry
	opts.ConnMaxLifetime = config.Env.Redis.MaxConnLifetime
	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis unavailable: %s", redactConnectionError(err, config.Env.Redis.CacheDSN))
	}
	if config.Env.Env == "staging" || config.Env.Env == "production" {
		if err := VerifyRedisNoEviction(ctx, rdb); err != nil {
			_ = rdb.Close()
			return nil, err
		}
	}

	registerHealthCheck("redis", redisReadinessCheck(rdb, config.MaintenanceRedisKey()))

	logrus.Info("Redis connection established")
	return rdb, nil
}

// VerifyRedisNoEviction fails closed because authentication state, rate limits,
// and the database-maintenance sentinel must never be evicted under pressure.
func VerifyRedisNoEviction(ctx context.Context, rdb *redis.Client) error {
	memoryInfo, err := rdb.Info(ctx, "memory").Result()
	if err != nil {
		return errors.New("cannot verify Redis eviction policy")
	}
	policy := redisInfoValue(memoryInfo, "maxmemory_policy")
	if policy != "noeviction" {
		return fmt.Errorf("Redis maxmemory_policy must be noeviction for authentication state; got %q", policy)
	}
	return nil
}

type redisReadinessClient interface {
	Ping(context.Context) *redis.StatusCmd
	Exists(context.Context, ...string) *redis.IntCmd
}

func redisReadinessCheck(rdb redisReadinessClient, maintenanceKey string) healthCheck {
	return func(ctx context.Context) error {
		if err := rdb.Ping(ctx).Err(); err != nil {
			return errors.New("redis unavailable")
		}
		maintenance, err := rdb.Exists(ctx, maintenanceKey).Result()
		if err != nil {
			return errors.New("redis unavailable")
		}
		if maintenance > 0 {
			return errors.New("database maintenance in progress")
		}
		return nil
	}
}

func redisInfoValue(info, key string) string {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+":") {
			return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, key+":")))
		}
	}
	return ""
}
