package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/scrypt"
	"gorm.io/gorm"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/email"
	"github.com/Cendana-Project/medikaone-api/internal/model/entity"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	hosprepo "github.com/Cendana-Project/medikaone-api/internal/repository/hospital"
	rolerepo "github.com/Cendana-Project/medikaone-api/internal/repository/role"
	userrepo "github.com/Cendana-Project/medikaone-api/internal/repository/user"
	ulog "github.com/Cendana-Project/medikaone-api/internal/util"
)

/* ==================== Types ==================== */

type EmailSender interface {
	SendWithContext(ctx context.Context, to, subject, htmlBody string) error
}

type emailJob struct {
	to                        string
	subject                   string
	htmlBody                  string
	resetPIN                  string
	recipientName             string
	identityHash              string
	deleteKeyOnFailure        string
	expectedValue             string
	replacementValueOnFailure string
	expiresAt                 time.Time
}

// TokenPair diexpose supaya metode exported tidak mengembalikan tipe unexported.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// RegistrationResult identifies a short-lived server-side registration
// challenge. No user row is created until the PIN for this challenge is valid.
type RegistrationResult struct {
	ChallengeID string `json:"challenge_id"`
	Email       string `json:"email"`
	Status      string `json:"status"`
}

type PasswordForgotResult struct {
	ChallengeID string `json:"challenge_id"`
	Status      string `json:"status"`
}

type registrationChallenge struct {
	Email        string `json:"email"`
	Username     string `json:"username"`
	Phone        string `json:"phone"`
	PasswordHash string `json:"password_hash"`
	PINHash      string `json:"pin_hash"`
}

type LoginHospitalResult struct {
	AccessToken  string               `json:"access_token"`
	RefreshToken string               `json:"refresh_token"`
	ExpiresIn    int64                `json:"expires_in"`
	TokenType    string               `json:"token_type"`
	HospitalID   string               `json:"hospital_id"`
	Roles        []response.RoleBrief `json:"roles"`
	AccessExp    time.Time            `json:"-"`
	RefreshExp   time.Time            `json:"-"`
}

type Service struct {
	users *userrepo.Repository
	roles *rolerepo.Repository
	redis *redis.Client
	hosp  *hosprepo.Repository
	email EmailSender

	loc               *time.Location
	pinTTL            time.Duration
	pinAttempts       int
	resendCooldown    time.Duration
	forgotRateLimit   int
	forgotRateWindow  time.Duration
	loginRateLimit    int
	loginRateWindow   time.Duration
	maxSessions       int
	accessTTL         time.Duration
	refreshTTL        time.Duration
	jwtSecret         []byte
	dummyPasswordHash string

	emailJobs       chan emailJob
	emailWorkerCtx  context.Context
	emailCancel     context.CancelFunc
	emailDone       chan struct{}
	emailSendBudget time.Duration
	emailMu         sync.RWMutex
	emailClosed     bool
}

func NewService(users *userrepo.Repository, roles *rolerepo.Repository, rdb *redis.Client, sender EmailSender, hosp *hosprepo.Repository) *Service {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("Asia/Jakarta", 7*60*60)
	}
	acc, ref := config.Env.JWT.ParseDurations()

	svc := &Service{
		users:             users,
		roles:             roles,
		redis:             rdb,
		email:             sender,
		hosp:              hosp,
		loc:               loc,
		pinTTL:            config.Env.Auth.PINTTL,
		pinAttempts:       config.Env.Auth.PINMaxAttempts,
		resendCooldown:    config.Env.Auth.PINResendCooldown,
		forgotRateLimit:   config.Env.Auth.ForgotPasswordRateLimit,
		forgotRateWindow:  config.Env.Auth.ForgotPasswordRateWindow,
		loginRateLimit:    config.Env.Auth.LoginRateLimit,
		loginRateWindow:   config.Env.Auth.LoginRateWindow,
		maxSessions:       config.Env.Auth.MaxActiveSessions,
		accessTTL:         acc,
		refreshTTL:        ref,
		jwtSecret:         []byte(config.Env.JWT.Secret),
		dummyPasswordHash: buildDummyPasswordHash(),
		emailSendBudget:   config.Env.SMTP.Timeout,
	}
	if svc.pinTTL <= 0 {
		svc.pinTTL = 10 * time.Minute
	}
	if svc.pinAttempts <= 0 {
		svc.pinAttempts = 5
	}
	if svc.resendCooldown <= 0 {
		svc.resendCooldown = time.Minute
	}
	if svc.forgotRateLimit <= 0 {
		svc.forgotRateLimit = 3
	}
	if svc.forgotRateWindow <= 0 {
		svc.forgotRateWindow = time.Hour
	}
	if svc.loginRateLimit <= 0 {
		svc.loginRateLimit = 10
	}
	if svc.loginRateWindow <= 0 {
		svc.loginRateWindow = 15 * time.Minute
	}
	if svc.maxSessions <= 0 {
		svc.maxSessions = 10
	}
	if svc.emailSendBudget <= 0 {
		svc.emailSendBudget = 30 * time.Second
	}
	if sender != nil {
		svc.emailWorkerCtx, svc.emailCancel = context.WithCancel(context.Background())
		// Two workers and a queue sized below half the default PIN lifetime keep
		// worst-case SMTP drain time bounded while the provider is slow.
		svc.emailJobs = make(chan emailJob, 32)
		svc.emailDone = make(chan struct{})
		go svc.runEmailWorkers(2)
	}
	return svc
}

/* ==================== Helpers ==================== */

