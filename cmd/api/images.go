package main

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
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
