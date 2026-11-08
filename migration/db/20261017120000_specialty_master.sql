-- +goose Up
-- Curated Indonesian clinical fields, not exhaustive or external terminology codes.
-- Sources: KMK HK.01.07/MENKES/1320/2023 (clinical specialty grouping), and
-- Kemenkes RS Online https://sirs.kemkes.go.id/fo/home/profile_rs/3374373
-- (dental fields). Core fields also follow the existing local department catalogue.
CREATE TABLE master_specialties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(160) NOT NULL,
    category VARCHAR(32) NOT NULL,
    sort_order SMALLINT NOT NULL CHECK (sort_order BETWEEN 1 AND 1000),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_master_specialty_code CHECK (code ~ '^SP-[A-Z0-9]+(-[A-Z0-9]+)*$'),
    CONSTRAINT chk_master_specialty_category CHECK (category IN ('GENERAL', 'MEDICAL_SPECIALIST', 'DENTAL_SPECIALIST'))
);

INSERT INTO master_specialties (code, name, category, sort_order) VALUES
    ('SP-UMUM', 'Dokter Umum', 'GENERAL', 10),
    ('SP-GIGI-UMUM', 'Dokter Gigi Umum', 'GENERAL', 20),
    ('SP-PENYAKIT-DALAM', 'Penyakit Dalam', 'MEDICAL_SPECIALIST', 30),
    ('SP-ANAK', 'Anak', 'MEDICAL_SPECIALIST', 40),
    ('SP-OBGYN', 'Obstetri dan Ginekologi', 'MEDICAL_SPECIALIST', 50),
    ('SP-MATA', 'Mata', 'MEDICAL_SPECIALIST', 60),
    ('SP-THT-KL', 'Telinga Hidung Tenggorok Bedah Kepala dan Leher', 'MEDICAL_SPECIALIST', 70),
    ('SP-SARAF', 'Neurologi', 'MEDICAL_SPECIALIST', 80),
    ('SP-JANTUNG', 'Jantung dan Pembuluh Darah', 'MEDICAL_SPECIALIST', 90),
    ('SP-KULIT-KELAMIN', 'Dermatologi, Venereologi, dan Estetika', 'MEDICAL_SPECIALIST', 100),
    ('SP-JIWA', 'Kedokteran Jiwa', 'MEDICAL_SPECIALIST', 110),
    ('SP-PARU', 'Pulmonologi dan Kedokteran Respirasi', 'MEDICAL_SPECIALIST', 120),
    ('SP-GIZI-KLINIK', 'Gizi Klinik', 'MEDICAL_SPECIALIST', 130),
    ('SP-BEDAH-UMUM', 'Bedah', 'MEDICAL_SPECIALIST', 140),
    ('SP-ORTHOPAEDI-TRAUMATOLOGI', 'Orthopaedi dan Traumatologi', 'MEDICAL_SPECIALIST', 150),
    ('SP-UROLOGI', 'Urologi', 'MEDICAL_SPECIALIST', 160),
    ('SP-BEDAH-SARAF', 'Bedah Saraf', 'MEDICAL_SPECIALIST', 170),
    ('SP-BEDAH-PLASTIK', 'Bedah Plastik Rekonstruksi dan Estetika', 'MEDICAL_SPECIALIST', 180),
    ('SP-BEDAH-ANAK', 'Bedah Anak', 'MEDICAL_SPECIALIST', 190),
    ('SP-BTKV', 'Bedah Toraks, Kardiak, dan Vaskular', 'MEDICAL_SPECIALIST', 200),
    ('SP-ANESTESIOLOGI', 'Anestesiologi dan Terapi Intensif', 'MEDICAL_SPECIALIST', 210),
    ('SP-REHABILITASI-MEDIK', 'Kedokteran Fisik dan Rehabilitasi', 'MEDICAL_SPECIALIST', 220),
    ('SP-RADIOLOGI', 'Radiologi', 'MEDICAL_SPECIALIST', 230),
    ('SP-PATOLOGI-KLINIK', 'Patologi Klinik', 'MEDICAL_SPECIALIST', 240),
    ('SP-PATOLOGI-ANATOMIK', 'Patologi Anatomik', 'MEDICAL_SPECIALIST', 250),
    ('SP-MIKROBIOLOGI-KLINIK', 'Mikrobiologi Klinik', 'MEDICAL_SPECIALIST', 260),
    ('SP-ONKOLOGI-RADIASI', 'Onkologi Radiasi', 'MEDICAL_SPECIALIST', 270),
    ('SP-KEDOKTERAN-NUKLIR', 'Kedokteran Nuklir dan Teranostik Molekuler', 'MEDICAL_SPECIALIST', 280),
    ('SP-BEDAH-MULUT', 'Bedah Mulut dan Maksilofasial', 'DENTAL_SPECIALIST', 290),
    ('SP-KONSERVASI-GIGI', 'Konservasi Gigi', 'DENTAL_SPECIALIST', 300),
    ('SP-ORTODONSIA', 'Ortodonsia', 'DENTAL_SPECIALIST', 310),
    ('SP-PERIODONSIA', 'Periodonsia', 'DENTAL_SPECIALIST', 320),
    ('SP-PROSTODONSIA', 'Prostodonsia', 'DENTAL_SPECIALIST', 330),
    ('SP-GIGI-ANAK', 'Kedokteran Gigi Anak', 'DENTAL_SPECIALIST', 340),
    ('SP-PENYAKIT-MULUT', 'Penyakit Mulut', 'DENTAL_SPECIALIST', 350),
    ('SP-RADIOLOGI-GIGI', 'Radiologi Kedokteran Gigi', 'DENTAL_SPECIALIST', 360);