func isNIK16(s string) bool {
	if len(s) != 16 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isValidRegistrationUsername(username string) bool {
	if len(username) < 3 || len(username) > 64 {
		return false
	}
	for _, char := range username {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') ||
			char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func randBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func randomID(n int) (string, error) {
	b, err := randBytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isCanonicalRandomID(value string, byteLength int) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == byteLength && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func sixDigitPIN() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return strconv6(int(value.Int64())), nil
}
func strconv6(n int) string {
	d := [6]byte{'0', '0', '0', '0', '0', '0'}
	for i := 5; i >= 0; i-- {
		d[i] = byte('0' + (n % 10))
		n /= 10
	}
	return string(d[:])
}

// ====== Key helpers (Redis) ======
func authRedisKey(suffix string) string    { return config.AuthRedisKeyPrefix() + suffix }
func keyRefresh(jti string) string         { return authRedisKey("refresh:" + jti) }
func keyUserRefreshSet(uid string) string  { return authRedisKey("user:refreshes:v2:" + uid) }
func keyAccessBlacklist(jti string) string { return authRedisKey("access:blacklist:" + jti) }
func keyAccessFamilyRevoked(uid, familyID string) string {
	return authRedisKey("access:family-revoked:" + uid + ":" + familyID)
}
func keySessionVersion(uid string) string { return authRedisKey("user:session-version:" + uid) }
func keyRegistration(id string) string    { return authRedisKey("registration:challenge:" + id) }
func keyRegistrationEmail(email string) string {
	return authRedisKey("registration:email:" + email)
}
func keyRegistrationUsername(username string) string {
	return authRedisKey("registration:username:" + username)
}
func keyRegistrationAttempts(id string) string { return authRedisKey("registration:attempts:" + id) }
func keyRegistrationAttemptOperation(id, operationID string) string {
	return authRedisKey("registration:attempt-op:" + id + ":" + operationID)
}
func keyRegistrationSendCooldown(email string) string {
	return authRedisKey("registration:send-cooldown:" + email)
}
func keyPasswordReset(challengeID string) string {
	return authRedisKey("password-reset:challenge:" + challengeID)
}
func keyPasswordResetCurrent(identityFingerprint string) string {
	return authRedisKey("password-reset:current:" + identityFingerprint)
}
func keyPasswordAttempts(challengeID string) string {
	return authRedisKey("password-reset:attempts:" + challengeID)
}
func keyPasswordResetProof(challengeID string) string {
	return authRedisKey("password-reset:proof:" + challengeID)
}
func keyPasswordResetAttemptOperation(challengeID, operationID string) string {
	return authRedisKey("password-reset:attempt-op:" + challengeID + ":" + operationID)
}
func keyPasswordResetConsumeOperation(challengeID, operationID string) string {
	return authRedisKey("password-reset:consume-op:" + challengeID + ":" + operationID)
}
func keyForgotRate(scope string) string   { return authRedisKey("password-forgot:rate:" + scope) }
func keyLoginRate(identity string) string { return authRedisKey("auth:login:rate:" + identity) }
func keyRefreshUsed(jti string) string    { return authRedisKey("refresh:used:" + jti) }
func keyRefreshResult(jti, operationFingerprint string) string {
	return authRedisKey("refresh:result:" + jti + ":" + operationFingerprint)
}

func loginRateKey(ctx context.Context, identity string, secret []byte) string {
	clientFingerprint, _ := ctx.Value(constant.ClientFingerprint).(string)
	clientFingerprint = strings.TrimSpace(clientFingerprint)
	if clientFingerprint == "" {
		// Non-HTTP callers still receive a bounded counter. Every public HTTP
		// route sets a caller fingerprint before invoking the service.
		clientFingerprint = "non-http"
	}
	return keyLoginRate(identityFingerprint(secret, "login-rate", clientFingerprint+"\x00"+identity))
}

func forgotRateKey(ctx context.Context, email string, secret []byte) string {
	clientFingerprint, _ := ctx.Value(constant.ClientFingerprint).(string)
	clientFingerprint = strings.TrimSpace(clientFingerprint)
	if clientFingerprint == "" {
		clientFingerprint = "non-http"
	}
	return keyForgotRate(identityFingerprint(secret, "forgot-rate", clientFingerprint+"\x00"+email))
}

func passwordResetIdentityKey(email string, secret []byte) string {
	return keyPasswordResetCurrent(identityFingerprint(secret, "password-reset-current", email))
}

func refreshRecord(userID, familyID, sessionVersion string) string {
	return userID + "|" + familyID + "|" + sessionVersion
}

func passwordResetRecord(userID, pinHash string) string {
	return userID + "|" + pinHash
}

func passwordResetProofRecord(userID, credentialBinding, proofHash string) string {
	return userID + "|" + credentialBinding + "|" + proofHash
}

func parsePasswordResetProofRecord(value string) (string, string, string, bool) {
	parts := strings.SplitN(value, "|", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[0] == "unavailable" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func (s *Service) passwordResetCredentialBinding(user *entity.User) string {
	if user == nil {
		return ""
	}
	return s.pinHash("password-reset-credential", user.ID, user.PasswordHash)
}

type refreshResultCache struct {
	Pair       TokenPair `json:"pair"`
	AccessExp  int64     `json:"access_exp"`
	RefreshExp int64     `json:"refresh_exp"`
}

func (s *Service) encryptRefreshResult(result refreshResultCache, associatedData string) (string, error) {
	plaintext, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256(append(append([]byte{}, s.jwtSecret...), []byte("\x00refresh-result")...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce, err := randBytes(gcm.NonceSize())
	if err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, []byte(associatedData))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (s *Service) decryptRefreshResult(encoded, associatedData string) (refreshResultCache, error) {
	var result refreshResultCache
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return result, err
	}
	key := sha256.Sum256(append(append([]byte{}, s.jwtSecret...), []byte("\x00refresh-result")...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return result, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < gcm.NonceSize() {
		return result, errors.New("invalid refresh result cache")
	}
	plaintext, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte(associatedData))
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(plaintext, &result); err != nil {
		return result, err
	}
	return result, nil
}

var (
	storeRegistrationScript = redis.NewScript(`
local old_email = redis.call('GET', KEYS[2])
local old_username = redis.call('GET', KEYS[3])
if old_username and old_username ~= ARGV[1] and old_username ~= old_email then
  return 0
end
if old_email and old_email ~= ARGV[1] then
  redis.call('DEL', ARGV[4] .. old_email, ARGV[5] .. old_email)
end
redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
redis.call('SET', KEYS[2], ARGV[1], 'EX', ARGV[3])
redis.call('SET', KEYS[3], ARGV[1], 'EX', ARGV[3])
return 1`)
	storeRefreshScript = redis.NewScript(`
local current = redis.call('GET', KEYS[3])
if not current or current ~= ARGV[1] then
  return 0
end
if redis.call('GET', KEYS[1]) == ARGV[2] then
  return 1
end
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', ARGV[5])
local max_sessions = tonumber(ARGV[7])
if not max_sessions or max_sessions < 1 then
  return -1
end
local excess = redis.call('ZCARD', KEYS[2]) - max_sessions + 1
if excess > 0 then
  local oldest = redis.call('ZRANGE', KEYS[2], 0, excess - 1)
  for _, jti in ipairs(oldest) do
    redis.call('DEL', ARGV[8] .. jti)
    redis.call('ZREM', KEYS[2], jti)
  end
end
redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
redis.call('ZADD', KEYS[2], ARGV[6], ARGV[4])
redis.call('EXPIRE', KEYS[2], ARGV[3])
return 1`)
	rotateRefreshScript = redis.NewScript(`
local current_version = redis.call('GET', KEYS[4])
if not current_version or current_version ~= ARGV[4] then
  return -3
end
local maximum = tonumber(ARGV[12])
if not maximum or maximum < 1 then
  return -4
end
local current = redis.call('GET', KEYS[1])
if current and current == ARGV[1] then
  redis.call('DEL', KEYS[1])
  redis.call('ZREM', KEYS[2], ARGV[2])
  redis.call('SET', KEYS[3], ARGV[13], 'EX', ARGV[3])

  redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', ARGV[10])
  local excess = redis.call('ZCARD', KEYS[2]) - maximum + 1
  if excess > 0 then
    local oldest = redis.call('ZRANGE', KEYS[2], 0, excess - 1)
    for _, jti in ipairs(oldest) do
      redis.call('DEL', ARGV[6] .. jti)
      redis.call('ZREM', KEYS[2], jti)
    end
  end
	  redis.call('SET', KEYS[5], ARGV[7], 'EX', ARGV[8])
	  redis.call('ZADD', KEYS[2], ARGV[11], ARGV[9])
	  redis.call('EXPIRE', KEYS[2], ARGV[8])
	  redis.call('SET', KEYS[6], ARGV[14], 'EX', ARGV[15])
	  return 1
end
local tombstone = redis.call('GET', KEYS[3])
if tombstone == ARGV[13] and redis.call('EXISTS', KEYS[6]) == 1 then
  return 2
end
if tombstone and string.sub(tombstone, 1, string.len(ARGV[1]) + 1) == ARGV[1] .. '|' then
  redis.call('SET', KEYS[4], ARGV[5])
  local members = redis.call('ZRANGE', KEYS[2], 0, -1)
  for _, jti in ipairs(members) do
    redis.call('DEL', ARGV[6] .. jti)
  end
  redis.call('DEL', KEYS[2])
  return -2
end
return 0`)
	revokeRefreshesScript = redis.NewScript(`
redis.call('SET', KEYS[2], ARGV[2])
local members = redis.call('ZRANGE', KEYS[1], 0, -1)
for _, jti in ipairs(members) do
  redis.call('DEL', ARGV[1] .. jti)
end
redis.call('DEL', KEYS[1])
return #members`)
	logoutFamilyScript = redis.NewScript(`
redis.call('SET', KEYS[1], '1', 'EX', ARGV[1])
redis.call('SET', KEYS[3], '1', 'EX', ARGV[2])
local members = redis.call('ZRANGE', KEYS[2], 0, -1)
local revoked = 0
for _, jti in ipairs(members) do
	local key = ARGV[3] .. jti
	if redis.call('GET', key) == ARGV[4] then
		redis.call('DEL', key)
    redis.call('ZREM', KEYS[2], jti)
    revoked = revoked + 1
  end
end
if redis.call('ZCARD', KEYS[2]) == 0 then
  redis.call('DEL', KEYS[2])
end
return revoked`)
	consumeRegistrationScript = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if not value or value ~= ARGV[1] then
  return 0
end
redis.call('DEL', KEYS[1], KEYS[4], KEYS[5])
if redis.call('GET', KEYS[2]) == ARGV[2] then redis.call('DEL', KEYS[2]) end
if redis.call('GET', KEYS[3]) == ARGV[2] then redis.call('DEL', KEYS[3]) end
return 1`)
	verifyRegistrationPINScript = redis.NewScript(`
local cached = redis.call('GET', KEYS[6])
if cached then
  return tonumber(cached)
end
local function finish(result)
  redis.call('SET', KEYS[6], tostring(result), 'EX', 60)
  return result
end
local raw = redis.call('GET', KEYS[1])
if not raw or raw ~= ARGV[1] then
  return finish(0)
end
local attempts = tonumber(redis.call('GET', KEYS[4])) or 0
local maximum = tonumber(ARGV[4])
local function delete_challenge()
  redis.call('DEL', KEYS[1], KEYS[4], KEYS[5])
  if redis.call('GET', KEYS[2]) == ARGV[2] then redis.call('DEL', KEYS[2]) end
  if redis.call('GET', KEYS[3]) == ARGV[2] then redis.call('DEL', KEYS[3]) end
end
if attempts >= maximum then
  delete_challenge()
  return finish(-2)
end
if ARGV[3] ~= '1' then
  attempts = redis.call('INCR', KEYS[4])
  if attempts == 1 then
    local ttl = redis.call('PTTL', KEYS[1])
    if ttl > 0 then redis.call('PEXPIRE', KEYS[4], ttl) end
  end
  if attempts >= maximum then
    delete_challenge()
    return finish(-2)
  end
  return finish(-1)
end
delete_challenge()
return finish(1)`)
	incrementWithTTLScript = redis.NewScript(`
local value = redis.call('INCR', KEYS[1])
if value == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end
return value`)
	storePasswordResetScript = redis.NewScript(`
local previous = redis.call('GET', KEYS[2])
if previous then
  redis.call('DEL', ARGV[4] .. previous, ARGV[5] .. previous, ARGV[6] .. previous)
end
redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
redis.call('SET', KEYS[2], ARGV[1], 'EX', ARGV[3])
return 1`)
	invalidateCurrentPasswordResetScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then
  return 0
end
redis.call('DEL', ARGV[1] .. current, ARGV[2] .. current, ARGV[3] .. current, KEYS[1])
return 1`)
	verifyPasswordResetPINAndMintProofScript = redis.NewScript(`
local cached = redis.call('GET', KEYS[3])
if cached then
  local result = tonumber(cached)
  if result <= 0 then
    return result
  end
  if redis.call('GET', KEYS[4]) ~= ARGV[3] or redis.call('GET', KEYS[5]) ~= ARGV[4] then
    return 0
  end
  local proof_ttl = redis.call('PTTL', KEYS[5])
  if proof_ttl <= 0 then
    return 0
  end
  return proof_ttl
end
local function finish(result)
  redis.call('SET', KEYS[3], tostring(result), 'EX', 60)
  return result
end
local current = redis.call('GET', KEYS[4])
if not current or current ~= ARGV[3] then
  return finish(0)
end
local value = redis.call('GET', KEYS[1])
if not value then
  return finish(0)
end
local attempts = tonumber(redis.call('GET', KEYS[2])) or 0
local maximum = tonumber(ARGV[2])
if attempts >= maximum then
  redis.call('DEL', KEYS[1], KEYS[2], KEYS[4], KEYS[5])
  return finish(-2)
end
if value == ARGV[1] then
  local challenge_ttl = redis.call('PTTL', KEYS[1])
  local proof_ttl = tonumber(ARGV[5])
  if challenge_ttl <= 0 or not proof_ttl or proof_ttl <= 0 then
    return finish(0)
  end
  local stored = redis.call('SET', KEYS[5], ARGV[4], 'PX', proof_ttl, 'NX')
  if not stored then
    return finish(0)
  end
  redis.call('DEL', KEYS[1], KEYS[2])
  redis.call('PEXPIRE', KEYS[4], proof_ttl)
  local operation_ttl = math.min(proof_ttl, 60000)
  redis.call('SET', KEYS[3], tostring(proof_ttl), 'PX', operation_ttl)
  return proof_ttl
end
attempts = redis.call('INCR', KEYS[2])
if attempts == 1 then
  local ttl = redis.call('PTTL', KEYS[1])
  if ttl > 0 then redis.call('PEXPIRE', KEYS[2], ttl) end
end
if attempts >= maximum then
  redis.call('DEL', KEYS[1], KEYS[2], KEYS[4], KEYS[5])
  return finish(-2)
end
return finish(-1)`)
	deleteIfValueScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`)
	replaceIfValueScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then
  return 0
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl <= 0 then
  return 0
end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ttl)
return ttl`)
	secretTTLIfValueScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then
  return -2
end
return redis.call('PTTL', KEYS[1])`)
	consumePasswordResetProofAndRevokeScript = redis.NewScript(`
local cached = redis.call('GET', KEYS[4])
if cached then
  return tonumber(cached)
end
local current = redis.call('GET', KEYS[5])
if not current or current ~= ARGV[4] then
  return 0
end
local value = redis.call('GET', KEYS[1])
if not value or value ~= ARGV[1] then
  return 0
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl <= 0 then
  return 0
end
redis.call('DEL', KEYS[1], KEYS[5])
redis.call('SET', KEYS[3], ARGV[2])
local members = redis.call('ZRANGE', KEYS[2], 0, -1)
for _, jti in ipairs(members) do
  redis.call('DEL', ARGV[3] .. jti)
end
redis.call('DEL', KEYS[2])
redis.call('SET', KEYS[4], tostring(ttl), 'PX', ttl)
return ttl`)
	restorePasswordResetProofIfMissingScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 or redis.call('EXISTS', KEYS[2]) == 1 then
  return 0
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2], 'NX')
redis.call('SET', KEYS[2], ARGV[3], 'PX', ARGV[2], 'NX')
return 1`)
)

func (s *Service) pinHash(purpose, id, pin string) string {
	mac := hmac.New(sha256.New, s.jwtSecret)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(id))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(pin))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func secureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func registrationPersistenceError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return constant.ErrDuplicateUsernameOrEmail
	}
	return constant.ErrInternalServerError
}

func existingRegistrationUserError(user *entity.User) error {
	if user == nil || strings.EqualFold(user.Status, "pending") {
		return nil
	}
	if strings.EqualFold(user.Status, "active") {
		return constant.ErrEmailAlreadyActive
	}
	return constant.ErrDuplicateUsernameOrEmail
}

func patientProfilePersistenceError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return constant.ErrDuplicateNIK
	}
	return constant.ErrInternalServerError
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func buildDummyPasswordHash() string {
	salt := []byte("medikaone-dummy!")
	key, err := scrypt.Key([]byte("not-a-real-password"), salt, 1<<15, 8, 1, 64)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(key) + ":" + base64.StdEncoding.EncodeToString(salt)
}

