-- +goose Up
-- +goose StatementBegin
ALTER TABLE doctor_profiles
    ADD COLUMN medikaone_id VARCHAR(20) NOT NULL
        DEFAULT ('MDO-' || UPPER(ENCODE(gen_random_bytes(8), 'hex'))),
    ADD CONSTRAINT doctor_profiles_medikaone_id_key UNIQUE (medikaone_id),
    ADD CONSTRAINT doctor_profiles_medikaone_id_format
        CHECK (medikaone_id ~ '^MDO-[0-9A-F]{16}$');

-- Legacy tenant doctors and historical clinician references may predate profile
-- completion. Give them an identity without inventing a SIP or specialty.
INSERT INTO doctor_profiles (user_id)
SELECT doctor.user_id FROM (
    SELECT assignment.user_id FROM user_roles assignment JOIN roles role ON role.id = assignment.role_id
        WHERE UPPER(role.slug) = 'DOCTOR'
    UNION
    SELECT assignment.user_id FROM hospital_user_roles assignment JOIN roles role ON role.id = assignment.role_id
        WHERE UPPER(role.slug) = 'DOCTOR'
    UNION SELECT doctor_id FROM doctor_hospital_invitations
    UNION SELECT doctor_id FROM doctor_hospital_affiliations
    UNION SELECT doctor_id FROM appointments
    UNION SELECT doctor_id FROM medical_encounters
) doctor
ON CONFLICT (user_id) DO NOTHING;

CREATE FUNCTION ensure_doctor_identity_on_role_assignment() RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM roles WHERE id = NEW.role_id AND UPPER(slug) = 'DOCTOR') THEN
        INSERT INTO doctor_profiles (user_id) VALUES (NEW.user_id)
        ON CONFLICT (user_id) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER global_doctor_identity_on_assignment
    AFTER INSERT OR UPDATE OF user_id, role_id ON user_roles
    FOR EACH ROW EXECUTE FUNCTION ensure_doctor_identity_on_role_assignment();
CREATE TRIGGER tenant_doctor_identity_on_assignment
    AFTER INSERT OR UPDATE OF user_id, role_id ON hospital_user_roles
    FOR EACH ROW EXECUTE FUNCTION ensure_doctor_identity_on_role_assignment();

-- The volatile default generates a separate identifier for every existing row
-- and for every new profile, including profiles created by seeders.
CREATE FUNCTION prevent_doctor_medikaone_id_change() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.medikaone_id IS DISTINCT FROM OLD.medikaone_id THEN
        RAISE EXCEPTION 'doctor MedikaOne ID is immutable' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER doctor_medikaone_id_immutable
    BEFORE UPDATE OF medikaone_id ON doctor_profiles
    FOR EACH ROW EXECUTE FUNCTION prevent_doctor_medikaone_id_change();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER global_doctor_identity_on_assignment ON user_roles;
DROP TRIGGER tenant_doctor_identity_on_assignment ON hospital_user_roles;
DROP FUNCTION ensure_doctor_identity_on_role_assignment();
DROP TRIGGER doctor_medikaone_id_immutable ON doctor_profiles;
DROP FUNCTION prevent_doctor_medikaone_id_change();
ALTER TABLE doctor_profiles DROP COLUMN medikaone_id;
-- +goose StatementEnd
