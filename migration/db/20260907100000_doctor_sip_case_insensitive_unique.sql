-- +goose Up
-- +goose StatementBegin
-- SIP is normalized by every API write path with BTRIM. Enforce the same
-- identity rule in PostgreSQL so concurrent requests and direct SQL cannot
-- register case/whitespace variants of one SIP for different doctors.
DO $$
DECLARE
    conflicting_sip TEXT;
    conflicting_count BIGINT;
BEGIN
    SELECT LOWER(BTRIM(sip_number)), COUNT(*)
      INTO conflicting_sip, conflicting_count
      FROM doctor_profiles
     WHERE NULLIF(BTRIM(sip_number), '') IS NOT NULL
     GROUP BY LOWER(BTRIM(sip_number))
    HAVING COUNT(*) > 1
     ORDER BY LOWER(BTRIM(sip_number))
     LIMIT 1;

    IF conflicting_sip IS NOT NULL THEN
        RAISE EXCEPTION USING
            ERRCODE = '23505',
            MESSAGE = 'cannot enforce case-insensitive doctor SIP uniqueness: duplicate values exist',
            DETAIL = FORMAT(
                'Normalized SIP %L is assigned to %s doctor profiles.',
                conflicting_sip,
                conflicting_count
            ),
            HINT = 'Resolve every duplicate returned by: SELECT LOWER(BTRIM(sip_number)), ARRAY_AGG(user_id), COUNT(*) FROM doctor_profiles WHERE NULLIF(BTRIM(sip_number), '''') IS NOT NULL GROUP BY LOWER(BTRIM(sip_number)) HAVING COUNT(*) > 1;';
    END IF;
END $$;

CREATE UNIQUE INDEX ux_doctor_profiles_sip_number_normalized
    ON doctor_profiles ((LOWER(BTRIM(sip_number))))
    WHERE NULLIF(BTRIM(sip_number), '') IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'doctor SIP case-insensitive uniqueness migration is intentionally irreversible';
END $$;
-- +goose StatementEnd
