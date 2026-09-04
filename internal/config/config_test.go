package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigUsesRenderPortAndLegacyDurationFallbacks(t *testing.T) {
	t.Chdir(t.TempDir())
	configureStorageEnv(t)
	t.Setenv("ENV", "development")
	t.Setenv("SERVER_PORT", "")
	t.Setenv("PORT", "9090")
	t.Setenv("SERVER_CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173")
	t.Setenv("DATABASE_DSN", "postgresql://postgres:postgres@localhost:5432/medikaone?sslmode=disable")
	t.Setenv("REDIS_CACHE_DSN", "redis://localhost:6379/0")
	t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("JWT_ACCESS_TTL", "")
	t.Setenv("JWT_REFRESH_TTL", "")
	t.Setenv("TOKEN_ACCESS_TOKEN_DURATION", "1h")
	t.Setenv("TOKEN_REFRESH_TOKEN_DURATION", "30h")
	t.Setenv("JWT_ACCESS_TTL_MINUTES", "5")
	t.Setenv("JWT_REFRESH_TTL_DAYS", "2")
	t.Setenv("SMTP_ENABLED", "false")

	if err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if Env.Server.Port != "9090" {
		t.Fatalf("server port = %q, want Render PORT fallback", Env.Server.Port)
	}
	if Env.JWT.AccessTTL != time.Hour || Env.JWT.RefreshTTL != 30*time.Hour {
		t.Fatalf("unexpected JWT duration aliases: access=%s refresh=%s", Env.JWT.AccessTTL, Env.JWT.RefreshTTL)
	}
	if Env.SMTP.Enabled {
		t.Fatal("SMTP_ENABLED=false was not applied")
	}
}

func TestLoadConfigUsesLegacyJWTUnitFallbacks(t *testing.T) {
	t.Chdir(t.TempDir())
	configureStorageEnv(t)
	t.Setenv("ENV", "development")
	t.Setenv("SERVER_CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	t.Setenv("DATABASE_DSN", "postgresql://postgres:postgres@localhost:5432/medikaone?sslmode=disable")
	t.Setenv("REDIS_CACHE_DSN", "redis://localhost:6379/0")
	t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("JWT_ACCESS_TTL", "")
	t.Setenv("JWT_REFRESH_TTL", "")
	t.Setenv("TOKEN_ACCESS_TOKEN_DURATION", "")
	t.Setenv("TOKEN_REFRESH_TOKEN_DURATION", "")
	t.Setenv("JWT_ACCESS_TTL_MINUTES", "45")
	t.Setenv("JWT_REFRESH_TTL_DAYS", "7")
	t.Setenv("SMTP_ENABLED", "false")

	if err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if Env.JWT.AccessTTL != 45*time.Minute || Env.JWT.RefreshTTL != 7*24*time.Hour {
		t.Fatalf("legacy JWT fallbacks = access %s, refresh %s", Env.JWT.AccessTTL, Env.JWT.RefreshTTL)
	}
}

func TestCommandValidationScopesDoNotRequireUnrelatedWebSecrets(t *testing.T) {
	databaseOnly := EnvConfig{
		Env:      "development",
		LogLevel: "info",
		Database: Database{
			DSN:             "postgresql://postgres:postgres@localhost:5432/medikaone?sslmode=disable",
			MaxIdleConns:    1,
			MaxOpenConns:    2,
			MaxConnLifetime: time.Minute,
		},
	}
	if err := databaseOnly.ValidateFor(ValidationDatabase); err != nil {
		t.Fatalf("database command scope rejected minimal database config: %v", err)
	}
	if err := databaseOnly.ValidateFor(ValidationServer); err == nil {
		t.Fatal("server scope unexpectedly accepted config without Redis/JWT/CORS")
	}

	targetOnly := EnvConfig{
		Env:      "development",
		LogLevel: "info",
		Database: Database{AdminDSN: "postgresql://owner:secret@localhost:5432/medikaone?sslmode=disable"},
	}
	if err := targetOnly.ValidateFor(ValidationDatabaseTarget); err != nil {
		t.Fatalf("database target scope rejected admin-only config: %v", err)
	}

	maintenance := databaseOnly
	maintenance.Server.WriteTimeout = 30 * time.Second
	maintenance.Redis = Redis{
		CacheDSN:        "redis://localhost:6379/0",
		MaxRetry:        1,
		MaxIdleConns:    1,
		MaxActiveConns:  2,
		MaxConnLifetime: time.Minute,
	}
	if err := maintenance.ValidateFor(ValidationMaintenance); err != nil {
		t.Fatalf("maintenance scope rejected config without web secrets: %v", err)
	}
}

func TestMigrationValidationRequiresRedisOnlyForSecureEnvironments(t *testing.T) {
	cfg := EnvConfig{
		Env:      "development",
		LogLevel: "info",
		Database: Database{
			DSN:             "postgresql://postgres:postgres@localhost:5432/medikaone?sslmode=disable",
			MaxIdleConns:    1,
			MaxOpenConns:    2,
			MaxConnLifetime: time.Minute,
		},
	}
	if err := cfg.ValidateFor(ValidationMigration); err != nil {
		t.Fatalf("development migration unexpectedly required Redis: %v", err)
	}

	cfg.Env = "staging"
	cfg.Database.DSN = "postgresql://app:secret@db.example.com/medikaone?sslmode=verify-full"
	if err := cfg.ValidateFor(ValidationMigration); err == nil || !strings.Contains(err.Error(), "redis.cache_dsn") {
		t.Fatalf("staging migration error = %v, want Redis requirement", err)
	}
}

func TestAuthRedisKeyPrefixSeparatesDatabasePortsWithoutUsingCredentials(t *testing.T) {
	previous := Env
	t.Cleanup(func() { Env = previous })

	Env = &EnvConfig{Env: "staging", Database: Database{
		DSN: "postgresql://app_one:secret-one@db.example.com:5432/medikaone?sslmode=verify-full",
	}}
	first := AuthRedisKeyPrefix()
	Env.Database.DSN = "postgresql://app_two:secret-two@db.example.com:5432/medikaone?sslmode=verify-full"
	if rotatedCredentials := AuthRedisKeyPrefix(); rotatedCredentials != first {
		t.Fatal("Redis namespace changed when only database credentials changed")
	}
	Env.Database.DSN = "postgresql://app_two:secret-two@db.example.com:6432/medikaone?sslmode=verify-full"
	if otherPort := AuthRedisKeyPrefix(); otherPort == first {
		t.Fatal("Redis namespace did not separate database targets on different ports")
	}
}

func TestValidateRejectsUnsafeProductionTransport(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "production"
	cfg.Server.CORSAllowedOrigins = []string{"http://app.example.com"}
	cfg.Database.DSN = "postgresql://user:pass@db.example.com/app?sslmode=disable"
	cfg.Redis.CacheDSN = "redis://default:secret@redis.example.com:6379"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected an error")
	}
	for _, message := range []string{"must use HTTPS", "database.dsn must use sslmode=verify-full", "redis.cache_dsn must use rediss://"} {
		if !strings.Contains(err.Error(), message) {
			t.Errorf("Validate() error %q does not contain %q", err, message)
		}
	}
}

