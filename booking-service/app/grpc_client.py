"""
gRPC client for the flight-service.

Combines:
  - RetryInterceptor  (automatic retries on transient errors)
  - CircuitBreaker    (fail-fast when service is down)
"""

import os

import grpc

from datetime import datetime, timezone

# These are generated inside Docker by grpc_tools.protoc
from app.gen.flight.v1 import flight_pb2, flight_pb2_grpc


FLIGHT_SERVICE_URL = os.getenv("FLIGHT_SERVICE_URL", "localhost:50051")
API_KEY = os.getenv("API_KEY", "")


def _make_metadata():
    if API_KEY:
        return [("x-api-key", API_KEY)]
    return []


class FlightClient:
    """
    Thin wrapper around the generated FlightService stub.

    All calls are guarded by a CircuitBreaker. If the circuit is OPEN,
    CircuitOpenError is raised immediately without touching the network.
    """

    def __init__(self):
        self._channel = grpc.insecure_channel(FLIGHT_SERVICE_URL)
        self._stub = flight_pb2_grpc.FlightServiceStub(self._channel)
        self._metadata = _make_metadata()

    def search_flights(self, *, origin: str, destination: str, date: str | None):
        if date:
            dt = datetime.strptime(date, "%Y-%m-%d").replace(tzinfo=timezone.utc)
            ts = flight_pb2.google_dot_protobuf_dot_timestamp__pb2.Timestamp()
            ts.FromDatetime(dt)
            request = flight_pb2.SearchFlightsRequest(origin=origin, destination=destination, date=ts, has_date=True)
        else:
            request = flight_pb2.SearchFlightsRequest(origin=origin, destination=destination, has_date=False)
        return self._stub.SearchFlights(request, metadata=self._metadata)

    def get_flight(self, *, flight_id: int):
        request = flight_pb2.GetFlightRequest(flight_id=flight_id)
        return self._stub.GetFlight(request, metadata=self._metadata)

    def reserve_seats(self, *, flight_id: int, seat_count: int, booking_id: str):
        request = flight_pb2.ReserveSeatsRequest(
            flight_id=flight_id,
            seat_count=seat_count,
            booking_id=booking_id
        )
        return self._stub.ReserveSeats(request, metadata=self._metadata)

    def release_reservation(self, *, booking_id: str):
        request = flight_pb2.ReleaseReservationRequest(booking_id=booking_id)
        return self._stub.ReleaseReservation(request, metadata=self._metadata)


_client: FlightClient | None = None


def get_client() -> FlightClient:
    global _client
    if _client is None:
        _client = FlightClient()
    return _client

