from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
import uuid

from app.database import get_raw_connection


@dataclass(frozen=True)
class Booking:
    id: str
    user_id: str
    flight_id: int
    passenger_name: str
    passenger_email: str
    seat_count: int
    total_cents: int
    status: str
    created_at: str
    updated_at: str


def _row_to_booking(row) -> Booking:
    return Booking(
        id=row[0],
        user_id=row[1],
        flight_id=row[2],
        passenger_name=row[3],
        passenger_email=row[4],
        seat_count=row[5],
        total_cents=row[6],
        status=row[7],
        created_at=row[8].astimezone(timezone.utc).isoformat(),
        updated_at=row[9].astimezone(timezone.utc).isoformat(),
    )


def create_booking(
    *,
    booking_id: str | None,
    user_id: str,
    flight_id: int,
    passenger_name: str,
    passenger_email: str,
    seat_count: int,
    total_cents: int,
) -> Booking:
    booking_id = booking_id or str(uuid.uuid4())
    now = datetime.now(tz=timezone.utc)

    conn = get_raw_connection()
    try:
        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO bookings
                  (id, user_id, flight_id, passenger_name, passenger_email,
                   seat_count, total_cents, status, created_at, updated_at)
                VALUES
                  (%s, %s, %s, %s, %s,
                   %s, %s, 'confirmed', %s, %s)
                RETURNING
                  id, user_id, flight_id, passenger_name, passenger_email,
                  seat_count, total_cents, status, created_at, updated_at
                """,
                (
                    booking_id,
                    user_id,
                    flight_id,
                    passenger_name,
                    passenger_email,
                    seat_count,
                    total_cents,
                    now,
                    now,
                ),
            )
            row = cur.fetchone()
        conn.commit()
        return _row_to_booking(row)
    finally:
        conn.close()


def get_booking(booking_id: str) -> Booking | None:
    conn = get_raw_connection()
    try:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT
                  id, user_id, flight_id, passenger_name, passenger_email,
                  seat_count, total_cents, status, created_at, updated_at
                FROM bookings
                WHERE id = %s
                """,
                (booking_id,),
            )
            row = cur.fetchone()
        if row is None:
            return None
        return _row_to_booking(row)
    finally:
        conn.close()


def cancel_booking(booking_id: str) -> Booking | None:
    conn = get_raw_connection()
    try:
        with conn.cursor() as cur:
            cur.execute(
                """
                UPDATE bookings
                SET status = 'cancelled', updated_at = now()
                WHERE id = %s AND status = 'confirmed'
                RETURNING
                  id, user_id, flight_id, passenger_name, passenger_email,
                  seat_count, total_cents, status, created_at, updated_at
                """,
                (booking_id,),
            )
            row = cur.fetchone()
        conn.commit()
        if row is None:
            return None
        return _row_to_booking(row)
    finally:
        conn.close()


def list_bookings(user_id: str) -> list[Booking]:
    conn = get_raw_connection()
    try:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT
                  id, user_id, flight_id, passenger_name, passenger_email,
                  seat_count, total_cents, status, created_at, updated_at
                FROM bookings
                WHERE user_id = %s
                ORDER BY created_at DESC
                """,
                (user_id,),
            )
            rows = cur.fetchall()
        return [_row_to_booking(r) for r in rows]
    finally:
        conn.close()