func TestValidateRejectsExampleJWTSecretOutsideDevelopment(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "staging"
	cfg.Server.CORSAllowedOrigins = []string{"https://staging.example.com"}
	cfg.Server.ClientIPHeader = "X-Forwarded-For"
	cfg.Database.DSN = "postgresql://user:pass@db.example.com/app?sslmode=verify-full"
	cfg.Database.AdminDSN = "postgresql://user:pass@db.example.com/app?sslmode=verify-full"
	cfg.Redis.CacheDSN = "rediss://default:secret@redis.example.com:6379"
	cfg.JWT.Secret = "development-only-change-this-secret"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "example placeholder") {
		t.Fatalf("Validate() error = %v, want rejected example JWT secret", err)
	}
}

func TestValidateRequiresTrustedClientIPHeaderForSecureProxyRateLimit(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "staging"
	cfg.Server.CORSAllowedOrigins = []string{"https://staging.example.com"}
	cfg.Database.DSN = "postgresql://user:pass@db.example.com/app?sslmode=verify-full"
	cfg.Database.AdminDSN = "postgresql://user:pass@db.example.com/app?sslmode=verify-full"
	cfg.Redis.CacheDSN = "rediss://default:secret@redis.example.com:6379"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "server.client_ip_header is required") {
		t.Fatalf("Validate() error = %v, want missing trusted proxy header", err)
	}

	cfg.Server.ClientIPHeader = "X-Forwarded-For"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid trusted proxy config rejected: %v", err)
	}
}

