package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"khel/internal/auth"
	"khel/internal/domain/accesscontrol"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// Extract token from:
// 1) Authorization header (mobile)
// 2) access_token cookie (web)
func (app *application) getAccessTokenFromRequest(r *http.Request) string {
	// Header first (mobile)
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	// Cookie next (web)
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		return c.Value
	}

	return ""
}

func (app *application) AuthTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		token := app.getAccessTokenFromRequest(r)
		if token == "" {
			app.unauthorizedErrorResponse(w, r, errors.New("not authorized"))
			return
		}
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
		if err != nil || user == nil {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("user not found"))
			return
		}

		ctx = context.WithValue(ctx, userCtx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (app *application) optionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		// If no authorization header, just continue without user
		if strings.TrimSpace(authHeader) == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Validate the authorization header format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			// For optional auth, we just continue without user instead of returning error
			next.ServeHTTP(w, r)
			return
		}

		token := parts[1]
		jwtToken, err := app.authenticator.ValidateAccessToken(token)
		if err != nil {
			// Token is invalid, but since this is optional auth, just continue
			next.ServeHTTP(w, r)
			return
		}

		claims, ok := jwtToken.Claims.(jwt.MapClaims)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		userID, err := strconv.ParseInt(fmt.Sprintf("%.f", claims["sub"]), 10, 64)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		user, err := app.store.Users.GetByID(ctx, userID)
		if err != nil {
			// User not found or other error, but continue without user
			next.ServeHTTP(w, r)
			return
		}

		// If we successfully got the user, add to context
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

func (app *application) CheckGameAdmin(next http.Handler) http.Handler {
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
		isOwner, err := app.store.VenuesReviews.IsReviewOwner(r.Context(), reviewID, userID)
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

// Only use this middleware for routes like /logout where token expiry should be ignored

func (app *application) AuthTokenIgnoreExpiryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if strings.TrimSpace(authHeader) == "" {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("authorization header is missing or blank"))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("authorization header is malformed"))
			return
		}

		tokenStr := parts[1]

		// ✅ Get secret by casting the authenticator
		jwtAuth, ok := app.authenticator.(*auth.JWTAuthenticator)
		if !ok {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("invalid authenticator type"))
			return
		}

		// Allow expired tokens to be parsed
		jwtToken, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
			}
			return []byte(jwtAuth.Secret()), nil
		}, jwt.WithLeeway(0), jwt.WithoutClaimsValidation())

		if err != nil && !errors.Is(err, jwt.ErrTokenExpired) {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("invalid token: %w", err))
			return
		}

		claims, ok := jwtToken.Claims.(jwt.MapClaims)
		if !ok {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("invalid claims"))
			return
		}

		userIDFloat, ok := claims["sub"].(float64)
		if !ok {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("missing or invalid sub in token"))
			return
		}
		userID := int64(userIDFloat)

		user, err := app.store.Users.GetByID(r.Context(), userID)
		if err != nil {
			app.unauthorizedErrorResponse(w, r, fmt.Errorf("user not found: %w", err))
			return
		}

		ctx := context.WithValue(r.Context(), userCtx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRoleMiddleware checks if the authenticated user has a specific role
func (app *application) RequireRoleMiddleware(role accesscontrol.RoleName) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := getUserFromContext(r)
			if user == nil {
				app.unauthorizedErrorResponse(w, r, fmt.Errorf("user not authenticated"))
				return
			}

			hasRole, err := app.store.AccessControl.UserHasRole(r.Context(), user.ID, string(role))
			if err != nil {
				app.internalServerError(w, r, fmt.Errorf("failed to check user role: %w", err))
				return
			}

			if !hasRole {
				app.forbiddenResponse(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (app *application) StrictLimiterMiddleware(limiter interface {
	Allow(ip string) (bool, time.Duration)
}) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if allow, retryAfter := limiter.Allow(ip); !allow {
				app.rateLimitExceededResponse(w, r, retryAfter.String())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	// If you are behind Cloudflare/Traefik, these headers help.
	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
		return ip
	}
	// X-Forwarded-For may contain multiple, first is client
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Traefik / some proxies
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}

	// chi middleware.RealIP sets RemoteAddr, but it may still include port
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
