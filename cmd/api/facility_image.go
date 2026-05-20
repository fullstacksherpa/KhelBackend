package main

import (
	"errors"
	"fmt"
	"khel/internal/domain/facilities"
	"net/http"
	"os"
	"strings"
	"time"
)

/* workflow for deleting facility image
If photo belongs only to facility:
    remove from facility image_urls
    delete from Cloudinary

If photo is also in venue.image_urls:
    remove from facility image_urls only
    DO NOT delete from Cloudinary
*/

func containsURL(urls []string, target string) bool {
	for _, url := range urls {
		if strings.TrimSpace(url) == strings.TrimSpace(target) {
			return true
		}
	}

	return false
}

// shouldDeleteFacilityPhotoFromCloudinary returns true only when the photo is
// facility-specific.
//
// Some facilities reuse venue image_urls as their default photos. In that case,
// deleting the photo from Cloudinary would also break the parent venue image.
// So if the photo exists in venue.image_urls, we only remove it from the
// facility.image_urls array and keep the Cloudinary asset.
func shouldDeleteFacilityPhotoFromCloudinary(venueImageURLs []string, photoURL string) bool {
	return !containsURL(venueImageURLs, photoURL)
}

// filterFacilityOnlyImageURLs returns only images that are safe to delete from
// Cloudinary.
//
// This is useful when deleting a whole facility. If a facility uses venue default
// images, those images must stay in Cloudinary because the venue still needs them.
func filterFacilityOnlyImageURLs(facilityImageURLs, venueImageURLs []string) []string {
	safeToDelete := make([]string, 0)

	for _, photoURL := range facilityImageURLs {
		photoURL = strings.TrimSpace(photoURL)
		if photoURL == "" {
			continue
		}

		if shouldDeleteFacilityPhotoFromCloudinary(venueImageURLs, photoURL) {
			safeToDelete = append(safeToDelete, photoURL)
		}
	}

	return safeToDelete
}

// GetFacilityPhotos godoc
//
//	@Summary		Retrieve all facility photo URLs
//	@Description	Returns all image URLs associated with a specific facility.
//	@Tags			Venue Facilities
//	@Produce		json
//	@Param			venueID		path		int				true	"Venue ID"
//	@Param			facilityID	path		int				true	"Facility ID"
//	@Success		200			{array}		string			"List of facility image URLs"
//	@Failure		400			{object}	ErrorResponse	"Invalid venueID/facilityID"
//	@Failure		404			{object}	ErrorResponse	"Facility not found"
//	@Failure		500			{object}	ErrorResponse	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/photos [get]
func (app *application) getFacilityAllPhotosHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	urls, err := app.store.Facilities.GetImageURLs(r.Context(), venueID, facilityID)
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, urls)
}

// UploadFacilityPhoto godoc
//
//	@Summary		Upload a new photo for a facility
//	@Description	Uploads one new facility photo to Cloudinary and adds the URL to the facility.
//	@Tags			Venue Facilities
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			venueID		path		int					true	"Venue ID"
//	@Param			facilityID	path		int					true	"Facility ID"
//	@Param			photo		formData	file				true	"Photo file to upload"
//	@Success		200			{object}	map[string]string	"Photo uploaded successfully"
//	@Failure		400			{object}	ErrorResponse		"Bad Request"
//	@Failure		404			{object}	ErrorResponse		"Facility not found"
//	@Failure		500			{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/photos [post]
func (app *application) uploadFacilityPhotoHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	ctx := r.Context()

	facility, err := app.store.Facilities.GetByID(ctx, venueID, facilityID)
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	urls, err := app.store.Facilities.GetImageURLs(ctx, venueID, facilityID)
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	if len(urls) >= 7 {
		app.badRequestResponse(w, r, fmt.Errorf("facility %d already has the max of 7 photos", facilityID))
		return
	}

	const maxBytes = 15 * 1024 * 1024 // 15MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	if err := r.ParseMultipartForm(maxBytes); err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}

	file, _, err := r.FormFile("photo")
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("failed to get photo from form: %w", err))
		return
	}
	defer file.Close()

	folder := "testFacilities"
	env := os.Getenv("APP_ENV")
	if env == "prod" || env == "production" {
		folder = "facilities"
	}

	safeName := app.createSafePublicID(facility.Name)

	publicID := fmt.Sprintf(
		"venue_%d_facility_%d_%s_%d",
		venueID,
		facilityID,
		safeName,
		time.Now().UnixNano(),
	)

	newPhotoURL, err := app.uploadToCloudinaryWithID(file, publicID, folder)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	if err := app.store.Facilities.AddPhotoURL(ctx, venueID, facilityID, newPhotoURL); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{
		"photo_url": newPhotoURL,
	})
}

// DeleteFacilityPhoto godoc
//
//	@Summary		Delete a facility photo
//	@Description	Removes a specific photo URL from a facility. If the photo is also a parent venue photo, it is not deleted from Cloudinary.
//	@Tags			Venue Facilities
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int					true	"Venue ID"
//	@Param			facilityID	path		int					true	"Facility ID"
//	@Param			photo_url	query		string				true	"Photo URL to delete"
//	@Success		200			{object}	map[string]string	"Photo deleted successfully"
//	@Failure		400			{object}	ErrorResponse		"Bad Request"
//	@Failure		404			{object}	ErrorResponse		"Facility not found"
//	@Failure		500			{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/photos [delete]
func (app *application) deleteFacilityPhotoHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	photoURL := strings.TrimSpace(r.URL.Query().Get("photo_url"))
	if photoURL == "" {
		app.badRequestResponse(w, r, errors.New("photo_url is required"))
		return
	}

	ctx := r.Context()

	// Make sure facility exists and belongs to the venue.
	facilityURLs, err := app.store.Facilities.GetImageURLs(ctx, venueID, facilityID)
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	if !containsURL(facilityURLs, photoURL) {
		app.badRequestResponse(w, r, fmt.Errorf("photo_url does not exist on this facility"))
		return
	}

	// Get venue image URLs before removing from facility.
	// These are protected because facilities may reuse the venue default images.
	venueURLs, err := app.store.Venues.GetImageURLs(ctx, venueID)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("could not get venue image URLs: %w", err))
		return
	}

	// Remove only from facility image_urls first.
	if err := app.store.Facilities.RemovePhotoURL(ctx, venueID, facilityID, photoURL); err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	// Important:
	// If the photo is one of the venue's own images, do NOT delete it from Cloudinary.
	// Just removing it from facility.image_urls is enough.
	if !containsURL(venueURLs, photoURL) {
		if err := app.deletePhotoFromCloudinary(photoURL); err != nil {
			app.internalServerError(w, r, err)
			return
		}
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{
		"message": "facility photo deleted successfully",
	})
}
