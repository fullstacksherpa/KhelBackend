package main

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/go-playground/validator/v10"
)

var Validate *validator.Validate

// init is special function in Go. it is used for package initialization and has some unique properties.  it is executed before main(), without explicitly called.
// each package can have multiple init() func, but they run only once when the package is imported.
// init() func in imported package run before the init() of the main package
// init() cannot take parameters or return values

func init() {
	Validate = validator.New(validator.WithRequiredStructEnabled())

	// Register custom validation for Nepali phone numbers
	Validate.RegisterValidation("nepaliphone", func(fl validator.FieldLevel) bool {
		phone := fl.Field().String()
		// Matches 98[4-9] followed by 7 digits (e.g., 9841234567)
		matched, _ := regexp.MatchString(`^98[4-9][0-9]{7}$`, phone)
		return matched
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}

// it parses body into Go struct.
func readJSON(w http.ResponseWriter, r *http.Request, data any) error {
	maxBytes := 1_048_578 //1mb
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(data)
}

func writeJSONError(w http.ResponseWriter, status int, message string) error {
	type envelope struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Status  int    `json:"status"`
	}

	return writeJSON(w, status, &envelope{
		Success: false,
		Message: message,
		Status:  status,
	})
}

func (app *application) jsonResponse(w http.ResponseWriter, status int, data any) error {
	type envelope struct {
		Data any `json:"data"`
	}
	return writeJSON(w, status, &envelope{Data: data})
}
