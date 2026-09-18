-- +goose Up
ALTER TABLE hospitals
    ALTER COLUMN description TYPE TEXT,
    ADD COLUMN email VARCHAR(190),
    ADD COLUMN website VARCHAR(2048),
    ADD COLUMN established_year SMALLINT CHECK (established_year BETWEEN 1800 AND 9999),
    ADD COLUMN timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Jakarta',
    ADD COLUMN opening_hours JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT chk_hospital_opening_hours CHECK (jsonb_typeof(opening_hours) = 'array');

CREATE TABLE hospital_images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    bucket TEXT NOT NULL,
    object_path TEXT NOT NULL UNIQUE,
    content_type VARCHAR(32) NOT NULL CHECK (content_type IN ('image/jpeg','image/png')),
    file_size BIGINT NOT NULL CHECK (file_size BETWEEN 1 AND 10485760),
    caption VARCHAR(200) NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 1000),
    is_cover BOOLEAN NOT NULL DEFAULT FALSE,
    uploaded_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_hospital_image_cover ON hospital_images(hospital_id) WHERE is_cover AND deleted_at IS NULL;
CREATE INDEX idx_hospital_images_list ON hospital_images(hospital_id, sort_order, created_at) WHERE deleted_at IS NULL;

CREATE TABLE hospital_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment TEXT NOT NULL DEFAULT '' CHECK (char_length(comment) <= 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(hospital_id, user_id)
);
CREATE INDEX idx_hospital_reviews_list ON hospital_reviews(hospital_id, created_at DESC, id) WHERE deleted_at IS NULL;
CREATE INDEX idx_appointment_review_eligibility ON appointments(hospital_id, patient_id, patient_record_id) WHERE status = 'COMPLETED';

-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'medikaone_app') THEN
        GRANT SELECT, INSERT, UPDATE ON hospital_images, hospital_reviews TO medikaone_app;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'hospital directory migration retains review and image history; restore a verified backup to roll back';
END $$;
-- +goose StatementEnd
