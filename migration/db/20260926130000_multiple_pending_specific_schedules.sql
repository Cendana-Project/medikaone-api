-- +goose Up
DROP INDEX IF EXISTS uq_doctor_schedule_pending_change;

-- A recurring snapshot replacement remains exclusive per affiliation, while
-- one-off additions may be reviewed independently and concurrently.
CREATE UNIQUE INDEX uq_doctor_schedule_pending_replace
    ON doctor_schedule_change_requests (affiliation_id)
    WHERE status = 'PENDING' AND operation = 'REPLACE';

-- Repeating a removal for the same active schedule is never meaningful, but
-- removals for different schedules may proceed independently.
CREATE UNIQUE INDEX uq_doctor_schedule_pending_remove
    ON doctor_schedule_change_requests (affiliation_id, target_schedule_id)
    WHERE status = 'PENDING' AND operation = 'REMOVE';

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM doctor_schedule_change_requests
        WHERE status = 'PENDING'
        GROUP BY affiliation_id
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'Cannot restore one-pending-per-affiliation while multiple proposals are pending';
    END IF;
END $$;
-- +goose StatementEnd

DROP INDEX IF EXISTS uq_doctor_schedule_pending_remove;
DROP INDEX IF EXISTS uq_doctor_schedule_pending_replace;
CREATE UNIQUE INDEX uq_doctor_schedule_pending_change
    ON doctor_schedule_change_requests (affiliation_id)
    WHERE status = 'PENDING';
