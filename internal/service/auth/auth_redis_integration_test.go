package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newAuthRedisIntegrationClient(t *testing.T) *redis.Client {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("TEST_REDIS_DSN"))
	if dsn == "" {
		t.Skip("TEST_REDIS_DSN is not set")
	}
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		t.Fatalf("parse TEST_REDIS_DSN: %v", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("ping test Redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func authRedisTestPrefix(t *testing.T, client *redis.Client) string {
	t.Helper()

	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("create Redis test prefix: %v", err)
	}
	prefix := "medikaone:auth-integration:" + hex.EncodeToString(random) + ":"
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var cursor uint64
		for {
			keys, next, err := client.Scan(ctx, cursor, prefix+"*", 100).Result()
			if err != nil {
				return
			}
			if len(keys) > 0 {
				_ = client.Del(ctx, keys...).Err()
			}
			cursor = next
			if cursor == 0 {
				return
			}
		}
	})
	return prefix
}

func mustRedisSet(t *testing.T, ctx context.Context, client *redis.Client, key string, value any, ttl time.Duration) {
	t.Helper()
	if err := client.Set(ctx, key, value, ttl).Err(); err != nil {
		t.Fatalf("SET %q: %v", key, err)
	}
}

func TestRedisRegistrationPINAttemptLimitAndOperationRetry(t *testing.T) {
	client := newAuthRedisIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("wrong attempts are capped and a retried operation counts once", func(t *testing.T) {
		prefix := authRedisTestPrefix(t, client)
		challengeKey := prefix + "challenge"
		emailKey := prefix + "email"
		usernameKey := prefix + "username"
		attemptsKey := prefix + "attempts"
		cooldownKey := prefix + "cooldown"
		challengeID := "challenge-id"
		raw := `{"email":"user@example.test","pin_hash":"expected"}`
		for key, value := range map[string]string{
			challengeKey: raw,
			emailKey:     challengeID,
			usernameKey:  challengeID,
			cooldownKey:  "1",
		} {
			mustRedisSet(t, ctx, client, key, value, 5*time.Minute)
		}

		run := func(operation string) int {
			t.Helper()
			result, err := verifyRegistrationPINScript.Run(
				ctx,
				client,
				[]string{challengeKey, emailKey, usernameKey, attemptsKey, cooldownKey, prefix + "operation:" + operation},
				raw,
				challengeID,
				"0",
				3,
			).Int()
			if err != nil {
				t.Fatalf("verify registration PIN operation %q: %v", operation, err)
			}
			return result
		}

		if got := run("one"); got != -1 {
			t.Fatalf("first wrong attempt result = %d, want -1", got)
		}
		if got := run("one"); got != -1 {
			t.Fatalf("same operation retry result = %d, want cached -1", got)
		}
		if got, err := client.Get(ctx, attemptsKey).Int(); err != nil || got != 1 {
			t.Fatalf("attempts after retry = %d, %v; want 1", got, err)
		}
		if got := run("two"); got != -1 {
			t.Fatalf("second wrong attempt result = %d, want -1", got)
		}
		if got := run("three"); got != -2 {
			t.Fatalf("third wrong attempt result = %d, want -2", got)
		}
		for _, key := range []string{challengeKey, emailKey, usernameKey, attemptsKey, cooldownKey} {
			if exists, err := client.Exists(ctx, key).Result(); err != nil || exists != 0 {
				t.Fatalf("key %q exists after hard cap: exists=%d err=%v", key, exists, err)
			}
		}
	})

	t.Run("successful consume is idempotent for the same operation", func(t *testing.T) {
		prefix := authRedisTestPrefix(t, client)
		challengeKey := prefix + "challenge"
		emailKey := prefix + "email"
		usernameKey := prefix + "username"
		attemptsKey := prefix + "attempts"
		cooldownKey := prefix + "cooldown"
		operationKey := prefix + "operation"
		challengeID := "challenge-id"
		raw := `{"email":"user@example.test","pin_hash":"expected"}`
		for key, value := range map[string]string{
			challengeKey: raw,
			emailKey:     challengeID,
			usernameKey:  challengeID,
			cooldownKey:  "1",
		} {
			mustRedisSet(t, ctx, client, key, value, 5*time.Minute)
		}

		run := func() int {
			t.Helper()
			result, err := verifyRegistrationPINScript.Run(
				ctx,
				client,
				[]string{challengeKey, emailKey, usernameKey, attemptsKey, cooldownKey, operationKey},
				raw,
				challengeID,
				"1",
				3,
			).Int()
			if err != nil {
				t.Fatalf("consume registration PIN: %v", err)
			}
			return result
		}

		if got := run(); got != 1 {
			t.Fatalf("first consume result = %d, want 1", got)
		}
		if got := run(); got != 1 {
			t.Fatalf("same operation retry result = %d, want cached 1", got)
		}
		if exists, err := client.Exists(ctx, challengeKey).Result(); err != nil || exists != 0 {
			t.Fatalf("challenge exists after consume: exists=%d err=%v", exists, err)
		}
	})
}