func (s *Service) runDummyPasswordVerification(ctx context.Context, password string) error {
	hash := s.dummyPasswordHash
	if hash == "" {
		hash = buildDummyPasswordHash()
	}
	return s.verifyAndMigratePassword(ctx, &entity.User{PasswordHash: hash}, password)
}

func (s *Service) runEmailWorkers(count int) {
	var workers sync.WaitGroup
	workers.Add(count)
	for range count {
		go func() {
			defer workers.Done()
			s.runEmailWorker()
		}()
	}
	workers.Wait()
	close(s.emailDone)
}

func (s *Service) runEmailWorker() {
	for job := range s.emailJobs {
		remaining, current := s.currentEmailJobTTL(job)
		if !current {
			continue
		}
		body := job.htmlBody
		if job.resetPIN != "" {
			validMinutes := max(1, int(math.Ceil(remaining.Minutes())))
			body = email.RenderResetPIN(job.recipientName, job.resetPIN, validMinutes)
		}
		if err := s.email.SendWithContext(s.emailWorkerCtx, job.to, job.subject, body); err != nil {
			ulog.Errorf(context.Background(), "async smtp send failed identity_hash=%s error_type=%T", job.identityHash, err)
			if job.deleteKeyOnFailure != "" {
				s.invalidateQueuedSecret(job)
			}
		}
	}
}

func (s *Service) currentEmailJobTTL(job emailJob) (time.Duration, bool) {
	localRemaining := s.pinTTL
	if localRemaining <= 0 {
		localRemaining = s.emailSendBudget + 2*time.Second
	}
	if !job.expiresAt.IsZero() && !time.Now().Before(job.expiresAt) {
		s.deleteQueuedSecret(job)
		return 0, false
	} else if !job.expiresAt.IsZero() {
		localRemaining = time.Until(job.expiresAt)
	}
	if job.deleteKeyOnFailure == "" {
		return localRemaining, true
	}
	if s.redis == nil {
		return 0, false
	}
	remainingMS, err := secretTTLIfValueScript.Run(
		s.emailWorkerCtx,
		s.redis,
		[]string{job.deleteKeyOnFailure},
		job.expectedValue,
	).Int64()
	if err != nil {
		ulog.Errorf(context.Background(), "queued email freshness check failed identity_hash=%s error_type=%T", job.identityHash, err)
		return 0, false
	}
	redisRemaining := time.Duration(remainingMS) * time.Millisecond
	if redisRemaining < localRemaining {
		localRemaining = redisRemaining
	}
	minimumRemaining := max(s.emailSendBudget+time.Second, s.pinTTL/2)
	if remainingMS <= 0 || localRemaining <= minimumRemaining {
		s.deleteQueuedSecret(job)
		return 0, false
	}
	return localRemaining, true
}

func (s *Service) deleteQueuedSecret(job emailJob) {
	if job.deleteKeyOnFailure == "" || s.redis == nil {
		return
	}
	s.invalidateQueuedSecret(job)
}

func (s *Service) invalidateQueuedSecret(job emailJob) {
	if job.replacementValueOnFailure != "" {
		s.replaceSecretBestEffort(
			s.emailWorkerCtx,
			job.deleteKeyOnFailure,
			job.expectedValue,
			job.replacementValueOnFailure,
		)
		return
	}
	s.deleteSecretBestEffort(s.emailWorkerCtx, job.deleteKeyOnFailure, job.expectedValue)
}

func cleanupContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
}

func (s *Service) deleteRedisKeysBestEffort(ctx context.Context, keys ...string) {
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	_ = s.redis.Del(cleanupCtx, keys...).Err()
}

func (s *Service) deleteSecretBestEffort(ctx context.Context, key, expected string) {
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	_ = deleteIfValueScript.Run(cleanupCtx, s.redis, []string{key}, expected).Err()
}

func (s *Service) replaceSecretBestEffort(ctx context.Context, key, expected, replacement string) {
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	if err := replaceIfValueScript.Run(cleanupCtx, s.redis, []string{key}, expected, replacement).Err(); err != nil {
		ulog.Errorf(context.Background(), "secret replacement failed error_type=%T", err)
	}
}

func (s *Service) restorePasswordResetProof(ctx context.Context, proofKey, currentKey, challengeID, expected string, ttlMS int64) {
	if ttlMS <= 0 {
		return
	}
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	if err := restorePasswordResetProofIfMissingScript.Run(
		cleanupCtx,
		s.redis,
		[]string{proofKey, currentKey},
		expected,
		ttlMS,
		challengeID,
	).Err(); err != nil {
		ulog.Errorf(context.Background(), "password reset compensation failed error_type=%T", err)
	}
}

func (s *Service) enqueueEmail(job emailJob) bool {
	if s.emailJobs == nil {
		return false
	}
	s.emailMu.RLock()
	defer s.emailMu.RUnlock()
	if s.emailClosed {
		return false
	}
	select {
	case s.emailJobs <- job:
		return true
	default:
		return false
	}
}

// CloseEmailDispatcher drains queued messages during graceful shutdown. If the
// shutdown deadline expires, the active SMTP connection is canceled.
func (s *Service) CloseEmailDispatcher(ctx context.Context) error {
	if s.emailJobs == nil {
		return nil
	}
	s.emailMu.Lock()
	if !s.emailClosed {
		s.emailClosed = true
		close(s.emailJobs)
	}
	s.emailMu.Unlock()

	select {
	case <-s.emailDone:
		if s.emailCancel != nil {
			s.emailCancel()
		}
		return nil
	case <-ctx.Done():
		if s.emailCancel != nil {
			s.emailCancel()
		}
		return ctx.Err()
	}
}

func identityFingerprint(secret []byte, purpose, identity string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strings.ToLower(strings.TrimSpace(identity))))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:12])
}

func (s *Service) revokeAllRefresh(ctx context.Context, userID string) error {
	newVersion, err := randomID(18)
	if err != nil {
		return err
	}
	_, err = revokeRefreshesScript.Run(ctx, s.redis,
		[]string{keyUserRefreshSet(userID), keySessionVersion(userID)}, keyRefresh(""), newVersion,
	).Result()
	return err
}

func (s *Service) incrementWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	ttlSeconds := ceilTTLSeconds(ttl)
	return incrementWithTTLScript.Run(ctx, s.redis, []string{key}, ttlSeconds).Int64()
}

// ====== JWT helpers ======
func parseJWTHS256(tokenStr string, secret []byte) (jwt.MapClaims, error) {
	if len(tokenStr) == 0 || len(tokenStr) > 4096 {
		return nil, constant.ErrInvalidToken
	}
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, jwt.ErrSignatureInvalid
		}
		return secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !tok.Valid {
		return nil, constant.ErrInvalidToken
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, constant.ErrInvalidToken
	}
	return claims, nil
}

// Konversi aman ke int64 dari berbagai tipe (float64, string, json.Number, int, dll).
func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
	case string:
		if t == "" {
			return 0
		}
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

/* ==================== JWT Issue & Rotate ==================== */

func (s *Service) sessionVersion(ctx context.Context, userID string) (string, error) {
	version, err := s.redis.Get(ctx, keySessionVersion(userID)).Result()
	if err != redis.Nil {
		return version, err
	}
	candidate, err := randomID(18)
	if err != nil {
		return "", err
	}
	if err := s.redis.SetNX(ctx, keySessionVersion(userID), candidate, 0).Err(); err != nil {
		return "", err
	}
	return s.redis.Get(ctx, keySessionVersion(userID)).Result()
}

func (s *Service) issueJWT(ctx context.Context, userID string) (TokenPair, string, time.Time, time.Time, error) {
	version, err := s.sessionVersion(ctx, userID)
	if err != nil {
		return TokenPair{}, "", time.Time{}, time.Time{}, err
	}
	return s.issueJWTAtVersion(ctx, userID, version)
}

