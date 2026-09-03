package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	ulog "github.com/Cendana-Project/medikaone-api/internal/util"
	"gorm.io/gorm"
)

func TestParseJWTHS256RejectsOtherHMACAlgorithms(t *testing.T) {
	secret := []byte("test-secret-with-enough-entropy")
	claims := jwt.MapClaims{
		"sub": "user-1", "typ": "access", "jti": "token-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("sign HS512 token: %v", err)
	}

	if _, err := parseJWTHS256(token, secret); err == nil {
		t.Fatal("HS512 token signed with the same key must not be accepted as HS256")
	}
}

func TestParseJWTHS256AcceptsValidHS256(t *testing.T) {
	secret := []byte("test-secret-with-enough-entropy")
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1", "typ": "access", "jti": "token-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(secret)
	if err != nil {
		t.Fatalf("sign HS256 token: %v", err)
	}

	claims, err := parseJWTHS256(token, secret)
	if err != nil {
		t.Fatalf("valid HS256 token rejected: %v", err)
	}
	if claims["sub"] != "user-1" {
		t.Fatalf("unexpected subject: %v", claims["sub"])
	}
}

func TestParseJWTHS256RequiresExpiration(t *testing.T) {
	secret := []byte("test-secret-with-enough-entropy")
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1", "typ": "access", "jti": "token-1",
	}).SignedString(secret)
	if err != nil {
		t.Fatalf("sign token without expiration: %v", err)
	}
	if _, err := parseJWTHS256(token, secret); err == nil {
		t.Fatal("token without expiration must be rejected")
	}
}

func TestCeilTTLSecondsNeverExpiresBlacklistBeforeToken(t *testing.T) {
	tests := []struct {
		name string
		ttl  time.Duration
		want int64
	}{
		{name: "sub-second", ttl: time.Millisecond, want: 1},
		{name: "exact-second", ttl: time.Second, want: 1},
		{name: "fractional-second", ttl: time.Second + time.Millisecond, want: 2},
		{name: "multiple-seconds", ttl: 5 * time.Second, want: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ceilTTLSeconds(test.ttl); got != test.want {
				t.Fatalf("ceilTTLSeconds(%s) = %d, want %d", test.ttl, got, test.want)
			}
		})
	}
}

func TestChooseRoleRejectsPrivilegedSelfAssignment(t *testing.T) {
	service := &Service{}
	for _, role := range []string{
		constant.RoleSuperAdmin,
		constant.RoleAdmin,
		constant.RoleDoctor,
		constant.RoleNurse,
	} {
		if err := service.ChooseRole(context.Background(), "user-1", role); err != constant.ErrForbidden {
			t.Fatalf("role %s: expected forbidden, got %v", role, err)
		}
	}
}

