package main

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/domain/facilities"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func parseInt64PathParam(r *http.Request, name string) (int64, error) {
	value := chi.URLParam(r, name)
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return id, nil
}

func (app *application) parseVenueAndFacilityID(r *http.Request) (int64, int64, error) {
	venueID, err := parseInt64PathParam(r, "venueID")
	if err != nil {
		return 0, 0, err
	}

	facilityID, err := parseInt64PathParam(r, "facilityID")
	if err != nil {
		return 0, 0, err
	}

	return venueID, facilityID, nil
}

// Ensures users cannot access another venue's facility by guessing facilityID.
func (app *application) requireFacilityBelongsToVenue(ctx context.Context, venueID, facilityID int64) error {
	ok, err := app.store.Facilities.BelongsToVenue(ctx, venueID, facilityID)
	if err != nil {
		return err
	}

	if !ok {
		return facilities.ErrFacilityNotFound
	}

	return nil
}

type CreateFacilityPayload struct {
	Name        string   `json:"name" validate:"required,max=120"`
	Description *string  `json:"description,omitempty" validate:"omitempty,max=500"`
	Sport       *string  `json:"sport,omitempty" validate:"omitempty,max=50"`
	SurfaceType *string  `json:"surface_type,omitempty" validate:"omitempty,max=80"`
	Capacity    *int     `json:"capacity,omitempty" validate:"omitempty,gt=0"`
	ImageURLs   []string `json:"image_urls,omitempty"`
	IsDefault   bool     `json:"is_default"`
}

type UpdateFacilityPayload struct {
	Name        *string  `json:"name,omitempty" validate:"omitempty,max=120"`
	Description *string  `json:"description,omitempty" validate:"omitempty,max=500"`
	Sport       *string  `json:"sport,omitempty" validate:"omitempty,max=50"`
	SurfaceType *string  `json:"surface_type,omitempty" validate:"omitempty,max=80"`
	Capacity    *int     `json:"capacity,omitempty" validate:"omitempty,gt=0"`
	ImageURLs   []string `json:"image_urls,omitempty"`
	IsActive    *bool    `json:"is_active,omitempty"`
	IsDefault   *bool    `json:"is_default,omitempty"`
}

