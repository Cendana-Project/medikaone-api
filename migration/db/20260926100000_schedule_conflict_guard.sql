-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.medikaone_schedule_windows_overlap(
    a_day INTEGER, a_date DATE, a_start TIME, a_end TIME, a_timezone TEXT,
    b_day INTEGER, b_date DATE, b_start TIME, b_end TIME, b_timezone TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
STABLE
SET search_path = pg_catalog, public
AS $$
DECLARE
    a_start_at TIMESTAMPTZ;
    a_end_at TIMESTAMPTZ;
    b_start_at TIMESTAMPTZ;
    b_end_at TIMESTAMPTZ;
    candidate DATE;
    reference_sunday CONSTANT DATE := DATE '2026-01-04';
    offset_count_a INTEGER;
    offset_count_b INTEGER;
    week_shift INTEGER;
BEGIN
    IF a_date IS NOT NULL AND b_date IS NOT NULL THEN
        a_start_at := (a_date + a_start) AT TIME ZONE a_timezone;
        a_end_at := (a_date + a_end) AT TIME ZONE a_timezone;
        b_start_at := (b_date + b_start) AT TIME ZONE b_timezone;
        b_end_at := (b_date + b_end) AT TIME ZONE b_timezone;
        RETURN a_start_at < b_end_at AND a_end_at > b_start_at;
    END IF;

    IF a_date IS NULL AND b_date IS NOT NULL THEN
        RETURN public.medikaone_schedule_windows_overlap(
            b_day, b_date, b_start, b_end, b_timezone,
            a_day, a_date, a_start, a_end, a_timezone
        );
    END IF;

    IF a_date IS NOT NULL THEN
        a_start_at := (a_date + a_start) AT TIME ZONE a_timezone;
        a_end_at := (a_date + a_end) AT TIME ZONE a_timezone;
        FOR candidate IN
            SELECT value::date
            FROM generate_series(
                ((a_start_at - INTERVAL '1 day') AT TIME ZONE b_timezone)::date,
                ((a_end_at + INTERVAL '1 day') AT TIME ZONE b_timezone)::date,
                INTERVAL '1 day'
            ) AS value
        LOOP
            IF EXTRACT(DOW FROM candidate)::integer = b_day THEN
                b_start_at := (candidate + b_start) AT TIME ZONE b_timezone;
                b_end_at := (candidate + b_end) AT TIME ZONE b_timezone;
                IF a_start_at < b_end_at AND a_end_at > b_start_at THEN
                    RETURN TRUE;
                END IF;
            END IF;
        END LOOP;
        RETURN FALSE;
    END IF;

    IF a_timezone = b_timezone THEN
        RETURN a_day = b_day AND a_start < b_end AND a_end > b_start;
    END IF;

    -- Recurring schedules in different zones are compared exactly when both
    -- zones keep a fixed UTC offset. If either zone changes offset (DST), fail
    -- closed because a currently safe wall-clock pair can collide later.
    IF a_timezone IN ('Asia/Jakarta', 'Asia/Makassar', 'Asia/Jayapura') THEN
        offset_count_a := 1;
    ELSE
        SELECT COUNT(DISTINCT EXTRACT(EPOCH FROM (
            (sample AT TIME ZONE a_timezone) - (sample AT TIME ZONE 'UTC')
        ))) INTO offset_count_a
        FROM generate_series(
            CURRENT_TIMESTAMP - INTERVAL '8 days',
            CURRENT_TIMESTAMP + INTERVAL '5 years 8 days',
            INTERVAL '1 day'
        ) AS sample;
    END IF;
    IF b_timezone IN ('Asia/Jakarta', 'Asia/Makassar', 'Asia/Jayapura') THEN
        offset_count_b := 1;
    ELSE
        SELECT COUNT(DISTINCT EXTRACT(EPOCH FROM (
            (sample AT TIME ZONE b_timezone) - (sample AT TIME ZONE 'UTC')
        ))) INTO offset_count_b
        FROM generate_series(
            CURRENT_TIMESTAMP - INTERVAL '8 days',
            CURRENT_TIMESTAMP + INTERVAL '5 years 8 days',
            INTERVAL '1 day'
        ) AS sample;
    END IF;
    IF offset_count_a <> 1 OR offset_count_b <> 1 THEN
        RETURN TRUE;
    END IF;

    a_start_at := (reference_sunday + a_day + a_start) AT TIME ZONE a_timezone;
    a_end_at := (reference_sunday + a_day + a_end) AT TIME ZONE a_timezone;
    FOREACH week_shift IN ARRAY ARRAY[-7, 0, 7]
    LOOP
        candidate := reference_sunday + b_day + week_shift;
        b_start_at := (candidate + b_start) AT TIME ZONE b_timezone;
        b_end_at := (candidate + b_end) AT TIME ZONE b_timezone;
        IF a_start_at < b_end_at AND a_end_at > b_start_at THEN
            RETURN TRUE;
        END IF;
    END LOOP;
    RETURN FALSE;
END;
$$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM public.doctor_hospital_schedules left_schedule
          JOIN public.doctor_hospital_affiliations left_affiliation
            ON left_affiliation.id = left_schedule.affiliation_id
          JOIN public.doctor_hospital_affiliations right_affiliation
            ON right_affiliation.doctor_id = left_affiliation.doctor_id
           AND right_affiliation.status = 'ACTIVE'
           AND right_affiliation.deleted_at IS NULL
          JOIN public.doctor_hospital_schedules right_schedule
            ON right_schedule.affiliation_id = right_affiliation.id
           AND right_schedule.id > left_schedule.id
           AND right_schedule.is_active = TRUE
         WHERE left_affiliation.status = 'ACTIVE'
           AND left_affiliation.deleted_at IS NULL
           AND left_schedule.is_active = TRUE
           AND (left_schedule.schedule_date IS NULL OR
                ((left_schedule.schedule_date + left_schedule.end_time) AT TIME ZONE left_schedule.timezone) > CURRENT_TIMESTAMP)
           AND (right_schedule.schedule_date IS NULL OR
                ((right_schedule.schedule_date + right_schedule.end_time) AT TIME ZONE right_schedule.timezone) > CURRENT_TIMESTAMP)
           AND public.medikaone_schedule_windows_overlap(
                left_schedule.day_of_week, left_schedule.schedule_date, left_schedule.start_time, left_schedule.end_time, left_schedule.timezone,
                right_schedule.day_of_week, right_schedule.schedule_date, right_schedule.start_time, right_schedule.end_time, right_schedule.timezone
           )
    ) THEN
        RAISE EXCEPTION 'existing active doctor schedules overlap; resolve conflicts before applying schedule guard migration';
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION public.medikaone_guard_doctor_schedule_overlap()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
DECLARE
    target_doctor UUID;
    target_affiliation_active BOOLEAN;
