-- +goose Up
-- +goose StatementBegin

-- Authentication accounts and patient identities have different lifecycles.
-- A walk-in may therefore have a patient record before they own an account.
CREATE TABLE patient_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    created_at_hospital_id UUID REFERENCES hospitals(id) ON DELETE RESTRICT,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL DEFAULT '',
    email VARCHAR(190),
    phone VARCHAR(32) NOT NULL,
    dob DATE NOT NULL,
    gender VARCHAR(1) NOT NULL,
    identity_type VARCHAR(24) NOT NULL,
    identity_number VARCHAR(64) NOT NULL,
    identity_number_normalized VARCHAR(64) NOT NULL,
    created_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    claimed_at TIMESTAMPTZ,
    claimed_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_patient_record_gender CHECK (gender IN ('L', 'P')),
    CONSTRAINT chk_patient_record_identity_type
        CHECK (identity_type IN ('NIK', 'PASSPORT', 'OTHER', 'MEDIKAONE_ID')),
    CONSTRAINT chk_patient_record_identity_number
        CHECK (char_length(identity_number_normalized) BETWEEN 3 AND 64),
    CONSTRAINT chk_patient_record_claim
        CHECK ((claimed_at IS NULL AND claimed_by IS NULL) OR
               (claimed_at IS NOT NULL AND claimed_by IS NOT NULL))
);
CREATE INDEX idx_patient_records_user
    ON patient_records (user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX uq_patient_records_identity
    ON patient_records (identity_type, identity_number_normalized);
CREATE INDEX idx_patient_records_name_dob
    ON patient_records (LOWER(first_name), LOWER(last_name), dob);
CREATE INDEX idx_patient_records_phone
    ON patient_records (phone);
CREATE INDEX idx_patient_records_email
    ON patient_records (LOWER(email)) WHERE email IS NOT NULL;

-- Existing appointments receive a patient record without changing or deleting
-- their account/profile data. Other users get a record lazily on first use.
-- MEDIKAONE_ID is used when no NIK is available.
WITH patient_users AS (
    SELECT DISTINCT u.id, u.email, u.first_name, u.last_name, u.phone, u.dob, u.gender, u.nik
    FROM users u
    WHERE u.deleted_at IS NULL
      AND EXISTS (SELECT 1 FROM appointments a WHERE a.patient_id = u.id)
)
INSERT INTO patient_records (
    id, user_id, first_name, last_name, email, phone, dob, gender,
    identity_type, identity_number, identity_number_normalized,
    claimed_at, claimed_by, created_at, updated_at
)
SELECT gen_random_uuid(), patient.id,
       COALESCE(NULLIF(BTRIM(patient.first_name), ''), 'Patient'),
       COALESCE(BTRIM(patient.last_name), ''), patient.email,
       COALESCE(NULLIF(BTRIM(patient.phone), ''), 'UNKNOWN-' || LEFT(patient.id::text, 8)),
       COALESCE(patient.dob, DATE '1900-01-01'),
       CASE WHEN patient.gender IN ('L', 'P') THEN patient.gender ELSE 'L' END,
       CASE WHEN NULLIF(BTRIM(patient.nik), '') IS NULL THEN 'MEDIKAONE_ID' ELSE 'NIK' END,
       COALESCE(NULLIF(BTRIM(patient.nik), ''), patient.id::text),
       UPPER(COALESCE(NULLIF(REGEXP_REPLACE(BTRIM(patient.nik), '[^[:alnum:]]', '', 'g'), ''), patient.id::text)),
       NOW(), patient.id, NOW(), NOW()
FROM patient_users patient
ON CONFLICT (identity_type, identity_number_normalized) DO NOTHING;

ALTER TABLE appointments
    ADD COLUMN patient_record_id UUID,
    ADD COLUMN source VARCHAR(16) NOT NULL DEFAULT 'SCHEDULED',
    ADD COLUMN created_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN consent_method VARCHAR(32) NOT NULL DEFAULT 'DIGITAL_SELF',
    ADD COLUMN check_in_method VARCHAR(24),
    ADD COLUMN check_in_override_reason TEXT,
    ADD COLUMN capacity_overridden BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN capacity_override_reason TEXT,
    ADD CONSTRAINT chk_appointment_source CHECK (source IN ('SCHEDULED', 'WALK_IN')),
    ADD CONSTRAINT chk_appointment_consent_method
        CHECK (consent_method IN ('DIGITAL_SELF', 'RECEPTIONIST_INFORMED')),
    ADD CONSTRAINT chk_appointment_check_in_method
        CHECK (check_in_method IS NULL OR check_in_method IN ('QR', 'CODE', 'IDENTITY', 'WALK_IN')),
    ADD CONSTRAINT chk_appointment_check_in_override_reason
        CHECK (check_in_override_reason IS NULL OR char_length(check_in_override_reason) <= 1000),
    ADD CONSTRAINT chk_appointment_capacity_override_reason
        CHECK (capacity_override_reason IS NULL OR char_length(capacity_override_reason) <= 1000),
    ADD CONSTRAINT chk_appointment_capacity_override_consistency
        CHECK ((capacity_overridden = FALSE AND capacity_override_reason IS NULL) OR
               (capacity_overridden = TRUE AND capacity_override_reason IS NOT NULL));

UPDATE appointments appointment
SET patient_record_id = record.id,
    created_by = appointment.patient_id
FROM patient_records record
WHERE record.user_id = appointment.patient_id;

ALTER TABLE appointments
    ALTER COLUMN patient_record_id SET NOT NULL,
    ALTER COLUMN patient_id DROP NOT NULL,
    ALTER COLUMN created_by SET NOT NULL,
    ADD CONSTRAINT fk_appointments_patient_record
        FOREIGN KEY (patient_record_id) REFERENCES patient_records(id) ON DELETE RESTRICT,
    ADD CONSTRAINT uq_appointments_patient_record_idempotency
        UNIQUE (patient_record_id, idempotency_key);

CREATE INDEX idx_appointments_check_in_search
    ON appointments (hospital_id, appointment_date, status, appointment_number);
CREATE INDEX idx_appointments_patient_record
    ON appointments (patient_record_id, scheduled_start_at DESC);

CREATE TABLE patient_record_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_record_id UUID NOT NULL REFERENCES patient_records(id) ON DELETE RESTRICT,
    actor_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    hospital_id UUID REFERENCES hospitals(id) ON DELETE RESTRICT,
    event_type VARCHAR(24) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_patient_record_event_type
        CHECK (event_type IN ('CREATED', 'LINKED', 'CLAIMED'))
);
CREATE INDEX idx_patient_record_events_record
    ON patient_record_events (patient_record_id, created_at);

