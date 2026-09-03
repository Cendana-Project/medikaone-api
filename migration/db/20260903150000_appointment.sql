-- +goose Up
-- +goose StatementBegin

-- A doctor may practice in more than one department/room in the same hospital.
-- Existing rows remain valid; only the uniqueness boundary becomes a placement.
DROP INDEX IF EXISTS uq_doctor_hospital_open_invitation;
CREATE UNIQUE INDEX uq_doctor_hospital_open_invitation
    ON doctor_hospital_invitations (
        hospital_id,
        doctor_id,
        department_id,
        COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid)
    )
    WHERE status IN ('PENDING', 'ACCEPTED');

ALTER TABLE doctor_hospital_affiliations
    DROP CONSTRAINT IF EXISTS doctor_hospital_affiliations_hospital_id_doctor_id_key;
CREATE UNIQUE INDEX uq_doctor_hospital_affiliation_placement
    ON doctor_hospital_affiliations (
        hospital_id,
        doctor_id,
        department_id,
        COALESCE(room_id, '00000000-0000-0000-0000-000000000000'::uuid)
    );

ALTER TABLE doctor_hospital_invitation_schedules
    ADD COLUMN booking_mode VARCHAR(20) NOT NULL DEFAULT 'FIXED_SLOT',
    ADD COLUMN slot_duration_minutes SMALLINT NOT NULL DEFAULT 30,
    ADD COLUMN capacity SMALLINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT chk_invitation_schedule_booking_mode
        CHECK (booking_mode IN ('FIXED_SLOT', 'SESSION_QUEUE')),
    ADD CONSTRAINT chk_invitation_schedule_slot_duration
        CHECK (slot_duration_minutes BETWEEN 5 AND 240),
    ADD CONSTRAINT chk_invitation_schedule_capacity
        CHECK (capacity BETWEEN 1 AND 500);

ALTER TABLE doctor_hospital_schedules
    ADD COLUMN booking_mode VARCHAR(20) NOT NULL DEFAULT 'FIXED_SLOT',
    ADD COLUMN slot_duration_minutes SMALLINT NOT NULL DEFAULT 30,
    ADD COLUMN capacity SMALLINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT chk_doctor_schedule_booking_mode
        CHECK (booking_mode IN ('FIXED_SLOT', 'SESSION_QUEUE')),
    ADD CONSTRAINT chk_doctor_schedule_slot_duration
        CHECK (slot_duration_minutes BETWEEN 5 AND 240),
    ADD CONSTRAINT chk_doctor_schedule_capacity
        CHECK (capacity BETWEEN 1 AND 500);

-- Every later schedule modification is proposed by one party and approved by
-- the other. The current schedule remains active while a proposal is pending.
CREATE TABLE doctor_schedule_change_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    affiliation_id UUID NOT NULL REFERENCES doctor_hospital_affiliations(id) ON DELETE RESTRICT,
    requested_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    requested_by_party VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    reason TEXT,
    reviewed_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    rejection_reason TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_schedule_change_party
        CHECK (requested_by_party IN ('DOCTOR', 'HOSPITAL')),
    CONSTRAINT chk_schedule_change_status
        CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'CANCELLED', 'EXPIRED')),
    CONSTRAINT chk_schedule_change_reason_length
        CHECK (reason IS NULL OR char_length(reason) <= 1000),
    CONSTRAINT chk_schedule_change_rejection_length
        CHECK (rejection_reason IS NULL OR char_length(rejection_reason) <= 1000)
);
CREATE UNIQUE INDEX uq_doctor_schedule_pending_change
    ON doctor_schedule_change_requests (affiliation_id)
    WHERE status = 'PENDING';
CREATE INDEX idx_doctor_schedule_changes_affiliation
    ON doctor_schedule_change_requests (affiliation_id, created_at DESC);

CREATE TABLE doctor_schedule_change_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    change_request_id UUID NOT NULL REFERENCES doctor_schedule_change_requests(id) ON DELETE CASCADE,
    day_of_week SMALLINT NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Jakarta',
    booking_mode VARCHAR(20) NOT NULL,
    slot_duration_minutes SMALLINT NOT NULL,
    capacity SMALLINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_schedule_change_item_day CHECK (day_of_week BETWEEN 0 AND 6),
    CONSTRAINT chk_schedule_change_item_time CHECK (start_time < end_time),
    CONSTRAINT chk_schedule_change_item_booking_mode
        CHECK (booking_mode IN ('FIXED_SLOT', 'SESSION_QUEUE')),
    CONSTRAINT chk_schedule_change_item_slot_duration
        CHECK (slot_duration_minutes BETWEEN 5 AND 240),
    CONSTRAINT chk_schedule_change_item_capacity
        CHECK (capacity BETWEEN 1 AND 500),
    UNIQUE (change_request_id, day_of_week, start_time, end_time)
);

