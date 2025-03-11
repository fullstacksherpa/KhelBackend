package main

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"khel/docs" //this is required to generate swagger docs
	"khel/internal/auth"
	"khel/internal/mailer"
	"khel/internal/store"

	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/zap"
)

type application struct {
	config        config
	store         store.Storage
	logger        *zap.SugaredLogger
	cld           *cloudinary.Cloudinary
	mailer        mailer.Client
	authenticator auth.Authenticator
}

type config struct {
	addr        string
	db          dbConfig
	env         string
	apiURL      string
	mail        mailConfig
	frontendURL string
	auth        authConfig
}

type authConfig struct {
	basic basicConfig
	token tokenConfig
}
type tokenConfig struct {
	refreshSecret   string
	secret          string
	accessTokenExp  time.Duration
	refreshTokenExp time.Duration
	iss             string
}
type basicConfig struct {
	user string
	pass string
}

type mailConfig struct {
	exp       time.Duration
	fromEmail string
	mailtrap  mailTrapConfig
}

type mailTrapConfig struct {
	apiKey string
}

type dbConfig struct {
	addr         string
	maxOpenConns int
	maxIdleConns int
	maxIdleTime  string
}

func (app *application) mount() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		// AllowedOrigins:   []string{"https://foo.com"}, // Use this to allow specific origin hosts
		AllowedOrigins: []string{"https://*", "http://*"},
		// AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))

	//Set a timeout value on the request context (ctx), that will signal through ctx.Done() that the request has timed out and further processing should be stopped
	r.Use(middleware.Timeout(60 * time.Second))

	r.Route("/v1", func(r chi.Router) {
		r.With(app.BasicAuthMiddleware()).Get("/health", app.healthCheckHandler)
		docsURL := fmt.Sprintf("%s/swagger/doc.json", app.config.addr)
		r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL(docsURL)))

		r.With(app.BasicAuthMiddleware()).Get("/debug/vars", expvar.Handler().ServeHTTP)

		r.Route("/venues", func(r chi.Router) {
			r.Post("/", app.createVenueHandler)
			//Call DELETE /venues/{venueID}/photos?photo_url={url}.
			r.Delete("/{venueID}/photos", app.deleteVenuePhotoHandler) // DELETE /venues/{venueID}/photos?photo_url={url}
			r.Post("/{venueID}/photos", app.uploadVenuePhotoHandler)   // POST /venues/{venueID}/photos
			// PATCH /venues/{venueID} - Update venue information
			r.Patch("/{venueID}", app.updateVenueInfo)

			r.Post("/{venueID}/reviews", app.createVenueReviewHandler)
			r.Get("/{venueID}/reviews", app.getVenueReviewsHandler)
			r.Delete("/{venueID}/reviews/{reviewID}", app.deleteVenueReviewHandler)
		})
		// Route that does NOT require authentication
		r.Put("/users/activate/{token}", app.activateUserHandler)

		r.Route("/users", func(r chi.Router) {
			r.Use(app.AuthTokenMiddleware)
			r.Put("/", app.updateUserHandler)
			r.Post("/profile-picture", app.uploadProfilePictureHandler)
			r.Put("/profile-picture", app.updateProfilePictureHandler)
			r.Post("/logout", app.logoutHandler)

			r.Route("/{userID}", func(r chi.Router) {
				r.Use(app.AuthTokenMiddleware)
				r.Put("/follow", app.followUserHandler)
				r.Put("/unfollow", app.unfollowUserHandler)
			})
		})
		r.Route("/games", func(r chi.Router) {
			r.Post("/create", app.createGameHandler)
			r.Route("/{gameID}", func(r chi.Router) {
				r.Get("/players", app.getGamePlayersHandler)
				r.Post("/request", app.CreateJoinRequest)
				r.Post("/accept", app.AcceptJoinRequest)
				r.Post("/reject", app.RejectJoinRequest)

			})
		})

		// Public routes
		r.Route("/authentication", func(r chi.Router) {
			r.Post("/user", app.registerUserHandler)
			r.Post("/token", app.createTokenHandler)
			r.Post("/refresh", app.refreshTokenHandler)

		})

	})
	return r
}

func (app *application) run(mux http.Handler) error {
	// Docs
	docs.SwaggerInfo.Version = version
	docs.SwaggerInfo.Host = app.config.apiURL
	docs.SwaggerInfo.BasePath = "/v1"

	srv := &http.Server{
		Addr:         app.config.addr,
		Handler:      mux,
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Minute,
	}

	// Implementing graceful shutdown
	shutdown := make(chan error)

	go func() {
		quit := make(chan os.Signal, 1)

		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		s := <-quit

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		app.logger.Infow("signal caught", "signal", s.String())

		shutdown <- srv.Shutdown(ctx)
	}()

	app.logger.Infow("server has started", "addr", app.config.addr, "env", app.config.env)

	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	err = <-shutdown
	if err != nil {
		return err
	}

	app.logger.Infow("server has stopped", "addr", app.config.addr, "env", app.config.env)

	return nil
}