func TestRedisPasswordResetAttemptLimitAndOperationRetry(t *testing.T) {
	client := newAuthRedisIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prefix := authRedisTestPrefix(t, client)
	secretKey := prefix + "secret"
	attemptsKey := prefix + "attempts"
	currentKey := prefix + "current"
	challengeID := "challenge-id"
	expected := "expected-pin-hash"
	mustRedisSet(t, ctx, client, secretKey, expected, 5*time.Minute)
	mustRedisSet(t, ctx, client, currentKey, challengeID, 5*time.Minute)

	run := func(operation string) int {
		t.Helper()
		result, err := verifyPasswordResetPINScript.Run(
			ctx,
			client,
			[]string{secretKey, attemptsKey, prefix + "operation:" + operation, currentKey},
			"wrong-pin-hash",
			3,
			challengeID,
		).Int()
		if err != nil {
			t.Fatalf("verify password reset PIN operation %q: %v", operation, err)
		}
		return result
	}

	if got := run("one"); got != -1 {
		t.Fatalf("first wrong attempt result = %d, want -1", got)
	}
	if got := run("one"); got != -1 {
		t.Fatalf("same operation retry result = %d, want cached -1", got)
	}
	if got, err := client.Get(ctx, attemptsKey).Int(); err != nil || got != 1 {
		t.Fatalf("attempts after retry = %d, %v; want 1", got, err)
	}
	if got := run("two"); got != -1 {
		t.Fatalf("second wrong attempt result = %d, want -1", got)
	}
	if got := run("three"); got != -2 {
		t.Fatalf("third wrong attempt result = %d, want -2", got)
	}
	for _, key := range []string{secretKey, attemptsKey, currentKey} {
		if exists, err := client.Exists(ctx, key).Result(); err != nil || exists != 0 {
			t.Fatalf("key %q exists after hard cap: exists=%d err=%v", key, exists, err)
		}
	}
}

func TestRedisPasswordResetIssuanceReplacesCurrentChallengeAtomically(t *testing.T) {
	client := newAuthRedisIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prefix := authRedisTestPrefix(t, client)
	challengePrefix := prefix + "challenge:"
	attemptsPrefix := prefix + "attempts:"
	currentKey := prefix + "current"

	issue := func(challengeID, record string) {
		t.Helper()
		if err := storePasswordResetScript.Run(
			ctx,
			client,
			[]string{challengePrefix + challengeID, currentKey},
			challengeID,
			record,
			300,
			challengePrefix,
			attemptsPrefix,
		).Err(); err != nil {
			t.Fatalf("issue password reset challenge %q: %v", challengeID, err)
		}
	}

	issue("old-challenge", "old-record")
	mustRedisSet(t, ctx, client, attemptsPrefix+"old-challenge", "2", 5*time.Minute)
	issue("new-challenge", "new-record")

	if got, err := client.Get(ctx, currentKey).Result(); err != nil || got != "new-challenge" {
		t.Fatalf("current challenge = %q, %v; want new-challenge", got, err)
	}
	if got, err := client.Get(ctx, challengePrefix+"new-challenge").Result(); err != nil || got != "new-record" {
		t.Fatalf("new challenge = %q, %v; want new-record", got, err)
	}
	for _, key := range []string{challengePrefix + "old-challenge", attemptsPrefix + "old-challenge"} {
		if exists, err := client.Exists(ctx, key).Result(); err != nil || exists != 0 {
			t.Fatalf("superseded key %q exists=%d err=%v", key, exists, err)
		}
	}

	result, err := verifyPasswordResetPINScript.Run(
		ctx,
		client,
		[]string{
			challengePrefix + "old-challenge",
			attemptsPrefix + "old-challenge",
			prefix + "operation:old",
			currentKey,
		},
		"old-record",
		3,
		"old-challenge",
	).Int()
	if err != nil {
		t.Fatalf("verify superseded challenge: %v", err)
	}
	if result != 0 {
		t.Fatalf("superseded challenge verification = %d, want 0", result)
	}

	consumeResult, err := consumePasswordResetAndRevokeScript.Run(
		ctx,
		client,
		[]string{
			challengePrefix + "old-challenge",
			attemptsPrefix + "old-challenge",
			prefix + "refresh-set",
			prefix + "session-version",
			prefix + "consume-operation:old",
			currentKey,
		},
		"old-record",
		"new-session-version",
		prefix+"refresh:",
		3,
		"old-challenge",
	).Int64()
	if err != nil {
		t.Fatalf("consume superseded challenge: %v", err)
	}
	if consumeResult != 0 {
		t.Fatalf("superseded challenge consume = %d, want 0", consumeResult)
	}

	restored, err := restoreSecretIfMissingScript.Run(
		ctx,
		client,
		[]string{challengePrefix + "old-challenge", currentKey},
		"old-record",
		300_000,
		"old-challenge",
	).Int()
	if err != nil {
		t.Fatalf("restore superseded challenge: %v", err)
	}
	if restored != 0 {
		t.Fatalf("restore superseded challenge = %d, want 0", restored)
	}
	if got, err := client.Get(ctx, currentKey).Result(); err != nil || got != "new-challenge" {
		t.Fatalf("current challenge after stale restore = %q, %v; want new-challenge", got, err)
	}

	invalidated, err := invalidateCurrentPasswordResetScript.Run(
		ctx,
		client,
		[]string{currentKey},
		challengePrefix,
		attemptsPrefix,
	).Int()
	if err != nil {
		t.Fatalf("invalidate current challenge after successful reset: %v", err)
	}
	if invalidated != 1 {
		t.Fatalf("current challenge invalidation = %d, want 1", invalidated)
	}
	for _, key := range []string{currentKey, challengePrefix + "new-challenge"} {
		if exists, err := client.Exists(ctx, key).Result(); err != nil || exists != 0 {
			t.Fatalf("key %q exists after successful reset cleanup: exists=%d err=%v", key, exists, err)
		}
	}
}