func TestSetProfileRejectsDoctorSelfService(t *testing.T) {
	service := &Service{}
	profile := json.RawMessage(`{"first_name":"Doctor","last_name":"Self"}`)
	if _, err := service.SetProfile(context.Background(), "user-1", constant.RoleDoctor, &profile); err != constant.ErrForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestSetProfileRejectsUnknownNestedFields(t *testing.T) {
	service := &Service{}
	profile := json.RawMessage(`{"first_name":"Patient","last_name":"One","unexpected":true}`)
	if _, err := service.SetProfile(context.Background(), "user-1", constant.RolePatient, &profile); err != constant.NewUnknownFieldError("unexpected") {
		t.Fatalf("expected strict nested validation error, got %v", err)
	}
}

func TestPasswordWhitespaceIsSignificant(t *testing.T) {
	service := &Service{}
	password := " Strong!Pass9 "
	hash, err := service.hashPasswordScrypt(context.Background(), password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &entity.User{PasswordHash: hash}

	if err := service.verifyAndMigratePassword(context.Background(), user, password); err != nil {
		t.Fatalf("exact password rejected: %v", err)
	}
	if err := service.verifyAndMigratePassword(context.Background(), user, "Strong!Pass9"); err != constant.ErrPasswordNotMatch {
		t.Fatalf("trimmed password should not match, got %v", err)
	}
}

func TestLoginRateKeyIsScopedByCallerFingerprint(t *testing.T) {
	identity := "known@example.test"
	secret := []byte("test-secret-with-enough-entropy")
	ctxA := context.WithValue(context.Background(), constant.ClientFingerprint, "caller-a")
	ctxB := context.WithValue(context.Background(), constant.ClientFingerprint, "caller-b")
	keyA := loginRateKey(ctxA, identity, secret)
	keyB := loginRateKey(ctxB, identity, secret)
	if keyA == keyB {
		t.Fatalf("different callers share login limiter key %q", keyA)
	}
	if strings.Contains(keyA, identity) || strings.Contains(keyB, identity) {
		t.Fatal("login limiter key leaked the raw identity")
	}
}

func TestPasswordResetCurrentKeyIsPseudonymous(t *testing.T) {
	email := "sensitive.user@example.test"
	secret := []byte("test-secret-with-enough-entropy")
	key := passwordResetIdentityKey(email, secret)
	if strings.Contains(strings.ToLower(key), email) {
		t.Fatalf("password reset current key leaked raw email: %q", key)
	}
	if key != passwordResetIdentityKey("  SENSITIVE.USER@EXAMPLE.TEST ", secret) {
		t.Fatal("password reset identity key must use normalized identity semantics")
	}
	if key == passwordResetIdentityKey(email, []byte("another-secret")) {
		t.Fatal("password reset identity key must be keyed")
	}
}

func TestIdentityFingerprintIsKeyedAndDomainSeparated(t *testing.T) {
	identity := "Known@Example.Test"
	secretA := []byte("secret-a")
	secretB := []byte("secret-b")
	logFingerprint := identityFingerprint(secretA, "log-identity", identity)
	if logFingerprint == identityFingerprint(secretA, "login-rate", identity) {
		t.Fatal("fingerprints from separate domains must differ")
	}
	if logFingerprint == identityFingerprint(secretB, "log-identity", identity) {
		t.Fatal("fingerprints from separate secrets must differ")
	}
	if logFingerprint != identityFingerprint(secretA, "log-identity", "  known@example.test ") {
		t.Fatal("identity normalization must be stable")
	}
}

func TestPINHashIsPurposeAndChallengeBound(t *testing.T) {
	service := &Service{jwtSecret: []byte("test-secret")}
	registrationHash := service.pinHash("registration", "challenge-a", "123456")
	if secureEqual(registrationHash, service.pinHash("registration", "challenge-b", "123456")) {
		t.Fatal("same PIN must have a different hash for another challenge")
	}
	if secureEqual(registrationHash, service.pinHash("password-reset", "challenge-a", "123456")) {
		t.Fatal("same PIN must have a different hash for another purpose")
	}
}

func TestIsCanonicalRandomID(t *testing.T) {
	valid, err := randomID(32)
	if err != nil {
		t.Fatalf("generate canonical random ID: %v", err)
	}

	tests := []struct {
		name       string
		value      string
		byteLength int
		want       bool
	}{
		{name: "generated 32-byte ID", value: valid, byteLength: 32, want: true},
		{name: "empty", value: "", byteLength: 32, want: false},
		{name: "wrong decoded length", value: valid, byteLength: 31, want: false},
		{name: "padded encoding", value: valid + "=", byteLength: 32, want: false},
		{name: "surrounding whitespace", value: " " + valid, byteLength: 32, want: false},
		{name: "invalid alphabet", value: valid[:len(valid)-1] + "*", byteLength: 32, want: false},
		{name: "canonical one byte", value: "AQ", byteLength: 1, want: true},
		{name: "non-canonical trailing bits", value: "AR", byteLength: 1, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isCanonicalRandomID(test.value, test.byteLength); got != test.want {
				t.Fatalf("isCanonicalRandomID(%q, %d) = %t, want %t", test.value, test.byteLength, got, test.want)
			}
		})
	}
}

func TestPasswordResetCredentialBindingChangesWithCredentials(t *testing.T) {
	service := &Service{jwtSecret: []byte("test-secret-with-enough-entropy")}
	user := &entity.User{ID: "user-1", PasswordHash: "password-hash-a"}

	original := service.passwordResetCredentialBinding(user)
	if original == "" {
		t.Fatal("credential binding must not be empty for a user")
	}
	if repeated := service.passwordResetCredentialBinding(user); !secureEqual(original, repeated) {
		t.Fatal("credential binding must be deterministic for unchanged credentials")
	}

	user.PasswordHash = "password-hash-b"
	if changedPassword := service.passwordResetCredentialBinding(user); secureEqual(original, changedPassword) {
		t.Fatal("credential binding must change when PasswordHash changes")
	}

	user.ID = "user-2"
	user.PasswordHash = "password-hash-a"
	if changedUser := service.passwordResetCredentialBinding(user); secureEqual(original, changedUser) {
		t.Fatal("credential binding must be scoped to the user ID")
	}
	if got := service.passwordResetCredentialBinding(nil); got != "" {
		t.Fatalf("nil user credential binding = %q, want empty", got)
	}
}

func TestRefreshResultEncryptionIsBoundToOperationContext(t *testing.T) {
	service := &Service{jwtSecret: []byte("test-secret-with-enough-entropy")}
	want := refreshResultCache{
		Pair:      TokenPair{AccessToken: "access-token", RefreshToken: "refresh-token"},
		AccessExp: time.Now().Add(time.Hour).Unix(), RefreshExp: time.Now().Add(24 * time.Hour).Unix(),
	}
	encoded, err := service.encryptRefreshResult(want, "old-jti\x00operation-a\x00user-a\x00family-a\x00version-a")
	if err != nil {
		t.Fatalf("encrypt refresh result: %v", err)
	}
	got, err := service.decryptRefreshResult(encoded, "old-jti\x00operation-a\x00user-a\x00family-a\x00version-a")
	if err != nil {
		t.Fatalf("decrypt refresh result: %v", err)
	}
	if got != want {
		t.Fatalf("decrypted refresh result = %#v, want %#v", got, want)
	}
	if _, err := service.decryptRefreshResult(encoded, "old-jti\x00operation-a\x00user-b\x00family-a\x00version-a"); err == nil {
		t.Fatal("ciphertext must not decrypt under another user context")
	}
}

func TestRefreshRequiresCanonicalUUIDv4IdempotencyKey(t *testing.T) {
	service := &Service{}
	for _, key := range []string{"", "predictable-operation", "550e8400-e29b-11d4-a716-446655440000", "550E8400-E29B-41D4-A716-446655440000"} {
		if _, _, _, err := service.Refresh(context.Background(), "not-a-token", key); err != constant.ErrInvalidIdempotencyKey {
			t.Fatalf("idempotency key %q: error = %v, want validation error", key, err)
		}
	}
	if _, _, _, err := service.Refresh(context.Background(), "not-a-token", "550e8400-e29b-41d4-a716-446655440000"); err != constant.ErrInvalidToken {
		t.Fatalf("canonical UUIDv4 should pass idempotency validation, got %v", err)
	}
}

func TestRefreshRequestValidationRequiresUUIDv4(t *testing.T) {
	valid := request.RefreshTokenRequest{
		RefreshToken:   "refresh-token",
		IdempotencyKey: "550e8400-e29b-41d4-a716-446655440000",
	}
	if err := ulog.ValidateStruct(&valid); err != nil {
		t.Fatalf("valid refresh request rejected: %v", err)
	}
	valid.IdempotencyKey = "550e8400-e29b-11d4-a716-446655440000"
	if err := ulog.ValidateStruct(&valid); err == nil {
		t.Fatal("non-v4 idempotency UUID accepted")
	}
}

func TestRegistrationPersistenceErrorMapsUniqueConflict(t *testing.T) {
	err := fmt.Errorf("insert user: %w", gorm.ErrDuplicatedKey)
	if got := registrationPersistenceError(err); got != constant.ErrDuplicateUsernameOrEmail {
		t.Fatalf("expected duplicate registration error, got %v", got)
	}
}

func TestRegistrationPersistenceErrorKeepsUnexpectedFailureInternal(t *testing.T) {
	if got := registrationPersistenceError(errors.New("database unavailable")); got != constant.ErrInternalServerError {
		t.Fatalf("expected internal error, got %v", got)
	}
}

func TestBlockedRegistrationUserCannotBeReactivated(t *testing.T) {
	blocked := &entity.User{Status: "blocked"}
	if got := existingRegistrationUserError(blocked); got != constant.ErrDuplicateUsernameOrEmail {
		t.Fatalf("blocked account registration error = %v", got)
	}
	pending := &entity.User{Status: "pending"}
	if got := existingRegistrationUserError(pending); got != nil {
		t.Fatalf("pending account should remain eligible for verification, got %v", got)
	}
}

func TestPatientProfilePersistenceErrorMapsUniqueNIKRace(t *testing.T) {
	err := fmt.Errorf("update user: %w", gorm.ErrDuplicatedKey)
	if got := patientProfilePersistenceError(err); got != constant.ErrDuplicateNIK {
		t.Fatalf("expected duplicate NIK error, got %v", got)
	}
}

type blockingEmailSender struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingEmailSender) SendWithContext(context.Context, string, string, string) error {
	close(s.started)
	<-s.release
	return nil
}

