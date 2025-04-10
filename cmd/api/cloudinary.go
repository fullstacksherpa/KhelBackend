package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
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
		if part == "upload" && i+1 < len(pathParts) {
			return strings.Join(pathParts[i+1:], "/"), nil
		}
	}

	return "", errors.New("failed to extract public ID from URL")
}

// -----------------------------------------------
// Cloudinary Upload Functions with Controlled Naming
// -----------------------------------------------

// uploadToCloudinaryWithID uploads a file to Cloudinary using a custom public ID.
func (app *application) uploadToCloudinaryWithID(file io.Reader, publicID string) (string, error) {
	resp, err := app.cld.Upload.Upload(
		context.Background(), // using a background context for external call
		file,
		uploader.UploadParams{
			Folder:    "venues",
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