func TestRedisPasswordResetConsumeOperationRetryIsIdempotent(t *testing.T) {
	client := newAuthRedisIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prefix := authRedisTestPrefix(t, client)
	secretKey := prefix + "secret"
	attemptsKey := prefix + "attempts"
	refreshSetKey := prefix + "refresh-set"
	sessionVersionKey := prefix + "session-version"
	operationKey := prefix + "consume-operation"
	currentKey := prefix + "current"
	refreshPrefix := prefix + "refresh:"
	challengeID := "challenge-id"
	expected := "expected-pin-hash"
	newVersion := "new-session-version"
	refreshJTI := "refresh-jti"
	mustRedisSet(t, ctx, client, secretKey, expected, 5*time.Minute)
	mustRedisSet(t, ctx, client, currentKey, challengeID, 5*time.Minute)
	mustRedisSet(t, ctx, client, refreshPrefix+refreshJTI, "user-record", 5*time.Minute)
	if err := client.ZAdd(ctx, refreshSetKey, redis.Z{Score: float64(time.Now().Add(5 * time.Minute).Unix()), Member: refreshJTI}).Err(); err != nil {
		t.Fatalf("seed refresh set: %v", err)
	}

	run := func() int64 {
		t.Helper()
		result, err := consumePasswordResetAndRevokeScript.Run(
			ctx,
			client,
			[]string{secretKey, attemptsKey, refreshSetKey, sessionVersionKey, operationKey, currentKey},
			expected,
			newVersion,
			refreshPrefix,
			3,
			challengeID,
		).Int64()
		if err != nil {
			t.Fatalf("consume password reset PIN: %v", err)
		}
		return result
	}

	first := run()
	if first <= 0 {
		t.Fatalf("first consume TTL = %d, want positive", first)
	}
	if second := run(); second != first {
		t.Fatalf("same operation retry TTL = %d, want cached %d", second, first)
	}
	if got, err := client.Get(ctx, sessionVersionKey).Result(); err != nil || got != newVersion {
		t.Fatalf("session version = %q, %v; want %q", got, err, newVersion)
	}
	for _, key := range []string{secretKey, refreshSetKey, refreshPrefix + refreshJTI, currentKey} {
		if exists, err := client.Exists(ctx, key).Result(); err != nil || exists != 0 {
			t.Fatalf("key %q exists after reset consume: exists=%d err=%v", key, exists, err)
		}
	}
}

