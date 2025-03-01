package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
func (app *application) extractPublicIDFromURL(url string) (string, error) {
	// Example URL: https://res.cloudinary.com/demo/image/upload/v1234567/venues/abc123.jpg
	// Public ID: venues/abc123
	parts := strings.Split(url, "/")
	if len(parts) < 9 {
		return "", errors.New("invalid Cloudinary URL")
	}
	return strings.Join(parts[8:], "/"), nil
}
