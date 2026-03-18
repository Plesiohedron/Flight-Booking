-- Flight booking schema
-- Uses IF NOT EXISTS everywhere for idempotency

CREATE TABLE IF NOT EXISTS flights (
    id              BIGSERIAL PRIMARY KEY,
    airline         TEXT        NOT NULL,
    flight_number   TEXT        NOT NULL,
    origin          CHAR(3)     NOT NULL,
    destination     CHAR(3)     NOT NULL,
    departure_time  TIMESTAMPTZ NOT NULL,
    arrival_time    TIMESTAMPTZ NOT NULL,
    total_seats     INTEGER     NOT NULL CHECK (total_seats > 0),
    available_seats INTEGER     NOT NULL CHECK (available_seats >= 0),
    price_cents     BIGINT      NOT NULL CHECK (price_cents > 0),
    status          INTEGER     NOT NULL DEFAULT 1,  -- 1=SCHEDULED, 2=DEPARTED, 3=CANCELLED, 4=COMPLETED
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT available_le_total CHECK (available_seats <= total_seats)
);

CREATE UNIQUE INDEX IF NOT EXISTS flights_flight_number_departure_date_uq
  ON flights (flight_number, ((departure_time AT TIME ZONE 'UTC')::date));

CREATE INDEX IF NOT EXISTS flights_route_idx
  ON flights (origin, destination, departure_time);

CREATE TABLE IF NOT EXISTS seat_reservations (
    id            BIGSERIAL PRIMARY KEY,
    flight_id     BIGINT      NOT NULL REFERENCES flights(id) ON DELETE RESTRICT,
    booking_id    TEXT        NOT NULL UNIQUE,
    seat_count    INTEGER     NOT NULL CHECK (seat_count > 0),
    status        INTEGER     NOT NULL DEFAULT 1,  -- 1=ACTIVE, 2=RELEASED, 3=EXPIRED
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


INSERT INTO flights (airline, flight_number, origin, destination, departure_time, arrival_time, total_seats, available_seats, price_cents, status)
VALUES
  ('Aeroflot', 'SU1234', 'SVO', 'LED', '2026-04-01T07:30:00Z', '2026-04-01T08:55:00Z', 180, 180, 15000, 1),
  ('Aeroflot', 'SU1234', 'SVO', 'LED', '2026-04-02T07:30:00Z', '2026-04-02T08:55:00Z', 180, 180, 15000, 1),
  ('Rossiya',  'FV9876', 'VKO', 'AER', '2026-04-01T10:00:00Z', '2026-04-01T12:10:00Z', 160, 160, 12000, 1)
ON CONFLICT DO NOTHING;

