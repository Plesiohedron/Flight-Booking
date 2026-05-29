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


def db_scalar(dsn: str, query: str, params: tuple):
    with psycopg2.connect(dsn) as conn:
        with conn.cursor() as cur:
            cur.execute(query, params)
            return cur.fetchone()[0]


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
    seats_before = db_scalar(FLIGHT_DB_URL, "SELECT available_seats FROM flights WHERE id = %s", (flight_id,))
    user_id = f"e2e-{uuid.uuid4()}"
    booking_id = None

    try:
        create = requests.post(
            f"{API_URL}/bookings",
            json={
                "user_id": user_id,
                "flight_id": flight_id,
                "passenger_name": "E2E Passenger",
                "passenger_email": "e2e@example.com",
                "seat_count": 2,
            },
            timeout=5,
        )
        assert create.status_code == 201, create.text
        booking = create.json()
        booking_id = booking["id"]
        assert booking["user_id"] == user_id
        assert booking["status"] == "confirmed"
        assert booking["seat_count"] == 2

        read = requests.get(f"{API_URL}/bookings/{booking_id}", timeout=5)
        assert read.status_code == 200, read.text
        assert read.json()["id"] == booking_id

        booking_status = db_scalar(BOOKING_DB_URL, "SELECT status FROM bookings WHERE id = %s", (booking_id,))
        reservation_status = db_scalar(
            FLIGHT_DB_URL,
            "SELECT status FROM seat_reservations WHERE booking_id = %s",
            (booking_id,),
        )
        seats_after_create = db_scalar(FLIGHT_DB_URL, "SELECT available_seats FROM flights WHERE id = %s", (flight_id,))
        assert booking_status == "confirmed"
        assert reservation_status == 1
        assert seats_after_create == seats_before - 2

        cancel = requests.post(f"{API_URL}/bookings/{booking_id}/cancel", timeout=5)
        assert cancel.status_code == 200, cancel.text
        assert cancel.json()["status"] == "cancelled"

        booking_status = db_scalar(BOOKING_DB_URL, "SELECT status FROM bookings WHERE id = %s", (booking_id,))
        reservation_status = db_scalar(
            FLIGHT_DB_URL,
            "SELECT status FROM seat_reservations WHERE booking_id = %s",
            (booking_id,),
        )
        seats_after_cancel = db_scalar(FLIGHT_DB_URL, "SELECT available_seats FROM flights WHERE id = %s", (flight_id,))
        assert booking_status == "cancelled"
        assert reservation_status == 2
        assert seats_after_cancel == seats_before
    finally:
        cleanup(booking_id)


if __name__ == "__main__":
    main()
