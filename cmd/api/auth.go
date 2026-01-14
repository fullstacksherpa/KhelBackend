package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"khel/internal/domain/users"
	"khel/internal/mailer"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ErrorBadRequestResponse represents the standard error format for bad request API responses.
//
//	@name			ErrorBadRequestResponse
//	@description	Standard error response format returned by all bad request API endpoints
type ErrorBadRequestResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"It show error from err.Error()"`
	Status  int    `json:"status" example:"400"`
}

// ErrorInternalServerResponse represents the standard error format for internal server API responses.
//
//	@name			ErrorInternalServerResponse
//	@description	Standard error response format returned by all internal server error API endpoints
type ErrorInternalServerResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"the server encountered a problem"`
	Status  int    `json:"status" example:"500"`
}

type RegisterUserPayload struct {
	FirstName string `json:"first_name" validate:"required,max=50"`
	LastName  string `json:"last_name" validate:"required,max=50"`
	Email     string `json:"email" validate:"required,email,max=255"`
	Phone     string `json:"phone" validate:"required,len=10,numeric"`
	Password  string `json:"password" validate:"required,min=3,max=72"`
}

// TODO: remove Token from response
type UserWithToken struct {
	*users.User `json:"user"`
	Token       string `json:"token"`
}

// registerUserHandler godoc
//
//	@Summary		Registers a user
//	@Description	Registers a user via Mobile App, Server will send activation url on email and need to click there to verify its your email
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		RegisterUserPayload			true	"User credentials"
//	@Success		201		{object}	UserWithToken				"User registered"
//
//	@Failure		400		{object}	ErrorBadRequestResponse		"Bad request"
//	@Failure		500		{object}	ErrorInternalServerResponse	"Internal Server Error"
//
//	@Router			/authentication/user [post]
func (app *application) registerUserHandler(w http.ResponseWriter, r *http.Request) {
	var payload RegisterUserPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := &users.User{
		FirstName: payload.FirstName,
		LastName:  payload.LastName,
		Email:     payload.Email,
		Phone:     payload.Phone,
	}
	// hash the user password.
	if err := user.Password.Set(payload.Password); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	ctx := r.Context()

	plainToken := uuid.New().String()

	hash := sha256.Sum256([]byte(plainToken))
	hashToken := hex.EncodeToString(hash[:])
	// store the user
	err := app.store.Users.CreateAndInvite(ctx, user, hashToken, app.config.mail.exp)
	if err != nil {
		switch err {
		case users.ErrDuplicateEmail:
			app.badRequestResponse(w, r, err)
		case users.ErrDuplicatePhoneNumber:
			app.badRequestResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return
	}
	userWithToken := UserWithToken{
		User:  user,
		Token: plainToken,
	}

	activationURL := fmt.Sprintf("%s/confirm?token=%s", app.config.frontendURL, plainToken)

	vars := struct {
		Username      string
		ActivationURL string
	}{
		Username:      user.FirstName,
		ActivationURL: activationURL,
	}

	//send email
	status, err := app.mailer.Send(mailer.UserWelcomeTemplate, user.FirstName, user.Email, vars)
	if err != nil {
		app.logger.Errorw("error sending welcome email", "error", err)

		// rollback user creation if email fails (SAGA pattern)
		if err := app.store.Users.Delete(ctx, user.ID); err != nil {
			app.logger.Errorw("error deleting user", "error", err)
		}

		app.internalServerError(w, r, err)
		return
	}

	app.logger.Infow("Email sent", "status code", status)

	if err := app.jsonResponse(w, http.StatusCreated, userWithToken); err != nil {
		app.internalServerError(w, r, err)
	}
}

type CreateUserTokenPayload struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=3,max=72"`
}

// TokenResponse represents the structure of the tokens in the response. made for swagger doc success output
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
	Role         string `json:"role"`
}

// Envelope is a wrapper for API responses.made for swagger doc success output
type Envelope struct {
	Data TokenResponse `json:"data"`
}

