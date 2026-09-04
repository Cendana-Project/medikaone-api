-- +goose Up
-- +goose StatementBegin

CREATE TABLE hospital_medications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hospital_id UUID NOT NULL REFERENCES hospitals(id) ON DELETE RESTRICT,
    code VARCHAR(64),
    kfa_code VARCHAR(64),
    generic_name VARCHAR(255) NOT NULL,
    brand_name VARCHAR(255),
    dosage_form VARCHAR(100) NOT NULL,
    strength VARCHAR(100) NOT NULL,
    default_unit VARCHAR(50) NOT NULL,
    default_route VARCHAR(50),
    controlled_category VARCHAR(24) NOT NULL DEFAULT 'NONE',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_hospital_medication_generic_name
        CHECK (char_length(BTRIM(generic_name)) BETWEEN 1 AND 255),
    CONSTRAINT chk_hospital_medication_dosage_form
        CHECK (char_length(BTRIM(dosage_form)) BETWEEN 1 AND 100),
    CONSTRAINT chk_hospital_medication_strength
        CHECK (char_length(BTRIM(strength)) BETWEEN 1 AND 100),
    CONSTRAINT chk_hospital_medication_default_unit
        CHECK (char_length(BTRIM(default_unit)) BETWEEN 1 AND 50),
    CONSTRAINT chk_hospital_medication_controlled
        CHECK (controlled_category IN ('NONE', 'NARCOTIC', 'PSYCHOTROPIC'))
);
CREATE UNIQUE INDEX uq_hospital_medication_code
    ON hospital_medications (hospital_id, LOWER(code)) WHERE code IS NOT NULL;
CREATE INDEX idx_hospital_medication_search
    ON hospital_medications (hospital_id, is_active, LOWER(generic_name));

CREATE TABLE prescriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    encounter_id UUID NOT NULL UNIQUE REFERENCES medical_encounters(id) ON DELETE RESTRICT,
    prescription_number VARCHAR(40) NOT NULL UNIQUE,
    status VARCHAR(24) NOT NULL DEFAULT 'DRAFT',
    current_revision_id UUID,
    no_medication_reason TEXT,
    cancelled_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    cancelled_at TIMESTAMPTZ,
    cancellation_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_prescription_status
        CHECK (status IN ('DRAFT', 'ISSUED', 'NO_MEDICATION', 'CANCELLED')),
    CONSTRAINT chk_prescription_no_medication
        CHECK ((status = 'NO_MEDICATION' AND NULLIF(BTRIM(no_medication_reason), '') IS NOT NULL)
            OR (status <> 'NO_MEDICATION' AND no_medication_reason IS NULL)),
    CONSTRAINT chk_prescription_cancellation
        CHECK ((status = 'CANCELLED' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL
                AND NULLIF(BTRIM(cancellation_reason), '') IS NOT NULL)
            OR (status <> 'CANCELLED' AND cancelled_by IS NULL AND cancelled_at IS NULL
                AND cancellation_reason IS NULL))
);
CREATE INDEX idx_prescriptions_status ON prescriptions (status, updated_at DESC);