CREATE TABLE doctor_schedule_change_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    change_request_id UUID NOT NULL REFERENCES doctor_schedule_change_requests(id) ON DELETE CASCADE,
    actor_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    event_type VARCHAR(16) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_schedule_change_event_type
        CHECK (event_type IN ('CREATED', 'APPROVED', 'REJECTED', 'CANCELLED', 'EXPIRED'))
);
CREATE INDEX idx_doctor_schedule_change_events
    ON doctor_schedule_change_events (change_request_id, created_at);

-- Counters produce human-readable numbers without using them as credentials.
CREATE TABLE appointment_daily_counters (
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    counter_date DATE NOT NULL,
    counter_type VARCHAR(20) NOT NULL,
    context_key VARCHAR(160) NOT NULL,
    last_value INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (hospital_id, counter_date, counter_type, context_key),
    CONSTRAINT chk_appointment_counter_type
        CHECK (counter_type IN ('APPOINTMENT', 'QUEUE')),
    CONSTRAINT chk_appointment_counter_value CHECK (last_value >= 0)
);

CREATE TABLE appointments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    appointment_number VARCHAR(48) NOT NULL UNIQUE,
    patient_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    affiliation_id UUID NOT NULL REFERENCES doctor_hospital_affiliations(id) ON DELETE RESTRICT,
    schedule_id UUID NOT NULL REFERENCES doctor_hospital_schedules(id) ON DELETE RESTRICT,
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    doctor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    department_id UUID NOT NULL REFERENCES hospital_departments(id) ON DELETE RESTRICT,
    room_id UUID REFERENCES hospital_rooms(id) ON DELETE RESTRICT,
    appointment_date DATE NOT NULL,
    scheduled_start_at TIMESTAMPTZ NOT NULL,
    scheduled_end_at TIMESTAMPTZ NOT NULL,
    booking_mode VARCHAR(20) NOT NULL,
    slot_position SMALLINT NOT NULL,
    queue_number VARCHAR(24) NOT NULL,
    queue_activated_at TIMESTAMPTZ,
    status VARCHAR(24) NOT NULL DEFAULT 'CONFIRMED',
    attendance_status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    reason_for_visit TEXT NOT NULL,
    note TEXT,
    consent_version VARCHAR(64) NOT NULL,
    consented_at TIMESTAMPTZ NOT NULL,
    consent_ip INET,
    consent_user_agent VARCHAR(512),
    idempotency_key UUID NOT NULL,
    idempotency_request_hash CHAR(64) NOT NULL,
    verification_used_at TIMESTAMPTZ,
    checked_in_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    cancelled_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    cancellation_reason TEXT,
    completed_at TIMESTAMPTZ,
    rescheduled_from_id UUID REFERENCES appointments(id) ON DELETE RESTRICT,
    rescheduled_to_id UUID REFERENCES appointments(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_appointment_time CHECK (scheduled_start_at < scheduled_end_at),
    CONSTRAINT chk_appointment_booking_mode
        CHECK (booking_mode IN ('FIXED_SLOT', 'SESSION_QUEUE')),
    CONSTRAINT chk_appointment_slot_position CHECK (slot_position BETWEEN 1 AND 500),
    CONSTRAINT chk_appointment_status CHECK (status IN (
        'CONFIRMED', 'CHECKED_IN', 'WAITING_VITALS', 'WAITING_DOCTOR',
        'IN_CONSULTATION', 'COMPLETED', 'CANCELLED', 'NO_SHOW', 'RESCHEDULED'
    )),
    CONSTRAINT chk_appointment_attendance
        CHECK (attendance_status IN ('PENDING', 'PRESENT', 'NO_SHOW')),
    CONSTRAINT chk_appointment_reason_length
        CHECK (char_length(reason_for_visit) BETWEEN 1 AND 2000),
    CONSTRAINT chk_appointment_note_length CHECK (note IS NULL OR char_length(note) <= 2000),
    CONSTRAINT chk_appointment_cancel_reason_length
        CHECK (cancellation_reason IS NULL OR char_length(cancellation_reason) <= 1000),
    UNIQUE (patient_id, idempotency_key),
    UNIQUE (hospital_id, appointment_date, queue_number)
);
CREATE UNIQUE INDEX uq_appointments_active_slot_position
    ON appointments (schedule_id, appointment_date, scheduled_start_at, slot_position)
    WHERE status IN (
        'CONFIRMED', 'CHECKED_IN', 'WAITING_VITALS', 'WAITING_DOCTOR', 'IN_CONSULTATION'
    );