// createFacilityHandler godoc
//
//	@Summary		Create a facility under a venue
//	@Description	Creates a bookable facility such as Ground A, Court 1, Cricket Net 2, etc. This endpoint accepts multipart/form-data because facility images can be uploaded together with normal form fields.
//	@Description	If one or more images are uploaded using the "images" field, those images are uploaded to Cloudinary and saved for the facility.
//	@Description	If no images are uploaded and image_action is "default", "skip", or empty, the facility will use the venue's existing default image URLs.
//	@Tags			Venue Facilities
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			venueID			path		int					true	"Venue ID"
//	@Param			name			formData	string				true	"Facility name. Example: Ground A, Court 1"
//	@Param			description		formData	string				false	"Facility description"
//	@Param			sport			formData	string				false	"Sport type. Example: football, futsal, cricket"
//	@Param			surface_type	formData	string				false	"Surface type. Example: turf, grass, concrete"
//	@Param			capacity		formData	int					false	"Maximum player/person capacity"
//	@Param			is_default		formData	bool				false	"Whether this facility is the default facility for the venue"
//	@Param			image_action	formData	string				false	"Image behavior when no image file is uploaded. Allowed values: default, skip. Empty also uses venue default images."
//	@Param			images			formData	file				false	"Facility image files. Can be sent multiple times with the same field name: images"
//	@Success		201				{object}	facilities.Facility	"Facility created successfully"
//	@Failure		400				{object}	ErrorResponse		"Invalid form data, validation error, or invalid image_action"
//	@Failure		401				{object}	ErrorResponse		"Unauthorized"
//	@Failure		403				{object}	ErrorResponse		"Forbidden"
//	@Failure		500				{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities [post]
func (app *application) createFacilityHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := parseInt64PathParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Facility create accepts multipart/form-data instead of JSON because
	// the client may send normal fields and image files in the same request.
	if err := r.ParseMultipartForm(maxFacilityImageMemory); err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid multipart form: %w", err))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		app.badRequestResponse(w, r, fmt.Errorf("name is required"))
		return
	}

	capacity, err := parseOptionalIntForm(r, "capacity")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	isDefault, err := parseBoolFormDefault(r, "is_default", false)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	payload := CreateFacilityPayload{
		Name:        name,
		Description: parseOptionalStringForm(r, "description"),
		Sport:       parseOptionalStringForm(r, "sport"),
		SurfaceType: parseOptionalStringForm(r, "surface_type"),
		Capacity:    capacity,
		IsDefault:   isDefault,
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var imageURLs []string

	imageAction := strings.ToLower(strings.TrimSpace(r.FormValue("image_action")))
	files := r.MultipartForm.File["images"]

	switch {
	case len(files) > 0:
		imageURLs, err = app.uploadFacilityImagesToCloudinary(files, venueID, name)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}

	case imageAction == "default" || imageAction == "skip" || imageAction == "":
		imageURLs, err = app.store.Venues.GetImageURLs(r.Context(), venueID)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}

	default:
		app.badRequestResponse(w, r, fmt.Errorf("invalid image_action"))
		return
	}

	facility, err := app.store.Facilities.Create(r.Context(), facilities.CreateFacilityInput{
		VenueID:     venueID,
		Name:        payload.Name,
		Description: payload.Description,
		Sport:       payload.Sport,
		SurfaceType: payload.SurfaceType,
		Capacity:    payload.Capacity,
		ImageURLs:   imageURLs,

		// Always create as non-default first.
		// If the client requested default, we call SetDefault after creation.
		IsDefault: false,
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// If the client wants this new facility to become the venue default,
	// use SetDefault so the old default is unset safely in one transaction.
	if payload.IsDefault {
		if err := app.store.Facilities.SetDefault(r.Context(), venueID, facility.ID); err != nil {
			app.internalServerError(w, r, err)
			return
		}

		// Re-fetch because SetDefault changed is_default after creation.
		facility, err = app.store.Facilities.GetByID(r.Context(), venueID, facility.ID)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}
	}

	app.jsonResponse(w, http.StatusCreated, facility)
}

// listFacilitiesHandler godoc
//
//	@Summary		List facilities under venue
//	@Description	Returns all facilities/grounds/courts under a venue.
//	@Tags			Venue Facilities
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Success		200		{array}		facilities.Facility	"Facilities"
//	@Failure		400		{object}	ErrorResponse		"Bad Request"
//	@Failure		401		{object}	ErrorResponse		"Unauthorized"
//	@Failure		500		{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities [get]
func (app *application) listFacilitiesHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := parseInt64PathParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	items, err := app.store.Facilities.ListByVenueID(r.Context(), venueID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, items)
}

// getFacilityHandler godoc
//
//	@Summary		Get facility detail
//	@Description	Returns one facility by venue ID and facility ID.
//	@Tags			Venue Facilities
//	@Produce		json
//	@Param			venueID		path		int					true	"Venue ID"
//	@Param			facilityID	path		int					true	"Facility ID"
//	@Success		200			{object}	facilities.Facility	"Facility"
//	@Failure		400			{object}	ErrorResponse		"Bad Request"
//	@Failure		404			{object}	ErrorResponse		"Facility not found"
//	@Failure		500			{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID} [get]
func (app *application) getFacilityHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	facility, err := app.store.Facilities.GetByID(r.Context(), venueID, facilityID)
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, facility)
}

