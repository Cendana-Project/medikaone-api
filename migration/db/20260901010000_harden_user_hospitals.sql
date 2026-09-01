-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM users
        WHERE deleted_at IS NULL
        GROUP BY LOWER(email)
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot enforce case-insensitive email uniqueness: duplicate active emails exist';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM users
        WHERE deleted_at IS NULL AND username IS NOT NULL
        GROUP BY LOWER(username)
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot enforce case-insensitive username uniqueness: duplicate active usernames exist';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM users
        WHERE deleted_at IS NULL AND nik IS NOT NULL
        GROUP BY nik
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot enforce NIK uniqueness: duplicate active NIK values exist';
    END IF;
END
$$;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS seed_key VARCHAR(128);

-- seed_key is internal immutable provenance. It is intentionally unique over
-- both active and soft-deleted rows so a fixture can always reactivate its own
-- row without adopting a real account that happens to reuse its email.
CREATE UNIQUE INDEX IF NOT EXISTS ux_users_seed_key
    ON users (seed_key)
    WHERE seed_key IS NOT NULL;

-- The original schema used table-wide unique constraints. Those constraints
-- conflict with the repository's soft-delete semantics: an identifier hidden
-- by deleted_at could pass an application precheck but still fail on INSERT.
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_email_key,
    DROP CONSTRAINT IF EXISTS users_username_key,
    DROP CONSTRAINT IF EXISTS users_nik_key;
DROP INDEX IF EXISTS ux_users_username_lower;

CREATE UNIQUE INDEX IF NOT EXISTS ux_users_email_lower
    ON users (LOWER(email))
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_users_username_lower
    ON users (LOWER(username))
    WHERE deleted_at IS NULL AND username IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_users_nik_active
    ON users (nik)
    WHERE deleted_at IS NULL AND nik IS NOT NULL;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM hospitals
        WHERE deleted_at IS NULL AND code IS NOT NULL
        GROUP BY LOWER(code) HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot enforce case-insensitive hospital code uniqueness: duplicates exist';
    END IF;
    IF EXISTS (
        SELECT 1 FROM hospitals
        WHERE deleted_at IS NULL
        GROUP BY LOWER(name) HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot enforce case-insensitive hospital name uniqueness: duplicates exist';
    END IF;
END
$$;

ALTER TABLE hospitals
    DROP CONSTRAINT IF EXISTS hospitals_code_key,
    ADD COLUMN IF NOT EXISTS seed_key VARCHAR(128);

CREATE UNIQUE INDEX IF NOT EXISTS ux_hospitals_seed_key
    ON hospitals (seed_key)
    WHERE seed_key IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_hospitals_code_lower
    ON hospitals (LOWER(code))
    WHERE deleted_at IS NULL AND code IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_hospitals_name_lower
    ON hospitals (LOWER(name))
    WHERE deleted_at IS NULL;

-- Fixture provenance can only be assigned by a role that belongs to the
-- table owner, and can never be moved to another row. Runtime DML roles may
-- insert ordinary rows with a NULL seed_key but cannot forge reset ownership.
CREATE OR REPLACE FUNCTION enforce_seed_key_provenance()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    table_owner OID;
BEGIN
    SELECT relowner INTO table_owner FROM pg_class WHERE oid = TG_RELID;
    IF TG_OP = 'INSERT' AND NEW.seed_key IS NOT NULL
       AND current_user::regrole::oid <> table_owner
       AND NOT pg_has_role(current_user, table_owner, 'MEMBER') THEN
        RAISE EXCEPTION 'seed_key can only be assigned by the table owner';
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.seed_key IS DISTINCT FROM NEW.seed_key THEN
        RAISE EXCEPTION 'seed_key is immutable';
    END IF;
    RETURN NEW;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'users_seed_key_provenance' AND tgrelid = 'users'::regclass) THEN
        CREATE TRIGGER users_seed_key_provenance
        BEFORE INSERT OR UPDATE OF seed_key ON users
        FOR EACH ROW EXECUTE FUNCTION enforce_seed_key_provenance();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'hospitals_seed_key_provenance' AND tgrelid = 'hospitals'::regclass) THEN
        CREATE TRIGGER hospitals_seed_key_provenance
        BEFORE INSERT OR UPDATE OF seed_key ON hospitals
        FOR EACH ROW EXECUTE FUNCTION enforce_seed_key_provenance();
    END IF;
END
$$;

ALTER TABLE user_hospitals
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

UPDATE user_hospitals
SET updated_at = COALESCE(updated_at, created_at, NOW())
WHERE updated_at IS NULL;

-- Repair legacy duplicate primary markers deterministically before enforcing
-- one primary hospital per user.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY created_at, id) AS position
    FROM user_hospitals
    WHERE is_primary = TRUE AND deleted_at IS NULL
)
UPDATE user_hospitals AS uh
SET is_primary = FALSE,
    updated_at = NOW()
FROM ranked
WHERE uh.id = ranked.id AND ranked.position > 1;

CREATE UNIQUE INDEX IF NOT EXISTS ux_user_hospitals_one_primary
    ON user_hospitals (user_id)
    WHERE is_primary = TRUE AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_user_hospitals_active_membership
    ON user_hospitals (user_id, hospital_id)
    WHERE is_active = TRUE AND deleted_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'migration 20260901010000 is intentionally irreversible: restoring table-wide unique constraints or removing soft-delete/fixture provenance columns can corrupt valid data; use a guarded staging reset instead';
END
$$;
-- +goose StatementEnd