func TestRedisRefreshRotationRetryAndReplay(t *testing.T) {
	client := newAuthRedisIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prefix := authRedisTestPrefix(t, client)
	refreshPrefix := prefix + "refresh:"
	oldJTI := "old-jti"
	nextJTI := "next-jti"
	retriedNextJTI := "retried-next-jti"
	replayNextJTI := "replay-next-jti"
	oldRefreshKey := refreshPrefix + oldJTI
	refreshSetKey := prefix + "refresh-set"
	tombstoneKey := prefix + "used:" + oldJTI
	sessionVersionKey := prefix + "session-version"
	version := "session-version"
	replayVersion := "replay-version"
	oldRecord := refreshRecord("user-id", "family-id", version)
	nextRecord := refreshRecord("user-id", "family-id", version)
	oldTTLSeconds := int64(300)
	newTTLSeconds := int64(600)
	now := time.Now()
	mustRedisSet(t, ctx, client, oldRefreshKey, oldRecord, time.Duration(oldTTLSeconds)*time.Second)
	mustRedisSet(t, ctx, client, sessionVersionKey, version, 0)
	if err := client.ZAdd(ctx, refreshSetKey, redis.Z{Score: float64(now.Add(5 * time.Minute).Unix()), Member: oldJTI}).Err(); err != nil {
		t.Fatalf("seed refresh set: %v", err)
	}

	run := func(successorJTI, operationFingerprint string) int {
		t.Helper()
		resultKey := prefix + "result:" + operationFingerprint
		result, err := rotateRefreshScript.Run(
			ctx,
			client,
			[]string{
				oldRefreshKey, refreshSetKey, tombstoneKey, sessionVersionKey,
				refreshPrefix + successorJTI, resultKey,
			},
			oldRecord,
			oldJTI,
			oldTTLSeconds,
			version,
			replayVersion,
			refreshPrefix,
			nextRecord,
			newTTLSeconds,
			successorJTI,
			now.Unix(),
			now.Add(time.Duration(newTTLSeconds)*time.Second).Unix(),
			10,
			oldRecord+"|"+operationFingerprint,
			"encrypted-result-for-"+operationFingerprint,
			120,
		).Int()
		if err != nil {
			t.Fatalf("rotate refresh to %q: %v", successorJTI, err)
		}
		return result
	}

	if got := run(nextJTI, "operation-a"); got != 1 {
		t.Fatalf("first rotation result = %d, want 1", got)
	}
	if got := run(retriedNextJTI, "operation-a"); got != 2 {
		t.Fatalf("same operation retry result = %d, want 2", got)
	}
	if got, err := client.Get(ctx, sessionVersionKey).Result(); err != nil || got != version {
		t.Fatalf("session version after same retry = %q, %v; want %q", got, err, version)
	}
	if got, err := client.Get(ctx, prefix+"result:operation-a").Result(); err != nil || got != "encrypted-result-for-operation-a" {
		t.Fatalf("cached refresh result = %q, %v", got, err)
	}
	if exists, err := client.Exists(ctx, refreshPrefix+retriedNextJTI).Result(); err != nil || exists != 0 {
		t.Fatalf("retry-created successor unexpectedly active: exists=%d err=%v", exists, err)
	}
	if err := client.Del(ctx, prefix+"result:operation-a").Err(); err != nil {
		t.Fatalf("expire cached refresh result: %v", err)
	}
	if got := run(retriedNextJTI, "operation-a"); got != -2 {
		t.Fatalf("same operation without cached result = %d, want replay -2", got)
	}
	if got, err := client.Get(ctx, sessionVersionKey).Result(); err != nil || got != replayVersion {
		t.Fatalf("session version after missing-cache replay = %q, %v; want %q", got, err, replayVersion)
	}

	// Seed a fresh family to verify that a genuinely different operation also
	// triggers replay revocation.
	version = "second-session-version"
	replayVersion = "second-replay-version"
	oldRecord = refreshRecord("user-id", "family-id", version)
	nextRecord = oldRecord
	mustRedisSet(t, ctx, client, oldRefreshKey, oldRecord, time.Duration(oldTTLSeconds)*time.Second)
	mustRedisSet(t, ctx, client, sessionVersionKey, version, 0)
	if err := client.ZAdd(ctx, refreshSetKey, redis.Z{Score: float64(now.Add(5 * time.Minute).Unix()), Member: oldJTI}).Err(); err != nil {
		t.Fatalf("reseed refresh set: %v", err)
	}
	if got := run(nextJTI, "operation-c"); got != 1 {
		t.Fatalf("second first rotation result = %d, want 1", got)
	}
	if got := run(replayNextJTI, "operation-b"); got != -2 {
		t.Fatalf("different operation replay result = %d, want -2", got)
	}
	if got, err := client.Get(ctx, sessionVersionKey).Result(); err != nil || got != replayVersion {
		t.Fatalf("session version after replay = %q, %v; want %q", got, err, replayVersion)
	}
	for _, key := range []string{
		refreshSetKey, refreshPrefix + nextJTI, refreshPrefix + retriedNextJTI, refreshPrefix + replayNextJTI,
	} {
		if exists, err := client.Exists(ctx, key).Result(); err != nil || exists != 0 {
			t.Fatalf("key %q exists after replay revocation: exists=%d err=%v", key, exists, err)
		}
	}
}

