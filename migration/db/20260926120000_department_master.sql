-- +goose Up
-- MedikaOne local catalogue aligned with the service names in Permenkes
-- 33/2023 and the specialty group used by Kemenkes RS Online. These are local
-- stable API codes, not SATUSEHAT, KKI, or SNOMED CT terminology codes.
CREATE TABLE master_departments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(40) NOT NULL UNIQUE,
    name VARCHAR(120) NOT NULL,
    category VARCHAR(32) NOT NULL,
    sort_order SMALLINT NOT NULL CHECK (sort_order BETWEEN 1 AND 1000),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_master_department_code CHECK (code = UPPER(code)),
    CONSTRAINT chk_master_department_category CHECK (category IN (
        'GENERAL', 'MEDICAL_SPECIALIST', 'SURGICAL_SPECIALIST',
        'DENTAL', 'SUPPORT_SPECIALIST'
    ))
);

INSERT INTO master_departments (code, name, category, sort_order) VALUES
    ('POLI-UMUM', 'Poli Umum', 'GENERAL', 10),
    ('POLI-KKLP', 'Poli Kedokteran Keluarga dan Layanan Primer', 'GENERAL', 20),
    ('POLI-GIGI-UMUM', 'Poli Gigi Umum', 'DENTAL', 30),
    ('POLI-PENYAKIT-DALAM', 'Poli Penyakit Dalam', 'MEDICAL_SPECIALIST', 40),
    ('POLI-ANAK', 'Poli Anak', 'MEDICAL_SPECIALIST', 50),
    ('POLI-OBGYN', 'Poli Obstetri dan Ginekologi', 'MEDICAL_SPECIALIST', 60),
    ('POLI-MATA', 'Poli Mata', 'MEDICAL_SPECIALIST', 70),
    ('POLI-THT-KL', 'Poli THT, Kepala, dan Leher', 'MEDICAL_SPECIALIST', 80),
    ('POLI-SARAF', 'Poli Saraf', 'MEDICAL_SPECIALIST', 90),
    ('POLI-JANTUNG', 'Poli Jantung dan Pembuluh Darah', 'MEDICAL_SPECIALIST', 100),
    ('POLI-KULIT-KELAMIN', 'Poli Dermatologi, Venereologi, dan Estetika', 'MEDICAL_SPECIALIST', 110),
    ('POLI-JIWA', 'Poli Kedokteran Jiwa', 'MEDICAL_SPECIALIST', 120),
    ('POLI-PARU', 'Poli Pulmonologi dan Kedokteran Respirasi', 'MEDICAL_SPECIALIST', 130),
    ('POLI-GIZI-KLINIK', 'Poli Gizi Klinik', 'MEDICAL_SPECIALIST', 140),
    ('POLI-ANDROLOGI', 'Poli Andrologi', 'MEDICAL_SPECIALIST', 150),
    ('POLI-KEDOKTERAN-OLAHRAGA', 'Poli Kedokteran Olahraga', 'MEDICAL_SPECIALIST', 160),
    ('POLI-KEDOKTERAN-OKUPASI', 'Poli Kedokteran Okupasi', 'MEDICAL_SPECIALIST', 170),
    ('POLI-FARMAKOLOGI-KLINIK', 'Poli Farmakologi Klinik', 'MEDICAL_SPECIALIST', 180),
    ('POLI-AKUPUNKTUR-MEDIK', 'Poli Akupunktur Medik', 'MEDICAL_SPECIALIST', 190),
    ('POLI-KEDARURATAN-MEDIK', 'Poli Kedaruratan Medik', 'MEDICAL_SPECIALIST', 200),
    ('POLI-BEDAH-UMUM', 'Poli Bedah Umum', 'SURGICAL_SPECIALIST', 210),
    ('POLI-ORTHOPAEDI-TRAUMATOLOGI', 'Poli Orthopaedi dan Traumatologi', 'SURGICAL_SPECIALIST', 220),
    ('POLI-UROLOGI', 'Poli Urologi', 'SURGICAL_SPECIALIST', 230),
    ('POLI-BEDAH-SARAF', 'Poli Bedah Saraf', 'SURGICAL_SPECIALIST', 240),
    ('POLI-BEDAH-PLASTIK', 'Poli Bedah Plastik Rekonstruksi dan Estetika', 'SURGICAL_SPECIALIST', 250),
    ('POLI-BEDAH-ANAK', 'Poli Bedah Anak', 'SURGICAL_SPECIALIST', 260),
    ('POLI-BTKV', 'Poli Bedah Toraks, Kardiak, dan Vaskular', 'SURGICAL_SPECIALIST', 270),
    ('POLI-BEDAH-MULUT', 'Poli Bedah Mulut dan Maksilofasial', 'DENTAL', 280),
    ('POLI-KONSERVASI-GIGI', 'Poli Konservasi Gigi', 'DENTAL', 290),
    ('POLI-ORTODONSIA', 'Poli Ortodonsia', 'DENTAL', 300),
    ('POLI-PERIODONSIA', 'Poli Periodonsia', 'DENTAL', 310),
    ('POLI-PROSTODONSIA', 'Poli Prostodonsia', 'DENTAL', 320),
    ('POLI-GIGI-ANAK', 'Poli Kedokteran Gigi Anak', 'DENTAL', 330),
    ('POLI-PENYAKIT-MULUT', 'Poli Penyakit Mulut', 'DENTAL', 340),
    ('POLI-RADIOLOGI-GIGI', 'Poli Radiologi Kedokteran Gigi', 'DENTAL', 350),
    ('POLI-ANESTESIOLOGI', 'Poli Anestesiologi dan Terapi Intensif', 'SUPPORT_SPECIALIST', 360),
    ('POLI-REHABILITASI-MEDIK', 'Poli Kedokteran Fisik dan Rehabilitasi', 'SUPPORT_SPECIALIST', 370),
    ('POLI-RADIOLOGI', 'Poli Radiologi', 'SUPPORT_SPECIALIST', 380),
    ('POLI-PATOLOGI-KLINIK', 'Poli Patologi Klinik', 'SUPPORT_SPECIALIST', 390),
    ('POLI-PATOLOGI-ANATOMIK', 'Poli Patologi Anatomik', 'SUPPORT_SPECIALIST', 400),
    ('POLI-MIKROBIOLOGI-KLINIK', 'Poli Mikrobiologi Klinik', 'SUPPORT_SPECIALIST', 410),
    ('POLI-PARASITOLOGI-KLINIK', 'Poli Parasitologi Klinik', 'SUPPORT_SPECIALIST', 420),
    ('POLI-ONKOLOGI-RADIASI', 'Poli Onkologi Radiasi', 'SUPPORT_SPECIALIST', 430),
    ('POLI-KEDOKTERAN-NUKLIR', 'Poli Kedokteran Nuklir dan Teranostik Molekuler', 'SUPPORT_SPECIALIST', 440),
    ('POLI-FORENSIK-MEDIKOLEGAL', 'Poli Kedokteran Forensik dan Medikolegal', 'SUPPORT_SPECIALIST', 450);

