-- +goose Up
-- Keep deactivation proposals in the existing counterpart-review/audit flow.
-- Existing table grants and RLS policies also cover these nullable columns.
ALTER TABLE doctor_schedule_change_requests
    DROP CONSTRAINT doctor_schedule_change_requests_operation_check,
    ADD CONSTRAINT doctor_schedule_change_requests_operation_check
        CHECK (operation IN ('REPLACE', 'ADD', 'REMOVE', 'DEACTIVATE')),
    ADD COLUMN deactivation_scope TEXT,
    ADD COLUMN deactivation_day_of_week INTEGER,
    ADD CONSTRAINT chk_schedule_change_deactivation_scope CHECK (
        (operation = 'DEACTIVATE' AND deactivation_scope IS NOT NULL AND (
            (deactivation_scope = 'ALL' AND deactivation_day_of_week IS NULL)
            OR (deactivation_scope = 'RECURRING_DAY' AND deactivation_day_of_week IS NOT NULL
                AND deactivation_day_of_week BETWEEN 0 AND 6)
        ))
        OR (operation <> 'DEACTIVATE' AND deactivation_scope IS NULL
            AND deactivation_day_of_week IS NULL)
    );

-- The item UUID stays a proposal identity. The separate FK freezes the original
-- schedule to deactivate, retaining historical references after approval.
ALTER TABLE doctor_schedule_change_items
    ADD COLUMN target_schedule_id UUID REFERENCES doctor_hospital_schedules(id);
CREATE UNIQUE INDEX uq_schedule_change_item_target
    ON doctor_schedule_change_items (change_request_id, target_schedule_id)
    WHERE target_schedule_id IS NOT NULL;
CREATE INDEX idx_schedule_change_item_target
    ON doctor_schedule_change_items (target_schedule_id)
    WHERE target_schedule_id IS NOT NULL;

CREATE UNIQUE INDEX uq_doctor_schedule_pending_deactivate_all
    ON doctor_schedule_change_requests (affiliation_id)
    WHERE status = 'PENDING' AND operation = 'DEACTIVATE' AND deactivation_scope = 'ALL';
CREATE UNIQUE INDEX uq_doctor_schedule_pending_deactivate_day
    ON doctor_schedule_change_requests (affiliation_id, deactivation_day_of_week)
    WHERE status = 'PENDING' AND operation = 'DEACTIVATE' AND deactivation_scope = 'RECURRING_DAY';

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM doctor_schedule_change_requests WHERE operation = 'DEACTIVATE')
       OR EXISTS (SELECT 1 FROM doctor_schedule_change_items WHERE target_schedule_id IS NOT NULL) THEN
        RAISE EXCEPTION 'Cannot remove schedule deactivation schema while proposal history exists';
    END IF;
END $$;
-- +goose StatementEnd
DROP INDEX uq_doctor_schedule_pending_deactivate_day;
DROP INDEX uq_doctor_schedule_pending_deactivate_all;
DROP INDEX idx_schedule_change_item_target;
DROP INDEX uq_schedule_change_item_target;
ALTER TABLE doctor_schedule_change_items DROP COLUMN target_schedule_id;
ALTER TABLE doctor_schedule_change_requests
    DROP CONSTRAINT chk_schedule_change_deactivation_scope,
    DROP COLUMN deactivation_scope,
    DROP COLUMN deactivation_day_of_week,
    DROP CONSTRAINT doctor_schedule_change_requests_operation_check,
    ADD CONSTRAINT doctor_schedule_change_requests_operation_check
        CHECK (operation IN ('REPLACE', 'ADD', 'REMOVE'));
