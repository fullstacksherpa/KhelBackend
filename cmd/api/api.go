package main

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"khel/docs" //this is required to generate swagger docs
	"khel/internal/auth"
	"khel/internal/mailer"
	"khel/internal/ratelimiter"
	"khel/internal/store"
	"log"

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
	rateLimiter   ratelimiter.Limiter
}

type config struct {
	addr        string
	db          dbConfig
	env         string
	apiURL      string
	mail        mailConfig
	frontendURL string
	auth        authConfig
	rateLimiter ratelimiter.Config
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

	//TODO:remove later
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Incoming request: %s %s\n", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	})

	r.Use(middleware.RequestID)
	r.Use(middleware.StripSlashes)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(app.RateLimiterMiddleware)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))

	//Set a timeout value on the request context (ctx), that will signal through ctx.Done() that the request has timed out and further processing should be stopped
	r.Use(middleware.Timeout(60 * time.Second))

	r.Route("/v1", func(r chi.Router) {
		r.Get("/venue/{id}", app.getVenueDetailHandler)
		r.Get("/list-venues", app.listVenuesHandler)
		r.Get("/get-games", app.getGamesHandler)
		r.Get("/health", app.healthCheckHandler)
		docsURL := fmt.Sprintf("%s/swagger/doc.json", app.config.addr)
		r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL(docsURL)))

		r.With(app.BasicAuthMiddleware()).Get("/debug/vars", expvar.Handler().ServeHTTP)

		r.Route("/app-reviews", func(r chi.Router) {
			r.Use(app.AuthTokenMiddleware)
			r.Post("/", app.submitReviewHandler)
			r.Get("/", app.getAllAppReviewsHandler)
		})

		r.Route("/venues", func(r chi.Router) {

			r.Use(app.AuthTokenMiddleware)
			r.Get("/favorites", app.listFavoritesHandler)
			// Expects URL: /venues/{venueID}/available-times?date=YYYY-MM-DD
			r.Get("/{venueID}/available-times", app.availableTimesHandler)
			r.Get("/is-venue-owner", app.isVenueOwnerHandler)
			r.Post("/", app.createVenueHandler)
			r.Post("/{venueID}/reviews", app.createVenueReviewHandler)
			r.Post("/{venueID}/bookings", app.bookVenueHandler)
			r.Get("/{venueID}/reviews", app.getVenueReviewsHandler)
			r.Post("/{venueID}/favorite", app.addFavoriteHandler)      // Add favorite
			r.Delete("/{venueID}/favorite", app.removeFavoriteHandler) // Remove favorite

			// Routes that require venue ownership
			r.Route("/{venueID}", func(r chi.Router) {
				r.Use(app.IsOwnerMiddleware)
				r.Put("/pricing/{pricingID}", app.updateVenuePricingHandler)
				r.Patch("/", app.updateVenueInfo)
				r.Delete("/photos", app.deleteVenuePhotoHandler)
				r.Post("/photos", app.uploadVenuePhotoHandler)
			})

			r.With(app.IsReviewOwnerMiddleware).Delete("/{venueID}/reviews/{reviewID}", app.deleteVenueReviewHandler)
		})
		// Route that does NOT require authentication
		r.Put("/users/activate/{token}", app.activateUserHandler)
		r.With(app.AuthTokenIgnoreExpiryMiddleware).Post("/users/logout", app.logoutHandler)
		r.Route("/users", func(r chi.Router) {

			r.Use(app.AuthTokenMiddleware)
			r.Put("/", app.updateUserHandler)
			r.Post("/profile-picture", app.uploadProfilePictureHandler)
			r.Put("/profile-picture", app.updateProfilePictureHandler)

			r.Route("/{userID}", func(r chi.Router) {
				r.Use(app.AuthTokenMiddleware)
				r.Put("/follow", app.followUserHandler)
				r.Put("/unfollow", app.unfollowUserHandler)
			})
		})

		r.Route("/games", func(r chi.Router) {
			r.Use(app.AuthTokenMiddleware)
			r.Get("/shortlist", app.listShortlistedGamesHandler)
			r.Post("/create", app.createGameHandler)
			r.Route("/{gameID}", func(r chi.Router) {
				r.Get("/qa", app.getGameQAHandler)
				r.Get("/", app.getGameDetailsHandler)
				r.Post("/shortlist", app.addShortlistedGameHandler)      // Add game to shortlist
				r.Delete("/shortlist", app.removeShortlistedGameHandler) // Remove game from shortlist
				r.With(app.CheckAdmin).Post("/assign-assistant/{playerID}", app.AssignAssistantHandler)
				r.Get("/players", app.getGamePlayersHandler)
				r.Post("/request", app.CreateJoinRequest)
				r.With(app.RequireGameAdminAssistant).Post("/accept", app.AcceptJoinRequest)
				r.With(app.RequireGameAdminAssistant).Get("/requests", app.getAllGameJoinRequestsHandler)
				r.With(app.RequireGameAdminAssistant).Post("/reject", app.RejectJoinRequest)
				r.With(app.RequireGameAdminAssistant).Patch("/toggle-match-full", app.toggleMatchFullHandler)
				r.With(app.RequireGameAdminAssistant).Patch("/cancel-game", app.cancelGameHandler)

				r.Route("/questions", func(r chi.Router) {
					r.Post("/", app.createQuestionHandler)
					r.Get("/", app.getGameQuestionsHandler)

					r.Route("/{questionID}", func(r chi.Router) {
						r.Delete("/", app.deleteQuestionHandler)
						r.With(app.RequireGameAdminAssistant).Post("/replies", app.createReplyHandler)
					})
				})

			})
		})

		//secure routes
		//TODO: Add with(authtokenmiddleware)
		r.Post("/authentication/refresh", app.refreshTokenHandler)
		r.Post("/authentication/reset-password", app.requestResetPasswordHandler)
		r.Patch("/authentication/reset-password", app.resetPasswordHandler)

		// Public routes
		r.Route("/authentication", func(r chi.Router) {
			r.Post("/user", app.registerUserHandler)
			r.Post("/token", app.createTokenHandler)

		})

	})
	return r
}

func (app *application) run(mux http.Handler) error {
	// Docs
	docs.SwaggerInfo.Version = version
	docs.SwaggerInfo.Host = app.config.apiURL
	docs.SwaggerInfo.BasePath = "/v1"

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Fallback to 8080 if PORT is not set
	}

	srv := &http.Server{
		Addr:         "0.0.0.0:" + port,
		Handler:      mux,
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Minute,
	}

	// Implementing graceful shutdown
	shutdown := make(chan error, 1)

	go func() {
		quit := make(chan os.Signal, 1)

		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		s := <-quit

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		app.logger.Infow("signal caught", "signal", s.String())

		if err := srv.Shutdown(ctx); err != nil {
			shutdown <- err
		}

		close(shutdown)

	}()

	app.logger.Infow("server has started", "addr", app.config.addr, "env", app.config.env)

	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		app.logger.Errorw("server error", "error", err)
		return err
	}
	// Wait for shutdown
	<-shutdown
	app.logger.Infow("server has stopped", "addr", app.config.addr, "env", app.config.env)

	return nil
}