func (s *Service) issueJWTAtVersion(ctx context.Context, userID string, sessionVersion string) (TokenPair, string, time.Time, time.Time, error) {
	familyID, err := randomID(16)
	if err != nil {
		return TokenPair{}, "", time.Time{}, time.Time{}, err
	}
	return s.issueJWTAtVersionForFamily(ctx, userID, sessionVersion, familyID, true)
}

func (s *Service) issueJWTAtVersionForFamily(ctx context.Context, userID, sessionVersion, familyID string, store bool) (TokenPair, string, time.Time, time.Time, error) {
	now := time.Now().In(s.loc)
	accessExp := now.Add(s.accessTTL)
	refreshExp := now.Add(s.refreshTTL)

	accessJTI, err := randomID(16)
	if err != nil {
		return TokenPair{}, "", time.Time{}, time.Time{}, err
	}
	acc := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID, "typ": "access", "jti": accessJTI, "sv": sessionVersion, "fid": familyID,
		"iat": now.Unix(), "exp": accessExp.Unix(),
	})
	at, err := acc.SignedString(s.jwtSecret)
	if err != nil {
		return TokenPair{}, "", time.Time{}, time.Time{}, err
	}

	refreshJTI, err := randomID(16)
	if err != nil {
		return TokenPair{}, "", time.Time{}, time.Time{}, err
	}
	ref := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID, "typ": "refresh", "jti": refreshJTI, "sv": sessionVersion, "fid": familyID,
		"iat": now.Unix(), "exp": refreshExp.Unix(),
	})
	rt, err := ref.SignedString(s.jwtSecret)
	if err != nil {
		return TokenPair{}, "", time.Time{}, time.Time{}, err
	}
	if !store {
		return TokenPair{AccessToken: at, RefreshToken: rt}, refreshJTI, accessExp.UTC(), refreshExp.UTC(), nil
	}

	ttlSeconds := ceilTTLSeconds(s.refreshTTL)
	stored, err := storeRefreshScript.Run(ctx, s.redis,
		[]string{keyRefresh(refreshJTI), keyUserRefreshSet(userID), keySessionVersion(userID)},
		sessionVersion, refreshRecord(userID, familyID, sessionVersion), ttlSeconds, refreshJTI, now.Unix(), refreshExp.Unix(), s.maxSessions, keyRefresh(""),
	).Int()
	if err != nil || stored != 1 {
		if err == nil {
			err = constant.ErrInvalidToken
		}
		return TokenPair{}, "", time.Time{}, time.Time{}, err
	}

	return TokenPair{AccessToken: at, RefreshToken: rt}, refreshJTI, accessExp.UTC(), refreshExp.UTC(), nil
}

/* ==================== Core Flows ==================== */

func (s *Service) RegisterLite(ctx context.Context, req *request.RegisterLiteRequest) (*RegistrationResult, error) {
	emailAddr := strings.ToLower(strings.TrimSpace(req.Email))
	username := strings.ToLower(strings.TrimSpace(req.Username))
	phone := strings.TrimSpace(req.Phone)
	ulog.Infof(ctx, "register attempt identity_hash=%s", identityFingerprint(s.jwtSecret, "log-identity", emailAddr))
	if emailAddr == "" {
		return nil, constant.ErrInvalidEmail
	}
	if !isValidRegistrationUsername(username) {
		return nil, constant.ErrInvalidUsername
	}
	if len(phone) < 8 || len(phone) > 32 {
		return nil, constant.NewInvalidFieldLengthError("phone", "between 8 and 32 characters long", "memiliki 8 sampai 32 karakter")
	}

	if !ulog.IsValidPassword(req.Password) {
		return nil, constant.ErrInvalidPassword
	}
	if ulog.IsPasswordSimilarToUserInfo(req.Password, username, emailAddr) {
		return nil, constant.ErrPasswordSimilarToUserInfo
	}

	existing, err := s.users.FindByEmail(ctx, emailAddr)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if statusErr := existingRegistrationUserError(existing); statusErr != nil {
		return nil, statusErr
	}

	usernameExists, err := s.users.ExistsUsername(ctx, username)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	ownedByPendingEmail := existing != nil && existing.Username != nil &&
		strings.EqualFold(*existing.Username, username) && strings.EqualFold(existing.Status, "pending")
	if usernameExists && !ownedByPendingEmail {
		return nil, constant.ErrUsernameAlreadyExists
	}
	pendingUsernameChallenge, err := s.redis.Get(ctx, keyRegistrationUsername(username)).Result()
	if err != nil && err != redis.Nil {
		return nil, constant.ErrInternalServerError
	}
	if err == nil {
		pendingEmailChallenge, emailErr := s.redis.Get(ctx, keyRegistrationEmail(emailAddr)).Result()
		if emailErr != nil && emailErr != redis.Nil {
			return nil, constant.ErrInternalServerError
		}
		if emailErr == redis.Nil || pendingEmailChallenge != pendingUsernameChallenge {
			return nil, constant.ErrUsernameRegistrationInProgress
		}
	}
	allowed, err := s.redis.SetNX(ctx, keyRegistrationSendCooldown(emailAddr), "1", s.resendCooldown).Result()
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if !allowed {
		return nil, constant.ErrRegistrationPINCooldown
	}
	keepCooldown := false
	defer func() {
		if !keepCooldown {
			s.deleteRedisKeysBestEffort(ctx, keyRegistrationSendCooldown(emailAddr))
		}
	}()

	passwordHash, err := s.hashPasswordScrypt(ctx, req.Password)
	if err != nil {
		if errors.Is(err, constant.ErrPasswordProcessingBusy) {
			return nil, constant.ErrPasswordProcessingBusy
		}
		return nil, constant.ErrInternalServerError
	}
	challengeID, err := randomID(24)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	pin, err := sixDigitPIN()
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	challenge := registrationChallenge{
		Email:        emailAddr,
		Username:     username,
		Phone:        phone,
		PasswordHash: passwordHash,
		PINHash:      s.pinHash("registration", challengeID, pin),
	}
	raw, err := json.Marshal(challenge)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	ttlSeconds := ceilTTLSeconds(s.pinTTL)
	stored, err := storeRegistrationScript.Run(ctx, s.redis,
		[]string{keyRegistration(challengeID), keyRegistrationEmail(emailAddr), keyRegistrationUsername(username)},
		challengeID, raw, ttlSeconds, keyRegistration(""), keyRegistrationAttempts(""),
	).Int()
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if stored != 1 {
		return nil, constant.ErrDuplicateUsernameOrEmail
	}
	if s.email != nil {
		html := email.RenderVerifyPIN("", pin, int(s.pinTTL.Minutes()))
		if err := s.email.SendWithContext(ctx, emailAddr, "PIN Verifikasi Akun MedikaOne", html); err != nil {
			ulog.Errorf(ctx, "smtp send failed stage=register identity_hash=%s error_type=%T", identityFingerprint(s.jwtSecret, "log-identity", emailAddr), err)
			s.deleteRegistrationChallenge(ctx, challengeID, &challenge, string(raw))
			if errors.Is(err, email.ErrDeliveryBusy) {
				return nil, constant.ErrEmailDeliveryBusy
			}
			return nil, constant.ErrEmailSendFailed
		}
	}
	keepCooldown = true
	ulog.Infof(ctx, "registration challenge created challenge_id=%s", challengeID[:12])
	return &RegistrationResult{ChallengeID: challengeID, Email: emailAddr, Status: "pending"}, nil
}

func (s *Service) loadRegistrationChallenge(ctx context.Context, emailAddr, challengeID string) (string, registrationChallenge, string, error) {
	emailAddr = strings.ToLower(strings.TrimSpace(emailAddr))
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return "", registrationChallenge{}, "", constant.ErrRegistrationPINInvalidOrExpired
	}
	raw, err := s.redis.Get(ctx, keyRegistration(challengeID)).Result()
	if err == redis.Nil {
		return "", registrationChallenge{}, "", constant.ErrRegistrationPINInvalidOrExpired
	}
	if err != nil {
		return "", registrationChallenge{}, "", constant.ErrInternalServerError
	}
	var challenge registrationChallenge
	if json.Unmarshal([]byte(raw), &challenge) != nil || !strings.EqualFold(challenge.Email, emailAddr) {
		return "", registrationChallenge{}, "", constant.ErrRegistrationPINInvalidOrExpired
	}
	return challengeID, challenge, raw, nil
}

func (s *Service) deleteRegistrationChallenge(ctx context.Context, id string, challenge *registrationChallenge, raw string) {
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	_, _ = consumeRegistrationScript.Run(cleanupCtx, s.redis,
		[]string{keyRegistration(id), keyRegistrationEmail(challenge.Email), keyRegistrationUsername(challenge.Username), keyRegistrationAttempts(id), keyRegistrationSendCooldown(challenge.Email)},
		raw, id,
	).Result()
}

func (s *Service) restoreRegistrationChallenge(ctx context.Context, id string, challenge *registrationChallenge, raw string) {
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	ttlSeconds := ceilTTLSeconds(s.pinTTL)
	_, _ = storeRegistrationScript.Run(cleanupCtx, s.redis,
		[]string{keyRegistration(id), keyRegistrationEmail(challenge.Email), keyRegistrationUsername(challenge.Username)},
		id, raw, ttlSeconds, keyRegistration(""), keyRegistrationAttempts(""),
	).Result()
}