// updateFacilityHandler godoc
//
//	@Summary		Update a facility
//	@Description	Updates a facility under a venue. This endpoint accepts multipart/form-data because the client may update normal facility fields and images in one request.
//	@Description	By default, existing images are kept. To change images, send image_action.
//	@Description	image_action=keep keeps current images.
//	@Description	image_action=replace uploads new images and deletes removed old Cloudinary images asynchronously.
//	@Description	image_action=delete removes all facility images and deletes old Cloudinary images asynchronously.
//	@Description	image_action=default replaces facility images with the parent venue's default image URLs.
//	@Description	If is_default=true is sent, this facility becomes the default facility for the venue using SetDefault. Sending is_default=false is not allowed because every venue should keep one default facility.
//	@Tags			Venue Facilities
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			venueID			path		int					true	"Venue ID"
//	@Param			facilityID		path		int					true	"Facility ID"
//	@Param			name			formData	string				false	"Facility name. Example: Ground A, Court 1"
//	@Param			description		formData	string				false	"Facility description"
//	@Param			sport			formData	string				false	"Sport type. Example: football, futsal, cricket"
//	@Param			surface_type	formData	string				false	"Surface type. Example: turf, grass, concrete"
//	@Param			capacity		formData	int					false	"Maximum player/person capacity"
//	@Param			is_active		formData	bool				false	"Whether this facility is active"
//	@Param			is_default		formData	bool				false	"Set to true to make this facility the venue default. False is not allowed."
//	@Param			image_action	formData	string				false	"Image update action. Allowed values: keep, replace, delete, default. Empty means keep."
//	@Param			images			formData	file				false	"New facility image files. Required when image_action=replace. Send multiple files using the same field name: images"
//	@Success		200				{object}	facilities.Facility	"Facility updated successfully"
//	@Failure		400				{object}	ErrorResponse		"Invalid form data, validation error, invalid image_action, or invalid is_default usage"
//	@Failure		401				{object}	ErrorResponse		"Unauthorized"
//	@Failure		403				{object}	ErrorResponse		"Forbidden"
//	@Failure		404				{object}	ErrorResponse		"Facility not found"
//	@Failure		500				{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID} [patch]
func (app *application) updateFacilityHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Load the current facility before updating.
	// We need this for two reasons:
	// 1. To know which old images should be deleted from Cloudinary after image changes.
	// 2. To validate default facility behavior safely.
	oldFacility, err := app.store.Facilities.GetByID(r.Context(), venueID, facilityID)
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// This endpoint accepts multipart/form-data because images and normal
	// text fields can be sent in one request.
	if err := r.ParseMultipartForm(maxFacilityImageMemory); err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid multipart form: %w", err))
		return
	}

	capacity, err := parseOptionalIntForm(r, "capacity")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	isActive, err := parseOptionalBoolForm(r, "is_active")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	isDefault, err := parseOptionalBoolForm(r, "is_default")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// We do not allow is_default=false through this update endpoint.
	// To change the default facility, the client should send is_default=true
	// on another active facility. This keeps the venue from accidentally
	// having no default facility.
	if isDefault != nil && !*isDefault {
		app.badRequestResponse(w, r, fmt.Errorf("is_default=false is not allowed; set another facility as default instead"))
		return
	}

	var name *string
	if rawName := strings.TrimSpace(r.FormValue("name")); rawName != "" {
		name = &rawName
	}

	payload := UpdateFacilityPayload{
		Name:        name,
		Description: parseOptionalStringForm(r, "description"),
		Sport:       parseOptionalStringForm(r, "sport"),
		SurfaceType: parseOptionalStringForm(r, "surface_type"),
		Capacity:    capacity,
		IsActive:    isActive,
		IsDefault:   isDefault,
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Important:
	// Do NOT pass IsDefault into the normal Update call.
	// Setting a default affects multiple rows, so it is handled separately
	// through SetDefault() after the normal update succeeds.
	input := facilities.UpdateFacilityInput{
		Name:        payload.Name,
		Description: payload.Description,
		Sport:       payload.Sport,
		SurfaceType: payload.SurfaceType,
		Capacity:    payload.Capacity,
		IsActive:    payload.IsActive,
		IsDefault:   nil,
	}

	imageAction := strings.ToLower(strings.TrimSpace(r.FormValue("image_action")))

	// New files are only required when image_action=replace.
	// Frontend should append multiple files using the same key: "images".
	files := r.MultipartForm.File["images"]

	var newImageURLs []string

	// This flag tells us whether the image list was intentionally changed.
	// If false, the repository should leave old image_urls unchanged.
	shouldUpdateImages := false

	switch imageAction {
	case "", "keep":
		// Default behavior: keep current facility images.
		// This prevents accidental image deletion when only text fields are updated.

	case "replace":
		// Replace means:
		// 1. Upload new facility images to Cloudinary.
		// 2. Save the new URLs in DB.
		// 3. After DB update succeeds, delete removed old images asynchronously.
		if len(files) == 0 {
			app.badRequestResponse(w, r, fmt.Errorf("images are required when image_action is replace"))
			return
		}

		facilityName := oldFacility.Name
		if payload.Name != nil {
			facilityName = *payload.Name
		}

		newImageURLs, err = app.uploadFacilityImagesToCloudinary(files, venueID, facilityName)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}

		input.ImageURLs = newImageURLs
		shouldUpdateImages = true

	case "delete":
		// Delete means this facility should have no images.
		// Old Cloudinary images are deleted only after the DB update succeeds.
		newImageURLs = []string{}
		input.ImageURLs = newImageURLs
		shouldUpdateImages = true

	case "default":
		// Default means use the parent venue's image URLs for this facility.
		// Any old facility-specific images removed by this change will be cleaned
		// from Cloudinary after the DB update succeeds.
		newImageURLs, err = app.store.Venues.GetImageURLs(r.Context(), venueID)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}

		input.ImageURLs = newImageURLs
		shouldUpdateImages = true

	default:
		app.badRequestResponse(w, r, fmt.Errorf("invalid image_action"))
		return
	}

	updatedFacility, err := app.store.Facilities.Update(r.Context(), venueID, facilityID, input)
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// If is_default=true was sent, switch the venue default using the dedicated
	// SetDefault repository method. This method unsets the old default and sets
	// the new default in one transaction.
	if payload.IsDefault != nil && *payload.IsDefault {
		if err := app.store.Facilities.SetDefault(r.Context(), venueID, facilityID); err != nil {
			if errors.Is(err, facilities.ErrFacilityNotFound) {
				app.notFoundResponse(w, r, err)
				return
			}
			app.internalServerError(w, r, err)
			return
		}

		// Re-fetch the facility because SetDefault changed is_default after
		// the normal update completed.
		updatedFacility, err = app.store.Facilities.GetByID(r.Context(), venueID, facilityID)
		if err != nil {
			if errors.Is(err, facilities.ErrFacilityNotFound) {
				app.notFoundResponse(w, r, err)
				return
			}
			app.internalServerError(w, r, err)
			return
		}
	}

	if shouldUpdateImages {
		// Only delete old images after the database update succeeds.
		// This avoids losing Cloudinary images if the DB update fails.
		removedURLs := findRemovedImageURLs(oldFacility.ImageURLs, updatedFacility.ImageURLs)
		app.deleteCloudinaryImagesAsync(removedURLs)
	}

	app.jsonResponse(w, http.StatusOK, updatedFacility)
}

