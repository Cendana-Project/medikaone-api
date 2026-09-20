// Package dbtarget describes managed PostgreSQL routing without retaining secrets.
package dbtarget

import (
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

var projectRefPattern = regexp.MustCompile(`^[a-z0-9]{20}$`)

type SupabaseEndpoint struct {
	ProjectRef    string
	Role          string
	SessionPooler bool
}

func (e SupabaseEndpoint) DirectHost() string {
	return "db." + e.ProjectRef + ".supabase.co"
}

// Supabase recognizes the hosted endpoints documented by Supabase. Other
// PostgreSQL hosts keep their original routing and role semantics.
// Shared poolers route by role.project-ref, while Postgres sees only role.
func Supabase(cfg *pgx.ConnConfig) (*SupabaseEndpoint, error) {
	if cfg == nil {
		return nil, errors.New("invalid database connection")
	}
	host := strings.ToLower(cfg.Host)
	shared := strings.HasSuffix(host, ".pooler.supabase.com")
	direct := strings.HasPrefix(host, "db.") && strings.HasSuffix(host, ".supabase.co")
	if !shared && !direct {
		return nil, nil
	}
	if cfg.Port != 5432 {
		return nil, errors.New("Supabase requires direct or session pooler port 5432; transaction pooling is incompatible with prepared statements and administrative sessions")
	}
	if len(cfg.Fallbacks) != 0 {
		return nil, errors.New("Supabase connection must contain exactly one target without fallbacks")
	}
	endpoint := &SupabaseEndpoint{Role: cfg.User, SessionPooler: shared}
	if shared {
		role, ref, ok := splitPoolerUser(cfg.User)
		if !ok {
			return nil, errors.New("Supabase session pooler username must be role.project-ref")
		}
		endpoint.Role, endpoint.ProjectRef = role, ref
	} else {
		endpoint.ProjectRef = strings.TrimSuffix(strings.TrimPrefix(host, "db."), ".supabase.co")
	}
	if endpoint.Role == "" || !projectRefPattern.MatchString(endpoint.ProjectRef) {
		return nil, errors.New("invalid Supabase project reference or database role")
	}
	return endpoint, nil
}

func splitPoolerUser(user string) (string, string, bool) {
	index := strings.LastIndexByte(user, '.')
	if index <= 0 || index == len(user)-1 {
		return "", "", false
	}
	return user[:index], user[index+1:], true
}

// ValidateAdmin allows persistent connections needed by the migration guard.
// Supabase session pooling reserves a server connection for each client session.
func ValidateAdmin(cfg *pgx.ConnConfig) error {
	if cfg == nil || cfg.Host == "" || cfg.Database == "" || cfg.User == "" {
		return errors.New("invalid database admin target")
	}
	if len(cfg.Fallbacks) != 0 {
		return errors.New("database admin DSN must contain exactly one target without fallbacks")
	}
	endpoint, err := Supabase(cfg)
	if err != nil {
		return err
	}
	if endpoint != nil {
		return nil
	}
	host := strings.ToLower(cfg.Host)
	if strings.Contains(host, "-pooler") || strings.Contains(host, "pgbouncer") {
		return errors.New("database admin DSN must use a direct/non-pooler endpoint or Supabase session pooler")
	}
	return nil
}

// VerifySession keeps checking the actual database, role and public schema.
// Only a recognized Supabase pooler can translate the routing username.
func VerifySession(cfg *pgx.ConnConfig, database, sessionUser, currentUser, schema string) error {
	endpoint, err := Supabase(cfg)
	if err != nil {
		return err
	}
	role := cfg.User
	if endpoint != nil {
		role = endpoint.Role
	}
	if database != cfg.Database || sessionUser != role || currentUser != role || schema != "public" {
		return errors.New("connected database/user/schema does not match the configured public target")
	}
	return nil
}
