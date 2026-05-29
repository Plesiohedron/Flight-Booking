"""
Booking Service — FastAPI application entry point.

Responsibilities:
  - Run database migrations on startup
  - Mount the REST router
"""

import glob
import os
import os.path
import time
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request, Response
from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest

from app.database import get_raw_connection
from app.routes import router


HTTP_REQUESTS_TOTAL = Counter(
    "http_requests_total",
    "Total HTTP requests handled by booking-service",
    ["method", "endpoint", "status"],
)
HTTP_REQUEST_ERRORS_TOTAL = Counter(
    "http_request_errors_total",
    "Total HTTP request errors handled by booking-service",
    ["method", "endpoint", "error_type"],
)
HTTP_REQUEST_DURATION_SECONDS = Histogram(
    "http_request_duration_seconds",
    "HTTP request duration in seconds for booking-service",
    ["method", "endpoint"],
)


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


@app.middleware("http")
async def prometheus_middleware(request: Request, call_next):
    if request.url.path == "/metrics":
        return await call_next(request)

    started = time.perf_counter()
    method = request.method
    status = "500"
    try:
        response = await call_next(request)
        status = str(response.status_code)
        if response.status_code >= 400:
            HTTP_REQUEST_ERRORS_TOTAL.labels(method, endpoint, f"http_{status}").inc()
        return response
    except Exception:
        HTTP_REQUEST_ERRORS_TOTAL.labels(method, endpoint, "unhandled_exception").inc()
        raise
    finally:
        endpoint = request.scope.get("route").path if request.scope.get("route") else request.url.path
        HTTP_REQUESTS_TOTAL.labels(method, endpoint, status).inc()
        HTTP_REQUEST_DURATION_SECONDS.labels(method, endpoint).observe(time.perf_counter() - started)


@app.get("/health")
def health():
    return {"status": "ok"}


@app.get("/metrics")
def metrics():
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)

