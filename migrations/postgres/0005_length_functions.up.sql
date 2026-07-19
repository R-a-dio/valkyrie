-- Convert between Go time.Duration (nanoseconds, stored as BIGINT) and seconds (INT).
-- MySQL DETERMINISTIC maps to PostgreSQL IMMUTABLE.
CREATE OR REPLACE FUNCTION from_go_duration(d BIGINT) RETURNS INT
LANGUAGE sql IMMUTABLE AS $$
  SELECT (d / 1000000000)::INT
$$;

CREATE OR REPLACE FUNCTION to_go_duration(s INT) RETURNS BIGINT
LANGUAGE sql IMMUTABLE AS $$
  SELECT s::BIGINT * 1000000000
$$;