CREATE INDEX idx_appointments_patient
    ON appointments (patient_id, scheduled_start_at DESC);
CREATE INDEX idx_appointments_doctor
    ON appointments (doctor_id, scheduled_start_at DESC);
CREATE INDEX idx_appointments_hospital
    ON appointments (hospital_id, scheduled_start_at DESC);
CREATE INDEX idx_appointments_active_schedule
    ON appointments (schedule_id, appointment_date, scheduled_start_at)
    WHERE status IN (
        'CONFIRMED', 'CHECKED_IN', 'WAITING_VITALS', 'WAITING_DOCTOR', 'IN_CONSULTATION'
    );

CREATE TABLE appointment_status_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    appointment_id UUID NOT NULL REFERENCES appointments(id) ON DELETE RESTRICT,
    actor_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    event_type VARCHAR(32) NOT NULL,
    from_status VARCHAR(24),
    to_status VARCHAR(24) NOT NULL,
    reason TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_appointment_event_type CHECK (event_type IN (
        'CREATED', 'CHECKED_IN', 'WAITING_VITALS', 'WAITING_DOCTOR',
        'IN_CONSULTATION', 'COMPLETED', 'CANCELLED', 'NO_SHOW', 'RESCHEDULED'
    )),
    CONSTRAINT chk_appointment_event_reason_length
        CHECK (reason IS NULL OR char_length(reason) <= 1000)
);
CREATE INDEX idx_appointment_status_events
    ON appointment_status_events (appointment_id, created_at);

CREATE TABLE appointment_reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    appointment_id UUID NOT NULL REFERENCES appointments(id) ON DELETE CASCADE,
    reminder_type VARCHAR(16) NOT NULL,
    due_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    attempts SMALLINT NOT NULL DEFAULT 0,
    last_error VARCHAR(255),
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_appointment_reminder_type CHECK (reminder_type IN ('24_HOURS', '2_HOURS')),
    CONSTRAINT chk_appointment_reminder_status
        CHECK (status IN ('PENDING', 'PROCESSING', 'SENT', 'FAILED', 'CANCELLED')),
    CONSTRAINT chk_appointment_reminder_attempts CHECK (attempts BETWEEN 0 AND 10),
    UNIQUE (appointment_id, reminder_type)
);
CREATE INDEX idx_appointment_reminders_due
    ON appointment_reminders (due_at)
    WHERE status IN ('PENDING', 'FAILED');

-- Static RBAC definitions belong to the schema rollout as well as the
-- development seeder. This makes staging/production usable immediately after
-- migration without running a demo-data seed command.
INSERT INTO permissions (name, slug, is_active, created_at, updated_at, deleted_at)
VALUES
    ('Appointment Create', 'appointment.create', TRUE, NOW(), NOW(), NULL),
    ('Appointment Cancel', 'appointment.cancel', TRUE, NOW(), NOW(), NULL),
    ('Appointment Reschedule', 'appointment.reschedule', TRUE, NOW(), NOW(), NULL),
    ('Appointment Check In', 'appointment.checkin', TRUE, NOW(), NOW(), NULL),
    ('Appointment Queue', 'appointment.queue', TRUE, NOW(), NOW(), NULL),
    ('Appointment Complete', 'appointment.complete', TRUE, NOW(), NOW(), NULL),
    ('Doctor Schedule View', 'doctor_schedule.view', TRUE, NOW(), NOW(), NULL),
    ('Doctor Schedule Propose', 'doctor_schedule.propose', TRUE, NOW(), NOW(), NULL),
    ('Doctor Schedule Approve', 'doctor_schedule.approve', TRUE, NOW(), NOW(), NULL)
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    is_active = TRUE,
    updated_at = NOW(),
    deleted_at = NULL;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT role.id, permission.id, NOW()
