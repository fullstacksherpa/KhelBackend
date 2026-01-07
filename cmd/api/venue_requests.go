package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"khel/internal/domain/venuerequest"
	"khel/internal/domain/venues"

	"github.com/go-chi/chi/v5"
)

// CreateVenueRequest godoc
//
//	@Summary		Request a new venue to be added
//	@Description	Public route. Creates a venue request (status=requested). Protected by Turnstile + strict rate limit + honeypot.
//	@Tags			Venue-Requests
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		createVenueRequestPayload	true	"Venue request payload"
//	@Success		201		{object}	venuerequest.VenueRequest
//	@Failure		400		{object}	error
//	@Failure		429		{object}	error
//	@Failure		500		{object}	error
//	@Router			/venue-requests [post]
func (app *application) createVenueRequestHandler(w http.ResponseWriter, r *http.Request) {
	var payload createVenueRequestPayload

	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Honeypot: bots often fill hidden fields
	// Your frontend should include this hidden input and keep it empty.
	if strings.TrimSpace(payload.Honey) != "" {
		// Don't reveal reason; act like generic invalid request.
		app.badRequestResponse(w, r, errInvalidRequest("invalid request"))
		return
	}

	ip := clientIP(r)
	app.logger.Infow("HTTP request",
		"ip", ip,
		"method", r.Method,
		"path", r.URL.Path,
	)

	// ✅ Turnstile verify (PROD only)
	// Allow Swagger/local testing without Turnstile when not in production.
	if app.config.env == "production" {
		_, err := app.verifyTurnstile(r.Context(), payload.TurnstileToken, ip)
		if err != nil {
			app.badRequestResponse(w, r, errInvalidRequest("invalid verification"))
			return
		}
	} else {
		// Optional: log if token missing in non-prod (helps you spot frontend issues)
		if strings.TrimSpace(payload.TurnstileToken) == "" {
			app.logger.Debugw("turnstile skipped (non-production) and token missing",
				"env", app.config.env,
				"ip", ip,
			)
		}
	}

	in := &venuerequest.CreateVenueRequestInput{
		Name:        strings.TrimSpace(payload.Name),
		Address:     strings.TrimSpace(payload.Address),
		Location:    payload.Location, // must be [lon, lat]
		Description: payload.Description,
		Amenities:   payload.Amenities,
		OpenTime:    payload.OpenTime,
		Sport:       strings.TrimSpace(payload.Sport),
		PhoneNumber: strings.TrimSpace(payload.PhoneNumber),
	}

	// Attach request metadata for abuse investigation
	ipStr := ip
	ua := r.UserAgent()
	in.RequesterIP = &ipStr
	if ua != "" {
		in.RequesterUserAgent = &ua
	}

	created, err := app.store.VenueRequests.CreateRequest(r.Context(), in)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	if err := app.jsonResponse(w, http.StatusCreated, created); err != nil {
		app.internalServerError(w, r, err)
	}
}

type createVenueRequestPayload struct {
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	Location    []float64 `json:"location"` // [lon, lat]
	Description *string   `json:"description,omitempty"`
	Amenities   []string  `json:"amenities,omitempty"`
	OpenTime    *string   `json:"open_time,omitempty"`
	Sport       string    `json:"sport"`
	PhoneNumber string    `json:"phone_number"`

	// Security fields:
	TurnstileToken string `json:"cf_turnstile_response"`
	Honey          string `json:"company"` // honeypot field name; hidden input in UI
}

// tiny helper to avoid leaking details
type errInvalidRequest string

func (e errInvalidRequest) Error() string { return string(e) }

