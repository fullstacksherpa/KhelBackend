package main

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/domain/venues"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type isOwnerResponse struct {
	IsOwner  bool    `json:"is_owner"`
	VenueIDs []int64 `json:"venue_ids,omitempty"`
}

// IsVenueOwner godoc
//
//	@Summary		Check if user is a venue owner
//	@Description	Determines whether the authenticated user owns at least one venue
//	@Tags			Venue-Owner
//	@Produce		json
//	@Success		200	{object}	isOwnerResponse	"Ownership check result"
//	@Failure		401	{object}	error			"Unauthorized"
//	@Failure		500	{object}	error			"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/venues/is-venue-owner [get]
func (app *application) isVenueOwnerHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	venueIDs, err := app.store.Venues.GetOwnedVenueIDs(r.Context(), user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	resp := isOwnerResponse{
		IsOwner:  len(venueIDs) > 0,
		VenueIDs: venueIDs,
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
//	@Tags			Venue-Owner
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

	fmt.Println("Deleted photo from the cloudinary")
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
//	@Tags			Venue-Owner
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

	// 🔍 Check how many images already exist
	ctx := r.Context()
	urls, err := app.store.Venues.GetImageURLs(ctx, venueID)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("could not get image urls: %w", err))
		return
	}

	if len(urls) >= 7 {
		app.badRequestResponse(w, r, fmt.Errorf("venue %d already has the max of 7 photos", venueID))
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
	newPhotoURL, err := app.uploadToCloudinaryWithID(file, publicID, "")
	if err != nil {
		return
	}

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
//	@Tags			Venue-Owner
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
	IsFavorite    bool      `json:"is_favorite"`
}

