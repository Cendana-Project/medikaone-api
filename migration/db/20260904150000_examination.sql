-- +goose Up
-- +goose StatementBegin

-- A medical encounter is the patient-owned clinical record produced by one
-- appointment. Authentication accounts remain optional because walk-in
-- patients may not have claimed their patient record yet.
CREATE TABLE medical_encounters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    appointment_id UUID NOT NULL UNIQUE REFERENCES appointments(id) ON DELETE RESTRICT,
    patient_record_id UUID NOT NULL REFERENCES patient_records(id) ON DELETE RESTRICT,
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    doctor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    department_id UUID NOT NULL REFERENCES hospital_departments(id) ON DELETE RESTRICT,
    status VARCHAR(16) NOT NULL DEFAULT 'OPEN',
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_medical_encounter_status CHECK (status IN ('OPEN', 'COMPLETED')),
    CONSTRAINT chk_medical_encounter_completion CHECK (
        (status = 'OPEN' AND completed_at IS NULL) OR
        (status = 'COMPLETED' AND completed_at IS NOT NULL)
    )
);
CREATE INDEX idx_medical_encounters_patient
    ON medical_encounters (patient_record_id, created_at DESC);
CREATE INDEX idx_medical_encounters_hospital
    ON medical_encounters (hospital_id, created_at DESC);
CREATE INDEX idx_medical_encounters_doctor
    ON medical_encounters (doctor_id, created_at DESC);

-- Drafts are mutable for usability. Finalized rows are immutable at the API
-- layer; corrections append a new version and retain their predecessor.
CREATE TABLE vital_sign_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    encounter_id UUID NOT NULL REFERENCES medical_encounters(id) ON DELETE RESTRICT,
    version INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'DRAFT',
    height_cm NUMERIC(5,2),
    weight_kg NUMERIC(6,2),
    bmi NUMERIC(5,2) GENERATED ALWAYS AS (
        CASE
            WHEN height_cm IS NULL OR weight_kg IS NULL OR height_cm = 0 THEN NULL
            ELSE ROUND(weight_kg / ((height_cm / 100.0) * (height_cm / 100.0)), 2)
        END
    ) STORED,
    temperature_c NUMERIC(4,1),
    systolic_mmhg SMALLINT,
    diastolic_mmhg SMALLINT,
    heart_rate_bpm SMALLINT,
    respiratory_rate_bpm SMALLINT,
    oxygen_saturation_percent NUMERIC(5,2),
    nurse_note TEXT,
    skipped_reason TEXT,
    recorded_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    finalized_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    finalized_at TIMESTAMPTZ,
    supersedes_revision_id UUID REFERENCES vital_sign_revisions(id) ON DELETE RESTRICT,
    correction_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_vital_status CHECK (status IN ('DRAFT', 'FINALIZED')),
    CONSTRAINT chk_vital_height CHECK (height_cm IS NULL OR height_cm BETWEEN 30 AND 300),
    CONSTRAINT chk_vital_weight CHECK (weight_kg IS NULL OR weight_kg BETWEEN 1 AND 1000),
    CONSTRAINT chk_vital_temperature CHECK (temperature_c IS NULL OR temperature_c BETWEEN 25 AND 50),
    CONSTRAINT chk_vital_systolic CHECK (systolic_mmhg IS NULL OR systolic_mmhg BETWEEN 40 AND 300),
    CONSTRAINT chk_vital_diastolic CHECK (diastolic_mmhg IS NULL OR diastolic_mmhg BETWEEN 20 AND 200),
    CONSTRAINT chk_vital_blood_pressure CHECK (
        systolic_mmhg IS NULL OR diastolic_mmhg IS NULL OR systolic_mmhg > diastolic_mmhg
    ),
    CONSTRAINT chk_vital_heart_rate CHECK (heart_rate_bpm IS NULL OR heart_rate_bpm BETWEEN 20 AND 300),
    CONSTRAINT chk_vital_respiratory_rate CHECK (respiratory_rate_bpm IS NULL OR respiratory_rate_bpm BETWEEN 5 AND 100),
    CONSTRAINT chk_vital_oxygen CHECK (oxygen_saturation_percent IS NULL OR oxygen_saturation_percent BETWEEN 1 AND 100),
    CONSTRAINT chk_vital_note_length CHECK (nurse_note IS NULL OR char_length(nurse_note) <= 4000),
    CONSTRAINT chk_vital_skip_length CHECK (skipped_reason IS NULL OR char_length(skipped_reason) <= 1000),
    CONSTRAINT chk_vital_correction_length CHECK (correction_reason IS NULL OR char_length(correction_reason) <= 1000),
	CONSTRAINT chk_vital_correction_provenance CHECK (
		(supersedes_revision_id IS NULL AND correction_reason IS NULL) OR
		(supersedes_revision_id IS NOT NULL AND NULLIF(BTRIM(correction_reason), '') IS NOT NULL)
	),
    CONSTRAINT chk_vital_finalization CHECK (
        (status = 'DRAFT' AND finalized_by IS NULL AND finalized_at IS NULL) OR
        (status = 'FINALIZED' AND finalized_by IS NOT NULL AND finalized_at IS NOT NULL)
    ),
    CONSTRAINT chk_vital_final_content CHECK (
        status = 'DRAFT' OR height_cm IS NOT NULL OR weight_kg IS NOT NULL OR
        temperature_c IS NOT NULL OR systolic_mmhg IS NOT NULL OR diastolic_mmhg IS NOT NULL OR
        heart_rate_bpm IS NOT NULL OR respiratory_rate_bpm IS NOT NULL OR
        oxygen_saturation_percent IS NOT NULL OR NULLIF(BTRIM(skipped_reason), '') IS NOT NULL
	),
	CONSTRAINT uq_vital_revision_encounter_id UNIQUE (encounter_id, id),
	CONSTRAINT fk_vital_supersedes_same_encounter FOREIGN KEY (encounter_id, supersedes_revision_id)
		REFERENCES vital_sign_revisions (encounter_id, id) ON DELETE RESTRICT,
	UNIQUE (encounter_id, version)
);
CREATE UNIQUE INDEX uq_vital_sign_draft
    ON vital_sign_revisions (encounter_id) WHERE status = 'DRAFT';
