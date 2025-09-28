package main

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/domain/ads"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// Create Ad Payload
type createAdPayload struct {
	Title        string  `form:"title" validate:"required,max=255"`
	Description  *string `form:"description" validate:"omitempty,max=1000"`
	ImageAlt     *string `form:"image_alt" validate:"omitempty,max=255"`
	Link         *string `form:"link" validate:"omitempty,url"`
	DisplayOrder int     `form:"display_order" validate:"min=0"`
	// We will get imageURl after uploading to cloudinary
}

// Update Ad Payload
type updateAdPayload struct {
	Title        *string `form:"title" validate:"omitempty,max=255"`
	Description  *string `form:"description" validate:"omitempty,max=1000"`
	ImageAlt     *string `form:"image_alt" validate:"omitempty,max=255"`
	Link         *string `form:"link" validate:"omitempty,url"`
	Active       *bool   `form:"active"`
	DisplayOrder *int    `form:"display_order" validate:"omitempty,min=0"`
	//  ImageURL-- we'll get it from file upload
}

// GetActiveAds godoc
//
//	@Summary		Get active ads
//	@Description	Retrieves all active ads for the mobile app carousel
//	@Tags			Ads
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Active ads"
//	@Failure		500	{object}	error					"Internal Server Error"
//	@Router			/ads/active [get]
func (app *application) getActiveAdsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	ads, err := app.store.Ads.GetActiveAds(ctx)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"ads": ads,
	}

	app.jsonResponse(w, http.StatusOK, response)
}

// GetAllAds godoc
//
//	@Summary		Get all ads (Admin)
//	@Description	Retrieves all ads with pagination for admin dashboard
//	@Tags			Admin
//	@Accept			json
//	@Produce		json
//	@Param			limit	query		int						false	"Limit results (default: 10, max: 100)"
//	@Param			offset	query		int						false	"Offset results (default: 0)"
//	@Success		200		{object}	map[string]interface{}	"All ads with pagination"
//	@Failure		400		{object}	error					"Bad Request: Invalid parameters"
//	@Failure		500		{object}	error					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/admin/ads [get]
func (app *application) getAllAdsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 10 // default
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			app.badRequestResponse(w, r, errors.New("invalid limit parameter"))
			return
		}
		if parsedLimit > 100 { // max limit to prevent abuse
			limit = 100
		} else {
			limit = parsedLimit
		}
	}

	offset := 0 // default
	if offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err != nil || parsedOffset < 0 {
			app.badRequestResponse(w, r, errors.New("invalid offset parameter"))
			return
		}
		offset = parsedOffset
	}

	ads, total, err := app.store.Ads.GetAllAds(ctx, limit, offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"ads":    ads,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}

	app.jsonResponse(w, http.StatusOK, response)
}

// GetAdByID godoc
//
//	@Summary		Get ad by ID (Admin)
//	@Description	Retrieves a single ad by its ID
//	@Tags			Admin
//	@Accept			json
//	@Produce		json
//	@Param			adID	path		int		true	"Ad ID"
//	@Success		200		{object}	ads.Ad	"Ad details"
//	@Failure		400		{object}	error	"Bad Request: Invalid ad ID"
//	@Failure		404		{object}	error	"Not Found: Ad not found"
//	@Failure		500		{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/admin/ads/{adID} [get]
func (app *application) getAdByIDHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	adID := chi.URLParam(r, "adID")
	aID, err := strconv.ParseInt(adID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid ad ID"))
		return
	}

	ad, err := app.store.Ads.GetAdByID(ctx, aID)
	if err != nil {
		if err.Error() == "ad not found" {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, ad)
}