// deleteFacilityHandler godoc
//
//	@Summary		Delete a facility
//	@Description	Deletes a facility under a venue.
//	@Description	Default facilities cannot be deleted. To delete a default facility, first set another active facility as the default.
//	@Description	After the database record is deleted, the facility images are deleted from Cloudinary asynchronously.
//	@Description	Cloudinary deletion happens in a goroutine so the API response does not wait for external image cleanup.
//	@Tags			Venue Facilities
//	@Produce		json
//	@Param			venueID		path	int	true	"Venue ID"
//	@Param			facilityID	path	int	true	"Facility ID"
//	@Success		204			"No Content"
//	@Failure		400			{object}	ErrorResponse	"Invalid venueID/facilityID or attempt to delete default facility"
//	@Failure		401			{object}	ErrorResponse	"Unauthorized"
//	@Failure		403			{object}	ErrorResponse	"Forbidden"
//	@Failure		404			{object}	ErrorResponse	"Facility not found"
//	@Failure		500			{object}	ErrorResponse	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID} [delete]
func (app *application) deleteFacilityHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Fetch the facility before deleting it so we can:
	// 1. Check whether it is the default facility.
	// 2. Keep image URLs for Cloudinary cleanup after successful DB deletion.
	facility, err := app.store.Facilities.GetByID(r.Context(), venueID, facilityID)
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// Do not allow deleting the default facility.
	// This protects the venue from ending up with no default facility.
	// To delete this facility, the client must first set another active facility
	// as default using PATCH with is_default=true.
	if facility.IsDefault {
		app.badRequestResponse(w, r, fmt.Errorf("cannot delete the default facility; set another facility as default first"))
		return
	}

	// Delete the database record first.
	// Cloudinary cleanup should only happen after DB delete succeeds.
	if err := app.store.Facilities.Delete(r.Context(), venueID, facilityID); err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// Delete Cloudinary images in the background.
	// Do not call app.internalServerError inside the goroutine because the
	// HTTP response is already completed or about to be completed.
	app.deleteCloudinaryImagesAsync(facility.ImageURLs)

	w.WriteHeader(http.StatusNoContent)
}

