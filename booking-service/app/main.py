"""
Booking Service — FastAPI application entry point.

Responsibilities:
  - Run database migrations on startup
  - Mount the REST router
"""

import glob
import os
import os.path
from contextlib import asynccontextmanager

from fastapi import FastAPI

from app.database import get_raw_connection
from app.routes import router


def run_migrations():
    """Apply SQL migration files in alphabetical order (idempotent)."""
    migrations_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "migrations"))

    sql_files = sorted(glob.glob(os.path.join(migrations_dir, "*.up.sql")))
    if not sql_files:
        print("No migration files found")
        return

    conn = get_raw_connection()
    try:
        with conn.cursor() as cur:
            for path in sql_files:
                print(f"Applying migration: {os.path.basename(path)}")
                with open(path, encoding="utf-8") as f:
                    cur.execute(f.read())
        conn.commit()
        print("Migrations applied successfully")
    except Exception as exc:
        conn.rollback()
        raise RuntimeError(f"Migration failed: {exc}") from exc
    finally:
        conn.close()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Run startup tasks before serving requests."""
    run_migrations()
    yield


app = FastAPI(
    title="Booking Service",
    description="REST API for searching flights and managing bookings",
    version="1.0.0",
    lifespan=lifespan,
)

app.include_router(router)


@app.get("/health")
def health():
    return {"status": "ok"}

