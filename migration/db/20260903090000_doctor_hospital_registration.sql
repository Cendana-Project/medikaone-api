-- +goose Up
-- +goose StatementBegin

CREATE TABLE hospital_departments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE CASCADE,
    code VARCHAR(40) NOT NULL,
    name VARCHAR(120) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (hospital_id, code),
    UNIQUE (hospital_id, id)
);
CREATE UNIQUE INDEX uq_hospital_departments_name
    ON hospital_departments (hospital_id, LOWER(name));
CREATE INDEX idx_hospital_departments_hospital
    ON hospital_departments (hospital_id, is_active);

CREATE TABLE hospital_rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE CASCADE,
    department_id UUID NOT NULL,
    code VARCHAR(40) NOT NULL,
    name VARCHAR(120) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (hospital_id, code),
    UNIQUE (hospital_id, department_id, id),
    CONSTRAINT fk_hospital_room_department
        FOREIGN KEY (hospital_id, department_id)
        REFERENCES hospital_departments(hospital_id, id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX uq_hospital_rooms_name
    ON hospital_rooms (hospital_id, LOWER(name));
CREATE INDEX idx_hospital_rooms_department
    ON hospital_rooms (hospital_id, department_id, is_active);

CREATE TABLE doctor_hospital_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE CASCADE,
    doctor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    department_id UUID NOT NULL,
    room_id UUID,
    invited_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    supersedes_invitation_id UUID REFERENCES doctor_hospital_invitations(id) ON DELETE RESTRICT,
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    message TEXT,
    rejection_reason TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_doctor_invitation_department
        FOREIGN KEY (hospital_id, department_id)
        REFERENCES hospital_departments(hospital_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_doctor_invitation_room
        FOREIGN KEY (hospital_id, department_id, room_id)
        REFERENCES hospital_rooms(hospital_id, department_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_doctor_hospital_invitation_status
        CHECK (status IN ('PENDING', 'ACCEPTED', 'REJECTED', 'CANCELLED', 'EXPIRED'))
);
CREATE UNIQUE INDEX uq_doctor_hospital_open_invitation
    ON doctor_hospital_invitations (hospital_id, doctor_id)
    WHERE status IN ('PENDING', 'ACCEPTED');
CREATE INDEX idx_doctor_hospital_invitations_doctor
    ON doctor_hospital_invitations (doctor_id, status, created_at DESC);
CREATE INDEX idx_doctor_hospital_invitations_hospital
    ON doctor_hospital_invitations (hospital_id, status, created_at DESC);
CREATE INDEX idx_doctor_hospital_invitations_supersedes
    ON doctor_hospital_invitations (supersedes_invitation_id)
    WHERE supersedes_invitation_id IS NOT NULL;

CREATE TABLE doctor_hospital_contracts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invitation_id UUID NOT NULL UNIQUE REFERENCES doctor_hospital_invitations(id) ON DELETE CASCADE,
    original_filename VARCHAR(255) NOT NULL,
    original_mime_type VARCHAR(100) NOT NULL,
    original_bucket VARCHAR(100) NOT NULL,
    original_object_path TEXT NOT NULL,
    original_file_size BIGINT NOT NULL,
    original_sha256 CHAR(64) NOT NULL,
    signed_filename VARCHAR(255),
    signed_mime_type VARCHAR(100),
    signed_bucket VARCHAR(100),
    signed_object_path TEXT,
    signed_file_size BIGINT,
    signed_sha256 CHAR(64),
    signed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_original_contract_size CHECK (original_file_size > 0 AND original_file_size <= 10485760),
    CONSTRAINT chk_signed_contract_size CHECK (signed_file_size IS NULL OR (signed_file_size > 0 AND signed_file_size <= 10485760))
);

CREATE TABLE doctor_hospital_invitation_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invitation_id UUID NOT NULL REFERENCES doctor_hospital_invitations(id) ON DELETE CASCADE,
    actor_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    event_type VARCHAR(32) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_doctor_hospital_invitation_event_type CHECK (
        event_type IN ('CREATED', 'RESENT', 'ACCEPTED', 'REJECTED', 'CANCELLED', 'EXPIRED')
    )
);
CREATE INDEX idx_doctor_hospital_invitation_events
    ON doctor_hospital_invitation_events (invitation_id, created_at);

CREATE TABLE doctor_hospital_invitation_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invitation_id UUID NOT NULL REFERENCES doctor_hospital_invitations(id) ON DELETE CASCADE,
    day_of_week SMALLINT NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Jakarta',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_invitation_schedule_day CHECK (day_of_week BETWEEN 0 AND 6),
    CONSTRAINT chk_invitation_schedule_time CHECK (start_time < end_time),
    UNIQUE (invitation_id, day_of_week, start_time, end_time)
);

