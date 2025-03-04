package main

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/store"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/go-chi/chi/v5"
)

type userKey string

const userCtx userKey = "user"

func (app *application) uploadProfilePictureHandler(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	// Parse the multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	// Retrieve the file from the form data
	file, _, err := r.FormFile("profile_picture")
	if err != nil {
		http.Error(w, "Unable to retrieve file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Upload the file to Cloudinary
	ctx := context.Background()
	uploadResult, err := app.cld.Upload.Upload(ctx, file, uploader.UploadParams{})
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

func (app *application) updateProfilePictureHandler(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	// Parse the multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	// Retrieve the file from the form data
	file, _, err := r.FormFile("profile_picture")
	if err != nil {
		http.Error(w, "Unable to retrieve file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Get the current profile picture URL from the database
	oldUrl, err := app.store.Users.GetProfileUrl(r.Context(), userID)
	if err != nil {
		http.Error(w, "Failed to retrieve current profile picture URL", http.StatusInternalServerError)
		return
	}

	// Upload the new file to Cloudinary
	uploadResult, err := app.cld.Upload.Upload(
		r.Context(), // Use the request context
		file,
		uploader.UploadParams{Folder: "venues"},
	)
	if err != nil {
		http.Error(w, "Failed to upload image to Cloudinary", http.StatusInternalServerError)
		return
	}

	// Save the new URL in the database
	err = app.store.Users.SetProfile(r.Context(), uploadResult.SecureURL, userID)
	if err != nil {
		http.Error(w, "Failed to update profile picture URL in database", http.StatusInternalServerError)
		return
	}

	// Delete the old image from Cloudinary
	if oldUrl != "" {
		publicID := extractPublicIDFromURL(oldUrl) // Use the correct variable name
		_, err = app.cld.Upload.Destroy(r.Context(), uploader.DestroyParams{PublicID: publicID})
		if err != nil {
			// Log the error but don't fail the request
			fmt.Printf("Failed to delete old profile picture from Cloudinary: %v\n", err)
		}
	}
	if err := app.jsonResponse(w, http.StatusOK, uploadResult.SecureURL); err != nil {
		app.internalServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Profile picture updated successfully: %s", uploadResult.SecureURL)))
}

// extractPublicIDFromURL extracts the public ID from a Cloudinary URL
func extractPublicIDFromURL(url string) string {
	// Split the URL by "/"
	parts := strings.Split(url, "/")

	// Find the index of "upload" in the URL
	uploadIndex := -1
	for i, part := range parts {
		if part == "upload" {
			uploadIndex = i
			break
		}
	}

	// If "upload" is not found, return an empty string
	if uploadIndex == -1 || uploadIndex >= len(parts)-1 {
		return ""
	}

	// The public ID starts after the version number (e.g., "v1740815725")
	// So we skip the version number and take everything after it
	publicIDWithExtension := strings.Join(parts[uploadIndex+2:], "/")

	// Remove the file extension (e.g., ".png", ".jpg")
	publicID := strings.TrimSuffix(publicIDWithExtension, ".png")
	publicID = strings.TrimSuffix(publicID, ".jpg") // Add more extensions if needed

	return publicID
}

func (app *application) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: change later from context
	// userID := chi.URLParam(r, "userID")
	var userID = 1
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
	followerUser := getUserFromContext(r)

	// TODO: revert back to auth UserID from ctx

	var payload FollowUser
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	ctx := r.Context()

	if err := app.store.Followers.Follow(ctx, followerUser.ID, payload.UserID); err != nil {
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

	if err := app.jsonResponse(w, http.StatusNoContent, ""); err != nil {
		app.internalServerError(w, r, err)
	}
}

// UnfollowUser gdoc
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
	unfollowerUser := getUserFromContext(r)
	var payload FollowUser
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
	}
	ctx := r.Context()

	if err := app.store.Followers.Unfollow(ctx, unfollowerUser.ID, payload.UserID); err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if err := app.jsonResponse(w, http.StatusNoContent, nil); err != nil {
		app.internalServerError(w, r, err)
	}
}

func (app *application) userContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
		if err != nil {
			app.badRequestResponse(w, r, err)
			return
		}
		ctx := r.Context()
		user, err := app.store.Users.GetByID(ctx, userID)
		if err != nil {
			switch err {
			case store.ErrNotFound:
				app.notFoundResponse(w, r, err)
				return
			default:
				log.Printf("Error at GetById: %v", err) // Add this line for debugging
				app.internalServerError(w, r, err)
				return
			}
		}

		ctx = context.WithValue(ctx, userCtx, user)
		next.ServeHTTP(w, r.WithContext(ctx))

	})
}

func getUserFromContext(r *http.Request) *store.User {
	user, _ := r.Context().Value(userCtx).(*store.User)
	return user
}
