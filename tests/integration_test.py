import os
import time
import uuid

import psycopg2
import requests


API_URL = os.getenv("BOOKING_API_URL", "http://localhost:8000")
BOOKING_DB_URL = os.getenv("BOOKING_DB_URL", "postgresql://postgres:postgres@localhost:5434/bookings")
FLIGHT_DB_URL = os.getenv("FLIGHT_DB_URL", "postgresql://postgres:postgres@localhost:5433/flights")


def wait_for_api() -> None:
    deadline = time.time() + 90
    while time.time() < deadline:
        try:
            response = requests.get(f"{API_URL}/health", timeout=2)
            if response.status_code == 200:
                return
        except requests.RequestException:
            pass
        time.sleep(2)
    raise RuntimeError("booking-service did not become healthy")


def fetch_available_seats(flight_id: int) -> int:
    with psycopg2.connect(FLIGHT_DB_URL) as conn:
        with conn.cursor() as cur:
            cur.execute("SELECT available_seats FROM flights WHERE id = %s", (flight_id,))
            row = cur.fetchone()
    assert row is not None
    return int(row[0])


def cleanup(booking_id: str | None) -> None:
    if not booking_id:
        return
    with psycopg2.connect(BOOKING_DB_URL) as conn:
        with conn.cursor() as cur:
            cur.execute("DELETE FROM bookings WHERE id = %s", (booking_id,))
    with psycopg2.connect(FLIGHT_DB_URL) as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                UPDATE flights
                SET available_seats = available_seats + sr.seat_count
                FROM seat_reservations sr
                WHERE sr.booking_id = %s
                  AND sr.flight_id = flights.id
                  AND sr.status = 1
                """,
                (booking_id,),
            )
            cur.execute("DELETE FROM seat_reservations WHERE booking_id = %s", (booking_id,))


def main() -> None:
    wait_for_api()
    flight_id = 1
    seats_before = fetch_available_seats(flight_id)
    user_id = f"integration-{uuid.uuid4()}"
    booking_id = None

    try:
        search = requests.get(
            f"{API_URL}/flights",
            params={"origin": "SVO", "destination": "LED", "date": "2026-04-01"},
            timeout=5,
        )
        assert search.status_code == 200, search.text
        flights = search.json()["flights"]
        assert any(f["id"] == flight_id for f in flights)

        create = requests.post(
            f"{API_URL}/bookings",
            json={
                "user_id": user_id,
                "flight_id": flight_id,
                "passenger_name": "Integration Test",
                "passenger_email": "integration@example.com",
                "seat_count": 1,
            },
            timeout=5,
        )
        assert create.status_code == 201, create.text
        payload = create.json()
        booking_id = payload["id"]
        assert payload["status"] == "confirmed"
        assert payload["total_cents"] == 15000

        with psycopg2.connect(BOOKING_DB_URL) as conn:
            with conn.cursor() as cur:
                cur.execute("SELECT status, seat_count FROM bookings WHERE id = %s", (booking_id,))
                booking_row = cur.fetchone()
        assert booking_row == ("confirmed", 1)

        with psycopg2.connect(FLIGHT_DB_URL) as conn:
            with conn.cursor() as cur:
                cur.execute(
                    "SELECT status, seat_count FROM seat_reservations WHERE booking_id = %s",
                    (booking_id,),
                )
                reservation_row = cur.fetchone()
        assert reservation_row == (1, 1)
        assert fetch_available_seats(flight_id) == seats_before - 1
    finally:
        cleanup(booking_id)


if __name__ == "__main__":
    main()
