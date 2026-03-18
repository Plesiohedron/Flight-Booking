from pydantic import BaseModel, Field, EmailStr


class CreateBookingRequest(BaseModel):
    user_id: str = Field(..., min_length=1)
    flight_id: int = Field(..., gt=0)
    passenger_name: str = Field(..., min_length=1)
    passenger_email: EmailStr
    seat_count: int = Field(..., gt=0)


class BookingResponse(BaseModel):
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

