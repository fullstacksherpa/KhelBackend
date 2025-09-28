package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

func (app *application) deletePhotoFromCloudinary(photoURL string) error {
	// Extract the public ID from the photo URL
	publicID, err := app.extractPublicIDFromURL(photoURL)
	if err != nil {
		return fmt.Errorf("failed to extract public ID: %w", err)
	}
	//TODO: Delete later
	fmt.Println(publicID)

	// Delete the asset from Cloudinary
	_, err = app.cld.Upload.Destroy(context.Background(), uploader.DestroyParams{
		PublicID: publicID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete photo from Cloudinary: %w", err)
	}

	return nil
}

// Helper function to extract the public ID from the Cloudinary URL
func (app *application) extractPublicIDFromURL(photoURL string) (string, error) {
	parsedURL, err := url.Parse(photoURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL format: %w", err)
	}

	pathParts := strings.Split(parsedURL.Path, "/")
	for i, part := range pathParts {
		if part == "upload" && i+2 < len(pathParts) {
			publicIDParts := pathParts[i+2:] // Skip "v..." version part
			publicID := strings.Join(publicIDParts, "/")

			// Remove file extension
			publicID = strings.TrimSuffix(publicID, filepath.Ext(publicID))
			return publicID, nil
		}
	}

	return "", errors.New("failed to extract public ID from URL")
}

// -----------------------------------------------
// Cloudinary Upload Functions with Controlled Naming
// -----------------------------------------------

// uploadToCloudinaryWithID uploads a file to Cloudinary using a custom public ID.
func (app *application) uploadToCloudinaryWithID(file io.Reader, publicID string) (string, error) {

	env := os.Getenv("APP_ENV")

	folder := "testVenues"
	if env == "prod" || env == "production" {
		folder = "venues"
	}
	resp, err := app.cld.Upload.Upload(
		context.Background(), // using a background context for external call
		file,
		uploader.UploadParams{
			Folder:    folder,
			PublicID:  publicID, // Set custom name (e.g., "venue_12_image_1")
			Overwrite: api.Bool(false),
		},
	)

	if err != nil {
		return "", fmt.Errorf("cloudinary upload: %w", err)
	}
	return resp.SecureURL, nil
}

// uploadImagesWithVenueID iterates over provided files and uploads them to Cloudinary,
// using the venueID along with an image index to control the public ID.
func (app *application) uploadImagesWithVenueID(
	files []*multipart.FileHeader,
	venueID int64,
) ([]string, error) {
	var urls []string

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		// It's important to close the file after we're done using it.
		// Since we are in a loop, we call Close() after each upload.
		defer file.Close()

		// Generate a custom Cloudinary public ID using the venue ID and image number.
		publicID := fmt.Sprintf("venue_%d_image_%d", venueID, time.Now().UnixNano())
		url, err := app.uploadToCloudinaryWithID(file, publicID)
		if err != nil {
			return nil, fmt.Errorf("cloudinary upload: %w", err)
		}

		urls = append(urls, url)
	}

	return urls, nil
}

func (app *application) uploadAdImageToCloudinary(file io.Reader, adTitle string) (string, error) {
	env := os.Getenv("APP_ENV")

	folder := "testAds"
	if env == "prod" || env == "production" {
		folder = "ads"
	}
	// Create safe public ID from title
	publicID := app.createSafePublicID(adTitle)
	publicID = fmt.Sprintf("%s_%d", publicID, time.Now().UnixNano())

	resp, err := app.cld.Upload.Upload(
		context.Background(),
		file,
		uploader.UploadParams{
			Folder:         folder,
			PublicID:       publicID,
			Overwrite:      api.Bool(false),
			Transformation: "w_800,h_450,c_fill,q_auto,f_auto",
		},
	)

	if err != nil {
		return "", fmt.Errorf("cloudinary upload failed: %w", err)
	}
	return resp.SecureURL, nil
}

// Helper function to validate ad image file types
func (app *application) isValidAdImageType(contentType string) bool {
	validTypes := []string{
		"image/jpeg",
		"image/png",
		"image/webp",
		"image/jpg",
	}

	for _, validType := range validTypes {
		if contentType == validType {
			return true
		}
	}

	return false
}

// createSafePublicID creates a safe public ID from ad title for Cloudinary
func (app *application) createSafePublicID(title string) string {
	// Convert to lowercase
	safeID := strings.ToLower(title)

	// Replace spaces and special characters with underscores
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	safeID = reg.ReplaceAllString(safeID, "_")

	// Remove leading/trailing underscores
	safeID = strings.Trim(safeID, "_")

	// Limit length to 50 characters
	if len(safeID) > 50 {
		safeID = safeID[:50]
	}

	// Ensure it's not empty
	if safeID == "" {
		safeID = "ad_image"
	}

	return safeID
}