BEGIN
    IF NOT NEW.is_active THEN
        RETURN NEW;
    END IF;
    SELECT affiliation.doctor_id,
           affiliation.status = 'ACTIVE' AND affiliation.deleted_at IS NULL
      INTO target_doctor, target_affiliation_active
      FROM public.doctor_hospital_affiliations affiliation
     WHERE affiliation.id = NEW.affiliation_id;
    IF target_doctor IS NULL OR NOT target_affiliation_active THEN
        RETURN NEW;
    END IF;

    PERFORM pg_advisory_xact_lock(hashtextextended(target_doctor::text, 0));
    IF EXISTS (
        SELECT 1
          FROM public.doctor_hospital_schedules existing
          JOIN public.doctor_hospital_affiliations affiliation
            ON affiliation.id = existing.affiliation_id
         WHERE affiliation.doctor_id = target_doctor
           AND affiliation.status = 'ACTIVE'
           AND affiliation.deleted_at IS NULL
           AND existing.is_active = TRUE
           AND existing.id <> NEW.id
           AND (existing.schedule_date IS NULL OR
                ((existing.schedule_date + existing.end_time) AT TIME ZONE existing.timezone) > CURRENT_TIMESTAMP)
           AND public.medikaone_schedule_windows_overlap(
                NEW.day_of_week, NEW.schedule_date, NEW.start_time, NEW.end_time, NEW.timezone,
                existing.day_of_week, existing.schedule_date, existing.start_time, existing.end_time, existing.timezone
           )
    ) THEN
        RAISE EXCEPTION 'doctor schedule overlaps another active affiliation'
            USING ERRCODE = '23P01', CONSTRAINT = 'doctor_schedule_no_overlap';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_doctor_schedule_no_overlap
