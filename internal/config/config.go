package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	mapstructure "github.com/go-viper/mapstructure/v2"
	"github.com/jackc/pgx/v5"
	goredis "github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var Env *EnvConfig

type ValidationScope string

const (
	ValidationServer           ValidationScope = "server"
	ValidationDatabase         ValidationScope = "database"
	ValidationMigration        ValidationScope = "migration"
	ValidationMaintenance      ValidationScope = "maintenance"
	ValidationDatabaseTarget   ValidationScope = "database-target"
	ValidationLocalFileCommand ValidationScope = "local-file-command"
)

// AuthRedisKeyPrefix isolates authentication state between environments and
// database targets even when operators accidentally reuse one Redis database.
// It contains no credential material.
func AuthRedisKeyPrefix() string {
	environment := "unconfigured"
	target := "local"
	if Env != nil {
		environment = strings.ToLower(strings.TrimSpace(Env.Env))
		if cfg, err := pgx.ParseConfig(strings.TrimSpace(Env.Database.DSN)); err == nil {
			target = strings.ToLower(cfg.Host) + ":" + strconv.Itoa(int(cfg.Port)) +
				"|" + cfg.Database + "|" + cfg.RuntimeParams["search_path"]
		}
	}
	sum := sha256.Sum256([]byte(environment + "|" + target))
	return "medikaone:" + environment + ":" + hex.EncodeToString(sum[:8]) + ":"
}

func MaintenanceRedisKey() string {
	return AuthRedisKeyPrefix() + "maintenance"
}

func ActiveRequestsRedisKey() string {
	return AuthRedisKeyPrefix() + "active-requests"
}

type EnvConfig struct {
	Env                     string        `mapstructure:"env"`
	LogLevel                string        `mapstructure:"log_level"`
	GracefulShutdownTimeout time.Duration `mapstructure:"graceful_shutdown_timeout"`
	Server                  Server        `mapstructure:"server"`
	Database                Database      `mapstructure:"database"`
	Redis                   Redis         `mapstructure:"redis"`
	SMTP                    SMTP          `mapstructure:"smtp"`
	JWT                     JWTConfig     `mapstructure:"jwt"`
	Auth                    AuthConfig    `mapstructure:"auth"`
	Storage                 Storage       `mapstructure:"storage"`
}

type JWTConfig struct {
	Secret           string        `mapstructure:"secret"`
	AccessTTL        time.Duration `mapstructure:"access_ttl"`
	RefreshTTL       time.Duration `mapstructure:"refresh_ttl"`
	AccessTTLMinutes int           `mapstructure:"access_ttl_minutes"`
	RefreshTTLDays   int           `mapstructure:"refresh_ttl_days"`
}

func (j JWTConfig) ParseDurations() (time.Duration, time.Duration) {
	return j.AccessTTL, j.RefreshTTL
}

type AuthConfig struct {
	PINTTL                   time.Duration `mapstructure:"pin_ttl"`
	PINMaxAttempts           int           `mapstructure:"pin_max_attempts"`
	PINResendCooldown        time.Duration `mapstructure:"pin_resend_cooldown"`
	MaxActiveSessions        int           `mapstructure:"max_active_sessions"`
	PublicIPRateLimit        int           `mapstructure:"public_ip_rate_limit"`
	PublicIPRateWindow       time.Duration `mapstructure:"public_ip_rate_window"`
	LoginRateLimit           int           `mapstructure:"login_rate_limit"`
	LoginRateWindow          time.Duration `mapstructure:"login_rate_window"`
	ForgotPasswordRateLimit  int           `mapstructure:"forgot_password_rate_limit"`
	ForgotPasswordRateWindow time.Duration `mapstructure:"forgot_password_rate_window"`
}

type Redis struct {
	CacheDSN        string        `mapstructure:"cache_dsn"`
	MaxRetry        int           `mapstructure:"max_retry"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxActiveConns  int           `mapstructure:"max_active_conns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
}

type Server struct {
	Port               string        `mapstructure:"port"`
	CORSAllowedOrigins []string      `mapstructure:"cors_allowed_origins"`
	ClientIPHeader     string        `mapstructure:"client_ip_header"`
	ReadHeaderTimeout  time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout        time.Duration `mapstructure:"read_timeout"`
	WriteTimeout       time.Duration `mapstructure:"write_timeout"`
	IdleTimeout        time.Duration `mapstructure:"idle_timeout"`
}

