package main

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/store"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type isOwnerResponse struct {
	IsOwner bool `json:"is_owner"`
}

// IsVenueOwner godoc
//
//	@Summary		Check if user is a venue owner
//	@Description	Determines whether the authenticated user owns at least one venue
//	@Tags			Venue
//	@Produce		json
//	@Success		200	{object}	isOwnerResponse	"Ownership check result"
//	@Failure		401	{object}	error			"Unauthorized"
//	@Failure		500	{object}	error			"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/venues/is-venue-owner [get]
func (app *application) isVenueOwnerHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	isOwner, err := app.store.Venues.IsOwnerOfAnyVenue(r.Context(), user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	resp := isOwnerResponse{
		IsOwner: isOwner,
	}

	if err := app.jsonResponse(w, http.StatusOK, resp); err != nil {
		app.internalServerError(w, r, err)
		return
	}
}

type CreateVenuePayload struct {
	Name        string    `json:"name" validate:"required,max=100"`
	Address     string    `json:"address" validate:"required,max=255"`
	Location    []float64 `json:"location" validate:"required,len=2"` // [longitude, latitude]
	Description *string   `json:"description,omitempty" validate:"max=500"`
	Amenities   []string  `json:"amenities,omitempty" validate:"max=100"` // Example validation for amenity count
	PhoneNumber string    `json:"phone_number" validate:"required,max=13,min=10"`
	OpenTime    *string   `json:"open_time,omitempty" validate:"max=50"` // Store operating hours (optional)
	Sport       string    `json:"sport" validate:"required,max=50"`
}

// DeleteVenuePhoto godoc
//
//	@Summary		Delete a venue photo
//	@Description	Deletes a specific venue photo from Cloudinary and removes it from the database.
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int64				true	"Venue ID"
//	@Param			photo_url	query		string				true	"Photo URL to delete"
//	@Success		200			{object}	map[string]string	"Photo deleted successfully"
//	@Failure		400			{object}	error				"Bad Request: Missing venue ID or photo URL"
//	@Failure		500			{object}	error				"Internal Server Error: Could not delete photo"
//	@Router			/venues/{venueID}/photos [delete]
//	@Security		ApiKeyAuth
func (app *application) deleteVenuePhotoHandler(w http.ResponseWriter, r *http.Request) {
	// Extract venue ID and photo URL from the request
	venueIDStr := chi.URLParam(r, "venueID")
	photoURL := r.URL.Query().Get("photo_url")

	// Convert venueID to int64
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %v", err))
		return
	}

	if venueID == 0 || photoURL == "" {
		app.badRequestResponse(w, r, errors.New("venue ID and photo URL are required"))
		return
	}

	// Delete the photo from Cloudinary
	if err := app.deletePhotoFromCloudinary(photoURL); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Remove the photo URL from the database
	ctx := r.Context()
	if err := app.store.Venues.RemovePhotoURL(ctx, venueID, photoURL); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Respond with success
	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "photo deleted successfully"})
}

// UploadVenuePhoto godoc
//
//	@Summary		Upload a new photo for a venue
//	@Description	Uploads a new venue photo to Cloudinary and adds the new photo URL to the venue record.
//	@Tags			Venue
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Param			photo	formData	file				true	"Photo file to upload"
//	@Success		200		{object}	map[string]string	"Photo uploaded successfully, returns {\"photo_url\": \"<newPhotoURL>\"}"
//	@Failure		400		{object}	error				"Bad Request: Invalid input or missing file"
//	@Failure		500		{object}	error				"Internal Server Error: Could not process the upload"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/photos [post]
func (app *application) uploadVenuePhotoHandler(w http.ResponseWriter, r *http.Request) {

	// Extract venue ID and photo URL from the request
	venueIDStr := chi.URLParam(r, "venueID")

	// Convert venueID to int64
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %v", err))
		return
	}
	// Parse the multipart form to get the new photo
	const maxBytes = 15 * 1024 * 1024 // 15MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}

	// Get the file from the form
	file, _, err := r.FormFile("photo")
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("failed to get photo from form: %w", err))
		return
	}
	defer file.Close()

	// Generate a custom Cloudinary public ID using the venue ID and image number.
	publicID := fmt.Sprintf("venue_%d_image_%d", venueID, time.Now().UnixNano())
	newPhotoURL, err := app.uploadToCloudinaryWithID(file, publicID)
	if err != nil {
		return
	}

	// Add the new photo URL to the venue's image_urls in the database
	ctx := r.Context()
	if err := app.store.Venues.AddPhotoURL(ctx, venueID, newPhotoURL); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Respond with the new photo URL
	app.jsonResponse(w, http.StatusOK, map[string]string{"photo_url": newPhotoURL})
}

