package main

import (
	"khel/internal/domain/bookings"
	"time"
)

type BookingResponse struct {
	ID            string    `json:"id"`
	VenueID       int64     `json:"venue_id"`
	FacilityID    int64     `json:"facility_id"`
	UserID        int64     `json:"user_id"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	TotalPrice    int       `json:"total_price"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	CustomerName  *string   `json:"customer_name,omitempty" swaggertype:"string"`
	CustomerPhone *string   `json:"customer_phone,omitempty" swaggertype:"string"`
	Note          *string   `json:"note,omitempty" swaggertype:"string"`
}

func (app *application) bookingToResponse(b *bookings.Booking) BookingResponse {
	return BookingResponse{
		ID:            app.EncodeBookingID(b.ID),
		VenueID:       b.VenueID,
		FacilityID:    b.FacilityID,
		UserID:        b.UserID,
		StartTime:     b.StartTime,
		EndTime:       b.EndTime,
		TotalPrice:    b.TotalPrice,
		Status:        b.Status,
		CreatedAt:     b.CreatedAt,
		UpdatedAt:     b.UpdatedAt,
		CustomerName:  b.CustomerName,
		CustomerPhone: b.CustomerPhone,
		Note:          b.Note,
	}
}

type PendingBookingResponse struct {
	BookingID    string    `json:"booking_id"`
	UserID       int64     `json:"user_id"`
	UserName     string    `json:"user_name"`
	UserImageURL *string   `json:"user_image"`
	UserPhone    string    `json:"user_number"`
	Price        int       `json:"price"`
	RequestedAt  time.Time `json:"requested_at"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
}

type PendingBookingsResponse struct {
	Bookings []PendingBookingResponse `json:"bookings"`
}

func (app *application) pendingBookingToResponse(b bookings.PendingBooking) PendingBookingResponse {
	return PendingBookingResponse{
		BookingID:    app.EncodeBookingID(b.BookingID),
		UserID:       b.UserID,
		UserName:     b.UserName,
		UserImageURL: b.UserImageURL,
		UserPhone:    b.UserPhone,
		Price:        b.Price,
		RequestedAt:  b.RequestedAt,
		StartTime:    b.StartTime,
		EndTime:      b.EndTime,
	}
}

func (app *application) pendingBookingsToResponse(items []bookings.PendingBooking) []PendingBookingResponse {
	out := make([]PendingBookingResponse, 0, len(items))

	for _, item := range items {
		out = append(out, app.pendingBookingToResponse(item))
	}

	return out
}

type CanceledBookingResponse struct {
	BookingID    string    `json:"booking_id"`
	UserID       int64     `json:"user_id"`
	UserName     string    `json:"user_name"`
	UserImageURL *string   `json:"user_image"`
	UserPhone    string    `json:"user_number"`
	Price        int       `json:"price"`
	RequestedAt  time.Time `json:"requested_at"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
}

type CanceledBookingsResponse struct {
	Bookings []CanceledBookingResponse `json:"bookings"`
}

func (app *application) canceledBookingToResponse(b bookings.CanceledBooking) CanceledBookingResponse {
	return CanceledBookingResponse{
		BookingID:    app.EncodeBookingID(b.BookingID),
		UserID:       b.UserID,
		UserName:     b.UserName,
		UserImageURL: b.UserImageURL,
		UserPhone:    b.UserPhone,
		Price:        b.Price,
		RequestedAt:  b.RequestedAt,
		StartTime:    b.StartTime,
		EndTime:      b.EndTime,
	}
}

func (app *application) canceledBookingsToResponse(items []bookings.CanceledBooking) []CanceledBookingResponse {
	out := make([]CanceledBookingResponse, 0, len(items))

	for _, item := range items {
		out = append(out, app.canceledBookingToResponse(item))
	}

	return out
}

type ScheduledBookingResponse struct {
	BookingID     string    `json:"booking_id"`
	UserID        int64     `json:"user_id"`
	UserName      string    `json:"user_name"`
	UserImageURL  *string   `json:"user_image"`
	UserPhone     string    `json:"user_number"`
	Price         int       `json:"price"`
	AcceptedAt    time.Time `json:"accepted_at"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	CustomerName  *string   `json:"customer_name,omitempty" swaggertype:"string"`
	CustomerPhone *string   `json:"customer_phone,omitempty" swaggertype:"string"`
	Note          *string   `json:"note,omitempty" swaggertype:"string"`
}

type ScheduledBookingsResponse struct {
	Bookings []ScheduledBookingResponse `json:"bookings"`
}

func (app *application) scheduledBookingToResponse(b bookings.ScheduledBooking) ScheduledBookingResponse {
	return ScheduledBookingResponse{
		BookingID:     app.EncodeBookingID(b.BookingID),
		UserID:        b.UserID,
		UserName:      b.UserName,
		UserImageURL:  b.UserImageURL,
		UserPhone:     b.UserPhone,
		Price:         b.Price,
		AcceptedAt:    b.AcceptedAt,
		StartTime:     b.StartTime,
		EndTime:       b.EndTime,
		CustomerName:  b.CustomerName,
		CustomerPhone: b.CustomerPhone,
		Note:          b.Note,
	}
}

func (app *application) scheduledBookingsToResponse(items []bookings.ScheduledBooking) []ScheduledBookingResponse {
	out := make([]ScheduledBookingResponse, 0, len(items))

	for _, item := range items {
		out = append(out, app.scheduledBookingToResponse(item))
	}

	return out
}