func TestValidateRequiresPublicBaseURLInSecureEnvironment(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "staging"
	cfg.Server.BaseURL = ""
	cfg.Server.CORSAllowedOrigins = []string{"https://staging.example.com"}
	cfg.Server.ClientIPHeader = "X-Forwarded-For"
	cfg.Database.DSN = "postgresql://user:pass@db.example.com/app?sslmode=verify-full"
	cfg.Redis.CacheDSN = "rediss://default:secret@redis.example.com:6379"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "server.base_url is required") {
		t.Fatalf("Validate() error = %v, want missing public base URL rejection", err)
	}
}

func TestValidateAllowsServerWithoutAdminDSNButRejectsPooledAdminDSN(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "staging"
	cfg.Server.CORSAllowedOrigins = []string{"https://staging.example.com"}
	cfg.Server.ClientIPHeader = "X-Forwarded-For"
	cfg.Database.DSN = "postgresql://user:pass@ep-example-pooler.example.com/app?sslmode=verify-full"
	cfg.Redis.CacheDSN = "rediss://default:secret@redis.example.com:6379"

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("server-only secure config without admin DSN rejected: %v", err)
	}

	cfg.Database.AdminDSN = "postgresql://user:pass@ep-example-pooler.example.com/app?sslmode=verify-full"
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "direct/non-pooler") {
		t.Fatalf("Validate() error = %v, want pooled admin DSN rejection", err)
	}

	cfg.Database.AdminDSN = "postgresql://user:pass@ep-example.example.com/app?sslmode=verify-full"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid direct admin DSN rejected: %v", err)
	}
}

func TestValidateUsesEffectivePGXTLSConfiguration(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "production"
	cfg.Server.CORSAllowedOrigins = []string{"https://app.example.com"}
	cfg.Server.ClientIPHeader = "X-Forwarded-For"
	cfg.Database.DSN = "postgresql://user:pass@db.example.com/app?sslmode=prefer"
	cfg.Database.AdminDSN = "postgresql://user:pass@db.example.com/app?sslmode=verify-full"
	cfg.Redis.CacheDSN = "rediss://default:secret@redis.example.com:6379"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sslmode=verify-full") {
		t.Fatalf("Validate() error = %v, want weak sslmode rejection", err)
	}
}

func TestValidateRequiresRedisAuthenticationOutsideDevelopment(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "staging"
	cfg.Server.CORSAllowedOrigins = []string{"https://staging.example.com"}
	cfg.Server.ClientIPHeader = "X-Forwarded-For"
	cfg.Database.DSN = "postgresql://user:pass@db.example.com/app?sslmode=verify-full"
	cfg.Redis.CacheDSN = "rediss://redis.example.com:6379"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "authentication credentials") {
		t.Fatalf("Validate() error = %v, want missing Redis authentication rejection", err)
	}

	cfg.Redis.CacheDSN = "rediss://default:secret@redis.example.com:6379"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("authenticated Redis DSN rejected: %v", err)
	}
}

func TestValidateRejectsRedisTLSVerificationBypass(t *testing.T) {
	cfg := validConfig()
	cfg.Env = "staging"
	cfg.Server.CORSAllowedOrigins = []string{"https://staging.example.com"}
	cfg.Server.ClientIPHeader = "X-Forwarded-For"
	cfg.Database.DSN = "postgresql://user:pass@db.example.com/app?sslmode=verify-full"
	cfg.Database.AdminDSN = "postgresql://owner:pass@db.example.com/app?sslmode=verify-full"
	cfg.Redis.CacheDSN = "rediss://default:secret@redis.example.com:6379?skip_verify=true"

	for _, validate := range []struct {
		name string
		fn   func() error
	}{
		{name: "server", fn: cfg.Validate},
		{name: "maintenance", fn: func() error { return cfg.ValidateFor(ValidationMaintenance) }},
	} {
		t.Run(validate.name, func(t *testing.T) {
			err := validate.fn()
			if err == nil || !strings.Contains(err.Error(), "must not disable TLS certificate verification") {
				t.Fatalf("validation error = %v, want TLS verification bypass rejection", err)
			}
		})
	}
}

func TestValidateRequiresHTTPHeadroomForSynchronousSMTP(t *testing.T) {
	cfg := validConfig()
	cfg.SMTP = SMTP{
		Enabled: true, Host: "smtp.example.com", Port: 2525,
		Username: "apikey", Password: "secret", From: "sender@example.com",
		Timeout: 30 * time.Second, UseSTARTTLS: true,
	}
	cfg.Server.WriteTimeout = 30 * time.Second
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "headroom") {
		t.Fatalf("Validate() error = %v, want SMTP/write timeout headroom rejection", err)
	}
	cfg.SMTP.Timeout = 15 * time.Second
	if err := cfg.Validate(); err != nil {
		t.Fatalf("safe SMTP/write timeout headroom rejected: %v", err)
	}
}

