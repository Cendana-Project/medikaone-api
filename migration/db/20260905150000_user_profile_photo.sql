-- +goose Up
-- +goose StatementBegin

ALTER TABLE users
    ADD COLUMN avatar_bucket VARCHAR(100),
    ADD COLUMN avatar_object_path VARCHAR(1000),
    ADD COLUMN avatar_content_type VARCHAR(32),
    ADD COLUMN avatar_file_size BIGINT,
    ADD COLUMN avatar_updated_at TIMESTAMPTZ,
    ADD CONSTRAINT chk_users_avatar_complete CHECK (
        (avatar_bucket IS NULL AND avatar_object_path IS NULL AND avatar_content_type IS NULL
            AND avatar_file_size IS NULL AND avatar_updated_at IS NULL)
        OR
        (avatar_bucket IS NOT NULL AND avatar_object_path IS NOT NULL AND avatar_content_type IS NOT NULL
            AND avatar_file_size IS NOT NULL AND avatar_updated_at IS NOT NULL)
    ),
    ADD CONSTRAINT chk_users_avatar_content_type CHECK (
        avatar_content_type IS NULL OR avatar_content_type IN ('image/jpeg', 'image/png')
    ),
    ADD CONSTRAINT chk_users_avatar_file_size CHECK (
        avatar_file_size IS NULL OR avatar_file_size BETWEEN 1 AND 10485760
    );

CREATE UNIQUE INDEX ux_users_avatar_object_path
    ON users (avatar_object_path)
    WHERE avatar_object_path IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Object storage and database changes cannot be rolled back atomically. Keep
-- the metadata so deployed clients never leave untracked private objects.
DO $$
BEGIN
    RAISE EXCEPTION 'profile photo migration is intentionally irreversible; restore a verified backup instead';
END
$$;

-- +goose StatementEnd