//	@Summary		List venues
//	@Description	Get paginated list of venues with filters
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			sport		query	string	false	"Filter by sport type"
//	@Param			lat			query	number	false	"Latitude for location filter"
//	@Param			lng			query	number	false	"Longitude for location filter"
//	@Param			distance	query	number	false	"Distance in meters from location"
//	@Param			page		query	int		false	"Page number"		default(1)
//	@Param			limit		query	int		false	"Items per page"	default(7)
//	@Success		200			{array}	VenueListResponse
//
//	@Security		ApiKeyAuth
//
//	@Router			/venues/list-venues [get]
func (app *application) listVenuesHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	q := r.URL.Query()
	// Parse pagination parameters
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 15 {
		limit = 13
	}

	// Check for a location filter and adjust pagination accordingly.
	if q.Get("lat") != "" && q.Get("lng") != "" && q.Get("distance") != "" {
		// Override pagination for map queries.
		limit = 1000 // Or another value that makes sense for your data size.
	}

	filter := venues.VenueFilter{
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

	var favMap map[int64]struct{}
	user := getUserFromContext(r)

	if user != nil {
		favMap, err = app.store.Venues.GetFavoriteVenueIDsByUser(r.Context(), user.ID)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}
	} else {
		favMap = make(map[int64]struct{}) // empty map for non-auth users
	}

	// Convert to response format
	response := make([]VenueListResponse, len(venues))
	for i, v := range venues {
		_, isFav := favMap[v.ID]
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
			IsFavorite:    isFav,
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
func (app *application) CreateAndUploadVenue(ctx context.Context, venue *venues.Venue, files []*multipart.FileHeader, w http.ResponseWriter, r *http.Request) error {
	if err := app.store.Venues.Create(ctx, venue); err != nil {
		return fmt.Errorf("error creating venue: %w", err)
	}

	imageUrls, err := app.uploadImagesWithVenueID(files, venue.ID)
	if err != nil {
		// Compensate: Delete the venue if image upload fails.
		if delErr := app.store.Venues.Delete(ctx, venue.ID); delErr != nil {
			app.logger.Errorw("failed to delete venue after image upload failure", "venueID", venue.ID, "delete_error", delErr)
		}
		return fmt.Errorf("error uploading images: %w", err)
	}

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
//	@Tags			Venue-Owner
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			venue	formData	string			true	"Venue details (JSON string)"
//	@Param			images	formData	[]file			false	"Venue images (up to 7 files)"
//	@Success		201		{object}	venues.Venue	"Venue created successfully"
//	@Failure		400		{object}	error			"Invalid request payload"
//	@Failure		401		{object}	error			"Unauthorized"
//	@Failure		500		{object}	error			"Internal server error"
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

	//for debug only
	for _, fh := range files {
		fmt.Printf("📷 got file: %q (%d bytes)\n", fh.Filename, fh.Size)
	}

	user := getUserFromContext(r)

	// Prepare the venue model without image URLs initially.
	venue := &venues.Venue{
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

type VenueDetailResponse struct {
	ID             int64     `json:"id"`
	OwnerID        int64     `json:"owner_id"`
	Name           string    `json:"name"`
	Address        string    `json:"address"`
	Location       []float64 `json:"location"` // [latitude, longitude]
	Description    *string   `json:"description,omitempty"`
	PhoneNumber    string    `json:"phone_number"`
	Amenities      []string  `json:"amenities,omitempty"`
	OpenTime       *string   `json:"open_time,omitempty"`
	Sport          string    `json:"sport"`
	ImageURLs      []string  `json:"image_urls,omitempty"`
	CreatedAt      string    `json:"created_at"`
	UpdatedAt      string    `json:"updated_at"`
	TotalReviews   int       `json:"total_reviews"`
	AverageRating  float64   `json:"average_rating"`
	UpcomingGames  int       `json:"upcoming_games"`
	CompletedGames int       `json:"completed_games"`
}

// getVenueDetailHandler handles the GET /venue/{id} endpoint.
//
//	@Summary		Get venue details
//	@Description	Retrieve detailed information for a venue including aggregated review and game statistics.
//	@Tags			Venue
//	@Produce		json
//	@Param			id	path		int					true	"Venue ID"
//	@Success		200	{object}	VenueDetailResponse	"Successful response with venue details"
//	@Failure		400	{object}	error				"Bad Request: Invalid venue id"
//	@Failure		404	{object}	error				"Not Found: Venue not found"
//	@Failure		500	{object}	error				"Internal Server Error"
//	@Router			/venue/{id} [get]
func (app *application) getVenueDetailHandler(w http.ResponseWriter, r *http.Request) {
	// Extract venue ID from the URL.
	venueIDStr := chi.URLParam(r, "id")
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil || venueID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venue id"))
		return
	}
	fmt.Printf("the venue id is %v", venueID)

	// Query the venue detail using the repository method.
	vd, err := app.store.Venues.GetVenueDetail(r.Context(), venueID)
	if err != nil {
		app.notFoundResponse(w, r, fmt.Errorf("venue not found: %v", err))
		return
	}

	// Optionally, you can format the timestamps as needed.
	resp := VenueDetailResponse{
		ID:             vd.ID,
		OwnerID:        vd.OwnerID,
		Name:           vd.Name,
		Address:        vd.Address,
		Location:       vd.Location,
		Description:    vd.Description,
		PhoneNumber:    vd.PhoneNumber,
		Amenities:      vd.Amenities,
		OpenTime:       vd.OpenTime,
		Sport:          vd.Sport,
		ImageURLs:      vd.ImageURLs,
		CreatedAt:      vd.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      vd.UpdatedAt.Format(time.RFC3339),
		TotalReviews:   vd.TotalReviews,
		AverageRating:  vd.AverageRating,
		UpcomingGames:  vd.UpcomingGames,
		CompletedGames: vd.CompletedGames,
	}

	// Send the response as JSON.
	if err := app.jsonResponse(w, http.StatusOK, resp); err != nil {
		app.internalServerError(w, r, err)
		return
	}
}

// -----------------------------------------------
// HTTP Handler for Venue Deletion
// -----------------------------------------------

// deleteVenueHandler handles deletion of a venue and its associated images from Cloudinary.

// DeleteVenue godoc
//
//	@Summary		Delete a venue from the system
//	@Description	Deletes a venue by ID and removes all associated images from Cloudinary.
//	@Tags			Venue-Owner
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Success		200		{object}	map[string]string	"Venue deleted successfully"
//	@Failure		400		{object}	error				"Invalid venue ID"
//	@Failure		401		{object}	error				"Unauthorized"
//	@Failure		404		{object}	error				"Venue not found"
//	@Failure		500		{object}	error				"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID} [delete]
func (app *application) deleteVenueHandler(w http.ResponseWriter, r *http.Request) {
	venueIDStr := chi.URLParam(r, "venueID")

	// Convert venueID to int64
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %v", err))
		return
	}
	urls, err := app.store.Venues.GetImageURLs(r.Context(), venueID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 2) Delete each from Cloudinary
	for _, url := range urls {
		fmt.Println(url)
		if err := app.deletePhotoFromCloudinary(url); err != nil {
			app.logger.Warnw("failed to delete image from Cloudinary", "url", url, "err", err)
			app.internalServerError(w, r, err)
			return

		}
	}

	if err := app.store.Venues.Delete(r.Context(), venueID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "Venue deleted successfully"})
}