ALTER TABLE hospital_departments
    ADD COLUMN master_department_id UUID REFERENCES master_departments(id) ON DELETE RESTRICT;

UPDATE hospital_departments department
SET master_department_id = master.id,
    code = master.code,
    name = master.name
FROM master_departments master
WHERE UPPER(department.code) = master.code;

CREATE UNIQUE INDEX uq_hospital_departments_master
    ON hospital_departments (hospital_id, master_department_id)
    WHERE master_department_id IS NOT NULL;
CREATE INDEX idx_master_departments_active_list
    ON master_departments (is_active, sort_order, name, id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.medikaone_sync_hospital_department_master()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    selected_code VARCHAR(40);
    selected_name VARCHAR(120);
BEGIN
    -- Nullable only preserves legacy production rows whose old free-form code
    -- cannot be mapped safely. New application writes always provide a master.
    IF NEW.master_department_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT code, name
      INTO selected_code, selected_name
      FROM public.master_departments
     WHERE id = NEW.master_department_id
       AND is_active = TRUE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'active master department not found'
            USING ERRCODE = '23503', CONSTRAINT = 'hospital_department_active_master';
    END IF;

    NEW.code := selected_code;
    NEW.name := selected_name;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_hospital_department_master
BEFORE INSERT OR UPDATE OF master_department_id, code, name
ON public.hospital_departments
FOR EACH ROW EXECUTE FUNCTION public.medikaone_sync_hospital_department_master();

REVOKE ALL ON FUNCTION public.medikaone_sync_hospital_department_master() FROM PUBLIC;

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'medikaone_app') THEN
        GRANT SELECT ON public.master_departments TO medikaone_app;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'department master migration changes the placement contract; restore a verified backup to roll back';
END $$;
-- +goose StatementEnd