CREATE TABLE prescription_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prescription_id UUID NOT NULL REFERENCES prescriptions(id) ON DELETE RESTRICT,
    version INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'DRAFT',
    general_note TEXT,
    allergies_reviewed BOOLEAN NOT NULL DEFAULT FALSE,
    repeats_allowed SMALLINT NOT NULL DEFAULT 0,
    authored_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    issued_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    issued_at TIMESTAMPTZ,
    supersedes_revision_id UUID REFERENCES prescription_revisions(id) ON DELETE RESTRICT,
    correction_reason TEXT,
    verification_token_hash CHAR(64),
    patient_name_snapshot VARCHAR(201),
    patient_date_of_birth_snapshot DATE,
    patient_gender_snapshot VARCHAR(1),
    patient_allergies_snapshot TEXT,
    hospital_name_snapshot VARCHAR(255),
    hospital_address_snapshot TEXT,
    hospital_phone_snapshot VARCHAR(32),
    doctor_name_snapshot VARCHAR(201),
    doctor_sip_number_snapshot VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_prescription_revision_status CHECK (status IN ('DRAFT', 'ISSUED')),
    CONSTRAINT chk_prescription_revision_repeats CHECK (repeats_allowed BETWEEN 0 AND 12),
    CONSTRAINT chk_prescription_revision_note CHECK (general_note IS NULL OR char_length(general_note) <= 4000),
    CONSTRAINT chk_prescription_revision_correction CHECK (correction_reason IS NULL OR char_length(correction_reason) <= 1000),
    CONSTRAINT chk_prescription_revision_issue
        CHECK ((status = 'DRAFT' AND issued_by IS NULL AND issued_at IS NULL AND verification_token_hash IS NULL
                AND patient_name_snapshot IS NULL AND patient_date_of_birth_snapshot IS NULL
                AND patient_gender_snapshot IS NULL AND patient_allergies_snapshot IS NULL
                AND hospital_name_snapshot IS NULL AND hospital_address_snapshot IS NULL
                AND hospital_phone_snapshot IS NULL AND doctor_name_snapshot IS NULL
                AND doctor_sip_number_snapshot IS NULL)
            OR (status = 'ISSUED' AND issued_by IS NOT NULL AND issued_at IS NOT NULL
                AND allergies_reviewed = TRUE AND verification_token_hash IS NOT NULL
                AND NULLIF(BTRIM(patient_name_snapshot), '') IS NOT NULL
                AND patient_date_of_birth_snapshot IS NOT NULL
                AND patient_gender_snapshot IN ('L', 'P')
                AND NULLIF(BTRIM(hospital_name_snapshot), '') IS NOT NULL
                AND NULLIF(BTRIM(doctor_name_snapshot), '') IS NOT NULL
                AND NULLIF(BTRIM(doctor_sip_number_snapshot), '') IS NOT NULL)),
    CONSTRAINT chk_prescription_revision_provenance
        CHECK ((supersedes_revision_id IS NULL AND correction_reason IS NULL)
            OR (supersedes_revision_id IS NOT NULL AND NULLIF(BTRIM(correction_reason), '') IS NOT NULL)),
    CONSTRAINT uq_prescription_revision_parent UNIQUE (prescription_id, id),
    CONSTRAINT fk_prescription_revision_supersedes_same_parent
        FOREIGN KEY (prescription_id, supersedes_revision_id)
        REFERENCES prescription_revisions (prescription_id, id) ON DELETE RESTRICT,
    UNIQUE (prescription_id, version),
    UNIQUE (verification_token_hash)
);
CREATE UNIQUE INDEX uq_prescription_draft
    ON prescription_revisions (prescription_id) WHERE status = 'DRAFT';
CREATE INDEX idx_prescription_revision_latest
    ON prescription_revisions (prescription_id, version DESC);

ALTER TABLE prescriptions
    ADD CONSTRAINT fk_prescriptions_current_revision
    FOREIGN KEY (id, current_revision_id)
    REFERENCES prescription_revisions (prescription_id, id) ON DELETE RESTRICT;

