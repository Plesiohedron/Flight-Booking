-- Booking service schema
-- Uses IF NOT EXISTS for idempotency

CREATE TABLE IF NOT EXISTS bookings (
    id            TEXT        PRIMARY KEY,
    user_id       TEXT        NOT NULL,
    flight_id     BIGINT      NOT NULL,
    passenger_name TEXT       NOT NULL,
    passenger_email TEXT      NOT NULL,
    seat_count    INTEGER     NOT NULL CHECK (seat_count > 0),
    total_cents   BIGINT      NOT NULL CHECK (total_cents > 0),
    status        TEXT        NOT NULL CHECK (status IN ('confirmed', 'cancelled')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS bookings_user_id_idx   ON bookings (user_id, created_at DESC);