FROM (VALUES
    ('SUPER_ADMIN', 'appointment.create'),
    ('SUPER_ADMIN', 'appointment.cancel'),
    ('SUPER_ADMIN', 'appointment.reschedule'),
    ('SUPER_ADMIN', 'appointment.checkin'),
    ('SUPER_ADMIN', 'appointment.queue'),
    ('SUPER_ADMIN', 'appointment.complete'),
    ('SUPER_ADMIN', 'doctor_schedule.view'),
    ('SUPER_ADMIN', 'doctor_schedule.propose'),
    ('SUPER_ADMIN', 'doctor_schedule.approve'),
    ('ADMIN', 'appointment.create'),
    ('ADMIN', 'appointment.cancel'),
    ('ADMIN', 'appointment.reschedule'),
    ('ADMIN', 'appointment.checkin'),
    ('ADMIN', 'appointment.queue'),
    ('ADMIN', 'appointment.complete'),
    ('ADMIN', 'doctor_schedule.view'),
    ('ADMIN', 'doctor_schedule.propose'),
    ('ADMIN', 'doctor_schedule.approve'),
    ('NURSE', 'appointment.queue'),
    ('NURSE', 'doctor_schedule.view'),
    ('RECEPTIONIST', 'appointment.cancel'),
    ('RECEPTIONIST', 'appointment.reschedule'),
    ('RECEPTIONIST', 'appointment.checkin'),
    ('RECEPTIONIST', 'appointment.queue'),
    ('RECEPTIONIST', 'doctor_schedule.view'),
    ('PATIENT', 'appointment.create'),
    ('PATIENT', 'appointment.cancel'),
    ('PATIENT', 'appointment.reschedule'),
    ('DOCTOR', 'appointment.complete'),
    ('DOCTOR', 'doctor_schedule.view'),
    ('DOCTOR', 'doctor_schedule.propose'),
    ('DOCTOR', 'doctor_schedule.approve')
) AS role_permission(role_slug, permission_slug)
JOIN roles role ON UPPER(role.slug) = role_permission.role_slug
                  AND role.deleted_at IS NULL
JOIN permissions permission ON permission.slug = role_permission.permission_slug
ON CONFLICT (role_id, permission_id) DO NOTHING;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'medikaone_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON '
            || 'doctor_schedule_change_requests, doctor_schedule_change_items, '
            || 'doctor_schedule_change_events, appointment_daily_counters, appointments, '
            || 'appointment_status_events, appointment_reminders TO medikaone_app';
    END IF;
END
$$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS appointment_reminders;
DROP TABLE IF EXISTS appointment_status_events;
DROP TABLE IF EXISTS appointments;
DROP TABLE IF EXISTS appointment_daily_counters;
DROP TABLE IF EXISTS doctor_schedule_change_events;
DROP TABLE IF EXISTS doctor_schedule_change_items;
DROP TABLE IF EXISTS doctor_schedule_change_requests;

ALTER TABLE doctor_hospital_schedules
    DROP CONSTRAINT IF EXISTS chk_doctor_schedule_capacity,
    DROP CONSTRAINT IF EXISTS chk_doctor_schedule_slot_duration,
    DROP CONSTRAINT IF EXISTS chk_doctor_schedule_booking_mode,
    DROP COLUMN IF EXISTS capacity,
    DROP COLUMN IF EXISTS slot_duration_minutes,
    DROP COLUMN IF EXISTS booking_mode;

ALTER TABLE doctor_hospital_invitation_schedules
    DROP CONSTRAINT IF EXISTS chk_invitation_schedule_capacity,
    DROP CONSTRAINT IF EXISTS chk_invitation_schedule_slot_duration,
    DROP CONSTRAINT IF EXISTS chk_invitation_schedule_booking_mode,
    DROP COLUMN IF EXISTS capacity,
    DROP COLUMN IF EXISTS slot_duration_minutes,
    DROP COLUMN IF EXISTS booking_mode;

DROP INDEX IF EXISTS uq_doctor_hospital_affiliation_placement;
ALTER TABLE doctor_hospital_affiliations
    ADD CONSTRAINT doctor_hospital_affiliations_hospital_id_doctor_id_key
        UNIQUE (hospital_id, doctor_id);

DROP INDEX IF EXISTS uq_doctor_hospital_open_invitation;
CREATE UNIQUE INDEX uq_doctor_hospital_open_invitation
    ON doctor_hospital_invitations (hospital_id, doctor_id)
    WHERE status IN ('PENDING', 'ACCEPTED');

-- +goose StatementEnd
