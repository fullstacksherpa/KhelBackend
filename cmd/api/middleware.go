package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

func (app *application) BasicAuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// read the auth header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				app.unauthorizedBasicErrorResponse(w, r, fmt.Errorf("authorization header is missing"))
				return
			}

			// parse it -> get the base64
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Basic" {
				app.unauthorizedBasicErrorResponse(w, r, fmt.Errorf("authorization header is malformed"))
				return
			}

			// decode it
			decoded, err := base64.StdEncoding.DecodeString(parts[1])
			if err != nil {
				app.unauthorizedBasicErrorResponse(w, r, err)
				return
			}

			// check the credentials
			username := app.config.auth.basic.user
			pass := app.config.auth.basic.pass

			creds := strings.SplitN(string(decoded), ":", 2)
			if len(creds) != 2 || creds[0] != username || creds[1] != pass {
				app.unauthorizedBasicErrorResponse(w, r, fmt.Errorf("invalid credentials"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (app *application) AuthTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("authorization header is missing"))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("authorization header is malformed"))
			return
		}

		token := parts[1]
		jwtToken, err := app.authenticator.ValidateAccessToken(token)
		if err != nil {
			app.unauthorizedErrorResponse(w, r, err)
			return
		}

		claims, _ := jwtToken.Claims.(jwt.MapClaims)

		userID, err := strconv.ParseInt(fmt.Sprintf("%.f", claims["sub"]), 10, 64)
		if err != nil {
			app.unauthorizedErrorResponse(w, r, err)
			return
		}

		ctx := r.Context()

		user, err := app.store.Users.GetByID(ctx, userID)
		if err != nil {
			app.unauthorizedErrorResponse(w, r, err)
			return
		}

		ctx = context.WithValue(ctx, userCtx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (app *application) RequireGameAdminAssistant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := getUserFromContext(r) // Fetch user from context

		// Extract gameID from URL
		gameIDStr := chi.URLParam(r, "gameID")
		gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid game ID")
			return
		}
		// TODO:remove later
		log.Println("Checking gameID ⚽:", gameID)
		log.Println("Checking userID 👷🏽‍♂️:", user.ID)

		// Check if user is admin or assistant
		isAdminAssistant, err := app.store.Games.IsAdminAssistant(r.Context(), gameID, user.ID)
		if err != nil {
			app.logger.Errorf("Error checking admin status: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if !isAdminAssistant {
			writeJSONError(w, http.StatusForbidden, "Insufficient privileges")
			return
		}

		// Proceed if user is an admin/assistant
		next.ServeHTTP(w, r)
	})
}

func (app *application) CheckAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := getUserFromContext(r)

		// Extract gameID from URL
		gameIDStr := chi.URLParam(r, "gameID")
		gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid game ID")
			return
		}

		// Check if user is admin
		isAdmin, err := app.store.Games.IsAdmin(r.Context(), gameID, user.ID)
		if err != nil {
			app.logger.Errorf("Error checking admin status: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Database error")
			return
		}
		if !isAdmin {
			writeJSONError(w, http.StatusForbidden, "Insufficient privileges")
			return
		}

		// Proceed if user is an admin/assistant
		next.ServeHTTP(w, r)
	})
}

// IsOwnerMiddleware checks if the user is the owner of the venue
func (app *application) IsOwnerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		venueIDStr := chi.URLParam(r, "venueID")
		venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %v", err))
			return
		}
		if venueID == 0 {
			app.badRequestResponse(w, r, errors.New("venue ID is required"))
			return
		}
		user := getUserFromContext(r)
		userID := user.ID

		// Check if the user is the owner of the venue
		isOwner, err := app.store.Venues.IsOwner(r.Context(), venueID, userID)
		if err != nil || !isOwner {
			app.forbiddenResponse(w, r)
			return
		}

		// Continue to the next middleware or handler
		next.ServeHTTP(w, r)
	})
}

func (app *application) IsReviewOwnerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the reviewID from the URL parameter
		reviewIDStr := chi.URLParam(r, "reviewID")
		reviewID, err := strconv.ParseInt(reviewIDStr, 10, 64)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("invalid review ID: %v", err))
			return
		}

		user := getUserFromContext(r)
		userID := user.ID

		// Check if the user is the owner of the review
		isOwner, err := app.store.Reviews.IsReviewOwner(r.Context(), reviewID, userID)
		if err != nil {
			app.internalServerError(w, r, fmt.Errorf("error checking review ownership: %v", err))
			return
		}

		if !isOwner {
			app.forbiddenResponse(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) RateLimiterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if app.config.rateLimiter.Enabled {
			if allow, retryAfter := app.rateLimiter.Allow(r.RemoteAddr); !allow {
				app.rateLimitExceededResponse(w, r, retryAfter.String())
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