CREATE TABLE prescription_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    revision_id UUID NOT NULL REFERENCES prescription_revisions(id) ON DELETE RESTRICT,
    item_order SMALLINT NOT NULL,
    item_type VARCHAR(24) NOT NULL,
    medication_id UUID REFERENCES hospital_medications(id) ON DELETE RESTRICT,
    medication_name VARCHAR(255) NOT NULL,
    dosage_form VARCHAR(100) NOT NULL,
    strength VARCHAR(100) NOT NULL,
    dose_amount NUMERIC(12,4) NOT NULL,
    dose_unit VARCHAR(50) NOT NULL,
    route VARCHAR(50) NOT NULL,
    frequency_per_day SMALLINT,
    interval_hours SMALLINT,
    timing_instructions VARCHAR(500),
    duration_value SMALLINT NOT NULL,
    duration_unit VARCHAR(16) NOT NULL,
    quantity NUMERIC(12,4) NOT NULL,
    quantity_unit VARCHAR(50) NOT NULL,
    directions TEXT NOT NULL,
    as_needed BOOLEAN NOT NULL DEFAULT FALSE,
    max_daily_dose VARCHAR(100),
    controlled_substance BOOLEAN NOT NULL DEFAULT FALSE,
    satusehat_medication_request_id VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_prescription_item_order CHECK (item_order > 0),
    CONSTRAINT chk_prescription_item_type CHECK (item_type IN ('NON_COMPOUND', 'COMPOUND')),
    CONSTRAINT chk_prescription_item_name CHECK (char_length(BTRIM(medication_name)) BETWEEN 1 AND 255),
    CONSTRAINT chk_prescription_item_dose CHECK (dose_amount > 0),
    CONSTRAINT chk_prescription_item_frequency CHECK (frequency_per_day IS NULL OR frequency_per_day BETWEEN 1 AND 24),
    CONSTRAINT chk_prescription_item_interval CHECK (interval_hours IS NULL OR interval_hours BETWEEN 1 AND 168),
    CONSTRAINT chk_prescription_item_schedule CHECK (frequency_per_day IS NOT NULL OR interval_hours IS NOT NULL OR as_needed = TRUE),
    CONSTRAINT chk_prescription_item_duration CHECK (duration_value BETWEEN 1 AND 3650),
    CONSTRAINT chk_prescription_item_duration_unit CHECK (duration_unit IN ('DAY', 'WEEK', 'MONTH')),
    CONSTRAINT chk_prescription_item_quantity CHECK (quantity > 0),
    CONSTRAINT chk_prescription_item_directions CHECK (char_length(BTRIM(directions)) BETWEEN 1 AND 2000),
    CONSTRAINT chk_prescription_item_not_controlled CHECK (controlled_substance = FALSE),
    CONSTRAINT chk_prescription_item_compound_catalog
        CHECK (item_type = 'NON_COMPOUND' OR medication_id IS NULL),
    UNIQUE (revision_id, item_order)
);
CREATE INDEX idx_prescription_items_revision ON prescription_items (revision_id, item_order);

CREATE TABLE prescription_item_components (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prescription_item_id UUID NOT NULL REFERENCES prescription_items(id) ON DELETE RESTRICT,
    component_order SMALLINT NOT NULL,
    medication_id UUID REFERENCES hospital_medications(id) ON DELETE RESTRICT,
    medication_name VARCHAR(255) NOT NULL,
    dosage_form VARCHAR(100) NOT NULL,
    strength VARCHAR(100) NOT NULL,
    amount NUMERIC(12,4) NOT NULL,
    unit VARCHAR(50) NOT NULL,
    controlled_substance BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_prescription_component_order CHECK (component_order > 0),
    CONSTRAINT chk_prescription_component_name CHECK (char_length(BTRIM(medication_name)) BETWEEN 1 AND 255),
    CONSTRAINT chk_prescription_component_amount CHECK (amount > 0),
    CONSTRAINT chk_prescription_component_not_controlled CHECK (controlled_substance = FALSE),
    UNIQUE (prescription_item_id, component_order)
);

CREATE TABLE prescription_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    revision_id UUID NOT NULL UNIQUE REFERENCES prescription_revisions(id) ON DELETE RESTRICT,
    bucket VARCHAR(100) NOT NULL,
    object_path VARCHAR(1000) NOT NULL UNIQUE,
    filename VARCHAR(255) NOT NULL,
    mime_type VARCHAR(100) NOT NULL DEFAULT 'application/pdf',
    file_size BIGINT NOT NULL,
    sha256 CHAR(64) NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT chk_prescription_document_mime CHECK (mime_type = 'application/pdf'),
    CONSTRAINT chk_prescription_document_size CHECK (file_size BETWEEN 1 AND 10485760)
);

