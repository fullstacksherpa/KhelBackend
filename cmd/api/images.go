package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

// New helper functions
func (app *application) parseVenueForm(w http.ResponseWriter, r *http.Request, data any) ([]*multipart.FileHeader, error) {
	const maxBytes = 15 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	// Parse multipart form
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		return nil, fmt.Errorf("failed to parse form: %w", err)
	}

	// Extract JSON venue data from the form field
	venueData := r.FormValue("venue")
	if venueData == "" {
		return nil, fmt.Errorf("missing venue data in form")
	}

	// Decode JSON payload
	if err := json.Unmarshal([]byte(venueData), data); err != nil {
		return nil, fmt.Errorf("failed to decode JSON venue data: %w", err)
	}

	// Validate payload
	if err := Validate.Struct(data); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Validate image count
	files := r.MultipartForm.File["images"]
	if len(files) > 7 {
		return nil, fmt.Errorf("maximum of 7 images are allowed")
	}

	return files, nil
}

func (app *application) uploadImages(w http.ResponseWriter, r *http.Request, files []*multipart.FileHeader) ([]string, error) {
	var urls []string
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}

		url, err := app.uploadVenuesToCloudinary(file)
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("cloudinary upload: %w", err)
		}

		urls = append(urls, url)
	}
	return urls, nil
}

func (app *application) uploadVenuesToCloudinary(file io.Reader) (string, error) {
	// Upload using the io.Reader directly
	resp, err := app.cld.Upload.Upload(
		context.Background(),
		file,
		uploader.UploadParams{Folder: "venues"},
	)
	if err != nil {
		return "", fmt.Errorf("cloudinary upload: %w", err)
	}
	return resp.SecureURL, nil
}