func TestCloseEmailDispatcherHonorsDeadlineWhenSenderIgnoresContext(t *testing.T) {
	sender := &blockingEmailSender{started: make(chan struct{}), release: make(chan struct{})}
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	service := &Service{
		email:           sender,
		emailJobs:       make(chan emailJob, 1),
		emailWorkerCtx:  workerCtx,
		emailCancel:     cancelWorker,
		emailDone:       make(chan struct{}),
		emailSendBudget: time.Second,
	}
	go service.runEmailWorkers(1)
	service.emailJobs <- emailJob{to: "user@example.test"}
	<-sender.started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := service.CloseEmailDispatcher(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CloseEmailDispatcher() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("CloseEmailDispatcher blocked for %s after its deadline", elapsed)
	}
	close(sender.release)
	select {
	case <-service.emailDone:
	case <-time.After(time.Second):
		t.Fatal("email worker did not finish after sender release")
	}
}

func TestRegistrationUsernameRejectsAmbiguousLoginIdentities(t *testing.T) {
	for _, username := range []string{"valid.user", "valid_user", "valid-user", "user9"} {
		if !isValidRegistrationUsername(username) {
			t.Fatalf("expected username %q to be valid", username)
		}
	}
	for _, username := range []string{"   ", "ab", "user@example.com", "has space"} {
		if isValidRegistrationUsername(username) {
			t.Fatalf("expected username %q to be invalid", username)
		}
	}
}