CREATE TABLE prescription_audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prescription_id UUID NOT NULL REFERENCES prescriptions(id) ON DELETE RESTRICT,
    revision_id UUID REFERENCES prescription_revisions(id) ON DELETE SET NULL,
    actor_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    action VARCHAR(48) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_prescription_audit_events
    ON prescription_audit_events (prescription_id, created_at);

CREATE FUNCTION guard_issued_prescription_revision()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status = 'ISSUED' THEN
            RAISE EXCEPTION 'prescription revisions must be issued through a draft transition';
        END IF;
        RETURN NEW;
    END IF;
    IF OLD.status = 'ISSUED' THEN
        RAISE EXCEPTION 'issued prescription revisions are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER trg_prescription_revision_immutable
BEFORE INSERT OR UPDATE OR DELETE ON prescription_revisions
FOR EACH ROW EXECUTE FUNCTION guard_issued_prescription_revision();

CREATE FUNCTION guard_issued_prescription_child()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    parent_revision_id UUID;
    parent_status VARCHAR(16);
    old_parent_revision_id UUID;
    old_parent_status VARCHAR(16);
BEGIN
    IF TG_TABLE_NAME = 'prescription_item_components' THEN
        SELECT item.revision_id INTO parent_revision_id
        FROM public.prescription_items item
        WHERE item.id = CASE WHEN TG_OP = 'DELETE' THEN OLD.prescription_item_id ELSE NEW.prescription_item_id END;

		IF TG_OP = 'UPDATE' THEN
			SELECT item.revision_id INTO old_parent_revision_id
			FROM public.prescription_items item
			WHERE item.id = OLD.prescription_item_id;
		END IF;
    ELSE
        parent_revision_id := CASE WHEN TG_OP = 'DELETE' THEN OLD.revision_id ELSE NEW.revision_id END;
		IF TG_OP = 'UPDATE' THEN
			old_parent_revision_id := OLD.revision_id;
		END IF;
    END IF;
    SELECT status INTO parent_status FROM public.prescription_revisions WHERE id = parent_revision_id;
    IF parent_status = 'ISSUED' THEN
        RAISE EXCEPTION 'issued prescription content is immutable';
    END IF;
	IF old_parent_revision_id IS NOT NULL THEN
		SELECT status INTO old_parent_status FROM public.prescription_revisions WHERE id = old_parent_revision_id;
		IF old_parent_status = 'ISSUED' THEN
			RAISE EXCEPTION 'issued prescription content is immutable';
		END IF;
	END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER trg_prescription_item_immutable
BEFORE INSERT OR UPDATE OR DELETE ON prescription_items
FOR EACH ROW EXECUTE FUNCTION guard_issued_prescription_child();
CREATE TRIGGER trg_prescription_component_immutable
BEFORE INSERT OR UPDATE OR DELETE ON prescription_item_components
FOR EACH ROW EXECUTE FUNCTION guard_issued_prescription_child();

CREATE FUNCTION validate_prescription_revision_for_issue()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
	IF OLD.status = 'DRAFT' AND NEW.status = 'ISSUED' THEN
		IF NOT EXISTS (
			SELECT 1 FROM public.prescription_items item WHERE item.revision_id = NEW.id
		) THEN
			RAISE EXCEPTION 'an issued prescription must contain at least one item';
		END IF;

		IF EXISTS (
			SELECT 1
			FROM public.prescription_items item
			WHERE item.revision_id = NEW.id
			  AND (
				(item.item_type = 'COMPOUND' AND (
					item.medication_id IS NOT NULL OR NOT EXISTS (
						SELECT 1 FROM public.prescription_item_components component
						WHERE component.prescription_item_id = item.id
					)
				))
				OR (item.item_type = 'NON_COMPOUND' AND EXISTS (
					SELECT 1 FROM public.prescription_item_components component
					WHERE component.prescription_item_id = item.id
				))
			  )
		) THEN
			RAISE EXCEPTION 'prescription compound structure is invalid';
		END IF;

		IF EXISTS (
			SELECT 1
			FROM public.prescription_items item
			JOIN public.prescription_revisions revision ON revision.id = item.revision_id
			JOIN public.prescriptions prescription ON prescription.id = revision.prescription_id
			JOIN public.medical_encounters encounter ON encounter.id = prescription.encounter_id
			JOIN public.hospital_medications medication ON medication.id = item.medication_id
			WHERE item.revision_id = NEW.id AND medication.hospital_id <> encounter.hospital_id
		) OR EXISTS (
			SELECT 1
			FROM public.prescription_item_components component
			JOIN public.prescription_items item ON item.id = component.prescription_item_id
			JOIN public.prescription_revisions revision ON revision.id = item.revision_id
			JOIN public.prescriptions prescription ON prescription.id = revision.prescription_id
			JOIN public.medical_encounters encounter ON encounter.id = prescription.encounter_id
			JOIN public.hospital_medications medication ON medication.id = component.medication_id
			WHERE item.revision_id = NEW.id AND medication.hospital_id <> encounter.hospital_id
		) THEN
			RAISE EXCEPTION 'prescription medication belongs to another hospital';
		END IF;
	END IF;
	RETURN NEW;