// createTokenHandler godoc
//
//	@Summary		Login to get Token
//	@Description	Creates a token for a user after signin or login.
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		CreateUserTokenPayload	true	"User credentials"
//	@Success		200		{object}	Envelope				"Token to save at MMKV"
//	@Failure		400		{object}	error
//	@Failure		401		{object}	error
//	@Failure		500		{object}	error
//	@Router			/authentication/token [post]
func (app *application) createTokenHandler(w http.ResponseWriter, r *http.Request) {
	var payload CreateUserTokenPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user, err := app.store.Users.GetByEmail(r.Context(), payload.Email)
	if err != nil {
		switch err {
		case users.ErrNotFound:
			app.unauthorizedErrorResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return
	}

	if err := user.Password.Compare(payload.Password); err != nil {
		app.unauthorizedErrorResponse(w, r, err)
		return
	}

	venueIDs, err := app.store.Venues.GetOwnedVenueIDs(r.Context(), user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	var role string
	if len(venueIDs) > 0 {
		role = "venue_owner"
	} else {
		role = "user"
	}

	accessToken, refreshToken, err := app.authenticator.GenerateTokens(user.ID, role)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Save refresh token in the database
	err = app.store.Users.SaveRefreshToken(r.Context(), user.ID, refreshToken)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	userIDStr := strconv.FormatInt(user.ID, 10)
	response := map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"user_id":       userIDStr,
		"role":          role,
	}
	fmt.Println(response)

	if err := app.jsonResponse(w, http.StatusOK, response); err != nil {
		app.internalServerError(w, r, err)
	}
}

// LogoutUser godoc
//
//	@Summary		logout user
//	@Description	logout user which will nullify refresh token
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Success		204	{string}	string	"No Content"
//	@Failure		500	{object}	error	"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/users/logout [post]
func (app *application) logoutHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	userID := user.ID

	// Delete refresh token from DB
	err := app.store.Users.DeleteRefreshToken(r.Context(), userID)
	if err != nil {
		fmt.Printf("the error from handler is %s", err)
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type RefreshPayload struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// refreshTokenHandler godoc
//
//	@Summary		Refresh authentication tokens
//	@Description	Validates the provided refresh token and issues new access and refresh tokens.
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		RefreshPayload	true	"Refresh token payload"
//	@Success		200		{object}	Envelope		"New access and refresh tokens"
//	@Failure		400		{object}	error			"Bad request"
//	@Failure		401		{object}	error			"Unauthorized"
//	@Failure		500		{object}	error			"Internal server error"
//	@Router			/authentication/refresh [post]
func (app *application) refreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RefreshToken string `json:"refresh_token" validate:"required"`
	}

	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	token, err := app.authenticator.ValidateRefreshToken(payload.RefreshToken)
	if err != nil || !token.Valid {
		app.unauthorizedErrorResponse(w, r, fmt.Errorf("invalid refresh token"))
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		app.unauthorizedErrorResponse(w, r, fmt.Errorf("invalid claims"))
		return
	}

	// Correctly handle sub claim
	subClaim, ok := claims["sub"].(float64)
	if !ok {
		app.unauthorizedErrorResponse(w, r, fmt.Errorf("invalid sub claim"))
		return
	}

	userID := int64(subClaim) // Convert float64 to int64

	// Ensure refresh token exists in DB
	savedToken, err := app.store.Users.GetRefreshToken(r.Context(), userID)
	if err != nil || savedToken != payload.RefreshToken {
		app.unauthorizedErrorResponse(w, r, fmt.Errorf("refresh token mismatch"))
		return
	}

	venueIDs, err := app.store.Venues.GetOwnedVenueIDs(r.Context(), userID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	var role string
	if len(venueIDs) > 0 {
		role = "venue_owner"
	} else {
		role = "user"
	}

	// Generate new tokens
	accessToken, newRefreshToken, err := app.authenticator.GenerateTokens(userID, role)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Update refresh token in DB
	err = app.store.Users.SaveRefreshToken(r.Context(), userID, newRefreshToken)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	userIDStr := strconv.FormatInt(userID, 10)

	response := map[string]string{
		"access_token":  accessToken,
		"refresh_token": newRefreshToken,
		"user_id":       userIDStr,
		"role":          role,
	}

	if err := app.jsonResponse(w, http.StatusOK, response); err != nil {
		app.internalServerError(w, r, err)
	}
}

type RequestResetPasswordPayload struct {
	Email string `json:"email" validate:"required,email,max=255"`
}

// requestResetPasswordHandler godoc
//
//	@Summary		Request password reset
//	@Description	Request password reset
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		RequestResetPasswordPayload	true	"User email"
//	@Success		200		{object}	map[string]string			"Reset token sent"
//	@Failure		400		{object}	error
//	@Failure		500		{object}	error
//	@Router			/authentication/reset-password [post]
func (app *application) requestResetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	var payload RequestResetPasswordPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	ctx := r.Context()

	// Generate a reset token
	resetToken := uuid.New().String()
	hash := sha256.Sum256([]byte(resetToken))
	hashToken := hex.EncodeToString(hash[:])

	resetTokenExpires := time.Now().UTC().Add(3 * time.Hour)

	user, err := app.store.Users.GetByEmail(ctx, payload.Email)
	if err != nil {
		app.notFoundResponse(w, r, err)
		return
	}

	// Update user with reset token and expiration time
	err = app.store.Users.UpdateResetToken(ctx, payload.Email, hashToken, resetTokenExpires)
	if err != nil {
		if err == users.ErrNotFound {
			app.badRequestResponse(w, r, errors.New("email not found"))
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// Send reset email
	resetURL := fmt.Sprintf("%s/reset-password/?token=%s", app.config.frontendURL, resetToken)

	vars := struct {
		Username string
		ResetURL string
	}{
		Username: user.FirstName,
		ResetURL: resetURL,
	}

	status, err := app.mailer.Send(mailer.ResetPasswordTemplate, payload.Email, payload.Email, vars)
	if err != nil {
		app.logger.Errorw("error sending reset password email", "error", err)
		app.internalServerError(w, r, err)
		return
	}

	app.logger.Infow("Reset password email sent", "status code", status)

	if err := app.jsonResponse(w, http.StatusOK, map[string]string{
		"message":    "Reset token sent",
		"resetToken": resetToken, // unhashed token
	}); err != nil {
		app.internalServerError(w, r, err)
	}
}

type ResetPasswordPayload struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required,min=3,max=72"`
}

// resetPasswordHandler godoc
//
//	@Summary		Reset password
//	@Description	Reset password
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		ResetPasswordPayload	true	"Reset password details"
//	@Success		200		{object}	map[string]string		"Password reset successful"
//	@Failure		400		{object}	error
//	@Failure		500		{object}	error
//	@Router			/authentication/reset-password [patch]
func (app *application) resetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	var payload ResetPasswordPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	ctx := r.Context()

	// Hash the token to compare with the stored hash
	hash := sha256.Sum256([]byte(payload.Token))
	hashToken := hex.EncodeToString(hash[:])
	fmt.Printf("this is hashToken: %s", hashToken)

	// Get user by reset token
	user, err := app.store.Users.GetByResetToken(ctx, hashToken)
	if err != nil {
		if err == users.ErrNotFound {
			app.badRequestResponse(w, r, errors.New("invalid or expired token"))
			return
		}
		app.internalServerError(w, r, err)
		fmt.Println(err)
		return
	}

	now := time.Now().UTC()
	if now.After(user.ResetPasswordExpires.UTC()) {
		err := fmt.Errorf("token expired at %s, current time is %s",
			user.ResetPasswordExpires.UTC().Format(time.RFC3339),
			now.Format(time.RFC3339),
		)
		app.badRequestResponse(w, r, err)
		return
	}

	// Update the user's password
	if err := user.Password.Set(payload.Password); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Clear the reset token and expiration time
	user.ResetPasswordToken = ""
	user.ResetPasswordExpires = time.Time{}

	// Save the updated user
	if err := app.store.Users.Update(ctx, user); err != nil {
		app.internalServerError(w, r, err)
		fmt.Println(err)
		return
	}

	if err := app.jsonResponse(w, http.StatusOK, map[string]string{"message": "Password reset successful"}); err != nil {
		app.internalServerError(w, r, err)
	}
}

