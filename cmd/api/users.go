package main

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/store"
	"net/http"
	"strconv"
	"time"

	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/go-chi/chi/v5"
)

type userKey string

const userCtx userKey = "user"

// for cloudinary uploadParams
func boolPtr(b bool) *bool {
	return &b
}

// uploadProfilePictureHandler godoc
//
//	@Summary		Upload profile picture
//	@Description	Uploads a user's profile picture and saves the URL in the database
//	@Tags			users
//	@Accept			mpfd
//	@Produce		json
//	@Param			profile_picture	formData	file	true	"Profile picture file size limit is 2MB"
//	@Success		200				{string}	string	"Profile picture uploaded successfully: <URL>"
//	@Failure		400				{object}	error	"Unable to parse form or retrieve file"
//	@Failure		500				{object}	error	"Failed to upload image to Cloudinary or save URL in database"
//	@Security		ApiKeyAuth
//	@Router			/users/profile-picture [post]
func (app *application) uploadProfilePictureHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	userID := user.ID
	overwrite := boolPtr(true) // Using the helper function

	// Parse the multipart form
	err := r.ParseMultipartForm(2 << 20) // 2 MB
	if err != nil {
		http.Error(w, "Unable to parse form, file size limit is 2MB", http.StatusBadRequest)
		return
	}

	// Retrieve the file from the form data
	file, fileHeader, err := r.FormFile("profile_picture")
	if err != nil {
		http.Error(w, "Unable to retrieve file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file type (allow only JPEG & PNG)
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType != "image/jpeg" && contentType != "image/png" {
		http.Error(w, "Only JPEG and PNG images are allowed", http.StatusBadRequest)
		return
	}

	// Upload the file to Cloudinary
	ctx := context.Background()

	uploadParams := uploader.UploadParams{
		PublicID:  fmt.Sprintf("/%d", userID), // Save with userID as filename
		Overwrite: overwrite,
		// Replace old profile pic
		Folder:         "profile_pictures",          // Organized storage
		Transformation: "w_300,h_300,c_fill,q_auto", // Resize to 300x300, auto quality
	}
	uploadResult, err := app.cld.Upload.Upload(ctx, file, uploadParams)
	if err != nil {
		http.Error(w, "Failed to upload image to Cloudinary", http.StatusInternalServerError)
		return
	}

	// Save the URL in the database

	if err := app.store.Users.SetProfile(r.Context(), uploadResult.SecureURL, userID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Profile picture uploaded successfully: %s", uploadResult.SecureURL)))
}

// updateProfilePictureHandler godoc
//
//	@Summary		Update profile picture
//	@Description	Updates a user's profile picture, saves the new URL in the database, and deletes the old one from Cloudinary
//	@Tags			users
//	@Accept			mpfd
//	@Produce		json
//	@Param			profile_picture	formData	file	true	"Profile picture file (max size: 2MB)"
//	@Success		200				{string}	string	"Profile picture updated successfully: <URL>"
//	@Failure		400				{object}	error	"Unable to parse form or retrieve file"
//	@Failure		500				{object}	error	"Failed to upload image to Cloudinary, update database, or delete old image"
//	@Security		ApiKeyAuth
//	@Router			/users/profile-picture [put]
func (app *application) updateProfilePictureHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	userID := user.ID

	// Parse the multipart form
	err := r.ParseMultipartForm(2 << 20) // 2 MB limit
	if err != nil {
		http.Error(w, "Unable to parse form, file size limit is 2MB", http.StatusBadRequest)
		return
	}

	// Retrieve the file from form data
	file, _, err := r.FormFile("profile_picture")
	if err != nil {
		http.Error(w, "Unable to retrieve file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Upload the new file to Cloudinary with specific PublicID to ensure replacement
	uploadParams := uploader.UploadParams{
		Folder:         "profile_pictures",
		Overwrite:      boolPtr(true),               // Ensure overwrite of the existing file
		Transformation: "w_300,h_300,c_fill,q_auto", // Optional transformations (e.g., resizing)
		PublicID:       fmt.Sprintf("/%d", userID),  // Use userID as the PublicID to replace the old image
	}

	uploadResult, err := app.cld.Upload.Upload(r.Context(), file, uploadParams)
	if err != nil {
		http.Error(w, "Failed to upload image to Cloudinary", http.StatusInternalServerError)
		return
	}

	// Save the new profile picture URL in the database
	err = app.store.Users.SetProfile(r.Context(), uploadResult.SecureURL, userID)
	if err != nil {
		http.Error(w, "Failed to update profile picture URL in database", http.StatusInternalServerError)
		return
	}

	// Return JSON response with new image URL
	if err := app.jsonResponse(w, http.StatusOK, uploadResult.SecureURL); err != nil {
		app.internalServerError(w, r, err)
		return
	}
}

// UpdateUser godoc
//
//	@Summary		Update user information
//	@Description	Update user information such as first name, last name, skill level, and phone number
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			body	body		object	true	"Request body containing fields to update: first_name, last_name, skill_level, phone"
//	@Success		204		{string}	string	"User info updated successfully"
//	@Failure		400		{object}	error	"Bad request, update values can't be nil"
//	@Failure		404		{object}	error	"User not found"
//	@Failure		500		{object}	error	"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/users [put]
func (app *application) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	userID := user.ID
	var payload struct {
		FirstName  *string `json:"first_name"`
		LastName   *string `json:"last_name"`
		SkillLevel *string `json:"skill_level"`
		Phone      *string `json:"phone"`
	}

	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Create update map dynamically
	updates := make(map[string]interface{})
	if payload.FirstName != nil {
		updates["first_name"] = *payload.FirstName
	}
	if payload.LastName != nil {
		updates["last_name"] = *payload.LastName
	}
	if payload.SkillLevel != nil {
		updates["skill_level"] = *payload.SkillLevel
	}
	if payload.Phone != nil {
		updates["phone"] = *payload.Phone
	}

	if len(updates) == 0 {
		app.badRequestResponse(w, r, errors.New("bad request, updates values can't be nil"))
		return
	}

	// Call update method
	if err := app.store.Users.UpdateUser(r.Context(), int64(userID), updates); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent) // No content response on success
	w.Write([]byte(fmt.Sprintf("User info updated successfully: %s", updates)))
}

type FollowUser struct {
	UserID int64 `json:"user_id"`
}

// FollowUser godoc
//
//	@Summary		Follows a user
//	@Description	Follows a user by ID
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			userID	path		int		true	"User ID"
//	@Success		204		{string}	string	"User followed"
//	@Failure		400		{object}	error	"User payload missing"
//	@Failure		404		{object}	error	"User not found"
//	@Security		ApiKeyAuth
//	@Router			/users/{userID}/follow [put]
func (app *application) followUserHandler(w http.ResponseWriter, r *http.Request) {
	followerUser := getUserFromContext(r)                                  //this is app user
	followedID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64) //this is user we want to follow
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	ctx := r.Context()

	if err := app.store.Followers.Follow(ctx, followedID, followerUser.ID); err != nil {
		switch err {
		case store.ErrConflict:
			app.conflictResponse(w, r, err)
			return
		default:
			app.internalServerError(w, r, err)
			return
		}

	}
	if err := app.jsonResponse(w, http.StatusNoContent, nil); err != nil {
		app.internalServerError(w, r, err)
	}
}