func (s *Service) ResendPIN(ctx context.Context, emailAddr, challengeID string) error {
	id, challenge, oldRaw, err := s.loadRegistrationChallenge(ctx, emailAddr, challengeID)
	if err != nil {
		return err
	}

	allowed, err := s.redis.SetNX(ctx, keyRegistrationSendCooldown(challenge.Email), "1", s.resendCooldown).Result()
	if err != nil {
		return constant.ErrInternalServerError
	}
	if !allowed {
		return constant.ErrRegistrationPINCooldown
	}

	pin, err := sixDigitPIN()
	if err != nil {
		s.deleteRedisKeysBestEffort(ctx, keyRegistrationSendCooldown(challenge.Email))
		return constant.ErrInternalServerError
	}
	challenge.PINHash = s.pinHash("registration", id, pin)
	newRaw, err := json.Marshal(challenge)
	if err != nil {
		s.deleteRedisKeysBestEffort(ctx, keyRegistrationSendCooldown(challenge.Email))
		return constant.ErrInternalServerError
	}
	ttlSeconds := ceilTTLSeconds(s.pinTTL)
	if _, err := storeRegistrationScript.Run(ctx, s.redis,
		[]string{keyRegistration(id), keyRegistrationEmail(challenge.Email), keyRegistrationUsername(challenge.Username)},
		id, newRaw, ttlSeconds, keyRegistration(""), keyRegistrationAttempts(""),
	).Result(); err != nil {
		s.deleteRedisKeysBestEffort(ctx, keyRegistrationSendCooldown(challenge.Email))
		return constant.ErrInternalServerError
	}
	if s.email != nil {
		html := email.RenderVerifyPIN("", pin, int(s.pinTTL.Minutes()))
		if err := s.email.SendWithContext(ctx, challenge.Email, "PIN Verifikasi Akun MedikaOne", html); err != nil {
			s.restoreRegistrationChallenge(ctx, id, &challenge, oldRaw)
			s.deleteRedisKeysBestEffort(ctx, keyRegistrationSendCooldown(challenge.Email))
			if errors.Is(err, email.ErrDeliveryBusy) {
				return constant.ErrEmailDeliveryBusy
			}
			return constant.ErrEmailSendFailed
		}
	}
	ulog.Infof(ctx, "registration pin resent challenge_id=%s", id[:12])
	return nil
}

func (s *Service) VerifyPIN(ctx context.Context, emailAddr, challengeID, pin string) (TokenPair, time.Time, time.Time, error) {
	id, challenge, raw, err := s.loadRegistrationChallenge(ctx, emailAddr, challengeID)
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, err
	}
	validFlag := "0"
	if secureEqual(challenge.PINHash, s.pinHash("registration", id, pin)) {
		validFlag = "1"
	}
	operationID, err := randomID(12)
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	result, err := verifyRegistrationPINScript.Run(ctx, s.redis,
		[]string{
			keyRegistration(id), keyRegistrationEmail(challenge.Email), keyRegistrationUsername(challenge.Username),
			keyRegistrationAttempts(id), keyRegistrationSendCooldown(challenge.Email),
			keyRegistrationAttemptOperation(id, operationID),
		},
		raw, id, validFlag, s.pinAttempts,
	).Int()
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	if result == -2 {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrRegistrationPINAttemptsExceeded
	}
	if result != 1 {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrRegistrationPINInvalidOrExpired
	}

	now := time.Now()
	u, err := s.users.FindByEmail(ctx, challenge.Email)
	if err != nil {
		s.restoreRegistrationChallenge(ctx, id, &challenge, raw)
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	if statusErr := existingRegistrationUserError(u); statusErr != nil {
		s.restoreRegistrationChallenge(ctx, id, &challenge, raw)
		return TokenPair{}, time.Time{}, time.Time{}, statusErr
	}
	if u == nil {
		username, phone := challenge.Username, challenge.Phone
		u = &entity.User{
			Email: challenge.Email, Username: &username, Phone: &phone,
			PasswordHash: challenge.PasswordHash, Status: "active", VerifiedAt: &now,
		}
		if err := s.users.Create(ctx, u); err != nil {
			s.restoreRegistrationChallenge(ctx, id, &challenge, raw)
			return TokenPair{}, time.Time{}, time.Time{}, registrationPersistenceError(err)
		}
	} else {
		updated, updateErr := s.users.ActivatePendingRegistration(ctx, u.ID, map[string]any{
			"username": challenge.Username, "phone": challenge.Phone,
			"password_hash": challenge.PasswordHash, "status": "active", "verified_at": now,
		})
		if updateErr != nil {
			s.restoreRegistrationChallenge(ctx, id, &challenge, raw)
			return TokenPair{}, time.Time{}, time.Time{}, registrationPersistenceError(updateErr)
		}
		if !updated {
			s.restoreRegistrationChallenge(ctx, id, &challenge, raw)
			return TokenPair{}, time.Time{}, time.Time{}, constant.ErrDuplicateUsernameOrEmail
		}
		u.Status = "active"
		u.PasswordHash = challenge.PasswordHash
		u.VerifiedAt = &now
	}

	pair, _, accessExp, refreshExp, err := s.issueJWT(ctx, u.ID)
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	ulog.Infof(ctx, "verify pin success user_id=%s", u.ID)
	return pair, accessExp, refreshExp, nil
}