END
$$;

CREATE TRIGGER trg_prescription_revision_validate_issue
BEFORE UPDATE ON prescription_revisions
FOR EACH ROW EXECUTE FUNCTION validate_prescription_revision_for_issue();

INSERT INTO permissions (name, slug, is_active, created_at, updated_at, deleted_at)
VALUES
    ('Prescription View', 'prescription.view', TRUE, NOW(), NOW(), NULL),
    ('Prescription Write', 'prescription.write', TRUE, NOW(), NOW(), NULL),
    ('Prescription Correct', 'prescription.correct', TRUE, NOW(), NOW(), NULL),
    ('Prescription Print', 'prescription.print', TRUE, NOW(), NOW(), NULL),
    ('Medication Catalogue Manage', 'medication_catalog.manage', TRUE, NOW(), NOW(), NULL),
    ('Prescription Self View', 'prescription.self.view', TRUE, NOW(), NOW(), NULL)
ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, is_active = TRUE,
    updated_at = NOW(), deleted_at = NULL;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT role.id, permission.id, NOW()
FROM (VALUES
    ('SUPER_ADMIN', 'prescription.view'), ('SUPER_ADMIN', 'prescription.write'),
    ('SUPER_ADMIN', 'prescription.correct'), ('SUPER_ADMIN', 'prescription.print'),
    ('SUPER_ADMIN', 'medication_catalog.manage'), ('SUPER_ADMIN', 'prescription.self.view'),
    ('ADMIN', 'prescription.view'), ('ADMIN', 'prescription.print'),
    ('ADMIN', 'medication_catalog.manage'),
    ('NURSE', 'prescription.view'), ('NURSE', 'prescription.print'),
    ('DOCTOR', 'prescription.view'), ('DOCTOR', 'prescription.write'),
    ('DOCTOR', 'prescription.correct'), ('DOCTOR', 'prescription.print'),
    ('PATIENT', 'prescription.self.view')
) AS role_permission(role_slug, permission_slug)
JOIN roles role ON UPPER(role.slug) = role_permission.role_slug AND role.deleted_at IS NULL
JOIN permissions permission ON permission.slug = role_permission.permission_slug
ON CONFLICT (role_id, permission_id) DO NOTHING;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'medikaone_app') THEN
		EXECUTE 'GRANT SELECT, INSERT, UPDATE ON hospital_medications, prescriptions TO medikaone_app';
		EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON prescription_revisions TO medikaone_app';
        EXECUTE 'GRANT SELECT, INSERT, DELETE ON prescription_items, prescription_item_components TO medikaone_app';
        EXECUTE 'GRANT SELECT, INSERT ON prescription_documents, prescription_audit_events TO medikaone_app';
    END IF;
END
$$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Issued prescriptions are clinical records. Restore a verified backup when a
-- rollback of this schema is required.
DO $$
BEGIN
    RAISE EXCEPTION 'prescription migration is intentionally irreversible; restore a verified backup instead';
END
$$;

-- +goose StatementEnd