// UpdateVenueInfo godoc
//
//	@Summary		Update venue information
//	@Description	Allows venue owners to update partial information about their venue.
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int						true	"Venue ID"
//	@Param			updateData	body		map[string]interface{}	true	"Venue update payload"
//	@Success		200			{object}	map[string]string		"Venue updated successfully"
//	@Failure		400			{object}	error					"Bad Request: Invalid input"
//	@Failure		500			{object}	error					"Internal Server Error: Could not update venue"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID} [patch]
func (app *application) updateVenueInfo(w http.ResponseWriter, r *http.Request) {
	venueIDStr := chi.URLParam(r, "venueID")
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %v", err))
		return
	}
	if venueID == 0 {
		app.badRequestResponse(w, r, errors.New("venue ID is required"))
		return
	}

	// Parse the request body into a map for partial updates
	var updateData map[string]interface{}
	if err := readJSON(w, r, &updateData); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Validate the update data (optional, depending on your requirements)
	// You can add validation logic here if needed

	// Update the venue in the database
	if err := app.store.Venues.Update(r.Context(), venueID, updateData); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Respond with success
	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "venue updated successfully"})
}

// VenueListResponse represents the trimmed venue response
type VenueListResponse struct {
	Address       string    `json:"address"`
	ID            int64     `json:"id"`
	ImageURLs     []string  `json:"image_urls"`
	Location      []float64 `json:"location"` // [longitude, latitude]
	Name          string    `json:"name"`
	OpenTime      *string   `json:"open_time,omitempty"`
	PhoneNumber   string    `json:"phone_number"`
	Sport         string    `json:"sport"`
	TotalReviews  int       `json:"total_reviews"`
	AverageRating float64   `json:"average_rating"`
}