type Database struct {
	DSN             string        `mapstructure:"dsn"`
	AdminDSN        string        `mapstructure:"admin_dsn"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
}

type SMTP struct {
	Enabled     bool          `mapstructure:"enabled"`
	Host        string        `mapstructure:"host"`
	Port        int           `mapstructure:"port"`
	Username    string        `mapstructure:"username"`
	Password    string        `mapstructure:"password"`
	From        string        `mapstructure:"from"`
	FromName    string        `mapstructure:"from_name"`
	Timeout     time.Duration `mapstructure:"timeout"`
	UseSTARTTLS bool          `mapstructure:"use_starttls"`
}

type Storage struct {
	Enabled          bool            `mapstructure:"enabled"`
	Provider         string          `mapstructure:"provider"`
	Bucket           string          `mapstructure:"bucket"`
	MaxFileSizeBytes int64           `mapstructure:"max_file_size_bytes"`
	SignedURLTTL     time.Duration   `mapstructure:"signed_url_ttl"`
	Supabase         SupabaseStorage `mapstructure:"supabase"`
}

type SupabaseStorage struct {
	URL       string `mapstructure:"url"`
	SecretKey string `mapstructure:"secret_key"`
}

func LoadConfig() error {
	return LoadConfigFor(ValidationServer)
}

// LoadConfigFor validates only the resources a command actually uses. This
// keeps migration/reset jobs from needing unrelated JWT or SMTP credentials,
// while the serving process still receives the complete validation policy.
func LoadConfigFor(scope ValidationScope) error {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yml")
	v.AddConfigPath(".")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	setDefaults(v)
	bindEnvVariables(v)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("read config: %w", err)
		}
		logrus.Info("config.yml not found; using environment variables")
	}

	cfg := new(EnvConfig)
	if err := v.Unmarshal(cfg, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	))); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	applyLegacyJWTDurationFallbacks(v, cfg)
	normalize(cfg)
	if err := cfg.ValidateFor(scope); err != nil {
		return err
	}

	Env = cfg
	return nil
}

func (c *EnvConfig) ValidateFor(scope ValidationScope) error {
	switch scope {
	case ValidationServer:
		return c.Validate()
	case ValidationDatabase:
		return errors.Join(c.validateCommandBase(), c.validateCommandDatabase(false))
	case ValidationMigration:
		if c.Env == "staging" || c.Env == "production" {
			return c.ValidateFor(ValidationMaintenance)
		}
		return c.ValidateFor(ValidationDatabase)
	case ValidationMaintenance:
		var timeoutErr error
		if c.Server.WriteTimeout <= 0 {
			timeoutErr = errors.New("server.write_timeout must be positive")
		}
		return errors.Join(
			c.validateCommandBase(),
			c.validateCommandDatabase(false),
			c.validateCommandRedis(),
			timeoutErr,
		)
	case ValidationDatabaseTarget:
		return errors.Join(c.validateCommandBase(), c.validateCommandDatabaseTarget())
	case ValidationLocalFileCommand:
		return c.validateCommandLogLevel()
	default:
		return fmt.Errorf("unknown config validation scope %q", scope)
	}
}

func (c *EnvConfig) validateCommandLogLevel() error {
	if _, err := logrus.ParseLevel(c.LogLevel); err != nil {
		return errors.New("log_level is invalid")
	}
	return nil
}

func (c *EnvConfig) validateCommandBase() error {
	var errs []error
	if c.Env != "development" && c.Env != "staging" && c.Env != "production" && c.Env != "test" {
		errs = append(errs, errors.New("env must be development, staging, production, or test"))
	}
	if err := c.validateCommandLogLevel(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (c *EnvConfig) validateCommandDatabase(requireAdmin bool) error {
	requireTLS := c.Env == "staging" || c.Env == "production"
	var errs []error
	if err := validatePostgresDSN(c.Database.DSN, "database.dsn", requireTLS, false); err != nil {
		errs = append(errs, err)
	}
	if requireAdmin && c.Database.AdminDSN == "" {
		errs = append(errs, errors.New("database.admin_dsn is required"))
	} else if c.Database.AdminDSN != "" {
		if err := validatePostgresDSN(c.Database.AdminDSN, "database.admin_dsn", requireTLS, true); err != nil {
			errs = append(errs, err)
		}
	}
	if c.Database.MaxIdleConns < 0 {
		errs = append(errs, errors.New("database.max_idle_conns cannot be negative"))
	}
	if c.Database.MaxOpenConns <= 0 {
		errs = append(errs, errors.New("database.max_open_conns must be positive"))
	}
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		errs = append(errs, errors.New("database.max_idle_conns cannot exceed max_open_conns"))
	}
	if c.Database.MaxConnLifetime <= 0 {
		errs = append(errs, errors.New("database.max_conn_lifetime must be positive"))
	}
	return errors.Join(errs...)
}

func (c *EnvConfig) validateCommandDatabaseTarget() error {
	requireTLS := c.Env == "staging" || c.Env == "production"
	if c.Database.AdminDSN == "" {
		return errors.New("database.admin_dsn is required")
	}
	return validatePostgresDSN(c.Database.AdminDSN, "database.admin_dsn", requireTLS, true)
}

func (c *EnvConfig) validateCommandRedis() error {
	requireTLS := c.Env == "staging" || c.Env == "production"
	var errs []error
	if err := validateRedisDSN(c.Redis.CacheDSN, requireTLS); err != nil {
		errs = append(errs, err)
	}
	if c.Redis.MaxRetry <= 0 {
		errs = append(errs, errors.New("redis.max_retry must be positive"))
	}
	if c.Redis.MaxIdleConns < 0 || c.Redis.MaxActiveConns <= 0 || c.Redis.MaxIdleConns > c.Redis.MaxActiveConns {
		errs = append(errs, errors.New("redis connection pool limits are invalid"))
	}
	if c.Redis.MaxConnLifetime <= 0 {
		errs = append(errs, errors.New("redis.max_conn_lifetime must be positive"))
	}
	return errors.Join(errs...)
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("env", "development")
	v.SetDefault("log_level", "info")
	v.SetDefault("graceful_shutdown_timeout", "30s")

	v.SetDefault("server.port", "8080")
	v.SetDefault("server.read_header_timeout", "5s")
	v.SetDefault("server.read_timeout", "15s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "60s")

	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.max_open_conns", 20)
	v.SetDefault("database.max_conn_lifetime", "1h")

	v.SetDefault("redis.max_retry", 3)
	v.SetDefault("redis.max_idle_conns", 5)
	v.SetDefault("redis.max_active_conns", 20)
	v.SetDefault("redis.max_conn_lifetime", "1h")

	v.SetDefault("jwt.access_ttl", "15m")
	v.SetDefault("jwt.refresh_ttl", "720h")

	v.SetDefault("auth.pin_ttl", "10m")
	v.SetDefault("auth.pin_max_attempts", 5)
	v.SetDefault("auth.pin_resend_cooldown", "1m")
	v.SetDefault("auth.max_active_sessions", 10)
	v.SetDefault("auth.public_ip_rate_limit", 30)
	v.SetDefault("auth.public_ip_rate_window", "15m")
	v.SetDefault("auth.login_rate_limit", 10)
	v.SetDefault("auth.login_rate_window", "15m")
	v.SetDefault("auth.forgot_password_rate_limit", 3)
	v.SetDefault("auth.forgot_password_rate_window", "1h")

	// Email delivery is opt-in. This keeps an env-only deployment without SMTP
	// credentials usable for non-email endpoints and matches config.yml.example.
	v.SetDefault("smtp.enabled", false)
	v.SetDefault("smtp.port", 587)
	v.SetDefault("smtp.from_name", "MedikaOne")
	v.SetDefault("smtp.timeout", "15s")
	v.SetDefault("smtp.use_starttls", true)

	v.SetDefault("storage.provider", "supabase")
	v.SetDefault("storage.bucket", "doctor-contracts")
	v.SetDefault("storage.max_file_size_bytes", 10*1024*1024)
	v.SetDefault("storage.signed_url_ttl", "5m")
}

func bindEnvVariables(v *viper.Viper) {
	bindings := map[string]string{
		"env":                         "ENV",
		"log_level":                   "LOG_LEVEL",
		"graceful_shutdown_timeout":   "GRACEFUL_SHUTDOWN_TIMEOUT",
		"server.cors_allowed_origins": "SERVER_CORS_ALLOWED_ORIGINS",
		"server.client_ip_header":     "SERVER_CLIENT_IP_HEADER",
		"server.read_header_timeout":  "SERVER_READ_HEADER_TIMEOUT",
		"server.read_timeout":         "SERVER_READ_TIMEOUT",
		"server.write_timeout":        "SERVER_WRITE_TIMEOUT",
		"server.idle_timeout":         "SERVER_IDLE_TIMEOUT",
		"database.dsn":                "DATABASE_DSN",
		"database.admin_dsn":          "DATABASE_ADMIN_DSN",
		"database.max_idle_conns":     "DATABASE_MAX_IDLE_CONNS",
		"database.max_open_conns":     "DATABASE_MAX_OPEN_CONNS",
		"database.max_conn_lifetime":  "DATABASE_MAX_CONN_LIFETIME",
		"redis.cache_dsn":             "REDIS_CACHE_DSN",
		"redis.max_retry":             "REDIS_MAX_RETRY",
		"redis.max_idle_conns":        "REDIS_MAX_IDLE_CONNS",
		"redis.max_active_conns":      "REDIS_MAX_ACTIVE_CONNS",
		"redis.max_conn_lifetime":     "REDIS_MAX_CONN_LIFETIME",
		"jwt.secret":                  "JWT_SECRET",
		"auth.pin_ttl":                "AUTH_PIN_TTL",
		"auth.pin_max_attempts":       "AUTH_PIN_MAX_ATTEMPTS",
		"auth.pin_resend_cooldown":    "AUTH_PIN_RESEND_COOLDOWN",
		"auth.max_active_sessions":    "AUTH_MAX_ACTIVE_SESSIONS",
		"auth.public_ip_rate_limit":   "AUTH_PUBLIC_IP_RATE_LIMIT",
		"auth.public_ip_rate_window":  "AUTH_PUBLIC_IP_RATE_WINDOW",
		"auth.login_rate_limit":       "AUTH_LOGIN_RATE_LIMIT",
		"auth.login_rate_window":      "AUTH_LOGIN_RATE_WINDOW",
		"smtp.enabled":                "SMTP_ENABLED",
		"smtp.host":                   "SMTP_HOST",
		"smtp.port":                   "SMTP_PORT",
		"smtp.username":               "SMTP_USERNAME",
		"smtp.password":               "SMTP_PASSWORD",
		"smtp.from":                   "SMTP_FROM",
		"smtp.from_name":              "SMTP_FROM_NAME",
		"smtp.timeout":                "SMTP_TIMEOUT",
		"smtp.use_starttls":           "SMTP_USE_STARTTLS",
		"storage.enabled":             "STORAGE_ENABLED",
		"storage.provider":            "STORAGE_PROVIDER",
		"storage.bucket":              "SUPABASE_STORAGE_BUCKET",
		"storage.max_file_size_bytes": "SUPABASE_STORAGE_MAX_FILE_SIZE_BYTES",
		"storage.signed_url_ttl":      "SUPABASE_STORAGE_SIGNED_URL_TTL",
		"storage.supabase.url":        "SUPABASE_URL",
		"storage.supabase.secret_key": "SUPABASE_SECRET_KEY",
	}
	for key, envName := range bindings {
		_ = v.BindEnv(key, envName)
	}
	_ = v.BindEnv("server.port", "SERVER_PORT", "PORT")
	_ = v.BindEnv("jwt.access_ttl", "JWT_ACCESS_TTL", "TOKEN_ACCESS_TOKEN_DURATION")
	_ = v.BindEnv("jwt.refresh_ttl", "JWT_REFRESH_TTL", "TOKEN_REFRESH_TOKEN_DURATION")
	_ = v.BindEnv("jwt.access_ttl_minutes", "JWT_ACCESS_TTL_MINUTES")
	_ = v.BindEnv("jwt.refresh_ttl_days", "JWT_REFRESH_TTL_DAYS")
	_ = v.BindEnv("auth.forgot_password_rate_limit", "AUTH_FORGOT_PASSWORD_RATE_LIMIT", "TOKEN_FORGOT_PASSWORD_RATE_LIMIT")
	_ = v.BindEnv("auth.forgot_password_rate_window", "AUTH_FORGOT_PASSWORD_RATE_WINDOW", "TOKEN_FORGOT_PASSWORD_RATE_WINDOW")
}

func applyLegacyJWTDurationFallbacks(v *viper.Viper, cfg *EnvConfig) {
	if !v.InConfig("jwt.access_ttl") && !anyNonBlankEnv("JWT_ACCESS_TTL", "TOKEN_ACCESS_TOKEN_DURATION") && cfg.JWT.AccessTTLMinutes > 0 {
		cfg.JWT.AccessTTL = time.Duration(cfg.JWT.AccessTTLMinutes) * time.Minute
	}
	if !v.InConfig("jwt.refresh_ttl") && !anyNonBlankEnv("JWT_REFRESH_TTL", "TOKEN_REFRESH_TOKEN_DURATION") && cfg.JWT.RefreshTTLDays > 0 {
		cfg.JWT.RefreshTTL = time.Duration(cfg.JWT.RefreshTTLDays) * 24 * time.Hour
	}
}

func anyNonBlankEnv(names ...string) bool {
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func normalize(c *EnvConfig) {
	c.Env = strings.ToLower(strings.TrimSpace(c.Env))
	c.LogLevel = strings.ToLower(strings.TrimSpace(c.LogLevel))
	c.Server.Port = strings.TrimSpace(c.Server.Port)
	c.Server.ClientIPHeader = strings.TrimSpace(c.Server.ClientIPHeader)
	c.Database.DSN = strings.TrimSpace(c.Database.DSN)
	c.Database.AdminDSN = strings.TrimSpace(c.Database.AdminDSN)
	c.Redis.CacheDSN = strings.TrimSpace(c.Redis.CacheDSN)
	c.SMTP.Host = strings.TrimSpace(c.SMTP.Host)
	c.SMTP.Username = strings.TrimSpace(c.SMTP.Username)
	c.SMTP.From = strings.TrimSpace(c.SMTP.From)
	c.SMTP.FromName = strings.TrimSpace(c.SMTP.FromName)
	c.Storage.Provider = strings.ToLower(strings.TrimSpace(c.Storage.Provider))
	c.Storage.Bucket = strings.TrimSpace(c.Storage.Bucket)
	c.Storage.Supabase.URL = strings.TrimRight(strings.TrimSpace(c.Storage.Supabase.URL), "/")
	c.Storage.Supabase.SecretKey = strings.TrimSpace(c.Storage.Supabase.SecretKey)

	if len(c.Server.CORSAllowedOrigins) == 0 && c.Env == "development" {
		c.Server.CORSAllowedOrigins = []string{"http://localhost:3000", "http://localhost:5173"}
	}
	seen := make(map[string]struct{}, len(c.Server.CORSAllowedOrigins))
	origins := make([]string, 0, len(c.Server.CORSAllowedOrigins))
	for _, origin := range c.Server.CORSAllowedOrigins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin == "" {
			continue
		}
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	c.Server.CORSAllowedOrigins = origins
}

func (c *EnvConfig) Validate() error {
	var errs []error
	requireTransportSecurity := c.Env == "staging" || c.Env == "production"
	if c.Env != "development" && c.Env != "staging" && c.Env != "production" && c.Env != "test" {
		errs = append(errs, fmt.Errorf("env must be development, staging, production, or test"))
	}
	if _, err := logrus.ParseLevel(c.LogLevel); err != nil {
		errs = append(errs, fmt.Errorf("log_level is invalid"))
	}
	if c.GracefulShutdownTimeout <= 0 {
		errs = append(errs, fmt.Errorf("graceful_shutdown_timeout must be positive"))
	}

	port, err := strconv.Atoi(c.Server.Port)
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("server.port must be between 1 and 65535"))
	}
	if len(c.Server.CORSAllowedOrigins) == 0 {
		errs = append(errs, fmt.Errorf("server.cors_allowed_origins must contain at least one origin"))
	}
	for _, origin := range c.Server.CORSAllowedOrigins {
		if err := validateOrigin(origin, requireTransportSecurity); err != nil {
			errs = append(errs, fmt.Errorf("server.cors_allowed_origins: %w", err))
		}
	}
	if c.Server.ClientIPHeader != "" &&
		!strings.EqualFold(c.Server.ClientIPHeader, "X-Forwarded-For") &&
		!strings.EqualFold(c.Server.ClientIPHeader, "CF-Connecting-IP") {
		errs = append(errs, fmt.Errorf("server.client_ip_header must be X-Forwarded-For or CF-Connecting-IP"))
	}
	if requireTransportSecurity && c.Auth.PublicIPRateLimit > 0 && c.Server.ClientIPHeader == "" {
		errs = append(errs, fmt.Errorf("server.client_ip_header is required for public auth rate limiting behind a trusted staging/production proxy"))
	}
	for name, duration := range map[string]time.Duration{
		"server.read_header_timeout": c.Server.ReadHeaderTimeout,
		"server.read_timeout":        c.Server.ReadTimeout,
		"server.write_timeout":       c.Server.WriteTimeout,
		"server.idle_timeout":        c.Server.IdleTimeout,
	} {
		if duration <= 0 {
			errs = append(errs, fmt.Errorf("%s must be positive", name))
		}
	}

	if err := validatePostgresDSN(c.Database.DSN, "database.dsn", requireTransportSecurity, false); err != nil {
		errs = append(errs, err)
	}
	// The serving process needs only DATABASE_DSN. Privileged commands enforce
	// an explicit DATABASE_ADMIN_DSN themselves so the server does not need to
	// carry DDL/TRUNCATE credentials at runtime.
	if c.Database.AdminDSN != "" {
		if err := validatePostgresDSN(c.Database.AdminDSN, "database.admin_dsn", requireTransportSecurity, true); err != nil {
			errs = append(errs, err)
		}
	}
	if c.Database.MaxIdleConns < 0 {
		errs = append(errs, fmt.Errorf("database.max_idle_conns cannot be negative"))
	}
	if c.Database.MaxOpenConns <= 0 {
		errs = append(errs, fmt.Errorf("database.max_open_conns must be positive"))
	}
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		errs = append(errs, fmt.Errorf("database.max_idle_conns cannot exceed max_open_conns"))
	}
	if c.Database.MaxConnLifetime <= 0 {
		errs = append(errs, fmt.Errorf("database.max_conn_lifetime must be positive"))
	}

	if err := validateRedisDSN(c.Redis.CacheDSN, requireTransportSecurity); err != nil {
		errs = append(errs, err)
	}
	if c.Redis.MaxRetry <= 0 {
		errs = append(errs, fmt.Errorf("redis.max_retry must be positive"))
	}
	if c.Redis.MaxIdleConns < 0 || c.Redis.MaxActiveConns <= 0 || c.Redis.MaxIdleConns > c.Redis.MaxActiveConns {
		errs = append(errs, fmt.Errorf("redis connection pool limits are invalid"))
	}
	if c.Redis.MaxConnLifetime <= 0 {
		errs = append(errs, fmt.Errorf("redis.max_conn_lifetime must be positive"))
	}

	if len(c.JWT.Secret) < 32 || strings.TrimSpace(c.JWT.Secret) != c.JWT.Secret {
		errs = append(errs, fmt.Errorf("jwt.secret must contain at least 32 non-whitespace-surrounded characters"))
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.RefreshTTL <= 0 || c.JWT.RefreshTTL <= c.JWT.AccessTTL {
		errs = append(errs, fmt.Errorf("jwt TTLs must be positive and refresh_ttl must exceed access_ttl"))
	}

	for name, duration := range map[string]time.Duration{
		"auth.pin_ttl":                     c.Auth.PINTTL,
		"auth.pin_resend_cooldown":         c.Auth.PINResendCooldown,
		"auth.login_rate_window":           c.Auth.LoginRateWindow,
		"auth.public_ip_rate_window":       c.Auth.PublicIPRateWindow,
		"auth.forgot_password_rate_window": c.Auth.ForgotPasswordRateWindow,
	} {
		if duration <= 0 {
			errs = append(errs, fmt.Errorf("%s must be positive", name))
		}
	}
	if c.Auth.PINMaxAttempts <= 0 || c.Auth.LoginRateLimit <= 0 || c.Auth.ForgotPasswordRateLimit <= 0 || c.Auth.PublicIPRateLimit <= 0 {
		errs = append(errs, fmt.Errorf("auth attempt and rate limits must be positive"))
	}
	if c.Auth.MaxActiveSessions < 1 || c.Auth.MaxActiveSessions > 100 {
		errs = append(errs, fmt.Errorf("auth.max_active_sessions must be between 1 and 100"))
	}
	if requireTransportSecurity {
		secret := strings.ToLower(c.JWT.Secret)
		if strings.Contains(secret, "development-only") || strings.Contains(secret, "change-this") || strings.Contains(secret, "replace-me") {
			errs = append(errs, fmt.Errorf("jwt.secret must not use an example placeholder in staging or production"))
		}
	}

	if c.SMTP.Enabled {
		if c.SMTP.Host == "" || c.SMTP.Port < 1 || c.SMTP.Port > 65535 {
			errs = append(errs, fmt.Errorf("smtp host and port are invalid"))
		}
		if c.SMTP.Username == "" || c.SMTP.Password == "" {
			errs = append(errs, fmt.Errorf("smtp credentials are required when smtp is enabled"))
		}
		if _, err := mail.ParseAddress(c.SMTP.From); err != nil {
			errs = append(errs, fmt.Errorf("smtp.from must be a valid email address"))
		}
		if c.SMTP.Timeout <= 0 {
			errs = append(errs, fmt.Errorf("smtp.timeout must be positive"))
		} else if c.Auth.PINTTL <= c.SMTP.Timeout+time.Second {
			errs = append(errs, fmt.Errorf("auth.pin_ttl must exceed smtp.timeout by more than one second"))
		} else if c.Server.WriteTimeout < c.SMTP.Timeout+5*time.Second {
			errs = append(errs, fmt.Errorf("server.write_timeout must provide at least five seconds of headroom beyond smtp.timeout"))
		}
		if !c.SMTP.UseSTARTTLS {
			errs = append(errs, fmt.Errorf("smtp.use_starttls must be true when smtp is enabled"))
		}
	}

	return errors.Join(errs...)
}

func validatePostgresDSN(raw, field string, requireTLS, requireDirect bool) error {
	if err := validateURL(raw, field, "postgres", "postgresql"); err != nil {
		return err
	}
	cfg, err := pgx.ParseConfig(raw)
	if err != nil || cfg.Host == "" || cfg.Database == "" || cfg.User == "" {
		return fmt.Errorf("%s must resolve to a host, database, and user", field)
	}
	if requireTLS {
		parsedURL, parseErr := url.Parse(raw)
		if parseErr != nil || !strings.EqualFold(parsedURL.Query().Get("sslmode"), "verify-full") {
			return fmt.Errorf("%s must use sslmode=verify-full in staging and production", field)
		}
		if cfg.TLSConfig == nil {
			return fmt.Errorf("%s must enable TLS in staging and production", field)
		}
		for _, fallback := range cfg.Fallbacks {
			if fallback.TLSConfig == nil {
				return fmt.Errorf("%s must not allow a non-TLS fallback in staging and production", field)
			}
		}
	}
	if requireDirect {
		if len(cfg.Fallbacks) != 0 {
			return fmt.Errorf("%s must contain exactly one direct target without fallbacks", field)
		}
		hosts := []string{cfg.Host}
		for _, fallback := range cfg.Fallbacks {
			hosts = append(hosts, fallback.Host)
		}
		for _, host := range hosts {
			normalized := strings.ToLower(host)
			if strings.Contains(normalized, "-pooler") || strings.Contains(normalized, "pgbouncer") {
				return fmt.Errorf("%s must use a direct/non-pooler endpoint", field)
			}
		}
	}
	return nil
}

func validateURL(raw, field string, allowedSchemes ...string) error {
	if raw == "" {
		return fmt.Errorf("%s is required", field)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%s must be a valid URL", field)
	}
	for _, scheme := range allowedSchemes {
		if strings.EqualFold(u.Scheme, scheme) {
			return nil
		}
	}
	return fmt.Errorf("%s has an unsupported scheme", field)
}

func validateRedisDSN(raw string, requireTLS bool) error {
	if err := validateURL(raw, "redis.cache_dsn", "redis", "rediss"); err != nil {
		return err
	}
	opts, err := goredis.ParseURL(raw)
	if err != nil {
		return errors.New("redis.cache_dsn must be a valid Redis URL")
	}
	if !requireTLS {
		return nil
	}

	var errs []error
	if opts.TLSConfig == nil {
		errs = append(errs, errors.New("redis.cache_dsn must use rediss:// in staging and production"))
	} else if opts.TLSConfig.InsecureSkipVerify {
		errs = append(errs, errors.New("redis.cache_dsn must not disable TLS certificate verification"))
	}
	if opts.Password == "" {
		errs = append(errs, errors.New("redis.cache_dsn must include authentication credentials in staging and production"))
	}
	return errors.Join(errs...)
}

func validateOrigin(origin string, requireHTTPS bool) error {
	if strings.Contains(origin, "*") {
		return fmt.Errorf("wildcards are not allowed")
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("%q is not an exact HTTP(S) origin", origin)
	}
	if requireHTTPS && u.Scheme != "https" {
		return fmt.Errorf("%q must use HTTPS in staging and production", origin)
	}
	return nil
}