// AdminCreateUserPayload is the request body for admin user creation.
type AdminCreateUserPayload struct {
	FirstName         string  `json:"first_name" validate:"required,min=1,max=50"`
	LastName          string  `json:"last_name" validate:"required,min=1,max=50"`
	Email             string  `json:"email" validate:"required,email"`
	Phone             string  `json:"phone" validate:"required"` // use `nepaliphone` if you want: validate:"required,nepaliphone"`
	Password          string  `json:"password" validate:"required,min=4,max=72"`
	SkillLevel        *string `json:"skill_level,omitempty" validate:"omitempty,oneof=beginner intermediate advanced"`
	ProfilePictureURL *string `json:"profile_picture_url,omitempty" validate:"omitempty,url"`
	NoOfGames         *int    `json:"no_of_games,omitempty" validate:"omitempty,gte=0,lte=32767"`
}

// adminCreateUserHandler godoc
//
//	@Summary		Admin creates a user
//	@Description	Creates a user directly from Admin Panel (no invitation/activation email). User is active by default.
//	@Tags			superadmin-role
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		AdminCreateUserPayload		true	"User details"
//	@Success		201		{object}	users.User					"User created"
//	@Failure		400		{object}	ErrorBadRequestResponse		"Bad request"
//	@Failure		500		{object}	ErrorInternalServerResponse	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/superadmin/users [post]
func (app *application) adminCreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var payload AdminCreateUserPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := &users.User{
		FirstName: payload.FirstName,
		LastName:  payload.LastName,
		Email:     payload.Email,
		Phone:     payload.Phone,
	}

	// Optional fields
	if payload.SkillLevel != nil {
		user.SkillLevel = sql.NullString{String: *payload.SkillLevel, Valid: true}
	}
	if payload.ProfilePictureURL != nil {
		user.ProfilePictureURL = sql.NullString{String: *payload.ProfilePictureURL, Valid: true}
	}
	if payload.NoOfGames != nil {
		user.NoOfGames = sql.NullInt16{Int16: int16(*payload.NoOfGames), Valid: true}
	}

	// hash password
	if err := user.Password.Set(payload.Password); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	ctx := r.Context()

	created, err := app.store.Users.AdminCreateUser(ctx, user)
	if err != nil {
		switch err {
		case users.ErrDuplicateEmail:
			app.badRequestResponse(w, r, err)
		case users.ErrDuplicatePhoneNumber:
			app.badRequestResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return
	}

	if err := app.jsonResponse(w, http.StatusCreated, created); err != nil {
		app.internalServerError(w, r, err)
	}
}