CREATE INDEX idx_vital_sign_encounter
    ON vital_sign_revisions (encounter_id, version DESC);

CREATE TABLE consultation_note_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    encounter_id UUID NOT NULL REFERENCES medical_encounters(id) ON DELETE RESTRICT,
    version INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'DRAFT',
    subjective TEXT,
    objective TEXT,
    assessment TEXT,
    plan TEXT,
    internal_note TEXT,
    authored_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    finalized_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    finalized_at TIMESTAMPTZ,
    supersedes_revision_id UUID REFERENCES consultation_note_revisions(id) ON DELETE RESTRICT,
    correction_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_consultation_note_status CHECK (status IN ('DRAFT', 'FINALIZED')),
    CONSTRAINT chk_consultation_subjective_length CHECK (subjective IS NULL OR char_length(subjective) <= 10000),
    CONSTRAINT chk_consultation_objective_length CHECK (objective IS NULL OR char_length(objective) <= 10000),
    CONSTRAINT chk_consultation_assessment_length CHECK (assessment IS NULL OR char_length(assessment) <= 10000),
    CONSTRAINT chk_consultation_plan_length CHECK (plan IS NULL OR char_length(plan) <= 10000),
    CONSTRAINT chk_consultation_internal_length CHECK (internal_note IS NULL OR char_length(internal_note) <= 4000),
    CONSTRAINT chk_consultation_correction_length CHECK (correction_reason IS NULL OR char_length(correction_reason) <= 1000),
	CONSTRAINT chk_consultation_correction_provenance CHECK (
		(supersedes_revision_id IS NULL AND correction_reason IS NULL) OR
		(supersedes_revision_id IS NOT NULL AND NULLIF(BTRIM(correction_reason), '') IS NOT NULL)
	),
    CONSTRAINT chk_consultation_finalization CHECK (
        (status = 'DRAFT' AND finalized_by IS NULL AND finalized_at IS NULL) OR
        (status = 'FINALIZED' AND finalized_by IS NOT NULL AND finalized_at IS NOT NULL)
    ),
    CONSTRAINT chk_consultation_final_content CHECK (
        status = 'DRAFT' OR (
            NULLIF(BTRIM(subjective), '') IS NOT NULL AND
            NULLIF(BTRIM(objective), '') IS NOT NULL AND
            NULLIF(BTRIM(assessment), '') IS NOT NULL AND
            NULLIF(BTRIM(plan), '') IS NOT NULL
        )
	),
	CONSTRAINT uq_consultation_revision_encounter_id UNIQUE (encounter_id, id),
	CONSTRAINT fk_consultation_supersedes_same_encounter FOREIGN KEY (encounter_id, supersedes_revision_id)
		REFERENCES consultation_note_revisions (encounter_id, id) ON DELETE RESTRICT,
	UNIQUE (encounter_id, version)
);
CREATE UNIQUE INDEX uq_consultation_note_draft
    ON consultation_note_revisions (encounter_id) WHERE status = 'DRAFT';
