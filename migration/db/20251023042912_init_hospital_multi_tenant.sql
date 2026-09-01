-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =========================
-- HOSPITALS (TENANTS)
-- =========================
CREATE TABLE hospitals (
                           id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                           code         VARCHAR(40)  UNIQUE,
                           name         VARCHAR(160) NOT NULL,
                           address      TEXT,
                           city         VARCHAR(100),
                           province     VARCHAR(100),
                           country      VARCHAR(100) DEFAULT 'Indonesia',
                           latitude     DECIMAL(9,6),
                           longitude    DECIMAL(9,6),
                           phone        VARCHAR(50),
                           description  VARCHAR(200),
                           facilities   JSONB,
                           is_active    BOOLEAN      DEFAULT TRUE,
                           created_at   TIMESTAMPTZ  DEFAULT NOW(),
                           updated_at   TIMESTAMPTZ  DEFAULT NOW(),
                           deleted_at   TIMESTAMPTZ
);

-- =========================
-- USER_HOSPITALS (membership)
-- =========================
CREATE TABLE user_hospitals (
                                id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                                hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE CASCADE,
                                is_active   BOOLEAN NOT NULL DEFAULT TRUE,
                                is_primary  BOOLEAN NOT NULL DEFAULT FALSE,
                                created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                UNIQUE (user_id, hospital_id)
);
CREATE INDEX IF NOT EXISTS idx_user_hospitals_user_id ON user_hospitals(user_id);
CREATE INDEX IF NOT EXISTS idx_user_hospitals_hospital_id ON user_hospitals(hospital_id);

-- =========================
-- HOSPITAL_USER_ROLES (role scoped)
-- =========================
CREATE TABLE hospital_user_roles (
                                     id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                     hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE CASCADE,
                                     user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                                     role_id     UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
                                     created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                     UNIQUE (hospital_id, user_id, role_id)
);
CREATE INDEX IF NOT EXISTS idx_hur_hospital_id ON hospital_user_roles(hospital_id);
CREATE INDEX IF NOT EXISTS idx_hur_user_id ON hospital_user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_hur_role_id ON hospital_user_roles(role_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS hospital_user_roles;
DROP TABLE IF EXISTS user_hospitals;
DROP TABLE IF EXISTS hospitals;
-- +goose StatementEnd
