import os

import psycopg2


DATABASE_URL = os.getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5434/bookings?sslmode=disable")


def get_raw_connection():
    """Return a raw psycopg2 connection. Caller is responsible for closing it."""
    return psycopg2.connect(DATABASE_URL)