// -----------------------------------------------------------------------------
// Facility image helpers
// -----------------------------------------------------------------------------

// uploadFacilityImagesToCloudinary uploads all facility image files to Cloudinary
// and returns their secure URLs.
//
// Folder behavior:
//   - production: facilities
//   - non-production: testFacilities
//
// Public ID format:
//
//	venue_{venueID}_facility_{safeFacilityName}_{timestamp}
//
// Note:
// We close each multipart file immediately after upload instead of using defer
// inside the loop. This prevents too many files staying open during bulk uploads.
func (app *application) uploadFacilityImagesToCloudinary(
	files []*multipart.FileHeader,
	venueID int64,
	facilityName string,
) ([]string, error) {
	urls := make([]string, 0, len(files))

	folder := "testFacilities"
	env := os.Getenv("APP_ENV")
	if env == "prod" || env == "production" {
		folder = "facilities"
	}

	safeName := app.createSafePublicID(facilityName)

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("open facility image: %w", err)
		}

		publicID := fmt.Sprintf(
			"venue_%d_facility_%s_%d",
			venueID,
			safeName,
			time.Now().UnixNano(),
		)

		url, err := app.uploadToCloudinaryWithID(file, publicID, folder)

		closeErr := file.Close()
		if closeErr != nil && err == nil {
			err = closeErr
		}

		if err != nil {
			return nil, fmt.Errorf("upload facility image: %w", err)
		}

		urls = append(urls, url)
	}

	return urls, nil
}

// deleteCloudinaryImagesAsync deletes images from Cloudinary in a goroutine.
// This is used after facility update/delete so the API response does not block
// on external Cloudinary deletion.
//
// Important:
// Do not write HTTP responses from inside this goroutine. Only log failures.
func (app *application) deleteCloudinaryImagesAsync(urls []string) {
	if len(urls) == 0 {
		return
	}

	go func(urls []string) {
		for _, url := range urls {
			if strings.TrimSpace(url) == "" {
				continue
			}

			if err := app.deletePhotoFromCloudinary(url); err != nil {
				app.logger.Warnw(
					"failed to delete image from Cloudinary",
					"url", url,
					"err", err,
				)
			}
		}
	}(urls)
}

// findRemovedImageURLs returns URLs that existed before but are no longer
// present after an update. Those removed URLs are safe candidates for
// Cloudinary deletion.
func findRemovedImageURLs(oldURLs, newURLs []string) []string {
	newSet := make(map[string]bool, len(newURLs))
	for _, url := range newURLs {
		newSet[url] = true
	}

	var removed []string
	for _, oldURL := range oldURLs {
		if !newSet[oldURL] {
			removed = append(removed, oldURL)
		}
	}

	return removed
}

const maxFacilityImageMemory = 20 << 20 // 20MB

func parseOptionalStringForm(r *http.Request, key string) *string {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil
	}
	return &value
}

func parseOptionalIntForm(r *http.Request, key string) (*int, error) {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil, nil
	}

	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("%s must be a positive number", key)
	}

	return &n, nil
}

func parseOptionalBoolForm(r *http.Request, key string) (*bool, error) {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return nil, nil
	}

	b, err := strconv.ParseBool(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be true or false", key)
	}

	return &b, nil
}

func parseBoolFormDefault(r *http.Request, key string, defaultValue bool) (bool, error) {
	value := strings.TrimSpace(r.FormValue(key))
	if value == "" {
		return defaultValue, nil
	}

	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}

	return b, nil
}