ALTER TABLE doctor_profiles RENAME COLUMN specialty TO specialty_legacy;
ALTER TABLE doctor_profiles ADD COLUMN specialty_id UUID
    CONSTRAINT doctor_profiles_specialty_id_fkey REFERENCES master_specialties(id) ON DELETE RESTRICT;

-- Only exact case-folded/trimmed canonical names, codes, and these explicit
-- unambiguous fixture aliases qualify. Unknown free text remains private audit data.
UPDATE doctor_profiles profile
SET specialty_id = master.id
FROM master_specialties master
WHERE LOWER(BTRIM(profile.specialty_legacy)) IN (LOWER(master.name), LOWER(master.code))
   OR (LOWER(BTRIM(profile.specialty_legacy)) = 'umum' AND master.code = 'SP-UMUM')
   OR (LOWER(BTRIM(profile.specialty_legacy)) = 'paru' AND master.code = 'SP-PARU');

CREATE INDEX idx_doctor_profiles_specialty_id ON doctor_profiles(specialty_id);
CREATE INDEX idx_master_specialties_active_list ON master_specialties(is_active, sort_order, name, id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.medikaone_guard_doctor_specialty()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF NEW.specialty_id IS NOT NULL THEN
        PERFORM id FROM public.master_specialties
         WHERE id = NEW.specialty_id AND is_active = TRUE FOR SHARE;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'active master specialty not found'
                USING ERRCODE = '23503', CONSTRAINT = 'doctor_profiles_active_specialty';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_doctor_profiles_active_specialty
BEFORE INSERT OR UPDATE OF specialty_id ON public.doctor_profiles
FOR EACH ROW EXECUTE FUNCTION public.medikaone_guard_doctor_specialty();
REVOKE ALL ON FUNCTION public.medikaone_guard_doctor_specialty() FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'medikaone_app') THEN
        GRANT SELECT ON public.master_specialties TO medikaone_app;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'specialty master migration preserves private legacy data; restore a verified backup to roll back';
END $$;
-- +goose StatementEnd