func TestValidateRejectsMissingSecretsAndInvalidLimits(t *testing.T) {
	cfg := validConfig()
	cfg.JWT.Secret = "short"
	cfg.Auth.PINMaxAttempts = 0
	cfg.Database.MaxIdleConns = cfg.Database.MaxOpenConns + 1

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected an error")
	}
	for _, message := range []string{"jwt.secret", "auth attempt", "max_idle_conns"} {
		if !strings.Contains(err.Error(), message) {
			t.Errorf("Validate() error %q does not contain %q", err, message)
		}
	}
}

func TestValidateRejectsUnsafeMedicalStorage(t *testing.T) {
	cfg := validConfig()
	cfg.Storage.MedicalBucket = cfg.Storage.Bucket
	cfg.Storage.MaxFileSizeBytes = 10*1024*1024 + 1

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected an error")
	}
	for _, message := range []string{"medical_bucket must be separate", "max_file_size_bytes"} {
		if !strings.Contains(err.Error(), message) {
			t.Errorf("Validate() error %q does not contain %q", err, message)
		}
	}
}

func TestValidateRejectsSharedProfileStorage(t *testing.T) {
	cfg := validConfig()
	cfg.Storage.ProfileBucket = cfg.Storage.MedicalBucket
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "profile_bucket must be separate") {
		t.Fatalf("Validate() error = %v, want profile bucket separation error", err)
	}
}

func configureStorageEnv(t *testing.T) {
	t.Helper()
	t.Setenv("STORAGE_ENABLED", "true")
	t.Setenv("STORAGE_PROVIDER", "supabase")
	t.Setenv("SUPABASE_STORAGE_BUCKET", "doctor-contracts")
	t.Setenv("SUPABASE_MEDICAL_STORAGE_BUCKET", "medical-records")
	t.Setenv("SUPABASE_PROFILE_STORAGE_BUCKET", "profile-images")
	t.Setenv("SUPABASE_URL", "https://project.supabase.co")
	t.Setenv("SUPABASE_SECRET_KEY", "sb_secret_test")
}

func validConfig() *EnvConfig {
	return &EnvConfig{
		Env:                     "development",
		LogLevel:                "info",
		GracefulShutdownTimeout: 30 * time.Second,
		Server: Server{
			Port:               "8080",
			BaseURL:            "https://api.example.com",
			CORSAllowedOrigins: []string{"http://localhost:3000"},
			ReadHeaderTimeout:  5 * time.Second,
			ReadTimeout:        15 * time.Second,
			WriteTimeout:       30 * time.Second,
			IdleTimeout:        60 * time.Second,
		},
		Database: Database{
			DSN:             "postgresql://postgres:postgres@localhost:5432/medikaone?sslmode=disable",
			MaxIdleConns:    5,
			MaxOpenConns:    20,
			MaxConnLifetime: time.Hour,
		},
		Redis: Redis{
			CacheDSN:        "redis://localhost:6379/0",
			MaxRetry:        3,
			MaxIdleConns:    5,
			MaxActiveConns:  20,
			MaxConnLifetime: time.Hour,
		},
		JWT: JWTConfig{
			Secret:     "01234567890123456789012345678901",
			AccessTTL:  15 * time.Minute,
			RefreshTTL: 30 * 24 * time.Hour,
		},
		Auth: AuthConfig{
			PINTTL:                   10 * time.Minute,
			PINMaxAttempts:           5,
			PINResendCooldown:        time.Minute,
			MaxActiveSessions:        10,
			PublicIPRateLimit:        30,
			PublicIPRateWindow:       15 * time.Minute,
			LoginRateLimit:           10,
			LoginRateWindow:          15 * time.Minute,
			ForgotPasswordRateLimit:  3,
			ForgotPasswordRateWindow: time.Hour,
		},
		SMTP: SMTP{Enabled: false},
		Storage: Storage{
			Enabled: true, Provider: "supabase", Bucket: "doctor-contracts", MedicalBucket: "medical-records", ProfileBucket: "profile-images",
			MaxFileSizeBytes: 10 * 1024 * 1024, SignedURLTTL: 5 * time.Minute,
			Supabase: SupabaseStorage{URL: "https://project.supabase.co", SecretKey: "sb_secret_test"},
		},
	}
}
