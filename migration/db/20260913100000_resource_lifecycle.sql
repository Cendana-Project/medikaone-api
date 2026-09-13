-- +goose Up
-- +goose StatementBegin
ALTER TABLE doctor_hospital_invitations ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE doctor_hospital_affiliations ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE notifications ADD COLUMN deleted_at TIMESTAMPTZ;

-- An accepted invitation is historical; the live affiliation now enforces
-- placement uniqueness. A removed affiliation can receive a fresh invitation.
DROP INDEX uq_doctor_hospital_open_invitation;
CREATE UNIQUE INDEX uq_doctor_hospital_open_invitation
    ON doctor_hospital_invitations (hospital_id, doctor_id, department_id,
        COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE status = 'PENDING' AND deleted_at IS NULL;

ALTER TABLE doctor_hospital_invitation_events
    DROP CONSTRAINT chk_doctor_hospital_invitation_event_type,
    ADD CONSTRAINT chk_doctor_hospital_invitation_event_type CHECK (
        event_type IN ('CREATED', 'RESENT', 'ACCEPTED', 'REJECTED', 'CANCELLED', 'EXPIRED', 'UPDATED', 'DELETED')
    );
ALTER TABLE doctor_hospital_affiliation_events
    DROP CONSTRAINT chk_doctor_hospital_affiliation_event_type,
    ADD CONSTRAINT chk_doctor_hospital_affiliation_event_type CHECK (
        event_type IN ('ACTIVATED', 'SUSPENDED', 'REACTIVATED', 'DELETED')
    );

DROP INDEX uq_doctor_hospital_affiliation_placement;
CREATE UNIQUE INDEX uq_doctor_hospital_affiliation_placement
    ON doctor_hospital_affiliations (hospital_id, doctor_id, department_id,
        COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE deleted_at IS NULL;
CREATE INDEX idx_doctor_hospital_invitations_visible
    ON doctor_hospital_invitations (hospital_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_notifications_visible
    ON notifications (user_id, created_at DESC) WHERE deleted_at IS NULL;

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'medikaone_app') THEN
        GRANT UPDATE ON hospitals TO medikaone_app;
        -- Only pending invitation draft schedule rows are replaced by PATCH;
        -- accepted schedules, contracts and clinical history are retained.
        GRANT DELETE ON doctor_hospital_invitation_schedules TO medikaone_app;
        GRANT DELETE ON hospital_user_roles TO medikaone_app;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Tombstones and events preserve history; rollback is intentionally refused
-- once lifecycle actions have been used, rather than resurrect archived data.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM doctor_hospital_invitations WHERE deleted_at IS NOT NULL)
       OR EXISTS (SELECT 1 FROM doctor_hospital_affiliations WHERE deleted_at IS NOT NULL)
       OR EXISTS (SELECT 1 FROM notifications WHERE deleted_at IS NOT NULL)
       OR EXISTS (SELECT 1 FROM doctor_hospital_invitation_events WHERE event_type IN ('UPDATED','DELETED'))
       OR EXISTS (SELECT 1 FROM doctor_hospital_affiliation_events WHERE event_type = 'DELETED') THEN
        RAISE EXCEPTION 'Cannot roll back resource lifecycle migration after lifecycle actions';
    END IF;
END $$;
DROP INDEX idx_notifications_visible;
DROP INDEX uq_doctor_hospital_open_invitation;
CREATE UNIQUE INDEX uq_doctor_hospital_open_invitation
    ON doctor_hospital_invitations (hospital_id, doctor_id, department_id,
        COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE status IN ('PENDING', 'ACCEPTED');
DROP INDEX idx_doctor_hospital_invitations_visible;
DROP INDEX uq_doctor_hospital_affiliation_placement;
CREATE UNIQUE INDEX uq_doctor_hospital_affiliation_placement
    ON doctor_hospital_affiliations (hospital_id, doctor_id, department_id,
        COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid));
ALTER TABLE doctor_hospital_invitation_events
    DROP CONSTRAINT chk_doctor_hospital_invitation_event_type,
    ADD CONSTRAINT chk_doctor_hospital_invitation_event_type CHECK (
        event_type IN ('CREATED', 'RESENT', 'ACCEPTED', 'REJECTED', 'CANCELLED', 'EXPIRED')
    );
ALTER TABLE doctor_hospital_affiliation_events
    DROP CONSTRAINT chk_doctor_hospital_affiliation_event_type,
    ADD CONSTRAINT chk_doctor_hospital_affiliation_event_type CHECK (
        event_type IN ('ACTIVATED', 'SUSPENDED', 'REACTIVATED')
    );
ALTER TABLE notifications DROP COLUMN deleted_at;
ALTER TABLE doctor_hospital_affiliations DROP COLUMN deleted_at;
ALTER TABLE doctor_hospital_invitations DROP COLUMN deleted_at;
-- +goose StatementEnd