// getVenueAllPhotosHandler retrieves all photo URLs associated with a venue.
//
// GetVenuePhotos godoc
//
//	@Summary		Retrieve all venue photo URLs
//	@Description	Returns a list of all image URLs associated with the specified venue.
//	@Tags			Venue-Owner
//	@Produce		json
//	@Param			venueID	path		int64	true	"Venue ID"
//	@Success		200		{array}		string	"List of image URLs"
//	@Failure		400		{object}	error	"Invalid venue ID"
//	@Failure		401		{object}	error	"Unauthorized"
//	@Failure		404		{object}	error	"Venue not found"
//	@Failure		500		{object}	error	"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/photos [get]
func (app *application) getVenueAllPhotosHandler(w http.ResponseWriter, r *http.Request) {
	venueIDStr := chi.URLParam(r, "venueID")

	// Convert venueID to int64
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %v", err))
		return
	}

	urls, err := app.store.Venues.GetImageURLs(r.Context(), venueID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, urls)

}

// getVenueInfoHandler retrieves detailed information about a single venue.
//
//	@Summary		Get venue information
//	@Description	Retrieves detailed information (name, address, description, status, etc.) about a venue by its ID.
//	@Tags			Venue-Owner
//	@Produce		json
//	@Param			venueID	path		int64				true	"Venue ID"
//	@Success		200		{object}	venues.VenueInfo	"Detailed venue information"
//	@Failure		400		{object}	error				"Invalid venue ID"
//	@Failure		401		{object}	error				"Unauthorized"
//	@Failure		404		{object}	error				"Venue not found"
//	@Failure		500		{object}	error				"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/venue-info [get]
func (app *application) getVenueInfoHandler(w http.ResponseWriter, r *http.Request) {
	venueIDStr := chi.URLParam(r, "venueID")

	// Convert venueID to int64
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %v", err))
		return
	}

	venueInfo, err := app.store.Venues.GetVenueInfo(r.Context(), venueID)
	if err != nil {
		if errors.Is(err, venues.ErrVenueNotFound) {
			app.notFoundResponse(w, r, venues.ErrVenueNotFound)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, venueInfo)
}

type VenueSearchResponse struct {
	Data struct {
		Venues     []venues.VenueListing `json:"venues"`
		Query      string                `json:"query"`
		SearchType string                `json:"search_type"`
	} `json:"data"`
}

// searchVenuesHandler handles the GET /venues/search endpoint.
//
//	@Summary		Search venues (basic)
//	@Description	Search active venues by name or sport using partial (ILIKE + trigram) matching.
//	@Tags			Venue
//	@Produce		json
//	@Param			q	query		string				true	"Search query (venue name or sport)"
//	@Success		200	{object}	VenueSearchResponse	"Successful response with matching venues"
//	@Failure		400	{object}	error				"Bad Request: Missing or empty search query"
//	@Failure		500	{object}	error				"Internal Server Error"
//	@Router			/venues/search [get]
func (app *application) searchVenuesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		app.badRequestResponse(w, r, fmt.Errorf("search query is required"))
		return
	}

	venues, err := app.store.Venues.SearchVenues(ctx, q)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"venues":      venues,
		"query":       q,
		"search_type": "basic",
	})
}

type VenueSearchFTSResponse struct {
	Data struct {
		Venues     []venues.VenueListingWithRank `json:"venues"`
		Query      string                        `json:"query"`
		SearchType string                        `json:"search_type"`
	} `json:"data"`
}

// fullTextSearchVenuesHandler handles the GET /venues/search/fts endpoint.
//
//	@Summary		Search venues (full text)
//	@Description	Perform full-text search on active venues using weighted ranking on name and sport.
//	@Tags			Venue
//	@Produce		json
//	@Param			q	query		string					true	"Search query (ranked full-text search)"
//	@Success		200	{object}	VenueSearchFTSResponse	"Successful response with ranked venue results"
//	@Failure		400	{object}	error					"Bad Request: Missing or empty search query"
//	@Failure		500	{object}	error					"Internal Server Error"
//	@Router			/venues/search/fts [get]
func (app *application) fullTextSearchVenuesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		app.badRequestResponse(w, r, fmt.Errorf("search query is required"))
		return
	}

	venues, err := app.store.Venues.FullTextSearchVenues(ctx, q)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"venues":      venues,
		"query":       q,
		"search_type": "full_text",
	})
}
