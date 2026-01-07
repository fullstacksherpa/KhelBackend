package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
)

// setAuthCookies sets access + refresh tokens as HttpOnly cookies.
// Web browsers store/send these automatically; JS cannot read them (HttpOnly).
func (app *application) setAuthCookies(w http.ResponseWriter, accessToken, refreshToken string) {

	domain := ""

	if app.config.env == "production" {
		domain = ".gocloudnepal.com"
	}
	// Access token cookie (short lived)
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     "/",
		Domain:   domain, // ✅ works for web.gocloudnepal.com + api.gocloudnepal.com
		HttpOnly: true,
		Secure:   app.config.env == "production", // ✅ must be true in production (HTTPS)
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(app.config.auth.token.accessTokenExp.Seconds()),
	})

	// Refresh token cookie (long lived)
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/v1/authentication", // ✅ refresh/logout only
		Domain:   domain,
		HttpOnly: true,
		Secure:   app.config.env == "production",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(app.config.auth.token.refreshTokenExp.Seconds()),
	})
}

func (app *application) clearAuthCookies(w http.ResponseWriter) {
	expire := func(name, path string) {

		domain := ""

		if app.config.env == "production" {
			domain = ".gocloudnepal.com"
		}
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     path,
			Domain:   domain,
			HttpOnly: true,
			Secure:   app.config.env == "production",
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
	}

	expire("access_token", "/")
	expire("refresh_token", "/v1/authentication")
}

// - same login logic
// - sets HttpOnly cookies for web
// - returns small JSON (user_id, role)
func (app *application) createTokenCookieHandler(w http.ResponseWriter, r *http.Request) {
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
		app.unauthorizedErrorResponse(w, r, err)
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

	isAdmin, err := app.store.AccessControl.UserHasRole(r.Context(), user.ID, "admin")
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to check user role: %w", err))
		return
	}

	isMerchant, err := app.store.AccessControl.UserHasRole(r.Context(), user.ID, "merchant")
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to check user role: %w", err))
		return
	}

	role := "user"
	if isAdmin {
		role = "admin"
	} else if isMerchant {
		role = "merchant"
	} else if len(venueIDs) > 0 {
		role = "venue_owner"
	}

	// after computing role
	if role == "user" {
		app.forbiddenResponse(w, r)
		return
	}

	accessToken, refreshToken, err := app.authenticator.GenerateTokens(user.ID, role)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Save refresh token in DB for rotation/revocation
	if err := app.store.Users.SaveRefreshToken(r.Context(), user.ID, refreshToken); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// ✅ set cookies
	app.setAuthCookies(w, accessToken, refreshToken)

	// Return minimal data (web doesn’t need tokens)
	userIDStr := strconv.FormatInt(user.ID, 10)
	_ = app.jsonResponse(w, http.StatusOK, map[string]string{
		"user_id": userIDStr,
		"role":    role,
	})
}

func (app *application) refreshTokenCookieHandler(w http.ResponseWriter, r *http.Request) {
	// take refresh token from cookie
	c, err := r.Cookie("refresh_token")
	if err != nil || c.Value == "" {
		app.unauthorizedErrorResponse(w, r, errors.New("missing refresh token"))
		return
	}

	token, err := app.authenticator.ValidateRefreshToken(c.Value)
	if err != nil || !token.Valid {
		app.unauthorizedErrorResponse(w, r, errors.New("invalid refresh token"))
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		app.unauthorizedErrorResponse(w, r, errors.New("invalid claims"))
		return
	}

	sub, ok := claims["sub"].(float64)
	if !ok {
		app.unauthorizedErrorResponse(w, r, errors.New("invalid sub claim"))
		return
	}
	userID := int64(sub)

	// Ensure refresh token matches DB (rotation safety)
	saved, err := app.store.Users.GetRefreshToken(r.Context(), userID)
	if err != nil || saved != c.Value {
		app.unauthorizedErrorResponse(w, r, errors.New("refresh token mismatch"))
		return
	}

	venueIDs, err := app.store.Venues.GetOwnedVenueIDs(r.Context(), userID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	isAdmin, err := app.store.AccessControl.UserHasRole(r.Context(), userID, "admin")
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to check user role: %w", err))
		return
	}

	isMerchant, err := app.store.AccessControl.UserHasRole(r.Context(), userID, "merchant")
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to check user role: %w", err))
		return
	}

	role := "user"
	if isAdmin {
		role = "admin"
	} else if isMerchant {
		role = "merchant"
	} else if len(venueIDs) > 0 {
		role = "venue_owner"
	}

	accessToken, newRefresh, err := app.authenticator.GenerateTokens(userID, role)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	if err := app.store.Users.SaveRefreshToken(r.Context(), userID, newRefresh); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// ✅ update cookies
	app.setAuthCookies(w, accessToken, newRefresh)

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) logoutCookieHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	if user == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("not authorized"))
		return
	}

	if err := app.store.Users.DeleteRefreshToken(r.Context(), user.ID); err != nil {

		app.logger.Warnw("failed to delete refresh token on logout", "user_id", user.ID, "error", err)
	}

	// Always clear cookies
	app.clearAuthCookies(w)

	w.WriteHeader(http.StatusNoContent)
}

type SessionResponse struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"expires_at"` // unix seconds (optional but nice)
}

// sessionHandler godoc
//
//	@Summary		Get current web session (cookie)
//	@Description	Reads access_token cookie, validates it, returns session info.
//	@Tags			authentication
//	@Produce		json
//	@Success		200	{object}	map[string]SessionResponse
//	@Failure		401	{object}	error
//	@Router			/authentication/session [get]
func (app *application) sessionHandler(w http.ResponseWriter, r *http.Request) {
	// 1) read cookie
	c, err := r.Cookie("access_token")
	if err != nil || c.Value == "" {
		app.unauthorizedErrorResponse(w, r, errors.New("not authorized"))
		return
	}

	// 2) validate jwt
	tok, err := app.authenticator.ValidateAccessToken(c.Value)
	if err != nil || tok == nil || !tok.Valid {
		app.unauthorizedErrorResponse(w, r, errors.New("not authorized"))
		return
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		app.unauthorizedErrorResponse(w, r, errors.New("not authorized"))
		return
	}

	// 3) extract sub + role + exp
	// sub is typically float64 in MapClaims
	subFloat, ok := claims["sub"].(float64)
	if !ok {
		app.unauthorizedErrorResponse(w, r, errors.New("not authorized"))
		return
	}
	userID := strconv.FormatInt(int64(subFloat), 10)

	role, _ := claims["role"].(string)

	var expUnix int64
	if exp, ok := claims["exp"].(float64); ok {
		expUnix = int64(exp)
	}

	// 4) optional: ensure role is allowed for panel
	// (already block "user" during cookie login, but this is extra safety)
	if role != "admin" && role != "venue_owner" && role != "merchant" {
		app.unauthorizedErrorResponse(w, r, errors.New("not authorized"))
		return
	}

	// 5) respond
	_ = app.jsonResponse(w, http.StatusOK, SessionResponse{
		UserID:    userID,
		Role:      role,
		ExpiresAt: expUnix,
	})
}