// ActivateUser godoc
//
//	@Summary		Activate user account
//	@Description	Activate a user account using an activation token provided in the URL
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			token	path		string	true	"Activation token"
//	@Success		204		{string}	string	"User activated"
//	@Failure		404		{object}	error	"User not found"
//	@Failure		500		{object}	error	"Internal server error"
//	@Router			/users/activate/{token} [put]
func (app *application) activateUserHandler(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	err := app.store.Users.Activate(r.Context(), token)
	if err != nil {
		switch err {
		case store.ErrNotFound:
			app.notFoundResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusNoContent, "")
}

// UnfollowUser godoc
//
//	@Summary		Unfollow a user
//	@Description	Unfollow a user by ID
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			userID	path		int		true	"User ID"
//	@Success		204		{string}	string	"User unfollowed"
//	@Failure		400		{object}	error	"User payload missing"
//	@Failure		404		{object}	error	"User not found"
//	@Security		ApiKeyAuth
//	@Router			/users/{userID}/unfollow [put]
func (app *application) unfollowUserHandler(w http.ResponseWriter, r *http.Request) {
	followerUser := getUserFromContext(r)
	unfollowedID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	ctx := r.Context()

	if err := app.store.Followers.Unfollow(ctx, unfollowedID, followerUser.ID); err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if err := app.jsonResponse(w, http.StatusNoContent, nil); err != nil {
		app.internalServerError(w, r, err)
	}
}

func getUserFromContext(r *http.Request) *store.User {
	user, _ := r.Context().Value(userCtx).(*store.User)
	return user
}

// editProfileHandler godoc
//
//	@Summary		Edit current user’s profile
//	@Description	Update any combination of first name, last name, phone, skill level, and/or profile picture in one call.
//	@Tags			users
//	@Accept			mpfd
//	@Produce		json
//	@Param			first_name		formData	string	false	"First name"
//	@Param			last_name		formData	string	false	"Last name"
//	@Param			phone			formData	string	false	"Phone number (10 digits)"
//	@Param			skill_level		formData	string	false	"Skill level"	Enums(beginner, intermediate, advanced)
//	@Param			profile_picture	formData	file	false	"JPEG or PNG image (max 5 MB)"
//	@Success		204				{string}	string	"Profile updated successfully"
//	@Failure		400				{object}	error	"Bad request (e.g. parse error, invalid field)"
//	@Failure		500				{object}	error	"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/users/update-profile [patch]
func (app *application) editProfileHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	userID := user.ID

	if err := r.ParseMultipartForm(5 << 20); err != nil {
		http.Error(w, "Could not parse form", http.StatusBadRequest)
		return
	}

	// Build updates map
	updates := make(map[string]interface{})
	allowed := []string{"first_name", "last_name", "phone", "skill_level"}
	for _, f := range allowed {
		if vals := r.MultipartForm.Value[f]; len(vals) > 0 {
			updates[f] = vals[0]
		}
	}

	// Handle optional image upload
	var newURL string
	file, header, err := r.FormFile("profile_picture")
	if err == nil {
		defer file.Close()
		ct := header.Header.Get("Content-Type")
		if ct != "image/jpeg" && ct != "image/png" {
			http.Error(w, "only jpeg/png", http.StatusBadRequest)
			return
		}
		uploadParams := uploader.UploadParams{
			PublicID:       fmt.Sprintf("/%d", userID),
			Overwrite:      boolPtr(true),
			Folder:         "profile_pictures",
			Transformation: "w_300,h_300,c_fill,q_auto",
		}
		res, err := app.cld.Upload.Upload(r.Context(), file, uploadParams)
		if err != nil {
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}
		newURL = res.SecureURL
	}

	// If no image was provided, keep existing URL:
	if newURL == "" {
		old, err := app.store.Users.GetProfileUrl(r.Context(), userID)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}
		newURL = old
	}

	// 4) Call our new UpdateAndUpload
	if err := app.store.Users.UpdateAndUpload(r.Context(), userID, updates, newURL); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getCurrentUserHandler godoc
