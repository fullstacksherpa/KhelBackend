package main

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/domain/accesscontrol"
	"khel/internal/domain/adminview"
	"khel/internal/domain/bookings"
	"khel/internal/domain/users"
	"khel/internal/helpers"
	"khel/internal/params"
	"net/http"
	"strconv"
	"strings"
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
		case users.ErrConflict:
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
		case users.ErrNotFound:
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

func getUserFromContext(r *http.Request) *users.User {
	if user, ok := r.Context().Value(userCtx).(*users.User); ok {
		return user
	}
	return nil
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
//	@Success		200	{object}	users.User	"Current user data"
//	@Failure		401	{object}	error		"Unauthorized"
//	@Failure		500	{object}	error		"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/users/me [get]
func (app *application) getCurrentUserHandler(w http.ResponseWriter, r *http.Request) {
	userCtx := getUserFromContext(r)
	if userCtx == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. (Optional) re-fetch fresh data from DB to avoid stale info
	user, err := app.store.Users.GetByID(r.Context(), userCtx.ID)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
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

// deleteUserAccountHandler godoc
//
//	@Summary		Delete current user account
//	@Description	Deletes the logged-in user's account and Cloudinary profile photo
//	@Tags			users
//	@Produce		json
//	@Success		204	{string}	string	"User deleted"
//	@Failure		401	{object}	error	"Unauthorized"
//	@Failure		500	{object}	error	"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/users/me [delete]
func (app *application) deleteUserAccountHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Delete profile picture if one exists
	if user.ProfilePictureURL.Valid {
		err := app.deletePhotoFromCloudinary(user.ProfilePictureURL.String)
		if err != nil {
			app.logger.Error(err, nil) // Log failure, don't block deletion
		} else {
			app.logger.Info("Successfully deleted profile picture from Cloudinary", map[string]any{
				"user_id": user.ID,
				"url":     user.ProfilePictureURL.String,
			})
		}
	}

	// Delete user from DB
	err := app.store.Users.Delete(r.Context(), user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListUsers (admin) godoc
//
//	@Summary		List users (admin)
//	@Description	Returns paginated users. Optional role filter (?role=admin|owner|customer|merchant). Includes each user's roles.
//	@Tags			Store-Admin-Users
//	@Produce		json
//	@Param			role	query		string			false	"Filter by role"	Enums(admin, owner, customer, merchant)
//	@Param			page	query		int				false	"Page number (default: 1)"
//	@Param			limit	query		int				false	"Items per page (default: 15)"
//	@Success		200		{object}	map[string]any	"users list with pagination and filters"
//	@Failure		401		{object}	error			"Unauthorized"
//	@Failure		403		{object}	error			"Forbidden"
//	@Failure		500		{object}	error			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/users [get]
func (app *application) adminListUsersHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r)
	if current == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	pg := params.ParsePagination(r.URL.Query())
	role := strings.TrimSpace(r.URL.Query().Get("role"))

	if role != "" {
		valid := map[string]bool{
			string(accesscontrol.RoleAdmin):    true,
			string(accesscontrol.RoleOwner):    true,
			string(accesscontrol.RoleCustomer): true,
			string(accesscontrol.RoleMerchant): true,
		}
		if !valid[role] {
			app.badRequestResponse(w, r, fmt.Errorf("invalid role filter: %s", role))
			return
		}
	}

	items, total, err := app.store.Users.ListAdminUsers(ctx, users.AdminListUsersFilters{Role: role}, pg.Limit, pg.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	pg.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"users":      items,
		"pagination": pg,
		"filters": map[string]any{
			"role": role,
		},
	})
}

// AdminUserOverviewHandler godoc
//
//	@Summary		Get user overview (admin)
//	@Description	Returns a user profile + aggregated stats + up to 5 recent items per section (orders, bookings, upcoming games).
//	@Tags			Store-Admin-Users
//	@Produce		json
//	@Param			userID	path		int64			true	"User ID"
//	@Success		200		{object}	map[string]any	"Envelope: { data: { user, stats, recent } }"
//	@Failure		400		{object}	error			"Bad Request: invalid userID"
//	@Failure		401		{object}	error			"Unauthorized"
//	@Failure		403		{object}	error			"Forbidden"
//	@Failure		404		{object}	error			"Not Found: user not found"
//	@Failure		500		{object}	error			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/users/{userID} [get]
func (app *application) AdminUserOverviewHandler(w http.ResponseWriter, r *http.Request) {
	current := getUserFromContext(r)
	if current == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("not authorized"))
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid userID"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
	defer cancel()

	// 1) Load user
	u, err := app.store.Users.GetByID(ctx, userID)
	if err != nil {
		// adapt to your actual error style
		if strings.Contains(err.Error(), "not found") {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// 2) Stats
	stats, err := app.store.Users.GetAdminUserStats(ctx, userID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 3) Recent Orders (5)
	recentOrders, _, err := app.store.Sales.Orders.ListByUser(ctx, userID, "", 5, 0)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 4) Recent Bookings (5)
	recentBookings, err := app.store.Bookings.GetBookingsByUser(ctx, userID, bookings.BookingFilter{
		Page:   1,
		Limit:  5,
		Status: nil,
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 5) Upcoming Games (slice 5)
	upcomingGames, err := app.store.Games.GetUpcomingGamesByUser(ctx, userID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if len(upcomingGames) > 5 {
		upcomingGames = upcomingGames[:5]
	}

	// --- Map domain -> DTO (to avoid import cycles) ---
	out := adminview.UserOverview{
		User:  helpers.ToAdminUserDTO(*u),
		Stats: helpers.ToAdminUserStatsDTO(*stats),
		Recent: adminview.UserRecent{
			Orders:   make([]adminview.OrderDTO, 0, len(recentOrders)),
			Bookings: make([]adminview.BookingDTO, 0, len(recentBookings)),
			Games:    make([]adminview.GameDTO, 0, len(upcomingGames)),
		},
	}

	for _, o := range recentOrders {
		out.Recent.Orders = append(out.Recent.Orders, helpers.ToOrderDTO(o))
	}
	for _, b := range recentBookings {
		out.Recent.Bookings = append(out.Recent.Bookings, helpers.ToBookingDTO(b))
	}
	for _, g := range upcomingGames {
		out.Recent.Games = append(out.Recent.Games, helpers.ToGameDTO(g))
	}

	// respond with your envelope
	if err := app.jsonResponse(w, http.StatusOK, out); err != nil {
		app.internalServerError(w, r, err)
		return
	}
}