BEFORE INSERT OR UPDATE OF affiliation_id, day_of_week, schedule_date, start_time, end_time, timezone, is_active
ON public.doctor_hospital_schedules
FOR EACH ROW EXECUTE FUNCTION public.medikaone_guard_doctor_schedule_overlap();

CREATE OR REPLACE FUNCTION public.medikaone_guard_affiliation_reactivation_overlap()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF NEW.status <> 'ACTIVE' OR NEW.deleted_at IS NOT NULL
       OR (OLD.status = 'ACTIVE' AND OLD.deleted_at IS NULL) THEN
        RETURN NEW;
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.doctor_id::text, 0));
    IF EXISTS (
        SELECT 1
          FROM public.doctor_hospital_schedules proposed
          JOIN public.doctor_hospital_schedules existing ON existing.id <> proposed.id
          JOIN public.doctor_hospital_affiliations other_affiliation
            ON other_affiliation.id = existing.affiliation_id
         WHERE proposed.affiliation_id = NEW.id
           AND proposed.is_active = TRUE
           AND existing.is_active = TRUE
           AND (
                existing.affiliation_id = NEW.id
                OR (
                    other_affiliation.doctor_id = NEW.doctor_id
                    AND other_affiliation.id <> NEW.id
                    AND other_affiliation.status = 'ACTIVE'
                    AND other_affiliation.deleted_at IS NULL
                )
           )
           AND (proposed.schedule_date IS NULL OR
                ((proposed.schedule_date + proposed.end_time) AT TIME ZONE proposed.timezone) > CURRENT_TIMESTAMP)
           AND (existing.schedule_date IS NULL OR
                ((existing.schedule_date + existing.end_time) AT TIME ZONE existing.timezone) > CURRENT_TIMESTAMP)
           AND public.medikaone_schedule_windows_overlap(
                proposed.day_of_week, proposed.schedule_date, proposed.start_time, proposed.end_time, proposed.timezone,
                existing.day_of_week, existing.schedule_date, existing.start_time, existing.end_time, existing.timezone
           )
    ) THEN
        RAISE EXCEPTION 'reactivated doctor affiliation contains overlapping schedules'
            USING ERRCODE = '23P01', CONSTRAINT = 'doctor_schedule_no_overlap';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_affiliation_reactivation_no_schedule_overlap
BEFORE UPDATE OF status, deleted_at ON public.doctor_hospital_affiliations
FOR EACH ROW EXECUTE FUNCTION public.medikaone_guard_affiliation_reactivation_overlap();

REVOKE ALL ON FUNCTION public.medikaone_schedule_windows_overlap(INTEGER, DATE, TIME, TIME, TEXT, INTEGER, DATE, TIME, TIME, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.medikaone_guard_doctor_schedule_overlap() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.medikaone_guard_affiliation_reactivation_overlap() FROM PUBLIC;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_affiliation_reactivation_no_schedule_overlap ON public.doctor_hospital_affiliations;
DROP FUNCTION IF EXISTS public.medikaone_guard_affiliation_reactivation_overlap();
DROP TRIGGER IF EXISTS trg_doctor_schedule_no_overlap ON public.doctor_hospital_schedules;
DROP FUNCTION IF EXISTS public.medikaone_guard_doctor_schedule_overlap();
DROP FUNCTION IF EXISTS public.medikaone_schedule_windows_overlap(INTEGER, DATE, TIME, TIME, TEXT, INTEGER, DATE, TIME, TIME, TEXT);
-- +goose StatementEnd
