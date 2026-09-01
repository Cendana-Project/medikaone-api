-- +goose Up
-- +goose StatementBegin
-- extension untuk gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =========================
-- USERS
-- =========================
CREATE TABLE users (
                       id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       email          VARCHAR(190) NOT NULL UNIQUE,
                       username       VARCHAR(64) UNIQUE,
                       first_name     VARCHAR(100),
                       last_name      VARCHAR(100),
                       phone          VARCHAR(32),
                       dob            DATE,
                       address        TEXT,
                       gender         VARCHAR(1),             -- 'L' | 'P'
                       nik            VARCHAR(16) UNIQUE,
                       password_hash  TEXT NOT NULL,
                       status         VARCHAR(16) NOT NULL DEFAULT 'pending', -- pending | active | blocked
                       verified_at    TIMESTAMPTZ,
                       created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                       updated_at     TIMESTAMPTZ,
                       deleted_at     TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
CREATE UNIQUE INDEX IF NOT EXISTS ux_users_username_lower ON users ((lower(username)));

-- =========================
-- ROLES
-- =========================
CREATE TABLE roles (
                       id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       name        VARCHAR(100) NOT NULL,
                       slug        VARCHAR(50)  NOT NULL UNIQUE,
                       description TEXT,
                       active      BOOLEAN      NOT NULL DEFAULT TRUE,
                       created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
                       updated_at  TIMESTAMPTZ,
                       deleted_at  TIMESTAMPTZ
);

-- =========================
-- PERMISSIONS
-- =========================
CREATE TABLE permissions (
                             id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                             name        VARCHAR(96) NOT NULL,
                             slug        VARCHAR(96) NOT NULL UNIQUE,
                             description TEXT,
                             is_active   BOOLEAN NOT NULL DEFAULT TRUE,
                             created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                             updated_at  TIMESTAMPTZ,
                             deleted_at  TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_permissions_name ON permissions(name);

-- =========================
-- USER_ROLES
-- =========================
CREATE TABLE user_roles (
                            id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                            user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                            role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
                            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_user_roles_user_role ON user_roles(user_id, role_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles(role_id);

-- =========================
-- ROLE_PERMISSIONS
-- =========================
CREATE TABLE role_permissions (
                                  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                  role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
                                  permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
                                  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_role_permissions ON role_permissions(role_id, permission_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_role_id ON role_permissions(role_id);
CREATE INDEX IF NOT EXISTS idx_role_permissions_permission_id ON role_permissions(permission_id);

-- =========================
-- PATIENT PROFILES
-- =========================
CREATE TABLE patient_profiles (
                                  user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
                                  height_cm    INTEGER,
                                  weight_kg    INTEGER,
                                  allergies    TEXT,
                                  medical_hist TEXT,
                                  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                  updated_at   TIMESTAMPTZ
);

-- =========================
-- DOCTOR PROFILES
-- =========================
CREATE TABLE doctor_profiles (
                                 user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
                                 sip_number VARCHAR(64) UNIQUE,
                                 specialty  VARCHAR(100),
                                 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                 updated_at TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS doctor_profiles;
DROP TABLE IF EXISTS patient_profiles;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