// CreateAd godoc
//
//	@Summary		Create a new ad (Admin)
//	@Description	Creates a new advertisement with file upload
//	@Tags			Admin
//	@Accept			mpfd
//	@Produce		json
//	@Param			title			formData	string	true	"Ad title"
//	@Param			description		formData	string	false	"Ad description"
//	@Param			image_alt		formData	string	false	"Image alt text"
//	@Param			link			formData	string	false	"Ad link URL"
//	@Param			display_order	formData	int		false	"Display order"
//	@Param			ad_image		formData	file	true	"Ad image file (max size: 5MB)"
//	@Success		201				{object}	ads.Ad	"Ad created successfully"
//	@Failure		400				{object}	error	"Bad Request: Invalid input"
//	@Failure		500				{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/admin/ads [post]
func (app *application) createAdHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Parse multipart form instead of JSON
	err := r.ParseMultipartForm(5 << 20) // 5 MB
	if err != nil {
		app.badRequestResponse(w, r, errors.New("unable to parse form, file size limit is 5MB"))
		return
	}

	// Extract form data manually
	var payload createAdPayload
	payload.Title = r.FormValue("title")
	if desc := r.FormValue("description"); desc != "" {
		payload.Description = &desc
	}
	if alt := r.FormValue("image_alt"); alt != "" {
		payload.ImageAlt = &alt
	}
	if link := r.FormValue("link"); link != "" {
		payload.Link = &link
	}
	if order := r.FormValue("display_order"); order != "" {
		if parsedOrder, err := strconv.Atoi(order); err == nil {
			payload.DisplayOrder = parsedOrder
		}
	}

	// Validate payload
	if err := Validate.Struct(&payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Handle file upload
	file, fileHeader, err := r.FormFile("ad_image")
	if err != nil {
		app.badRequestResponse(w, r, errors.New("ad image is required"))
		return
	}
	defer file.Close()

	// Validate file type
	contentType := fileHeader.Header.Get("Content-Type")
	if !app.isValidAdImageType(contentType) {
		app.badRequestResponse(w, r, errors.New("only JPEG, PNG, JPG, and WebP images are allowed"))
		return
	}

	// Upload to Cloudinary
	imageURL, err := app.uploadAdImageToCloudinary(file, payload.Title)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to upload image: %w", err))
		return
	}

	// Create ad with uploaded image URL
	req := ads.CreateAdRequest{
		Title:        payload.Title,
		Description:  payload.Description,
		ImageURL:     imageURL,
		ImageAlt:     payload.ImageAlt,
		Link:         payload.Link,
		DisplayOrder: payload.DisplayOrder,
	}

	ad, err := app.store.Ads.CreateAd(ctx, req)
	if err != nil {
		// Clean up uploaded image on failure
		app.deletePhotoFromCloudinary(imageURL)
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusCreated, ad)
}

//✅ Summary of updateAds

//Parse ad ID and get current ad.

//Parse form data and handle optional fields.

//Validate fields.

//Handle optional image upload:

//Validate type

//Upload new image

//Keep old image URL for cleanup

//Convert to repository request and update ad.

//Cleanup old image if replaced.

//Return updated ad JSON.

