"""
REST API routes for the booking service.

Endpoints:
  GET  /flights          — search flights (origin, destination, date query params)
  GET  /flights/{id}     — get single flight
  POST /bookings         — create a booking (reserves seats + saves booking)
  GET  /bookings/{id}    — get booking by ID
  POST /bookings/{id}/cancel  — cancel a booking (releases reservation)
  GET  /bookings          — list bookings for a user (?user_id=X)
"""

from fastapi import APIRouter, HTTPException, Query

import grpc

from app.grpc_client import get_client
from app.models import CreateBookingRequest, BookingResponse
import app.repository as repo

router = APIRouter()


def _grpc_ts_to_iso(ts) -> str:
    # Timestamp -> RFC3339 string
    if ts is None:
        return ""
    from datetime import timezone

    dt = ts.ToDatetime()
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc).isoformat()


def _grpc_flight_to_dict(f) -> dict:
    return {
        "id": f.id,
        "airline": f.airline,
        "flight_number": f.flight_number,
        "origin": f.origin,
        "destination": f.destination,
        "departure_time": _grpc_ts_to_iso(f.departure_time),
        "arrival_time": _grpc_ts_to_iso(f.arrival_time),
        "total_seats": f.total_seats,
        "available_seats": f.available_seats,
        "price_cents": f.price_cents,
        "status": f.status,
    }

# ---------------------------------------------------------------------------
# Flights (read-only, proxied from flight-service)
# ---------------------------------------------------------------------------

@router.get("/flights")
def search_flights(
    origin: str = Query(..., min_length=3, max_length=3),
    destination: str = Query(..., min_length=3, max_length=3),
    date: str | None = Query(default=None, description="Optional YYYY-MM-DD"),
):
    """Search available flights."""
    client = get_client()
    try:
        response = client.search_flights(origin=origin.upper(), destination=destination.upper(), date=date)
        return {"flights": [_grpc_flight_to_dict(f) for f in response.flights]}
    except grpc.RpcError as exc:
        raise HTTPException(status_code=502, detail=f"Upstream error: {exc.details()}")


@router.get("/flights/{flight_id}")
def get_flight(flight_id: int):
    """Get a single flight by ID."""
    client = get_client()
    try:
        response = client.get_flight(flight_id=flight_id)
        return _grpc_flight_to_dict(response.flight)
    except grpc.RpcError as exc:
        if exc.code() == grpc.StatusCode.NOT_FOUND:
            raise HTTPException(status_code=404, detail=f"Flight {flight_id} not found")
        raise HTTPException(status_code=502, detail=f"Upstream error: {exc.details()}")


# ---------------------------------------------------------------------------
# Bookings (CRUD operations)
# ---------------------------------------------------------------------------

@router.post("/bookings", response_model=BookingResponse, status_code=201)
def create_booking(request: CreateBookingRequest):
    """Create a new booking."""
    client = get_client()

    import uuid
    booking_id = str(uuid.uuid4())

    try:
        f = client.get_flight(flight_id=request.flight_id).flight
    except grpc.RpcError as exc:
        if exc.code() == grpc.StatusCode.NOT_FOUND:
            raise HTTPException(status_code=404, detail=f"Flight {request.flight_id} not found")
        raise HTTPException(status_code=502, detail=f"Upstream error: {exc.details()}")

    try:
        client.reserve_seats(
            flight_id=request.flight_id,
            seat_count=request.seat_count,
            booking_id=booking_id,
        )
    except grpc.RpcError as exc:
        if exc.code() == grpc.StatusCode.RESOURCE_EXHAUSTED:
            raise HTTPException(status_code=409, detail=f"Not enough seats: {exc.details()}")
        if exc.code() == grpc.StatusCode.NOT_FOUND:
            raise HTTPException(status_code=404, detail=f"Flight not found: {exc.details()}")
        raise HTTPException(status_code=502, detail=f"Upstream error: {exc.details()}")

    total_cents = int(f.price_cents) * int(request.seat_count)
    booking = repo.create_booking(
        booking_id=booking_id,
        user_id=request.user_id,
        flight_id=request.flight_id,
        passenger_name=request.passenger_name,
        passenger_email=str(request.passenger_email),
        seat_count=request.seat_count,
        total_cents=total_cents,
    )

    return BookingResponse(**booking.__dict__)


@router.get("/bookings/{booking_id}", response_model=BookingResponse)
def get_booking(booking_id: str):
    """Get a booking by ID."""
    booking = repo.get_booking(booking_id)
    if booking is None:
        raise HTTPException(status_code=404, detail=f"Booking {booking_id} not found")
    return BookingResponse(**booking.__dict__)


@router.post("/bookings/{booking_id}/cancel", response_model=BookingResponse)
def cancel_booking(booking_id: str):
    """Cancel a booking."""
    booking = repo.get_booking(booking_id)
    if booking is None:
        raise HTTPException(status_code=404, detail=f"Booking {booking_id} not found")
    if booking.status == "cancelled":
        return BookingResponse(**booking.__dict__)

    # Best-effort: release reservation first
    client = get_client()
    try:
        client.release_reservation(booking_id=booking_id)
    except grpc.RpcError:
        pass

    cancelled = repo.cancel_booking(booking_id)
    if cancelled is None:
        raise HTTPException(status_code=409, detail="Booking could not be cancelled")
    return BookingResponse(**cancelled.__dict__)


@router.get("/bookings", response_model=list[BookingResponse])
def list_bookings(user_id: str = Query(..., min_length=1)):
    """List all bookings for a given user."""
    bookings = repo.list_bookings(user_id)
    return [BookingResponse(**b.__dict__) for b in bookings]