// @Summary		List venues
// @Description	Get paginated list of venues with filters
// @Tags			Venue
// @Accept			json
// @Produce		json
// @Param			sport		query	string	false	"Filter by sport type"
// @Param			lat			query	number	false	"Latitude for location filter"
// @Param			lng			query	number	false	"Longitude for location filter"
// @Param			distance	query	number	false	"Distance in meters from location"
// @Param			page		query	int		false	"Page number"		default(1)
// @Param			limit		query	int		false	"Items per page"	default(20)
// @Success		200			{array}	VenueListResponse
// @Router			/list-venues [get]
func (app *application) listVenuesHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	q := r.URL.Query()
	// Parse pagination parameters
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 25 {
		limit = 12
	}

	fmt.Printf("the query object of listVenuesHandler is %v", q)

	filter := store.VenueFilter{
		Sport: nullString(q.Get("sport")),
		Page:  page,
		Limit: limit,
	}

	// Parse location filter
	if lat := q.Get("lat"); lat != "" {
		if lng := q.Get("lng"); lng != "" {
			if distance := q.Get("distance"); distance != "" {
				parsedLat, _ := strconv.ParseFloat(lat, 64)
				parsedLng, _ := strconv.ParseFloat(lng, 64)
				parsedDistance, _ := strconv.ParseFloat(distance, 64)

				filter.Latitude = &parsedLat
				filter.Longitude = &parsedLng
				filter.Distance = &parsedDistance
			}
		}
	}

	// Get venues from store
	venues, err := app.store.Venues.List(r.Context(), filter)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Convert to response format
	response := make([]VenueListResponse, len(venues))
	for i, v := range venues {
		response[i] = VenueListResponse{
			ID:            v.ID,
			Name:          v.Name,
			Address:       v.Address,
			Location:      []float64{v.Longitude, v.Latitude},
			ImageURLs:     v.ImageURLs,
			OpenTime:      v.OpenTime,
			PhoneNumber:   v.PhoneNumber,
			Sport:         v.Sport,
			TotalReviews:  v.TotalReviews,
			AverageRating: v.AverageRating,
		}
	}

	app.jsonResponse(w, http.StatusOK, response)
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// CreateAndUploadVenue implements the SAGA pattern.
// 1. Create a venue record in the database (without images).
// 2. Upload images externally to Cloudinary using custom public IDs.
// 3. Update the venue record with the returned image URLs.
// If an error occurs at any external step, the venue is deleted to ensure consistency.
func (app *application) CreateAndUploadVenue(ctx context.Context, venue *store.Venue, files []*multipart.FileHeader, w http.ResponseWriter, r *http.Request) error {
	// Step 1: Create the venue in the database (without images).
	fmt.Print("calling database for Create")
	if err := app.store.Venues.Create(ctx, venue); err != nil {
		return fmt.Errorf("error creating venue: %w", err)
	}
	fmt.Printf("venue created, id is %v", venue.ID)
	// Step 2: Upload images using the venue ID.
	imageUrls, err := app.uploadImagesWithVenueID(files, venue.ID)
	if err != nil {
		// Compensate: Delete the venue if image upload fails.
		if delErr := app.store.Venues.Delete(ctx, venue.ID); delErr != nil {
			app.logger.Errorw("failed to delete venue after image upload failure", "venueID", venue.ID, "delete_error", delErr)
		}
		return fmt.Errorf("error uploading images: %w", err)
	}

	fmt.Print("calling database to updateImageURLs")
	// Step 3: Update the venue with the uploaded image URLs.
	if err := app.store.Venues.UpdateImageURLs(ctx, venue.ID, imageUrls); err != nil {
		// Compensate: Delete the venue if updating image URLs fails.
		if delErr := app.store.Venues.Delete(ctx, venue.ID); delErr != nil {
			app.logger.Errorw("failed to delete venue after update image URLs failure", "venueID", venue.ID, "delete_error", delErr)
		}
		return fmt.Errorf("error updating venue images: %w", err)
	}

	// Optionally update the venue struct with URLs.
	venue.ImageURLs = imageUrls
	return nil
}

// -----------------------------------------------
// HTTP Handler for Venue Creation
// -----------------------------------------------

// createVenueHandler processes the HTTP request to create a venue.
// It parses the form data, checks for duplicate venues, and then uses the SAGA pattern.

// CreateVenue godoc
//
//	@Summary		Register a venue in our system
//	@Description	Register a new venue with details such as name, address, location, and amenities.
//	@Tags			Venue
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			venue	formData	string		true	"Venue details (JSON string)"
//	@Param			images	formData	[]file		false	"Venue images (up to 7 files)"
//	@Success		201		{object}	store.Venue	"Venue created successfully"
//	@Failure		400		{object}	error		"Invalid request payload"
//	@Failure		401		{object}	error		"Unauthorized"
//	@Failure		500		{object}	error		"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/venues [post]
func (app *application) createVenueHandler(w http.ResponseWriter, r *http.Request) {
	var payload CreateVenuePayload

	// Parse form and JSON payload from multipart form.
	files, err := app.parseVenueForm(w, r, &payload)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Retrieve the current user (owner) from the request context.
	user := getUserFromContext(r)

	// Check for an existing venue with the same name for this owner.
	exists, err := app.store.Venues.CheckIfVenueExists(r.Context(), payload.Name, user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if exists {
		app.alreadyExistVenue(w, r, fmt.Errorf("a venue with this name already exists for this owner"))
		return
	}

	// Prepare the venue model without image URLs initially.
	venue := &store.Venue{
		OwnerID:     user.ID,
		Name:        payload.Name,
		Address:     payload.Address,
		Location:    payload.Location,
		Description: payload.Description,
		Amenities:   payload.Amenities,
		OpenTime:    payload.OpenTime,
		Sport:       payload.Sport,
		PhoneNumber: payload.PhoneNumber,
		// ImageURLs field remains empty until updated.
	}

	// Use the SAGA pattern to create the venue record and upload images externally.
	if err := app.CreateAndUploadVenue(r.Context(), venue, files, w, r); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Return a JSON response with the created venue details.
	if err := app.jsonResponse(w, http.StatusCreated, venue); err != nil {
		app.internalServerError(w, r, err)
	}
}