CREATE INDEX idx_consultation_note_encounter
    ON consultation_note_revisions (encounter_id, version DESC);

CREATE TABLE encounter_diagnoses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    consultation_revision_id UUID NOT NULL REFERENCES consultation_note_revisions(id) ON DELETE RESTRICT,
    diagnosis_type VARCHAR(16) NOT NULL,
    diagnosis_status VARCHAR(16) NOT NULL,
    icd10_code VARCHAR(16),
    diagnosis_name VARCHAR(500) NOT NULL,
    note TEXT,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_encounter_diagnosis_type CHECK (diagnosis_type IN ('PRIMARY', 'SECONDARY')),
    CONSTRAINT chk_encounter_diagnosis_status CHECK (diagnosis_status IN ('SUSPECTED', 'CONFIRMED', 'RULED_OUT')),
    CONSTRAINT chk_encounter_diagnosis_name CHECK (char_length(BTRIM(diagnosis_name)) BETWEEN 1 AND 500),
    CONSTRAINT chk_encounter_diagnosis_note CHECK (note IS NULL OR char_length(note) <= 4000)
);
CREATE UNIQUE INDEX uq_encounter_primary_diagnosis
    ON encounter_diagnoses (consultation_revision_id) WHERE diagnosis_type = 'PRIMARY';
CREATE INDEX idx_encounter_diagnoses_revision
    ON encounter_diagnoses (consultation_revision_id, created_at);

CREATE TABLE medical_record_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    encounter_id UUID NOT NULL REFERENCES medical_encounters(id) ON DELETE RESTRICT,
    document_type VARCHAR(24) NOT NULL,
    filename VARCHAR(255) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    bucket VARCHAR(100) NOT NULL,
    object_path VARCHAR(1000) NOT NULL UNIQUE,
    file_size BIGINT NOT NULL,
    sha256 CHAR(64) NOT NULL,
    note TEXT,
    uploaded_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_medical_attachment_type CHECK (document_type IN ('LAB_RESULT', 'IMAGING', 'CLINICAL_DOCUMENT', 'OTHER')),
    CONSTRAINT chk_medical_attachment_mime CHECK (mime_type IN ('application/pdf', 'image/jpeg', 'image/png')),
    CONSTRAINT chk_medical_attachment_size CHECK (file_size BETWEEN 1 AND 10485760),
    CONSTRAINT chk_medical_attachment_note CHECK (note IS NULL OR char_length(note) <= 1000)
);
CREATE INDEX idx_medical_record_attachments_encounter
    ON medical_record_attachments (encounter_id, created_at);

CREATE TABLE medical_record_audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    encounter_id UUID NOT NULL REFERENCES medical_encounters(id) ON DELETE RESTRICT,
    appointment_id UUID NOT NULL REFERENCES appointments(id) ON DELETE RESTRICT,
    patient_record_id UUID NOT NULL REFERENCES patient_records(id) ON DELETE RESTRICT,
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action VARCHAR(48) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_medical_record_audit_encounter
    ON medical_record_audit_events (encounter_id, created_at);
CREATE INDEX idx_medical_record_audit_actor
    ON medical_record_audit_events (actor_id, created_at);

-- Defense in depth for clinical immutability. Drafts may be edited and may
-- transition once to FINALIZED; finalized revisions can only be superseded by
-- a newly inserted revision.
CREATE FUNCTION guard_finalized_medical_revision()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
	IF TG_OP = 'INSERT' THEN
		IF NEW.status = 'FINALIZED' THEN
			RAISE EXCEPTION 'medical revisions must be finalized through a draft transition';
		END IF;
		RETURN NEW;
	END IF;
    IF OLD.status = 'FINALIZED' THEN
        RAISE EXCEPTION 'finalized medical revisions are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER trg_vital_revision_immutable
BEFORE INSERT OR UPDATE OR DELETE ON vital_sign_revisions
FOR EACH ROW EXECUTE FUNCTION guard_finalized_medical_revision();

