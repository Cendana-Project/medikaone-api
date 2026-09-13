-- +goose Up
ALTER TABLE doctor_hospital_schedules ADD COLUMN schedule_date DATE;
ALTER TABLE doctor_hospital_invitation_schedules ADD COLUMN schedule_date DATE;
ALTER TABLE doctor_schedule_change_items ADD COLUMN schedule_date DATE;

-- Keep the derived weekday for matching recurring and one-off intervals.
ALTER TABLE doctor_hospital_schedules ADD CONSTRAINT chk_schedule_specific_weekday CHECK (schedule_date IS NULL OR EXTRACT(DOW FROM schedule_date) = day_of_week);
ALTER TABLE doctor_hospital_invitation_schedules ADD CONSTRAINT chk_invitation_specific_weekday CHECK (schedule_date IS NULL OR EXTRACT(DOW FROM schedule_date) = day_of_week);
ALTER TABLE doctor_schedule_change_items ADD CONSTRAINT chk_change_specific_weekday CHECK (schedule_date IS NULL OR EXTRACT(DOW FROM schedule_date) = day_of_week);

-- Replace only occurrence uniqueness constraints; existing rows and IDs are preserved.
-- +goose StatementBegin
DO $$
DECLARE item RECORD;
BEGIN
    FOR item IN SELECT conrelid::regclass AS table_name, conname FROM pg_constraint
        WHERE conrelid IN ('doctor_hospital_schedules'::regclass, 'doctor_hospital_invitation_schedules'::regclass, 'doctor_schedule_change_items'::regclass)
          AND contype = 'u' AND pg_get_constraintdef(oid) LIKE '%day_of_week%'
    LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', item.table_name, item.conname);
    END LOOP;
END $$;
-- +goose StatementEnd
CREATE UNIQUE INDEX uq_doctor_schedule_occurrence ON doctor_hospital_schedules (affiliation_id, day_of_week, (COALESCE(schedule_date, 'infinity'::date)), start_time, end_time) WHERE is_active = TRUE;
CREATE UNIQUE INDEX uq_invitation_schedule_occurrence ON doctor_hospital_invitation_schedules (invitation_id, day_of_week, (COALESCE(schedule_date, 'infinity'::date)), start_time, end_time);
CREATE UNIQUE INDEX uq_change_schedule_occurrence ON doctor_schedule_change_items (change_request_id, day_of_week, (COALESCE(schedule_date, 'infinity'::date)), start_time, end_time);

ALTER TABLE doctor_schedule_change_requests ADD COLUMN operation VARCHAR(10) NOT NULL DEFAULT 'REPLACE' CHECK (operation IN ('REPLACE', 'ADD', 'REMOVE'));
ALTER TABLE doctor_schedule_change_requests ADD COLUMN target_schedule_id UUID REFERENCES doctor_hospital_schedules(id);
ALTER TABLE doctor_schedule_change_requests ADD CONSTRAINT chk_schedule_change_target CHECK ((operation = 'REMOVE') = (target_schedule_id IS NOT NULL));

-- +goose Down
-- Refuse lossy rollback once one-off schedules or mutation requests have been used.
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM doctor_hospital_schedules WHERE schedule_date IS NOT NULL)
       OR EXISTS (SELECT 1 FROM doctor_hospital_invitation_schedules WHERE schedule_date IS NOT NULL)
       OR EXISTS (SELECT 1 FROM doctor_schedule_change_items WHERE schedule_date IS NOT NULL)
       OR EXISTS (SELECT 1 FROM doctor_schedule_change_requests WHERE operation <> 'REPLACE')
       OR EXISTS (SELECT 1 FROM doctor_hospital_schedules GROUP BY affiliation_id, day_of_week, start_time, end_time HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Cannot roll back specific schedules while one-off schedules or mutation requests exist';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE doctor_schedule_change_requests DROP CONSTRAINT chk_schedule_change_target, DROP COLUMN target_schedule_id, DROP COLUMN operation;
DROP INDEX uq_doctor_schedule_occurrence;
DROP INDEX uq_invitation_schedule_occurrence;
DROP INDEX uq_change_schedule_occurrence;
ALTER TABLE doctor_hospital_schedules DROP COLUMN schedule_date, ADD UNIQUE (affiliation_id, day_of_week, start_time, end_time);
ALTER TABLE doctor_hospital_invitation_schedules DROP COLUMN schedule_date, ADD UNIQUE (invitation_id, day_of_week, start_time, end_time);
ALTER TABLE doctor_schedule_change_items DROP COLUMN schedule_date, ADD UNIQUE (change_request_id, day_of_week, start_time, end_time);