//
//	@Summary		Get current user profile
//	@Description	Retrieve the authenticated user’s profile information
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	store.User	"Current user data"
//	@Failure		401	{object}	error		"Unauthorized"
//	@Failure		500	{object}	error		"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/users/me [get]
func (app *application) getCurrentUserHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Extract *store.User from context
	userCtx := getUserFromContext(r)
	if userCtx == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. (Optional) re-fetch fresh data from DB to avoid stale info
	user, err := app.store.Users.GetByID(r.Context(), userCtx.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			app.internalServerError(w, r, err)
		}
		return
	}

	// 3. Prepare a safe response DTO (omit sensitive fields)
	resp := struct {
		ID                int64   `json:"id"`
		FirstName         string  `json:"first_name"`
		LastName          string  `json:"last_name"`
		Email             string  `json:"email"`
		ProfilePictureURL *string `json:"profile_picture_url,omitempty"`
		SkillLevel        *string `json:"skill_level,omitempty"`
		Phone             string  `json:"phone"`
		NoOfGames         int     `json:"no_of_games"`
		CreatedAt         string  `json:"created_at"`
		UpdatedAt         string  `json:"updated_at"`
	}{
		ID:        user.ID,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Email:     user.Email,
		Phone:     user.Phone,
		NoOfGames: int(user.NoOfGames.Int16),
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
		UpdatedAt: user.UpdatedAt.Format(time.RFC3339),
	}
	if user.ProfilePictureURL.Valid {
		resp.ProfilePictureURL = &user.ProfilePictureURL.String
	}
	if user.SkillLevel.Valid {
		resp.SkillLevel = &user.SkillLevel.String
	}

	// 4. JSON-encode and return
	if err := app.jsonResponse(w, http.StatusOK, resp); err != nil {
		app.internalServerError(w, r, err)
	}
}
