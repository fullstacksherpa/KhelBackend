package venuerequest

import (
	"context"
	"errors"
	"time"
)

var ErrVenueRequestNotFound = errors.New("venue request not found")

type VenueRequestStatus string

const (
	VenueRequestRequested VenueRequestStatus = "requested"
	VenueRequestApproved  VenueRequestStatus = "approved"
	VenueRequestRejected  VenueRequestStatus = "rejected"
)

// VenueRequest is the public-submitted request (NOT a real venue yet)
type VenueRequest struct {
	ID          int64              `json:"id"`
	Name        string             `json:"name"`
	Address     string             `json:"address"`
	Location    []float64          `json:"location"` // [lon, lat] for inserts in your Create() style
	Description *string            `json:"description,omitempty"`
	Amenities   []string           `json:"amenities,omitempty"`
	OpenTime    *string            `json:"open_time,omitempty"`
	Sport       string             `json:"sport"`
	PhoneNumber string             `json:"phone_number"`
	Status      VenueRequestStatus `json:"status"`

	AdminNote *string `json:"admin_note,omitempty"`

	RequesterIP        *string `json:"requester_ip,omitempty"`
	RequesterUserAgent *string `json:"requester_user_agent,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	ApprovedBy *int64     `json:"approved_by,omitempty"`
	RejectedAt *time.Time `json:"rejected_at,omitempty"`
	RejectedBy *int64     `json:"rejected_by,omitempty"`
}

// CreateVenueRequestInput is what your handler will pass in
type CreateVenueRequestInput struct {
	Name        string
	Address     string
	Location    []float64 // [lon, lat]
	Description *string
	Amenities   []string
	OpenTime    *string
	Sport       string
	PhoneNumber string

	RequesterIP        *string
	RequesterUserAgent *string
}

type VenueRequestFilter struct {
	Status *VenueRequestStatus
	Page   int
	Limit  int
}

type RequestStore interface {
	CreateRequest(ctx context.Context, in *CreateVenueRequestInput) (*VenueRequest, error)
	GetRequestByID(ctx context.Context, requestID int64) (*VenueRequest, error)
	ListRequests(ctx context.Context, filter VenueRequestFilter) ([]VenueRequest, error)

	MarkRequestApproved(ctx context.Context, requestID int64, approvedBy int64, adminNote *string) error
	MarkRequestRejected(ctx context.Context, requestID int64, rejectedBy int64, adminNote *string) error
}
