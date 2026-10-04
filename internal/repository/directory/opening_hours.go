package directory

// HospitalOpenExpression returns NULL for unknown hours/timezone. Normalized
// HH:mm values compare lexically; no unchecked JSON-to-time casts are needed.
// Overnight periods carry into the following day, including Saturday/Sunday.
const HospitalOpenExpression = `(SELECT CASE
	WHEN jsonb_typeof(directory.opening_hours) = 'array' AND directory.opening_hours <> '[]'::jsonb THEN EXISTS (
		SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(directory.opening_hours) = 'array' THEN directory.opening_hours ELSE '[]'::jsonb END) day
		WHERE COALESCE(day->>'is_closed', 'false') <> 'true' AND (
			(day->>'day_of_week' = EXTRACT(DOW FROM local_clock.at)::integer::text AND day->>'is_24_hours' = 'true')
			OR EXISTS (
				SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(day->'periods') = 'array' THEN day->'periods' ELSE '[]'::jsonb END) period
				WHERE period->>'open' ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
				  AND period->>'close' ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
				  AND (
					(period->>'close' > period->>'open' AND day->>'day_of_week' = EXTRACT(DOW FROM local_clock.at)::integer::text
					 AND TO_CHAR(local_clock.at, 'HH24:MI') >= period->>'open' AND TO_CHAR(local_clock.at, 'HH24:MI') < period->>'close')
					OR (period->>'close' < period->>'open' AND (
						(day->>'day_of_week' = EXTRACT(DOW FROM local_clock.at)::integer::text AND TO_CHAR(local_clock.at, 'HH24:MI') >= period->>'open')
						OR (day->>'day_of_week' = ((EXTRACT(DOW FROM local_clock.at)::integer + 6) % 7)::text AND TO_CHAR(local_clock.at, 'HH24:MI') < period->>'close')
					))
				  )
			)
		)
	) ELSE NULL END
	FROM pg_timezone_names zone
	CROSS JOIN LATERAL (SELECT CURRENT_TIMESTAMP AT TIME ZONE zone.name AS at) local_clock
	WHERE zone.name = directory.timezone LIMIT 1)`