func (s *Service) Login(ctx context.Context, identity, password string) (pair struct {
	AccessToken  string
	RefreshToken string
}, roles []response.RoleBrief, accessExp, refreshExp time.Time, err error) {
	identity = strings.ToLower(strings.TrimSpace(identity))
	ulog.Infof(ctx, "login attempt identity_hash=%s", identityFingerprint(s.jwtSecret, "log-identity", identity))
	rateKey := loginRateKey(ctx, identity, s.jwtSecret)
	attempts, rateErr := s.incrementWithTTL(ctx, rateKey, s.loginRateWindow)
	if rateErr != nil {
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	if attempts > int64(s.loginRateLimit) {
		return pair, nil, time.Time{}, time.Time{}, constant.ErrLoginAttemptsExceeded
	}

	tx := s.users.Begin(ctx)
	if tx.Error != nil {
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	defer func() { _ = tx.Rollback().Error }()
	users := s.users.WithTx(tx)
	u, err := users.FindByIdentityForAuth(ctx, identity)
	if err != nil {
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	if u == nil {
		if verifyErr := s.runDummyPasswordVerification(ctx, password); errors.Is(verifyErr, constant.ErrPasswordProcessingBusy) {
			return pair, nil, time.Time{}, time.Time{}, constant.ErrPasswordProcessingBusy
		}
		ulog.Errorf(ctx, "login fail: invalid credentials")
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInvalidCredentials
	}
	if strings.ToLower(u.Status) != "active" {
		if verifyErr := s.runDummyPasswordVerification(ctx, password); errors.Is(verifyErr, constant.ErrPasswordProcessingBusy) {
			return pair, nil, time.Time{}, time.Time{}, constant.ErrPasswordProcessingBusy
		}
		ulog.Errorf(ctx, "login fail: invalid credentials")
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInvalidCredentials
	}
	capturedVersion, versionErr := s.sessionVersion(ctx, u.ID)
	if versionErr != nil {
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}

	// verify password via hybrid (bcrypt → migrate ke scrypt jika perlu)
	if err := s.verifyAndMigratePasswordWithRepository(ctx, users, u, password); err != nil {
		if errors.Is(err, constant.ErrPasswordProcessingBusy) {
			return pair, nil, time.Time{}, time.Time{}, constant.ErrPasswordProcessingBusy
		}
		ulog.Errorf(ctx, "login fail: invalid credentials")
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInvalidCredentials
	}
	s.deleteRedisKeysBestEffort(ctx, rateKey)

	rows, rerr := s.roles.WithTx(tx).ListRolesByUser(ctx, u.ID)
	if rerr != nil {
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	out := make([]response.RoleBrief, 0, len(rows))
	for _, r := range rows {
		out = append(out, response.RoleBrief{ID: r.ID, Slug: r.Slug, Name: r.Name})
	}
	if err := tx.Commit().Error; err != nil {
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	j, _, aexp, rexp, jerr := s.issueJWTAtVersion(ctx, u.ID, capturedVersion)
	if jerr != nil {
		return pair, nil, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	pair.AccessToken, pair.RefreshToken = j.AccessToken, j.RefreshToken
	accessExp, refreshExp = aexp, rexp

	ulog.Infof(ctx, "login success user_id=%s", u.ID)
	return pair, out, accessExp, refreshExp, nil
}

func (s *Service) LoginHospital(ctx context.Context, identifier, password, hospitalHint string) (*LoginHospitalResult, error) {
	id := strings.ToLower(strings.TrimSpace(identifier))
	rateKey := loginRateKey(ctx, id, s.jwtSecret)
	attempts, rateErr := s.incrementWithTTL(ctx, rateKey, s.loginRateWindow)
	if rateErr != nil {
		return nil, constant.ErrInternalServerError
	}
	if attempts > int64(s.loginRateLimit) {
		return nil, constant.ErrLoginAttemptsExceeded
	}

	hID, err := s.hosp.ResolveHospitalID(ctx, hospitalHint)
	if err != nil || hID == "" {
		return nil, constant.ErrHospitalNotFound
	}

	tx := s.users.Begin(ctx)
	if tx.Error != nil {
		return nil, constant.ErrInternalServerError
	}
	defer func() { _ = tx.Rollback().Error }()
	users := s.users.WithTx(tx)
	u, err := users.FindByIdentityForAuth(ctx, id)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if u == nil {
		if verifyErr := s.runDummyPasswordVerification(ctx, password); errors.Is(verifyErr, constant.ErrPasswordProcessingBusy) {
			return nil, constant.ErrPasswordProcessingBusy
		}
		return nil, constant.ErrInvalidCredentials
	}
	if strings.ToLower(u.Status) != "active" {
		if verifyErr := s.runDummyPasswordVerification(ctx, password); errors.Is(verifyErr, constant.ErrPasswordProcessingBusy) {
			return nil, constant.ErrPasswordProcessingBusy
		}
		return nil, constant.ErrInvalidCredentials
	}
	capturedVersion, versionErr := s.sessionVersion(ctx, u.ID)
	if versionErr != nil {
		return nil, constant.ErrInternalServerError
	}

	// password verify & migrate jika perlu
	if err := s.verifyAndMigratePasswordWithRepository(ctx, users, u, password); err != nil {
		if errors.Is(err, constant.ErrPasswordProcessingBusy) {
			return nil, constant.ErrPasswordProcessingBusy
		}
		return nil, constant.ErrInvalidCredentials
	}
	s.deleteRedisKeysBestEffort(ctx, rateKey)

	ok, lerr := s.hosp.WithTx(tx).IsUserLinkedToHospital(ctx, u.ID, hID)
	if lerr != nil {
		return nil, constant.ErrInternalServerError
	}
	if !ok {
		return nil, constant.ErrUserNotLinkedToHospital
	}

	rs, rerr := s.roles.WithTx(tx).ListHospitalRolesByUser(ctx, hID, u.ID)
	if rerr != nil {
		return nil, constant.ErrInternalServerError
	}
	rbrief := make([]response.RoleBrief, 0, len(rs))
	for _, r := range rs {
		rbrief = append(rbrief, response.RoleBrief{ID: r.ID, Slug: r.Slug, Name: r.Name})
	}
	if err := tx.Commit().Error; err != nil {
		return nil, constant.ErrInternalServerError
	}
	j, _, aexp, rexp, jerr := s.issueJWTAtVersion(ctx, u.ID, capturedVersion)
	if jerr != nil {
		return nil, constant.ErrInternalServerError
	}

	ulog.Infof(ctx, "login hospital success user_id=%s hospital_id=%s", u.ID, hID)
	return &LoginHospitalResult{
		AccessToken: j.AccessToken, RefreshToken: j.RefreshToken,
		ExpiresIn: int64(s.accessTTL / time.Second), TokenType: "Bearer",
		HospitalID: hID, Roles: rbrief,
		AccessExp: aexp, RefreshExp: rexp,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken, idempotencyKey string) (TokenPair, time.Time, time.Time, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	operationID, parseErr := uuid.Parse(idempotencyKey)
	if parseErr != nil || operationID.Version() != uuid.Version(4) ||
		operationID.Variant() != uuid.RFC4122 || operationID.String() != idempotencyKey {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInvalidIdempotencyKey
	}
	idempotencyKey = operationID.String()
	claims, err := parseJWTHS256(refreshToken, s.jwtSecret)
	if err != nil || claims["typ"] != "refresh" {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInvalidToken
	}
	jti, _ := claims["jti"].(string)
	sub, _ := claims["sub"].(string)
	if jti == "" || sub == "" {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInvalidToken
	}

	u, err := s.users.GetByID(ctx, sub)
	if errors.Is(err, gorm.ErrRecordNotFound) || u == nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInvalidToken
	}
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	if !strings.EqualFold(u.Status, "active") {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInvalidToken
	}

	tokenVersion, ok := claims["sv"].(string)
	if !ok || tokenVersion == "" {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInvalidToken
	}
	familyID, ok := claims["fid"].(string)
	if !ok || familyID == "" {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInvalidToken
	}
	currentVersion, err := s.redis.Get(ctx, keySessionVersion(sub)).Result()
	if err == redis.Nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInvalidToken
	}
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	if tokenVersion != currentVersion {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrTokenExpired
	}

	operationFingerprint := identityFingerprint(s.jwtSecret, "refresh-idempotency", idempotencyKey)
	resultKey := keyRefreshResult(jti, operationFingerprint)
	resultAAD := strings.Join([]string{resultKey, sub, familyID, tokenVersion}, "\x00")
	loadCachedResult := func() (TokenPair, time.Time, time.Time, bool, error) {
		encoded, cacheErr := s.redis.Get(ctx, resultKey).Result()
		if cacheErr == redis.Nil {
			return TokenPair{}, time.Time{}, time.Time{}, false, nil
		}
		if cacheErr != nil {
			return TokenPair{}, time.Time{}, time.Time{}, false, cacheErr
		}
		cached, decryptErr := s.decryptRefreshResult(encoded, resultAAD)
		if decryptErr != nil {
			return TokenPair{}, time.Time{}, time.Time{}, false, decryptErr
		}
		accessExpiry := time.Unix(cached.AccessExp, 0)
		refreshExpiry := time.Unix(cached.RefreshExp, 0)
		if cached.Pair.AccessToken == "" || cached.Pair.RefreshToken == "" ||
			!accessExpiry.After(time.Now()) || !refreshExpiry.After(time.Now()) {
			return TokenPair{}, time.Time{}, time.Time{}, false, nil
		}
		return cached.Pair, accessExpiry, refreshExpiry, true, nil
	}
	if cachedPair, cachedAccessExp, cachedRefreshExp, ok, cacheErr := loadCachedResult(); cacheErr != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	} else if ok {
		ulog.Infof(ctx, "refresh idempotent retry user_id=%s", sub)
		return cachedPair, cachedAccessExp, cachedRefreshExp, nil
	}

	pair, nextJTI, aexp, rexp, err := s.issueJWTAtVersionForFamily(ctx, sub, tokenVersion, familyID, false)
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	replayVersion, err := randomID(18)
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	oldTTLSeconds := ceilTTLSeconds(time.Until(time.Unix(toInt64(claims["exp"]), 0)))
	newTTLSeconds := ceilTTLSeconds(time.Until(rexp))
	resultTTLSeconds := min(oldTTLSeconds, int64(120))
	oldRecord := refreshRecord(sub, familyID, tokenVersion)
	encryptedResult, err := s.encryptRefreshResult(refreshResultCache{
		Pair: pair, AccessExp: aexp.Unix(), RefreshExp: rexp.Unix(),
	}, resultAAD)
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	rotated, err := rotateRefreshScript.Run(
		ctx,
		s.redis,
		[]string{
			keyRefresh(jti), keyUserRefreshSet(sub), keyRefreshUsed(jti),
			keySessionVersion(sub), keyRefresh(nextJTI), resultKey,
		},
		oldRecord,
		jti,
		oldTTLSeconds,
		tokenVersion,
		replayVersion,
		keyRefresh(""),
		refreshRecord(sub, familyID, tokenVersion),
		newTTLSeconds,
		nextJTI,
		time.Now().Unix(),
		rexp.Unix(),
		s.maxSessions,
		oldRecord+"|"+operationFingerprint,
		encryptedResult,
		resultTTLSeconds,
	).Int()
	if err != nil {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
	}
	if rotated == 2 {
		cachedPair, cachedAccessExp, cachedRefreshExp, ok, cacheErr := loadCachedResult()
		if cacheErr != nil {
			return TokenPair{}, time.Time{}, time.Time{}, constant.ErrInternalServerError
		}
		if !ok {
			return TokenPair{}, time.Time{}, time.Time{}, constant.ErrTokenExpired
		}
		ulog.Infof(ctx, "refresh idempotent retry user_id=%s", sub)
		return cachedPair, cachedAccessExp, cachedRefreshExp, nil
	}
	if rotated != 1 {
		return TokenPair{}, time.Time{}, time.Time{}, constant.ErrTokenExpired
	}
	ulog.Infof(ctx, "refresh success user_id=%s", sub)
	return pair, aexp, rexp, nil
}

/* ==================== Role & Profiles ==================== */

func (s *Service) ChooseRole(ctx context.Context, userID, roleSlug string) error {
	roleSlug = strings.ToUpper(strings.TrimSpace(roleSlug))
	if roleSlug != constant.RolePatient {
		return constant.ErrSelfServicePatientRoleOnly
	}
	r, err := s.roles.FindBySlug(ctx, constant.RolePatient)
	if err != nil || r == nil || !r.Active {
		return constant.ErrAccountRoleNotFound
	}
	if err := s.roles.Assign(ctx, userID, r.ID); err != nil {
		return constant.ErrInternalServerError
	}
	ulog.Infof(ctx, "choose role user_id=%s role=%s", userID, roleSlug)
	return nil
}

func (s *Service) CompletePatientProfile(ctx context.Context, userID string, req *request.PatientProfileRequest) error {
	return s.users.Transaction(ctx, func(tx *gorm.DB) error {
		return s.completePatientProfile(ctx, s.users.WithTx(tx), userID, req)
	})
}

func (s *Service) completePatientProfile(ctx context.Context, users *userrepo.Repository, userID string, req *request.PatientProfileRequest) error {
	if err := ulog.ValidateStruct(req); err != nil {
		return ulog.MapValidationError(err)
	}
	updates := map[string]any{
		"first_name": req.FirstName,
		"last_name":  req.LastName,
		"address":    req.Address,
		"gender":     req.Gender,
		"nik":        req.NIK,
	}
	if req.NIK != nil && !isNIK16(*req.NIK) {
		return constant.NewInvalidFieldValueError("nik", "exactly 16 numeric digits", "tepat 16 digit angka")
	}
	if req.NIK != nil {
		exists, err := users.ExistsNIKExcludingUser(ctx, *req.NIK, userID)
		if err != nil {
			return constant.ErrInternalServerError
		}
		if exists {
			return constant.ErrDuplicateNIK
		}
	}
	if req.DOB != nil && *req.DOB != "" {
		tm, err := time.ParseInLocation("2006-01-02", *req.DOB, s.loc)
		if err != nil || tm.After(time.Now().In(s.loc)) {
			return constant.ErrInvalidDateFormat
		}
		updates["dob"] = tm
	}
	if err := users.UpdateByID(ctx, userID, updates); err != nil {
		return patientProfilePersistenceError(err)
	}

	prof := map[string]any{
		"user_id":      userID,
		"height_cm":    req.HeightCM,
		"weight_kg":    req.WeightKG,
		"allergies":    req.Allergies,
		"medical_hist": req.MedicalHistory,
	}
	if err := users.UpsertPatientProfile(ctx, prof); err != nil {
		return constant.ErrInternalServerError
	}
	ulog.Infof(ctx, "patient profile updated user_id=%s", userID)
	return nil
}

func (s *Service) CompleteDoctorProfile(ctx context.Context, userID string, req *request.DoctorProfileRequest) error {
	return s.users.Transaction(ctx, func(tx *gorm.DB) error {
		return s.completeDoctorProfile(ctx, s.users.WithTx(tx), userID, req)
	})
}

func (s *Service) completeDoctorProfile(ctx context.Context, users *userrepo.Repository, userID string, req *request.DoctorProfileRequest) error {
	if err := ulog.ValidateStruct(req); err != nil {
		return ulog.MapValidationError(err)
	}
	updates := map[string]any{
		"first_name": req.FirstName,
		"last_name":  req.LastName,
		"address":    req.Address,
		"gender":     req.Gender,
	}
	if err := users.UpdateByID(ctx, userID, updates); err != nil {
		return constant.ErrInternalServerError
	}

	prof := map[string]any{
		"user_id":    userID,
		"sip_number": req.SIPNumber,
		"specialty":  req.Specialty,
	}
	if err := users.UpsertDoctorProfile(ctx, prof); err != nil {
		return constant.ErrInternalServerError
	}
	ulog.Infof(ctx, "doctor profile updated user_id=%s", userID)
	return nil
}

func (s *Service) SetProfile(ctx context.Context, userID, roleSlugUpper string, rawProfile *json.RawMessage) (*response.SetProfileResponse, error) {
	if rawProfile == nil {
		return nil, constant.NewFieldRequiredError("profile")
	}
	role := strings.ToUpper(strings.TrimSpace(roleSlugUpper))
	if role != constant.RolePatient {
		return nil, constant.ErrSelfServicePatientRoleOnly
	}

	var req request.PatientProfileRequest
	if err := ulog.UnmarshalStrictJSON(*rawProfile, &req); err != nil {
		return nil, ulog.MapJSONDecodeError(err)
	}
	if err := ulog.ValidateStruct(&req); err != nil {
		return nil, ulog.MapValidationError(err)
	}

	r, err := s.roles.FindBySlug(ctx, role)
	if err != nil || r == nil || !r.Active {
		return nil, constant.ErrAccountRoleNotFound
	}
	if err := s.users.Transaction(ctx, func(tx *gorm.DB) error {
		users := s.users.WithTx(tx)
		roles := s.roles.WithTx(tx)
		profileExists, err := users.ExistsPatientProfile(ctx, userID)
		if err != nil {
			return constant.ErrInternalServerError
		}
		hasRole, err := roles.UserHasRole(ctx, userID, constant.RolePatient)
		if err != nil {
			return constant.ErrInternalServerError
		}
		if profileExists && hasRole {
			return constant.ErrProfileAlreadySet
		}
		if !profileExists {
			if err := s.completePatientProfile(ctx, users, userID, &req); err != nil {
				return err
			}
		}
		if !hasRole {
			if err := roles.Assign(ctx, userID, r.ID); err != nil {
				return constant.ErrInternalServerError
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	u, err := s.users.GetByID(ctx, userID)
	if err != nil || u == nil {
		return nil, constant.ErrInternalServerError
	}
	var dobStr *string
	if u.DOB != nil {
		tmp := u.DOB.In(s.loc).Format("2006-01-02")
		dobStr = &tmp
	}
	prof := response.UserProfile{
		ID:        u.ID,
		Email:     u.Email,
		Username:  u.Username,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Phone:     u.Phone,
		DOB:       dobStr,
		Address:   u.Address,
		Gender:    u.Gender,
		NIK:       u.NIK,
	}
	h, w, a, m, err := s.users.GetPatientProfileByUserID(ctx, userID)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	prof.HeightCM, prof.WeightKG, prof.Allergies, prof.MedicalHistory = h, w, a, m

	ulog.Infof(ctx, "set-profile success user_id=%s role=%s", userID, role)
	return &response.SetProfileResponse{
		Role:    role,
		Profile: prof,
	}, nil
}

/* ==================== Password helpers ==================== */

func (s *Service) hashPasswordScrypt(ctx context.Context, password string) (string, error) {
	hash, err := ulog.HashPasswordScrypt(ctx, password)
	if errors.Is(err, ulog.ErrPasswordWorkLimit) {
		return "", constant.ErrPasswordProcessingBusy
	}
	return hash, err
}

// verifyAndMigratePassword:
// - Jika stored bcrypt, verifikasi bcrypt -> kalau OK, rehash ke scrypt & update DB.
// - Jika stored scrypt, verifikasi scrypt seperti biasa.
func (s *Service) verifyAndMigratePassword(ctx context.Context, u *entity.User, plain string) error {
	return s.verifyAndMigratePasswordWithRepository(ctx, s.users, u, plain)
}

func (s *Service) verifyAndMigratePasswordWithRepository(ctx context.Context, users *userrepo.Repository, u *entity.User, plain string) error {
	stored := u.PasswordHash

	// BCRYPT
	if strings.HasPrefix(stored, "$2a$") ||
		strings.HasPrefix(stored, "$2b$") ||
		strings.HasPrefix(stored, "$2y$") {
		matched, err := ulog.VerifyPasswordBcrypt(ctx, stored, plain)
		if errors.Is(err, ulog.ErrPasswordWorkLimit) {
			return constant.ErrPasswordProcessingBusy
		}
		if err != nil {
			return constant.ErrInternalServerError
		}
		if !matched {
			return constant.ErrPasswordNotMatch
		}
		if newHash, err := s.hashPasswordScrypt(ctx, plain); err == nil && newHash != "" {
			_ = users.UpdateByID(ctx, u.ID, map[string]any{"password_hash": newHash})
			u.PasswordHash = newHash
		}
		return nil
	}

	matched, err := ulog.VerifyPasswordScrypt(ctx, stored, plain)
	if errors.Is(err, ulog.ErrPasswordWorkLimit) {
		return constant.ErrPasswordProcessingBusy
	}
	if err != nil {
		return constant.ErrInternalServerError
	}
	if !matched {
		return constant.ErrPasswordNotMatch
	}
	return nil
}

/* ==================== Forgot/Reset/Change Password ==================== */

func (s *Service) PasswordForgot(ctx context.Context, req *request.PasswordForgotRequest) (*PasswordForgotResult, error) {
	started := time.Now()
	defer waitForMinimumDuration(ctx, started, 500*time.Millisecond)

	emailAddr := strings.ToLower(strings.TrimSpace(req.Email))
	rate, err := s.incrementWithTTL(ctx, forgotRateKey(ctx, emailAddr, s.jwtSecret), s.forgotRateWindow)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if rate > int64(s.forgotRateLimit) {
		return nil, constant.ErrPasswordResetRequestsExceeded
	}
	challengeID, err := randomID(24)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	result := &PasswordForgotResult{ChallengeID: challengeID, Status: "pin_sent"}

	u, err := s.users.FindByEmail(ctx, emailAddr)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	pin, err := sixDigitPIN()
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	key := keyPasswordReset(challengeID)
	currentKey := passwordResetIdentityKey(emailAddr, s.jwtSecret)
	active := u != nil && strings.EqualFold(u.Status, "active")
	ttlSeconds := ceilTTLSeconds(s.pinTTL)
	credentialBinding := "unavailable"
	if active {
		credentialBinding = s.passwordResetCredentialBinding(u)
	}
	pinHash := s.pinHash("password-reset", challengeID+"\x00"+emailAddr+"\x00"+credentialBinding, pin)
	resetRecord := passwordResetRecord("unavailable", pinHash)
	if active {
		resetRecord = passwordResetRecord(u.ID, pinHash)
	}
	if err := storePasswordResetScript.Run(
		ctx,
		s.redis,
		[]string{key, currentKey},
		challengeID,
		resetRecord,
		ttlSeconds,
		keyPasswordReset(""),
		keyPasswordAttempts(""),
		keyPasswordResetProof(""),
	).Err(); err != nil {
		return nil, constant.ErrInternalServerError
	}
	// Unknown, deleted, pending, and active accounts all perform the same Redis
	// round trip and return the same response. SMTP runs on a bounded worker so
	// its latency cannot reveal whether this address is registered.
	if !active {
		return result, nil
	}
	if !s.enqueueEmail(emailJob{
		to: emailAddr, subject: "PIN Reset Password MedikaOne", resetPIN: pin, recipientName: u.FirstName,
		identityHash: identityFingerprint(s.jwtSecret, "log-identity", emailAddr), deleteKeyOnFailure: key, expectedValue: resetRecord,
		replacementValueOnFailure: passwordResetRecord("unavailable", pinHash),
		expiresAt:                 time.Now().Add(s.pinTTL),
	}) {
		s.replaceSecretBestEffort(ctx, key, resetRecord, passwordResetRecord("unavailable", pinHash))
		ulog.Errorf(ctx, "password reset email queue unavailable identity_hash=%s", identityFingerprint(s.jwtSecret, "log-identity", emailAddr))
		return result, nil
	}
	ulog.Infof(ctx, "password forgot pin queued identity_hash=%s challenge_id=%s", identityFingerprint(s.jwtSecret, "log-identity", emailAddr), challengeID[:12])
	return result, nil
}

func waitForMinimumDuration(ctx context.Context, started time.Time, minimum time.Duration) {
	remaining := minimum - time.Since(started)
	if remaining <= 0 {
		return
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

func (s *Service) PasswordResetVerifyPIN(ctx context.Context, req *request.PasswordResetVerifyPINRequest) (*response.PasswordResetVerifyPINResponse, error) {
	started := time.Now()
	defer waitForMinimumDuration(ctx, started, 500*time.Millisecond)

	emailAddr := strings.ToLower(strings.TrimSpace(req.Email))
	challengeID := strings.TrimSpace(req.ChallengeID)
	pin := strings.TrimSpace(req.PIN)

	u, err := s.users.FindByEmail(ctx, emailAddr)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	userID := ""
	credentialBinding := ""
	if u != nil && strings.EqualFold(u.Status, "active") {
		userID = u.ID
		credentialBinding = s.passwordResetCredentialBinding(u)
	}
	pinHash := s.pinHash("password-reset", challengeID+"\x00"+emailAddr+"\x00"+credentialBinding, pin)
	expectedChallenge := passwordResetRecord(userID, pinHash)

	resetToken, err := randomID(32)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	proofHash := s.pinHash("password-reset-proof", challengeID+"\x00"+credentialBinding, resetToken)
	proofRecord := passwordResetProofRecord(userID, credentialBinding, proofHash)
	operationID, err := randomID(12)
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	remainingTTLMS, err := verifyPasswordResetPINAndMintProofScript.Run(
		ctx,
		s.redis,
		[]string{
			keyPasswordReset(challengeID),
			keyPasswordAttempts(challengeID),
			keyPasswordResetAttemptOperation(challengeID, operationID),
			passwordResetIdentityKey(emailAddr, s.jwtSecret),
			keyPasswordResetProof(challengeID),
		},
		expectedChallenge,
		s.pinAttempts,
		challengeID,
		proofRecord,
		s.pinTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return nil, constant.ErrInternalServerError
	}
	if remainingTTLMS == -2 {
		return nil, constant.ErrPasswordResetPINAttemptsExceeded
	}
	if remainingTTLMS <= 0 || userID == "" {
		return nil, constant.ErrPasswordResetPINInvalidOrExpired
	}

	ulog.Infof(ctx, "password reset pin verified user_id=%s", userID)
	return &response.PasswordResetVerifyPINResponse{
		Status:     "pin_verified",
		ResetToken: resetToken,
		ExpiresIn:  ceilTTLSeconds(time.Duration(remainingTTLMS) * time.Millisecond),
	}, nil
}

func (s *Service) PasswordReset(ctx context.Context, req *request.PasswordResetRequest) error {
	challengeID := strings.TrimSpace(req.ChallengeID)
	resetToken := strings.TrimSpace(req.ResetToken)
	newPass := req.NewPassword
	if !isCanonicalRandomID(resetToken, 32) {
		return constant.ErrInvalidResetToken
	}
	if !ulog.IsValidPassword(newPass) {
		return constant.ErrInvalidPassword
	}

	proofKey := keyPasswordResetProof(challengeID)
	proofRecord, err := s.redis.Get(ctx, proofKey).Result()
	if errors.Is(err, redis.Nil) {
		return constant.ErrInvalidResetToken
	}
	if err != nil {
		return constant.ErrInternalServerError
	}
	userID, credentialBinding, storedProofHash, ok := parsePasswordResetProofRecord(proofRecord)
	if !ok {
		return constant.ErrInvalidResetToken
	}
	proofHash := s.pinHash("password-reset-proof", challengeID+"\x00"+credentialBinding, resetToken)
	if !secureEqual(storedProofHash, proofHash) {
		return constant.ErrInvalidResetToken
	}

	tx := s.users.Begin(ctx)
	if tx.Error != nil {
		return constant.ErrInternalServerError
	}
	defer func() { _ = tx.Rollback().Error }()
	users := s.users.WithTx(tx)
	u, err := users.GetByIDForUpdate(ctx, userID)
	if err != nil {
		return constant.ErrInternalServerError
	}
	if u == nil || !strings.EqualFold(u.Status, "active") {
		return constant.ErrInvalidResetToken
	}
	if !secureEqual(credentialBinding, s.passwordResetCredentialBinding(u)) {
		return constant.ErrInvalidResetToken
	}

	expectedProof := passwordResetProofRecord(u.ID, credentialBinding, proofHash)
	emailAddr := strings.ToLower(strings.TrimSpace(u.Email))
	if ulog.IsPasswordSimilarToUserInfo(newPass, valueOrEmpty(u.Username), u.Email) {
		return constant.ErrPasswordSimilarToUserInfo
	}
	newHash, err := s.hashPasswordScrypt(ctx, newPass)
	if err != nil {
		if errors.Is(err, constant.ErrPasswordProcessingBusy) {
			return constant.ErrPasswordProcessingBusy
		}
		return constant.ErrInternalServerError
	}
	newSessionVersion, err := randomID(18)
	if err != nil {
		return constant.ErrInternalServerError
	}
	consumeOperationID, err := randomID(12)
	if err != nil {
		return constant.ErrInternalServerError
	}
	currentKey := passwordResetIdentityKey(emailAddr, s.jwtSecret)
	var consumedTTLMS int64
	redisConsumed := false
	err = func() error {
		updated, updateErr := users.UpdateActivePassword(ctx, u.ID, newHash)
		if updateErr != nil {
			return constant.ErrInternalServerError
		}
		if !updated {
			return constant.ErrInvalidResetToken
		}
		consumedTTLMS, updateErr = consumePasswordResetProofAndRevokeScript.Run(
			ctx,
			s.redis,
			[]string{
				proofKey,
				keyUserRefreshSet(u.ID),
				keySessionVersion(u.ID),
				keyPasswordResetConsumeOperation(challengeID, consumeOperationID),
				currentKey,
			},
			expectedProof,
			newSessionVersion,
			keyRefresh(""),
			challengeID,
		).Int64()
		if updateErr != nil {
			return constant.ErrInternalServerError
		}
		if consumedTTLMS <= 0 {
			return constant.ErrInvalidResetToken
		}
		redisConsumed = true
		return nil
	}()
	if err != nil {
		if redisConsumed {
			s.restorePasswordResetProof(ctx, proofKey, currentKey, challengeID, expectedProof, consumedTTLMS)
		}
		if errors.Is(err, constant.ErrInvalidResetToken) {
			return constant.ErrInvalidResetToken
		}
		return constant.ErrInternalServerError
	}
	if err := tx.Commit().Error; err != nil {
		// Commit errors can be ambiguous at the network boundary. Never restore a
		// one-time reset token when the password change may already be durable.
		return constant.ErrInternalServerError
	}
	// Invalidate a challenge or proof issued concurrently before this reset
	// completed. A password reset leaves no older recovery credential usable.
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	if err := invalidateCurrentPasswordResetScript.Run(
		cleanupCtx,
		s.redis,
		[]string{currentKey},
		keyPasswordReset(""),
		keyPasswordAttempts(""),
		keyPasswordResetProof(""),
	).Err(); err != nil {
		// The password and session-version updates are already durable. Recovery
		// credentials are also bound to the previous password hash, so a stale
		// challenge cannot become valid even when this best-effort cleanup fails.
		ulog.Errorf(context.Background(), "password reset credential cleanup failed user_id=%s error_type=%T", u.ID, err)
	}

	ulog.Infof(ctx, "password reset success user_id=%s", u.ID)
	return nil
}

func (s *Service) PasswordChange(ctx context.Context, userID string, req *request.PasswordChangeRequest) error {
	oldPass := req.OldPassword
	newPass := req.NewPassword

	if !ulog.IsValidPassword(newPass) {
		return constant.ErrInvalidPassword
	}
	if secureEqual(oldPass, newPass) {
		return constant.ErrNewPasswordSame
	}

	tx := s.users.Begin(ctx)
	if tx.Error != nil {
		return constant.ErrInternalServerError
	}
	defer func() { _ = tx.Rollback().Error }()
	users := s.users.WithTx(tx)
	u, err := users.GetByIDForUpdate(ctx, userID)
	if err != nil {
		return constant.ErrInternalServerError
	}
	if u == nil {
		return constant.ErrUserNotFound
	}
	if !strings.EqualFold(u.Status, "active") {
		return constant.ErrAccountInactive
	}
	if ulog.IsPasswordSimilarToUserInfo(newPass, valueOrEmpty(u.Username), u.Email) {
		return constant.ErrPasswordSimilarToUserInfo
	}

	if err := s.verifyAndMigratePasswordWithRepository(ctx, users, u, oldPass); err != nil {
		return err
	}

	newHash, err := s.hashPasswordScrypt(ctx, newPass)
	if err != nil {
		if errors.Is(err, constant.ErrPasswordProcessingBusy) {
			return constant.ErrPasswordProcessingBusy
		}
		return constant.ErrInternalServerError
	}
	updated, err := users.UpdateActivePassword(ctx, userID, newHash)
	if err != nil {
		return constant.ErrInternalServerError
	}
	if !updated {
		return constant.ErrAccountInactive
	}
	if err := s.revokeAllRefresh(ctx, userID); err != nil {
		return constant.ErrInternalServerError
	}
	if err := tx.Commit().Error; err != nil {
		return constant.ErrInternalServerError
	}

	ulog.Infof(ctx, "password change success user_id=%s", userID)
	return nil
}

/* ==================== Logout APIs ==================== */

func (s *Service) accessTokenIdentity(accessToken string) (jti, subject, familyID, sessionVersion string, ttl time.Duration, err error) {
	if strings.TrimSpace(accessToken) == "" {
		return "", "", "", "", 0, constant.ErrUnauthorized
	}
	claims, err := parseJWTHS256(accessToken, s.jwtSecret)
	if err != nil {
		return "", "", "", "", 0, err
	}
	if typ, _ := claims["typ"].(string); typ != "access" {
		return "", "", "", "", 0, constant.ErrInvalidToken
	}
	jti, _ = claims["jti"].(string)
	subject, _ = claims["sub"].(string)
	familyID, _ = claims["fid"].(string)
	sessionVersion, _ = claims["sv"].(string)
	expiresAt := toInt64(claims["exp"])
	if jti == "" || subject == "" || sessionVersion == "" || expiresAt == 0 {
		return "", "", "", "", 0, constant.ErrInvalidToken
	}
	ttl = time.Until(time.Unix(expiresAt, 0))
	if ttl <= 0 {
		return "", "", "", "", 0, constant.ErrTokenExpired
	}
	return jti, subject, familyID, sessionVersion, ttl, nil
}

func (s *Service) Logout(ctx context.Context, accessToken string, refreshToken string) error {
	accessJTI, subject, familyID, sessionVersion, ttl, err := s.accessTokenIdentity(accessToken)
	if err != nil {
		return err
	}
	if familyID == "" {
		return constant.ErrInvalidToken
	}

	if strings.TrimSpace(refreshToken) != "" {
		refreshClaims, parseErr := parseJWTHS256(refreshToken, s.jwtSecret)
		if parseErr != nil || refreshClaims["typ"] != "refresh" {
			return constant.ErrInvalidToken
		}
		refreshJTI, _ := refreshClaims["jti"].(string)
		refreshSubject, _ := refreshClaims["sub"].(string)
		refreshFamilyID, _ := refreshClaims["fid"].(string)
		refreshVersion, _ := refreshClaims["sv"].(string)
		if refreshJTI == "" || refreshSubject != subject || refreshFamilyID != familyID || refreshVersion != sessionVersion {
			return constant.ErrInvalidToken
		}
	}

	ttlSeconds := ceilTTLSeconds(ttl)
	familyTTLSeconds := ceilTTLSeconds(max(ttl, s.accessTTL))
	if err := logoutFamilyScript.Run(
		ctx,
		s.redis,
		[]string{
			keyAccessBlacklist(accessJTI),
			keyUserRefreshSet(subject),
			keyAccessFamilyRevoked(subject, familyID),
		},
		ttlSeconds,
		familyTTLSeconds,
		keyRefresh(""),
		refreshRecord(subject, familyID, sessionVersion),
	).Err(); err != nil {
		return constant.ErrInternalServerError
	}

	ulog.Infof(ctx, "logout success user_id=%s", subject)
	return nil
}

func ceilTTLSeconds(ttl time.Duration) int64 {
	seconds := int64(ttl / time.Second)
	if ttl%time.Second != 0 {
		seconds++
	}
	return max(int64(1), seconds)
}

func (s *Service) LogoutAll(ctx context.Context, accessToken string) error {
	_, subject, _, _, _, err := s.accessTokenIdentity(accessToken)
	if err != nil {
		return err
	}
	if err := s.revokeAllRefresh(ctx, subject); err != nil {
		return constant.ErrInternalServerError
	}

	ulog.Infof(ctx, "logout-all success user_id=%s", subject)
	return nil
}
