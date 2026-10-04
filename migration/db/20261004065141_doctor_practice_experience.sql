-- +goose Up
-- +goose StatementBegin
ALTER TABLE doctor_profiles ADD COLUMN practice_started_on date;
ALTER TABLE doctor_profiles ADD CONSTRAINT doctor_profiles_practice_started_on_valid
    CHECK (practice_started_on IS NULL OR practice_started_on >= DATE '1900-01-01');
COMMENT ON COLUMN doctor_profiles.practice_started_on IS 'Self-reported first practice date; NULL means unknown, not zero years.';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Preserve professional profile data. Roll back application code without dropping this column.
SELECT 1;
-- +goose StatementEnd
