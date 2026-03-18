package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Flight mirrors the proto Flight message using plain Go types.
type Flight struct {
	ID             int64
	Airline        string
	FlightNumber   string
	Origin         string
	Destination    string
	DepartureTime  time.Time
	ArrivalTime    time.Time
	TotalSeats     int32
	AvailableSeats int32
	PriceCents     int64
	Status         int32 // maps to FlightStatus enum values
}

// SeatReservation mirrors the proto SeatReservation message.
type SeatReservation struct {
	ID        int64
	FlightID  int64
	BookingID string
	SeatCount int32
	Status    int32 // maps to ReservationStatus enum values
	CreatedAt time.Time
}

// Repository handles all database operations for flights and reservations.
type Repository struct {
	db *sql.DB
}

// New creates a new Repository.
func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// SearchFlights returns flights matching origin, destination, and optional date.
func (r *Repository) SearchFlights(ctx context.Context, origin, destination string, date *time.Time) ([]*Flight, error) {
	query := `
		SELECT id, airline, flight_number, origin, destination,
		       departure_time, arrival_time,
		       total_seats, available_seats, price_cents, status
		FROM flights
		WHERE origin = $1
		  AND destination = $2
		  AND status = 1
		  AND ($3::date IS NULL OR departure_time::date = $3::date)
		ORDER BY departure_time`

	var dateVal any
	if date != nil {
		dateVal = *date
	} else {
		dateVal = nil
	}

	rows, err := r.db.QueryContext(ctx, query, origin, destination, dateVal)
	if err != nil {
		return nil, fmt.Errorf("query flights: %w", err)
	}
	defer rows.Close()

	var flights []*Flight
	for rows.Next() {
		f := &Flight{}
		if err := rows.Scan(
			&f.ID, &f.Airline, &f.FlightNumber, &f.Origin, &f.Destination,
			&f.DepartureTime, &f.ArrivalTime,
			&f.TotalSeats, &f.AvailableSeats, &f.PriceCents, &f.Status,
		); err != nil {
			return nil, fmt.Errorf("scan flight: %w", err)
		}
		flights = append(flights, f)
	}
	return flights, rows.Err()
}

// GetFlight returns a single flight by ID.
func (r *Repository) GetFlight(ctx context.Context, id int64) (*Flight, error) {
	query := `
		SELECT id, airline, flight_number, origin, destination,
		       departure_time, arrival_time,
		       total_seats, available_seats, price_cents, status
		FROM flights
		WHERE id = $1`

	f := &Flight{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&f.ID, &f.Airline, &f.FlightNumber, &f.Origin, &f.Destination,
		&f.DepartureTime, &f.ArrivalTime,
		&f.TotalSeats, &f.AvailableSeats, &f.PriceCents, &f.Status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get flight %d: %w", id, err)
	}
	return f, nil
}

// ReserveSeats atomically reserves seats and creates a reservation (one transaction).
// It is idempotent: if a reservation for the same bookingID already exists, it returns it.
func (r *Repository) ReserveSeats(ctx context.Context, flightID int64, seatCount int32, bookingID string) (*SeatReservation, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Idempotency: check if reservation already exists for this bookingID
	var existing SeatReservation
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT id, flight_id, booking_id, seat_count, status, created_at
		FROM seat_reservations
		WHERE booking_id = $1`, bookingID,
	).Scan(
		&existing.ID, &existing.FlightID, &existing.BookingID,
		&existing.SeatCount, &existing.Status, &createdAt,
	)
	if err == nil {
		// Reservation already exists — return it (idempotent)
		existing.CreatedAt = createdAt
		return &existing, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("check existing reservation: %w", err)
	}

	// Lock the flight row to prevent concurrent over-booking
	var f Flight
	err = tx.QueryRowContext(ctx, `
		SELECT id, available_seats, status
		FROM flights
		WHERE id = $1
		FOR UPDATE`, flightID,
	).Scan(&f.ID, &f.AvailableSeats, &f.Status)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("flight %d not found", flightID)
	}
	if err != nil {
		return nil, fmt.Errorf("lock flight: %w", err)
	}

	// Validate availability
	if f.Status != 1 { // FLIGHT_STATUS_SCHEDULED
		return nil, fmt.Errorf("flight is not available for booking")
	}
	if f.AvailableSeats < seatCount {
		return nil, fmt.Errorf("not enough seats: available=%d requested=%d", f.AvailableSeats, seatCount)
	}

	// Decrement available seats
	_, err = tx.ExecContext(ctx, `
		UPDATE flights
		SET available_seats = available_seats - $1
		WHERE id = $2`, seatCount, flightID,
	)
	if err != nil {
		return nil, fmt.Errorf("update available seats: %w", err)
	}

	// Insert reservation
	var res SeatReservation
	var resCreatedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO seat_reservations (flight_id, booking_id, seat_count, status)
		VALUES ($1, $2, $3, 1)
		RETURNING id, flight_id, booking_id, seat_count, status, created_at`,
		flightID, bookingID, seatCount,
	).Scan(
		&res.ID, &res.FlightID, &res.BookingID,
		&res.SeatCount, &res.Status, &resCreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert reservation: %w", err)
	}
	res.CreatedAt = resCreatedAt

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &res, nil
}

// ReleaseReservation returns seats and marks reservation as RELEASED (one transaction).
func (r *Repository) ReleaseReservation(ctx context.Context, bookingID string) (*SeatReservation, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Lock reservation row
	var res SeatReservation
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT id, flight_id, booking_id, seat_count, status, created_at
		FROM seat_reservations
		WHERE booking_id = $1
		FOR UPDATE`, bookingID,
	).Scan(
		&res.ID, &res.FlightID, &res.BookingID,
		&res.SeatCount, &res.Status, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("reservation for booking %s not found", bookingID)
	}
	if err != nil {
		return nil, fmt.Errorf("get reservation: %w", err)
	}
	res.CreatedAt = createdAt

	if res.Status != 1 {
		return &res, nil
	}

	// Mark as released
	_, err = tx.ExecContext(ctx, `
		UPDATE seat_reservations SET status = 2 WHERE booking_id = $1`, bookingID,
	)
	if err != nil {
		return nil, fmt.Errorf("update reservation status: %w", err)
	}

	// Return seats to the flight
	_, err = tx.ExecContext(ctx, `
		UPDATE flights
		SET available_seats = available_seats + $1
		WHERE id = $2`, res.SeatCount, res.FlightID,
	)
	if err != nil {
		return nil, fmt.Errorf("return seats: %w", err)
	}

	res.Status = 2 // RESERVATION_STATUS_RELEASED
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &res, nil
}