func TestRedisLogoutRevokesOnlyCurrentRefreshFamily(t *testing.T) {
	client := newAuthRedisIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prefix := authRedisTestPrefix(t, client)
	refreshPrefix := prefix + "refresh:"
	refreshSetKey := prefix + "refresh-set"
	blacklistKey := prefix + "blacklist:access-jti"
	familyRevokedKey := prefix + "family-revoked:family-a"
	userID := "user-id"
	version := "session-version"
	familyARecord := refreshRecord(userID, "family-a", version)
	familyBRecord := refreshRecord(userID, "family-b", version)
	members := []struct {
		jti    string
		record string
	}{
		{jti: "family-a-old", record: familyARecord},
		{jti: "family-a-new", record: familyARecord},
		{jti: "family-b", record: familyBRecord},
	}
	for index, member := range members {
		mustRedisSet(t, ctx, client, refreshPrefix+member.jti, member.record, 10*time.Minute)
		if err := client.ZAdd(ctx, refreshSetKey, redis.Z{Score: float64(index + 1), Member: member.jti}).Err(); err != nil {
			t.Fatalf("seed refresh member %q: %v", member.jti, err)
		}
	}

	revoked, err := logoutFamilyScript.Run(
		ctx,
		client,
		[]string{blacklistKey, refreshSetKey, familyRevokedKey},
		300,
		900,
		refreshPrefix,
		familyARecord,
	).Int()
	if err != nil {
		t.Fatalf("logout family: %v", err)
	}
	if revoked != 2 {
		t.Fatalf("revoked refresh count = %d, want 2", revoked)
	}
	for _, jti := range []string{"family-a-old", "family-a-new"} {
		if exists, err := client.Exists(ctx, refreshPrefix+jti).Result(); err != nil || exists != 0 {
			t.Fatalf("revoked refresh %q still exists=%d err=%v", jti, exists, err)
		}
	}
	if got, err := client.Get(ctx, refreshPrefix+"family-b").Result(); err != nil || got != familyBRecord {
		t.Fatalf("other refresh family = %q, %v; want preserved", got, err)
	}
	if _, err := client.ZScore(ctx, refreshSetKey, "family-b").Result(); err != nil {
		t.Fatalf("other refresh family missing from index: %v", err)
	}
	if got, err := client.Get(ctx, blacklistKey).Result(); err != nil || got != "1" {
		t.Fatalf("access blacklist = %q, %v", got, err)
	}
	if got, err := client.Get(ctx, familyRevokedKey).Result(); err != nil || got != "1" {
		t.Fatalf("access family revocation = %q, %v", got, err)
	}
}

func TestRedisFailedPasswordResetDeliveryBecomesDecoy(t *testing.T) {
	client := newAuthRedisIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prefix := authRedisTestPrefix(t, client)
	challengeKey := prefix + "password-reset"
	activeRecord := passwordResetRecord("user-id", "pin-hash")
	decoyRecord := passwordResetRecord("unavailable", "pin-hash")
	mustRedisSet(t, ctx, client, challengeKey, activeRecord, 5*time.Minute)

	remainingMS, err := replaceIfValueScript.Run(
		ctx, client, []string{challengeKey}, activeRecord, decoyRecord,
	).Int64()
	if err != nil {
		t.Fatalf("replace active reset challenge: %v", err)
	}
	if remainingMS <= 0 {
		t.Fatalf("replacement TTL = %d, want positive", remainingMS)
	}
	if got, err := client.Get(ctx, challengeKey).Result(); err != nil || got != decoyRecord {
		t.Fatalf("replacement challenge = %q, %v; want decoy", got, err)
	}
	if got, err := replaceIfValueScript.Run(
		ctx, client, []string{challengeKey}, activeRecord, "attacker-value",
	).Int64(); err != nil || got != 0 {
		t.Fatalf("stale CAS replacement = %d, %v; want 0", got, err)
	}
}
