package main

import (
	"errors"
	"fmt"
	"khel/internal/store"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type CreateVenuePayload struct {
	Name        string    `json:"name" validate:"required,max=100"`
	Address     string    `json:"address" validate:"required,max=255"`
	Location    []float64 `json:"location" validate:"required,len=2"` // [longitude, latitude]
	Description *string   `json:"description,omitempty" validate:"max=500"`
	Amenities   []string  `json:"amenities,omitempty" validate:"max=100"` // Example validation for amenity count
	OpenTime    *string   `json:"open_time,omitempty" validate:"max=50"`  // Store operating hours (optional)
}

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

	// 1. Parse form  and get files without uploading
	files, err := app.parseVenueForm(w, r, &payload)
	if err != nil {
		app.badRequestResponse(w, r, err) // Unified error handling
		return
	}

	user := getUserFromContext(r)

	// 3. Check for existing venue before uploading images
	exists, err := app.store.Venues.CheckIfVenueExists(r.Context(), payload.Name, user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if exists {
		app.alreadyExistVenue(w, r, errors.New("a venue with this name already exists for this owner"))
		return
	}

	// 4. Upload images only if venue doesn't exist
	imageUrls, err := app.uploadImages(w, r, files)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Proceed with venue creation
	venue := &store.Venue{
		OwnerID:     user.ID,
		Name:        payload.Name,
		Address:     payload.Address,
		Location:    payload.Location,
		Description: payload.Description,
		Amenities:   payload.Amenities,
		OpenTime:    payload.OpenTime,
		ImageURLs:   imageUrls,
	}

	if err := app.store.Venues.Create(r.Context(), venue); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	if err := app.jsonResponse(w, http.StatusCreated, venue); err != nil {
		app.internalServerError(w, r, err)
		return
	}
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

	// Upload the new photo to Cloudinary
	newPhotoURL, err := app.uploadVenuesToCloudinary(file)
	if err != nil {
		app.internalServerError(w, r, err)
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
