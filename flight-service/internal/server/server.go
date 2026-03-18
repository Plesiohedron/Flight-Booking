package server

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	flightv1 "github.com/soa/flight-service/gen/flight/v1"
	"github.com/soa/flight-service/internal/cache"
	"github.com/soa/flight-service/internal/repository"
)

import "strings"

// Server implements the FlightService gRPC server.
type Server struct {
	flightv1.UnimplementedFlightServiceServer
	repo  *repository.Repository
	cache *cache.Cache
}

// New creates a new Server.
func New(repo *repository.Repository, cache *cache.Cache) *Server {
	return &Server{repo: repo, cache: cache}
}

// SearchFlights handles the SearchFlights RPC with Cache-Aside.
func (s *Server) SearchFlights(ctx context.Context, req *flightv1.SearchFlightsRequest) (*flightv1.SearchFlightsResponse, error) {
	origin := req.GetOrigin()
	destination := req.GetDestination()
	if origin == "" || destination == "" {
		return nil, status.Error(codes.InvalidArgument, "origin and destination are required")
	}

	var dateStr string = ""
	var date *time.Time
	if req.GetHasDate() {
		ts := req.GetDate()
		if ts == nil {
			return nil, status.Error(codes.InvalidArgument, "date must be set when has_date=true")
		}
		t := ts.AsTime().UTC()
		onlyDate := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		date = &onlyDate
		dateStr = onlyDate.Format("2006-01-02")
	}

	// Cache-Aside: check cache first
	cached, err := s.cache.GetSearch(ctx, origin, destination, dateStr)
	if err != nil {
		log.Printf("cache.GetSearch error (ignored): %v", err)
	}
	if cached != nil {
		return &flightv1.SearchFlightsResponse{Flights: repoFlightsToProto(cached)}, nil
	}

	// Cache miss: query database
	flights, err := s.repo.SearchFlights(ctx, origin, destination, date)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search flights: %v", err)
	}

	// Populate cache
	if err := s.cache.SetSearch(ctx, origin, destination, dateStr, flights); err != nil {
		log.Printf("cache.SetSearch error (ignored): %v", err)
	}

	return &flightv1.SearchFlightsResponse{
		Flights: repoFlightsToProto(flights),
	}, nil
}

// GetFlight handles the GetFlight RPC with Cache-Aside.
func (s *Server) GetFlight(ctx context.Context, req *flightv1.GetFlightRequest) (*flightv1.GetFlightResponse, error) {
	id := req.GetFlightId()
	if id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "flight_id must be positive")
	}

	// Cache-Aside: check cache first
	cached, err := s.cache.GetFlight(ctx, id)
	if err != nil {
		log.Printf("cache.GetFlight error (ignored): %v", err)
	}
	if cached != nil {
		return &flightv1.GetFlightResponse{Flight: repoFlightToProto(cached)}, nil
	}

	// Cache miss: query database
	f, err := s.repo.GetFlight(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get flight: %v", err)
	}
	if f == nil {
		return nil, status.Errorf(codes.NotFound, "flight %d not found", id)
	}

	// Populate cache
	if err := s.cache.SetFlight(ctx, id, f); err != nil {
		log.Printf("cache.SetFlight error (ignored): %v", err)
	}

	return &flightv1.GetFlightResponse{
		Flight: repoFlightToProto(f),
	}, nil
}

// ReserveSeats handles the ReserveSeats RPC.
// After mutation, invalidates relevant cache entries.
func (s *Server) ReserveSeats(ctx context.Context, req *flightv1.ReserveSeatsRequest) (*flightv1.ReserveSeatsResponse, error) {
	flightID := req.GetFlightId()
	seatCount := req.GetSeatCount()
	bookingID := req.GetBookingId()

	if bookingID == "" {
		return nil, status.Error(codes.InvalidArgument, "booking_id is required")
	}
	if flightID <= 0 {
		return nil, status.Error(codes.InvalidArgument, "flight_id must be positive")
	}
	if seatCount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "seat_count must be positive")
	}

	res, err := s.repo.ReserveSeats(ctx, flightID, seatCount, bookingID)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not found"):
			return nil, status.Errorf(codes.NotFound, "reserve seats: %v", err)
		case strings.Contains(msg, "not enough seats"):
			return nil, status.Errorf(codes.ResourceExhausted, "reserve seats: %v", err)
		default:
			return nil, status.Errorf(codes.FailedPrecondition, "reserve seats: %v", err)
		}
	}

	// Invalidate cached flight data (available_seats changed)
	if err := s.cache.DeleteFlight(ctx, flightID); err != nil {
		log.Printf("cache.DeleteFlight error (ignored): %v", err)
	}
	// Invalidate all search results (they contain available_seats)
	if err := s.cache.DeleteSearchByPattern(ctx, "search:*"); err != nil {
		log.Printf("cache.DeleteSearchByPattern error (ignored): %v", err)
	}

	return &flightv1.ReserveSeatsResponse{
		Reservation: repoReservationToProto(res),
	}, nil
}

// ReleaseReservation handles the ReleaseReservation RPC.
// After mutation, invalidates relevant cache entries.
func (s *Server) ReleaseReservation(ctx context.Context, req *flightv1.ReleaseReservationRequest) (*flightv1.ReleaseReservationResponse, error) {
	bookingID := req.GetBookingId()
	if bookingID == "" {
		return nil, status.Error(codes.InvalidArgument, "booking_id is required")
	}

	res, err := s.repo.ReleaseReservation(ctx, bookingID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "release reservation: %v", err)
	}

	// Invalidate cached flight data (available_seats changed)
	if err := s.cache.DeleteFlight(ctx, res.FlightID); err != nil {
		log.Printf("cache.DeleteFlight error (ignored): %v", err)
	}
	if err := s.cache.DeleteSearchByPattern(ctx, "search:*"); err != nil {
		log.Printf("cache.DeleteSearchByPattern error (ignored): %v", err)
	}

	return &flightv1.ReleaseReservationResponse{
		Reservation: repoReservationToProto(res),
	}, nil
}

// --- helpers ---

func repoFlightToProto(f *repository.Flight) *flightv1.Flight {
	return &flightv1.Flight{
		Id:             f.ID,
		Airline:        f.Airline,
		FlightNumber:   f.FlightNumber,
		Origin:         f.Origin,
		Destination:    f.Destination,
		DepartureTime:  timestamppb.New(f.DepartureTime.UTC()),
		ArrivalTime:    timestamppb.New(f.ArrivalTime.UTC()),
		TotalSeats:     f.TotalSeats,
		AvailableSeats: f.AvailableSeats,
		PriceCents:     f.PriceCents,
		Status:         flightv1.FlightStatus(f.Status),
	}
}

func repoFlightsToProto(fs []*repository.Flight) []*flightv1.Flight {
	out := make([]*flightv1.Flight, 0, len(fs))
	for _, f := range fs {
		out = append(out, repoFlightToProto(f))
	}
	return out
}

func repoReservationToProto(res *repository.SeatReservation) *flightv1.SeatReservation {
	return &flightv1.SeatReservation{
		Id:         res.ID,
		FlightId:   res.FlightID,
		BookingId:  res.BookingID,
		SeatCount:  res.SeatCount,
		Status:     flightv1.ReservationStatus(res.Status),
		CreatedAt:  timestamppb.New(res.CreatedAt.UTC()),
	}
}
