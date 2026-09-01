package infrastructure

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestRedisInfoValueReadsEvictionPolicy(t *testing.T) {
	info := "# Memory\r\nused_memory:1234\r\nmaxmemory_policy:noeviction\r\n"
	if got := redisInfoValue(info, "maxmemory_policy"); got != "noeviction" {
		t.Fatalf("redisInfoValue() = %q, want noeviction", got)
	}
	if got := redisInfoValue(info, "missing"); got != "" {
		t.Fatalf("missing Redis INFO value = %q, want empty", got)
	}
}

type fakeRedisReadinessClient struct {
	pingErr       error
	existsErr     error
	exists        int64
	requestedKeys []string
}

func (f *fakeRedisReadinessClient) Ping(context.Context) *redis.StatusCmd {
	return redis.NewStatusResult("PONG", f.pingErr)
}

func (f *fakeRedisReadinessClient) Exists(_ context.Context, keys ...string) *redis.IntCmd {
	f.requestedKeys = append(f.requestedKeys, keys...)
	return redis.NewIntResult(f.exists, f.existsErr)
}

func TestRedisReadinessCheck(t *testing.T) {
	tests := []struct {
		name       string
		client     *fakeRedisReadinessClient
		wantErr    bool
		wantExists bool
	}{
		{name: "ready", client: &fakeRedisReadinessClient{}, wantExists: true},
		{name: "ping failure", client: &fakeRedisReadinessClient{pingErr: errors.New("offline")}, wantErr: true},
		{name: "exists failure", client: &fakeRedisReadinessClient{existsErr: errors.New("offline")}, wantErr: true, wantExists: true},
		{name: "maintenance", client: &fakeRedisReadinessClient{exists: 1}, wantErr: true, wantExists: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const key = "test:maintenance"
			err := redisReadinessCheck(tc.client, key)(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("redisReadinessCheck() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantExists {
				if len(tc.client.requestedKeys) != 1 || tc.client.requestedKeys[0] != key {
					t.Fatalf("maintenance keys = %v, want [%s]", tc.client.requestedKeys, key)
				}
			} else if len(tc.client.requestedKeys) != 0 {
				t.Fatalf("EXISTS called after failed PING with keys %v", tc.client.requestedKeys)
			}
		})
	}
}
