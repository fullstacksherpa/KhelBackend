package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