// UpdateAd godoc
//
//	@Summary		Update an ad (Admin)
//	@Description	Updates an existing advertisement with optional file upload
//	@Tags			Admin
//	@Accept			mpfd
//	@Produce		json
//	@Param			adID			path		int		true	"Ad ID"
//	@Param			title			formData	string	false	"Ad title"
//	@Param			description		formData	string	false	"Ad description"
//	@Param			image_alt		formData	string	false	"Image alt text"
//	@Param			link			formData	string	false	"Ad link URL"
//	@Param			active			formData	boolean	false	"Ad active status"
//	@Param			display_order	formData	int		false	"Display order"
//	@Param			ad_image		formData	file	false	"New ad image file (max size: 5MB)"
//	@Success		200				{object}	ads.Ad	"Ad updated successfully"
//	@Failure		400				{object}	error	"Bad Request: Invalid input"
//	@Failure		404				{object}	error	"Not Found: Ad not found"
//	@Failure		500				{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/admin/ads/{adID} [put]
func (app *application) updateAdHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	adID := chi.URLParam(r, "adID")
	aID, err := strconv.ParseInt(adID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid ad ID"))
		return
	}

	// First, get the current ad to access the current image URL
	currentAd, err := app.store.Ads.GetAdByID(ctx, aID)
	if err != nil {
		if err.Error() == "ad not found" {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// Parse multipart form
	err = r.ParseMultipartForm(5 << 20) // 5 MB
	if err != nil {
		app.badRequestResponse(w, r, errors.New("unable to parse form, file size limit is 5MB"))
		return
	}

	// Extract form data
	var payload updateAdPayload
	if title := r.FormValue("title"); title != "" {
		payload.Title = &title
	}
	if desc := r.FormValue("description"); desc != "" {
		payload.Description = &desc
	}
	if alt := r.FormValue("image_alt"); alt != "" {
		payload.ImageAlt = &alt
	}
	if link := r.FormValue("link"); link != "" {
		payload.Link = &link
	}
	if active := r.FormValue("active"); active != "" {
		if parsedActive, err := strconv.ParseBool(active); err == nil {
			payload.Active = &parsedActive
		}
	}
	if order := r.FormValue("display_order"); order != "" {
		if parsedOrder, err := strconv.Atoi(order); err == nil {
			payload.DisplayOrder = &parsedOrder
		}
	}

	// Validate payload
	if err := Validate.Struct(&payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Handle image upload if provided
	var newImageURL string
	var oldImageURL string
	file, fileHeader, err := r.FormFile("ad_image")
	if err == nil {
		// New image file is provided
		defer file.Close()

		// Validate file type
		contentType := fileHeader.Header.Get("Content-Type")
		if !app.isValidAdImageType(contentType) {
			app.badRequestResponse(w, r, errors.New("only JPEG, PNG, JPG, and WebP images are allowed"))
			return
		}

		// Generate title for cloudinary upload (use new title if provided, otherwise current)
		title := currentAd.Title
		if payload.Title != nil {
			title = *payload.Title
		}

		// Upload new image to Cloudinary
		newImageURL, err = app.uploadAdImageToCloudinary(file, title)
		if err != nil {
			app.internalServerError(w, r, fmt.Errorf("failed to upload new image: %w", err))
			return
		}

		// Store old image URL for cleanup
		oldImageURL = currentAd.ImageURL
	} else if err != http.ErrMissingFile {
		// Error other than missing file
		app.badRequestResponse(w, r, errors.New("error processing image file"))
		return
	}

	// Convert payload to store request
	req := ads.UpdateAdRequest{
		Title:        payload.Title,
		Description:  payload.Description,
		ImageAlt:     payload.ImageAlt,
		Link:         payload.Link,
		Active:       payload.Active,
		DisplayOrder: payload.DisplayOrder,
	}

	// Add new image URL to request if uploaded
	if newImageURL != "" {
		req.ImageURL = &newImageURL
	}

	ad, err := app.store.Ads.UpdateAd(ctx, aID, req)
	if err != nil {
		// If update fails and we uploaded a new image, clean it up
		if newImageURL != "" {
			if deleteErr := app.deletePhotoFromCloudinary(newImageURL); deleteErr != nil {
				app.logger.Errorw("failed to cleanup new uploaded image after update failure",
					"image_url", newImageURL, "error", deleteErr)
			}
		}

		if err.Error() == "ad not found" {
			app.notFoundResponse(w, r, err)
			return
		}
		if err.Error() == "no fields to update" {
			app.badRequestResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// If update was successful and we have a new image, delete the old one
	if oldImageURL != "" && newImageURL != "" {
		if err := app.deletePhotoFromCloudinary(oldImageURL); err != nil {
			// Log the error but don't fail the request
			app.logger.Errorw("failed to delete old ad image from Cloudinary",
				"old_image_url", oldImageURL, "ad_id", aID, "error", err)
		}
	}

	app.jsonResponse(w, http.StatusOK, ad)
}

// DeleteAd godoc
//
//	@Summary		Delete an ad (Admin)
//	@Description	Deletes an advertisement
//	@Tags			Admin
//	@Accept			json
//	@Produce		json
//	@Param			adID	path		int					true	"Ad ID"
//	@Success		200		{object}	map[string]string	"Ad deleted successfully"
//	@Failure		400		{object}	error				"Bad Request: Invalid ad ID"
//	@Failure		404		{object}	error				"Not Found: Ad not found"
//	@Failure		500		{object}	error				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/admin/ads/{adID} [delete]
func (app *application) deleteAdHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	adID := chi.URLParam(r, "adID")
	aID, err := strconv.ParseInt(adID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid ad ID"))
		return
	}

	// Get ad first to retrieve image URL
	ad, err := app.store.Ads.GetAdByID(ctx, aID)
	if err != nil {
		if err.Error() == "ad not found" {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	err = app.store.Ads.DeleteAd(ctx, aID)
	if err != nil {
		if err.Error() == "ad not found" {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// Delete image from Cloudinary
	if err := app.deletePhotoFromCloudinary(ad.ImageURL); err != nil {
		app.logger.Errorw("failed to delete ad image from Cloudinary",
			"ad_id", aID, "image_url", ad.ImageURL, "error", err)
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "ad deleted successfully"})
}

// ToggleAdStatus godoc
//
//	@Summary		Toggle ad status (Admin)
//	@Description	Toggles the active status of an advertisement
//	@Tags			Admin
//	@Accept			json
//	@Produce		json
//	@Param			adID	path		int		true	"Ad ID"
//	@Success		200		{object}	ads.Ad	"Ad status toggled successfully"
//	@Failure		400		{object}	error	"Bad Request: Invalid ad ID"
//	@Failure		404		{object}	error	"Not Found: Ad not found"
//	@Failure		500		{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/admin/ads/{adID}/toggle [post]
func (app *application) toggleAdStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	adID := chi.URLParam(r, "adID")
	aID, err := strconv.ParseInt(adID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid ad ID"))
		return
	}

	ad, err := app.store.Ads.ToggleAdStatus(ctx, aID)
	if err != nil {
		if err.Error() == "ad not found" {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, ad)
}

// TrackImpression godoc
//
//	@Summary		Track ad impression
//	@Description	Increments the impression count for an advertisement
//	@Tags			Ads
//	@Accept			json
//	@Produce		json
//	@Param			adID	path		int					true	"Ad ID"
//	@Success		200		{object}	map[string]string	"Impression tracked successfully"
//	@Failure		400		{object}	error				"Bad Request: Invalid ad ID"
//	@Failure		404		{object}	error				"Not Found: Ad not found"
//	@Failure		500		{object}	error				"Internal Server Error"
//	@Router			/ads/{adID}/impression [post]
func (app *application) trackImpressionHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	adID := chi.URLParam(r, "adID")
	aID, err := strconv.ParseInt(adID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid ad ID"))
		return
	}

	err = app.store.Ads.IncrementImpressions(ctx, aID)
	if err != nil {
		if err.Error() == "ad not found" {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "impression tracked"})
}

// TrackClick godoc
//
//	@Summary		Track ad click
//	@Description	Increments the click count for an advertisement
//	@Tags			Ads
//	@Accept			json
//	@Produce		json
//	@Param			adID	path		int					true	"Ad ID"
//	@Success		200		{object}	map[string]string	"Click tracked successfully"
//	@Failure		400		{object}	error				"Bad Request: Invalid ad ID"
//	@Failure		404		{object}	error				"Not Found: Ad not found"
//	@Failure		500		{object}	error				"Internal Server Error"
//	@Router			/ads/{adID}/click [post]
func (app *application) trackClickHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	adID := chi.URLParam(r, "adID")
	aID, err := strconv.ParseInt(adID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid ad ID"))
		return
	}

	err = app.store.Ads.IncrementClicks(ctx, aID)
	if err != nil {
		if err.Error() == "ad not found" {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "click tracked"})
}

// GetAdsAnalytics godoc
//
//	@Summary		Get ads analytics (Admin)
//	@Description	Retrieves analytics data for all advertisements
//	@Tags			Admin
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	ads.Analytics	"Analytics data"
//	@Failure		500	{object}	error			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/admin/ads/analytics [get]
func (app *application) getAdsAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	analytics, err := app.store.Ads.GetAdsAnalytics(ctx)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, analytics)
}

// Bulk update display order payload
type bulkUpdateDisplayOrderPayload struct {
	Updates []ads.DisplayOrderUpdate `json:"updates" validate:"required,min=1"`
}

// BulkUpdateDisplayOrder godoc
//
//	@Summary		Bulk update display order (Admin)
//	@Description	Updates display order for multiple ads in a single transaction
//	@Tags			Admin
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		bulkUpdateDisplayOrderPayload	true	"Bulk update payload"
//	@Success		200		{object}	map[string]string				"Display orders updated successfully"
//	@Failure		400		{object}	error							"Bad Request: Invalid input"
//	@Failure		500		{object}	error							"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/admin/ads/bulk-update-order [post]
func (app *application) bulkUpdateDisplayOrderHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	var payload bulkUpdateDisplayOrderPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Validate payload
	if err := Validate.Struct(&payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	err := app.store.Ads.BulkUpdateDisplayOrder(ctx, payload.Updates)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "display orders updated successfully"})
}