CREATE TABLE doctor_hospital_affiliations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE CASCADE,
    doctor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    department_id UUID NOT NULL,
    room_id UUID,
    invitation_id UUID NOT NULL UNIQUE REFERENCES doctor_hospital_invitations(id) ON DELETE RESTRICT,
    status VARCHAR(16) NOT NULL DEFAULT 'ACTIVE',
    joined_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (hospital_id, doctor_id),
    CONSTRAINT fk_doctor_affiliation_department
        FOREIGN KEY (hospital_id, department_id)
        REFERENCES hospital_departments(hospital_id, id) ON DELETE RESTRICT,
    CONSTRAINT fk_doctor_affiliation_room
        FOREIGN KEY (hospital_id, department_id, room_id)
        REFERENCES hospital_rooms(hospital_id, department_id, id) ON DELETE RESTRICT,
    CONSTRAINT chk_doctor_hospital_affiliation_status
        CHECK (status IN ('ACTIVE', 'SUSPENDED'))
);
CREATE INDEX idx_doctor_hospital_affiliations_hospital
    ON doctor_hospital_affiliations (hospital_id, status, doctor_id);
CREATE INDEX idx_doctor_hospital_affiliations_doctor
    ON doctor_hospital_affiliations (doctor_id, status, hospital_id);

CREATE TABLE doctor_hospital_affiliation_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    affiliation_id UUID NOT NULL REFERENCES doctor_hospital_affiliations(id) ON DELETE CASCADE,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    event_type VARCHAR(24) NOT NULL,
    from_status VARCHAR(16),
    to_status VARCHAR(16) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_doctor_hospital_affiliation_event_type CHECK (
        event_type IN ('ACTIVATED', 'SUSPENDED', 'REACTIVATED')
    ),
    CONSTRAINT chk_doctor_hospital_affiliation_event_statuses CHECK (
        (from_status IS NULL OR from_status IN ('ACTIVE', 'SUSPENDED'))
        AND to_status IN ('ACTIVE', 'SUSPENDED')
    )
);
CREATE INDEX idx_doctor_hospital_affiliation_events
    ON doctor_hospital_affiliation_events (affiliation_id, created_at);

CREATE TABLE doctor_hospital_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    affiliation_id UUID NOT NULL REFERENCES doctor_hospital_affiliations(id) ON DELETE CASCADE,
    day_of_week SMALLINT NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Jakarta',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_doctor_schedule_day CHECK (day_of_week BETWEEN 0 AND 6),
    CONSTRAINT chk_doctor_schedule_time CHECK (start_time < end_time),
    UNIQUE (affiliation_id, day_of_week, start_time, end_time)
);
CREATE INDEX idx_doctor_hospital_schedules_affiliation
    ON doctor_hospital_schedules (affiliation_id, day_of_week, is_active);

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(80) NOT NULL,
    title VARCHAR(180) NOT NULL,
    body TEXT NOT NULL,
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_notifications_user_created
    ON notifications (user_id, created_at DESC);
CREATE INDEX idx_notifications_user_unread
    ON notifications (user_id, created_at DESC)
    WHERE read_at IS NULL;

-- Keep the production runtime role least-privileged while allowing it to use
-- this feature. The block is conditional so local databases without this role
-- can still migrate successfully.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'medikaone_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON '
            || 'hospital_departments, hospital_rooms, doctor_hospital_invitations, '
            || 'doctor_hospital_contracts, doctor_hospital_invitation_events, '
            || 'doctor_hospital_invitation_schedules, doctor_hospital_affiliations, '
            || 'doctor_hospital_affiliation_events, doctor_hospital_schedules, notifications '
            || 'TO medikaone_app';
    END IF;
END
$$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS doctor_hospital_schedules;
DROP TABLE IF EXISTS doctor_hospital_affiliation_events;
DROP TABLE IF EXISTS doctor_hospital_affiliations;
DROP TABLE IF EXISTS doctor_hospital_invitation_schedules;
DROP TABLE IF EXISTS doctor_hospital_invitation_events;
DROP TABLE IF EXISTS doctor_hospital_contracts;
DROP TABLE IF EXISTS doctor_hospital_invitations;
DROP TABLE IF EXISTS hospital_rooms;
DROP TABLE IF EXISTS hospital_departments;

-- +goose StatementEnd