INSERT INTO patient_record_events (patient_record_id, actor_id, event_type, metadata, created_at)
SELECT id, user_id, 'LINKED', '{"source":"migration"}'::jsonb, NOW()
FROM patient_records
WHERE user_id IS NOT NULL;

INSERT INTO permissions (name, slug, is_active, created_at, updated_at, deleted_at)
VALUES
    ('Appointment Walk-in Create', 'appointment.walkin.create', TRUE, NOW(), NOW(), NULL),
    ('Appointment Walk-in Capacity Override', 'appointment.walkin.override_capacity', TRUE, NOW(), NOW(), NULL),
    ('Patient Record Claim', 'patient_record.claim', TRUE, NOW(), NOW(), NULL)
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    is_active = TRUE,
    updated_at = NOW(),
    deleted_at = NULL;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT role.id, permission.id, NOW()
FROM (VALUES
    ('SUPER_ADMIN', 'appointment.walkin.create'),
    ('SUPER_ADMIN', 'appointment.walkin.override_capacity'),
    ('ADMIN', 'appointment.walkin.create'),
    ('ADMIN', 'appointment.walkin.override_capacity'),
    ('RECEPTIONIST', 'appointment.walkin.create'),
    ('PATIENT', 'patient_record.claim')
) AS role_permission(role_slug, permission_slug)
JOIN roles role ON UPPER(role.slug) = role_permission.role_slug
                  AND role.deleted_at IS NULL
JOIN permissions permission ON permission.slug = role_permission.permission_slug
ON CONFLICT (role_id, permission_id) DO NOTHING;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'medikaone_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON patient_records TO medikaone_app';
        EXECUTE 'GRANT SELECT, INSERT ON patient_record_events TO medikaone_app';
    END IF;
END
$$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Reverting would require deleting or fabricating authentication accounts for
-- walk-in patients. Fail explicitly so production patient data is preserved.
DO $$
BEGIN
    RAISE EXCEPTION 'check-in/walk-in migration is intentionally irreversible; restore a verified backup instead';
END
$$;

-- +goose StatementEnd