// AdminListVenueRequests godoc
//
//	@Summary		List venue requests (admin)
//	@Description	Admin route. List venue requests by status with pagination.
//	@Tags			Admin Venue-Requests
//	@Produce		json
//	@Param			status	query		string	false	"requested|approved|rejected"
//	@Param			page	query		int		false	"page number (default 1)"
//	@Param			limit	query		int		false	"page size (default 20, max 60)"
//	@Success		200		{object}	[]venuerequest.VenueRequest
//	@Failure		401		{object}	error
//	@Failure		403		{object}	error
//	@Failure		500		{object}	error
//	@Security		ApiKeyAuth
//	@Router			/admin/venue-requests [get]
func (app *application) adminListVenueRequestsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page := readInt(q.Get("page"), 1)
	limit := readInt(q.Get("limit"), 20)

	var statusPtr *venuerequest.VenueRequestStatus
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		st := venuerequest.VenueRequestStatus(s)
		switch st {
		case venuerequest.VenueRequestRequested, venuerequest.VenueRequestApproved, venuerequest.VenueRequestRejected:
			statusPtr = &st
		default:
			app.badRequestResponse(w, r, errInvalidRequest("invalid status"))
			return
		}
	}

	out, err := app.store.VenueRequests.ListRequests(r.Context(), venuerequest.VenueRequestFilter{
		Status: statusPtr,
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	if err := app.jsonResponse(w, http.StatusOK, out); err != nil {
		app.internalServerError(w, r, err)
	}
}

func readInt(raw string, def int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// AdminApproveVenueRequest godoc
//
//	@Summary		Approve a venue request (admin)
//	@Description	Approves venue request and creates real venues row with owner_id.
//	@Tags			Admin Venue-Requests
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int64						true	"Venue request ID"
//	@Param			payload	body		adminApproveVenueReqPayload	true	"Owner assignment + optional admin note"
//	@Success		201		{object}	venues.Venue
//	@Failure		400		{object}	error
//	@Failure		401		{object}	error
//	@Failure		403		{object}	error
//	@Failure		404		{object}	error
//	@Failure		500		{object}	error
//	@Security		ApiKeyAuth
//	@Router			/admin/venue-requests/{id}/approve [post]
func (app *application) adminApproveVenueRequestHandler(w http.ResponseWriter, r *http.Request) {

	requestID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || requestID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venue-requests ID"))
		return
	}

	var payload adminApproveVenueReqPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if payload.OwnerID <= 0 {
		app.badRequestResponse(w, r, errInvalidRequest("owner_id is required"))
		return
	}

	// admin user from auth context
	admin := getUserFromContext(r)

	// 1) load request
	req, err := app.store.VenueRequests.GetRequestByID(r.Context(), requestID)
	if err != nil {
		if err == venuerequest.ErrVenueRequestNotFound {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}
	if req.Status != venuerequest.VenueRequestRequested {
		app.badRequestResponse(w, r, errInvalidRequest("request is not in requested state"))
		return
	}

	// 2) create real venue (no images at approval)
	v := &venues.Venue{
		OwnerID:     payload.OwnerID,
		Name:        req.Name,
		Address:     req.Address,
		Location:    req.Location, // [lon, lat]
		Description: req.Description,
		Amenities:   req.Amenities,
		OpenTime:    req.OpenTime,
		Sport:       req.Sport,
		PhoneNumber: req.PhoneNumber,
		ImageURLs:   []string{},
	}

	if err := app.store.Venues.Create(r.Context(), v); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 3) mark approved
	if err := app.store.VenueRequests.MarkRequestApproved(r.Context(), requestID, admin.ID, payload.AdminNote); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	if err := app.jsonResponse(w, http.StatusCreated, v); err != nil {
		app.internalServerError(w, r, err)
	}
}

type adminApproveVenueReqPayload struct {
	OwnerID   int64   `json:"owner_id"`
	AdminNote *string `json:"admin_note,omitempty"`
}

// AdminRejectVenueRequest godoc
//
//	@Summary		Reject a venue request (admin)
//	@Description	Rejects venue request with optional admin note.
//	@Tags			Admin Venue-Requests
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int64						true	"Venue request ID"
//	@Param			payload	body		adminRejectVenueReqPayload	false	"Optional admin note"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	error
//	@Failure		401		{object}	error
//	@Failure		403		{object}	error
//	@Failure		404		{object}	error
//	@Failure		500		{object}	error
//	@Security		ApiKeyAuth
//	@Router			/admin/venue-requests/{id}/reject [post]
func (app *application) adminRejectVenueRequestHandler(w http.ResponseWriter, r *http.Request) {
	requestID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || requestID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venue-requests ID"))
		return
	}

	var payload adminRejectVenueReqPayload
	_ = readJSON(w, r, &payload) // allow empty body

	admin := getUserFromContext(r)

	// ensure exists + requested state (optional but nicer)
	req, err := app.store.VenueRequests.GetRequestByID(r.Context(), requestID)
	if err != nil {
		if err == venuerequest.ErrVenueRequestNotFound {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}
	if req.Status != venuerequest.VenueRequestRequested {
		app.badRequestResponse(w, r, errInvalidRequest("request is not in requested state"))
		return
	}

	if err := app.store.VenueRequests.MarkRequestRejected(r.Context(), requestID, admin.ID, payload.AdminNote); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	if err := app.jsonResponse(w, http.StatusOK, map[string]string{"message": "request rejected"}); err != nil {
		app.internalServerError(w, r, err)
	}
}

type adminRejectVenueReqPayload struct {
	AdminNote *string `json:"admin_note,omitempty"`
}
