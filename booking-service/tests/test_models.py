import pytest
from pydantic import ValidationError

from app.models import CreateBookingRequest


def test_create_booking_request_accepts_valid_payload():
    request = CreateBookingRequest(
        user_id="unit-user",
        flight_id=1,
        passenger_name="Test Passenger",
        passenger_email="test@example.com",
        seat_count=2,
    )

    assert request.flight_id == 1
    assert request.seat_count == 2


def test_create_booking_request_rejects_negative_seat_count():
    with pytest.raises(ValidationError):
        CreateBookingRequest(
            user_id="unit-user",
            flight_id=1,
            passenger_name="Test Passenger",
            passenger_email="test@example.com",
            seat_count=-1,
        )