CREATE TRIGGER trg_consultation_revision_immutable
BEFORE INSERT OR UPDATE OR DELETE ON consultation_note_revisions
FOR EACH ROW EXECUTE FUNCTION guard_finalized_medical_revision();

CREATE FUNCTION guard_finalized_diagnosis_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    revision_id UUID;
    revision_status VARCHAR(16);
BEGIN
    revision_id := CASE WHEN TG_OP = 'DELETE' THEN OLD.consultation_revision_id ELSE NEW.consultation_revision_id END;
    SELECT status INTO revision_status
    FROM public.consultation_note_revisions
    WHERE id = revision_id;
    IF revision_status = 'FINALIZED' THEN
        RAISE EXCEPTION 'diagnoses on finalized consultations are immutable';
    END IF;
    IF TG_OP = 'UPDATE' AND NEW.consultation_revision_id <> OLD.consultation_revision_id THEN
        SELECT status INTO revision_status
        FROM public.consultation_note_revisions
        WHERE id = OLD.consultation_revision_id;
        IF revision_status = 'FINALIZED' THEN
            RAISE EXCEPTION 'diagnoses on finalized consultations are immutable';
        END IF;
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER trg_diagnosis_immutable
BEFORE INSERT OR UPDATE OR DELETE ON encounter_diagnoses
FOR EACH ROW EXECUTE FUNCTION guard_finalized_diagnosis_mutation();

INSERT INTO permissions (name, slug, is_active, created_at, updated_at, deleted_at)
VALUES
    ('Examination View', 'examination.view', TRUE, NOW(), NOW(), NULL),
    ('Examination Vitals Write', 'examination.vitals.write', TRUE, NOW(), NOW(), NULL),
    ('Examination Consultation Write', 'examination.consultation.write', TRUE, NOW(), NOW(), NULL),
    ('Examination Record Correct', 'examination.correct', TRUE, NOW(), NOW(), NULL),
    ('Examination Attachment Manage', 'examination.attachment.manage', TRUE, NOW(), NOW(), NULL),
    ('Medical Record Self View', 'medical_record.self.view', TRUE, NOW(), NOW(), NULL)
ON CONFLICT (slug) DO UPDATE SET
    name = EXCLUDED.name,
    is_active = TRUE,
    updated_at = NOW(),
    deleted_at = NULL;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT role.id, permission.id, NOW()
FROM (VALUES
    ('SUPER_ADMIN', 'examination.view'),
    ('SUPER_ADMIN', 'examination.vitals.write'),
    ('SUPER_ADMIN', 'examination.consultation.write'),
    ('SUPER_ADMIN', 'examination.correct'),
    ('SUPER_ADMIN', 'examination.attachment.manage'),
    ('SUPER_ADMIN', 'medical_record.self.view'),
    ('ADMIN', 'examination.view'),
    ('ADMIN', 'examination.vitals.write'),
    ('ADMIN', 'examination.correct'),
    ('ADMIN', 'examination.attachment.manage'),
    ('NURSE', 'examination.view'),
    ('NURSE', 'examination.vitals.write'),
    ('NURSE', 'examination.correct'),
    ('NURSE', 'examination.attachment.manage'),
    ('DOCTOR', 'examination.view'),
    ('DOCTOR', 'examination.consultation.write'),
    ('DOCTOR', 'examination.correct'),
    ('DOCTOR', 'examination.attachment.manage'),
    ('PATIENT', 'medical_record.self.view')
) AS role_permission(role_slug, permission_slug)
JOIN roles role ON UPPER(role.slug) = role_permission.role_slug
                  AND role.deleted_at IS NULL
JOIN permissions permission ON permission.slug = role_permission.permission_slug
ON CONFLICT (role_id, permission_id) DO NOTHING;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'medikaone_app') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON medical_encounters, vital_sign_revisions, consultation_note_revisions TO medikaone_app';
        EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON encounter_diagnoses TO medikaone_app';
        EXECUTE 'GRANT SELECT, INSERT ON medical_record_attachments, medical_record_audit_events TO medikaone_app';
    END IF;
END
$$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Medical records are intentionally irreversible. Restore a verified backup
-- when a clinical-data rollback is required.
DO $$
BEGIN
    RAISE EXCEPTION 'examination migration is intentionally irreversible; restore a verified backup instead';
END
$$;

-- +goose StatementEnd
