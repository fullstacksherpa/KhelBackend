package main

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"khel/docs" //this is required to generate swagger docs
	"khel/internal/auth"
	"khel/internal/domain/accesscontrol"
	"khel/internal/domain/storage"
	"khel/internal/mailer"
	"khel/internal/notifications"
	"khel/internal/payments"
	"khel/internal/ratelimiter"

	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/speps/go-hashids/v2"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/zap"
)

type application struct {
	config        config
	store         *storage.Container
	logger        *zap.SugaredLogger
	cld           *cloudinary.Cloudinary
	mailer        mailer.Client
	authenticator auth.Authenticator
	rateLimiter   ratelimiter.Limiter

	// strict limiter for public venue request endpoint
	venueRequestLimiter ratelimiter.Limiter
	push                *notifications.ExpoAdapter
	hashID              *hashids.HashID
	payments            *payments.PaymentManager
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
	payment     paymentConfig

	turnstile turnstileConfig
}

type turnstileConfig struct {
	secretKey        string
	expectedHostname string
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
	maxIdleTime  string
}

type paymentConfig struct {
	Esewa  esewaConfig
	Khalti khaltiConfig
}

type esewaConfig struct {
	MerchantID string
	SecretKey  string
	SuccessURL string
	FailureURL string
}

type khaltiConfig struct {
	SecretKey  string
	ReturnURL  string
	WebsiteURL string
}

func (app *application) mount() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.StripSlashes)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(app.RateLimiterMiddleware)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://web.gocloudnepal.com", "http://localhost:3000", "https://admin.gocloudnepal.com"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))

	//Set a timeout value on the request context (ctx), that will signal through ctx.Done() that the request has timed out and further processing should be stopped
	r.Use(middleware.Timeout(60 * time.Second))

	r.Route("/v1", func(r chi.Router) {
		r.Get("/venue/{id}", app.getVenueDetailHandler)
		r.Get("/venues/search", app.searchVenuesHandler)
		r.Get("/venues/search/fts", app.fullTextSearchVenuesHandler)
		r.Get("/health", app.healthCheckHandler)
		docsURL := fmt.Sprintf("%s/v1/swagger/doc.json", app.config.addr)
		r.With(app.BasicAuthMiddleware()).Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL(docsURL)))

		r.With(app.BasicAuthMiddleware()).Get("/debug/vars", expvar.Handler().ServeHTTP)

		r.Route("/venue-requests", func(r chi.Router) {
			// strict limiter ONLY for this endpoint
			r.With(app.StrictLimiterMiddleware(app.venueRequestLimiter)).Post("/", app.createVenueRequestHandler)
		})

		r.Route("/app-reviews", func(r chi.Router) {
			r.Use(app.AuthTokenMiddleware)
			r.Post("/", app.submitReviewHandler)
		})

		// Public ads routes
		r.Route("/ads", func(r chi.Router) {
			r.Get("/active", app.getActiveAdsHandler)
			r.Post("/{adID}/impression", app.trackImpressionHandler)
			r.Post("/{adID}/click", app.trackClickHandler)
		})

		// Admin: => Merchant:  ads routes
		r.Route("/admin/ads", func(r chi.Router) {
			r.Use(app.AuthTokenMiddleware)
			r.Use(app.RequireRoleMiddleware(accesscontrol.RoleMerchant))

			r.Get("/", app.getAllAdsHandler)
			r.Post("/", app.createAdHandler)
			r.Get("/{adID}", app.getAdByIDHandler)
			r.Put("/{adID}", app.updateAdHandler)
			r.Delete("/{adID}", app.deleteAdHandler)
			r.Post("/{adID}/toggle", app.toggleAdStatusHandler)
			r.Get("/analytics", app.getAdsAnalyticsHandler)
			r.Post("/bulk-update-order", app.bulkUpdateDisplayOrderHandler)
		})

		r.With(app.optionalAuth).Get("/venues/list-venues", app.listVenuesHandler)

		r.With(app.optionalAuth).Get("/venues/{venueID}/reviews", app.getVenueReviewsHandler)
		r.Route("/venues", func(r chi.Router) {

			r.Use(app.AuthTokenMiddleware)

			r.Get("/favorites", app.listFavoritesHandler)
			r.Get("/{venueID}/available-times", app.availableTimesHandler)
			r.Get("/is-venue-owner", app.isVenueOwnerHandler)
			r.Post("/", app.createVenueHandler)
			r.Post("/{venueID}/reviews", app.createVenueReviewHandler)
			r.Post("/{venueID}/cancel-bookings/{bookingID}", app.cancelBookingHandler)
			r.Post("/{venueID}/bookings", app.bookVenueHandler)

			r.Post("/{venueID}/favorite", app.addFavoriteHandler)      // Add favorite
			r.Delete("/{venueID}/favorite", app.removeFavoriteHandler) // Remove favorite

			// Routes that require venue ownership
			r.Route("/{venueID}", func(r chi.Router) {
				r.Use(app.IsOwnerMiddleware)
				r.Patch("/status", app.updateVenueStatusOwnerHandler)
				r.Post("/bookings/manual", app.createManualBookingHandler)
				r.Get("/pricing", app.getVenuePricing)
				r.Delete("/", app.deleteVenueHandler)
				r.Get("/pending-bookings", app.getPendingBookingsHandler)
				r.Get("/canceled-bookings", app.getCanceledBookingsHandler)
				r.Get("/venue-info", app.getVenueInfoHandler)
				r.Get("/scheduled-bookings", app.getScheduledBookingsHandler)
				r.Post("/pending-bookings/{bookingID}/accept", app.acceptBookingHandler)
				r.Post("/pending-bookings/{bookingID}/reject", app.rejectBookingHandler)
				r.Post("/pricing", app.createVenuePricingHandler)
				r.Put("/pricing/{pricingID}", app.updateVenuePricingHandler)
				r.Delete("/pricing/{pricingID}", app.deleteVenuePricingHandler)
				r.Patch("/", app.updateVenueInfo)
				r.Get("/photos", app.getVenueAllPhotosHandler)
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
			r.Post("/push-tokens", app.savePushTokenHandler)
			r.Post("/push-tokens/prune", app.pruneStaleTokensHandler)
			r.Post("/push-tokens/bulk-remove", app.bulkRemoveTokensHandler)
			r.Delete("/push-tokens", app.removePushTokenHandler)
			r.Get("/bookings", app.getBookingsByUserHandler)
			r.Get("/me", app.getCurrentUserHandler)
			r.Delete("/me", app.deleteUserAccountHandler)
			r.Patch("/update-profile", app.editProfileHandler)
			r.Put("/", app.updateUserHandler)
			r.Post("/profile-picture", app.uploadProfilePictureHandler)
			r.Put("/profile-picture", app.updateProfilePictureHandler)

			r.Route("/{userID}", func(r chi.Router) {
				r.Use(app.AuthTokenMiddleware)
				r.Put("/follow", app.followUserHandler)
				r.Put("/unfollow", app.unfollowUserHandler)
			})
		})

		r.With(app.optionalAuth).Get("/games/get-games", app.getGamesHandler)
		r.With(app.optionalAuth).Get("/games/{gameID}", app.getGameDetailsHandler)

		r.With(app.optionalAuth).Get("/games/{venueID}/upcoming", app.getUpcomingGamesByVenueHandler)

		r.With(app.AuthTokenMiddleware).Get("/games/get-upcoming", app.getUpcomingGamesForUser)
		r.With(app.AuthTokenMiddleware).Get("/games/shortlist", app.listShortlistedGamesHandler)
		r.With(app.AuthTokenMiddleware).Post("/games/create", app.createGameHandler)

		r.Route("/games", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(app.optionalAuth)

				r.Get("/{gameID}/qa", app.getGameQAHandler)

			})

			r.Route("/{gameID}", func(r chi.Router) {
				r.Use(app.AuthTokenMiddleware)
				r.Post("/shortlist", app.addShortlistedGameHandler)      // Add game to shortlist
				r.Delete("/shortlist", app.removeShortlistedGameHandler) // Remove game from shortlist
				r.With(app.CheckGameAdmin).Post("/assign-assistant/{playerID}", app.AssignAssistantHandler)
				r.Get("/players", app.getGamePlayersHandler)
				r.Post("/request", app.CreateJoinRequest)
				r.Delete("/request", app.DeleteJoinRequest)
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
		// for mobile
		r.Post("/authentication/refresh", app.refreshTokenHandler)
		//for web
		r.Post("/authentication/refresh/cookie", app.refreshTokenCookieHandler)
		r.Post("/authentication/logout/cookie", app.logoutCookieHandler)
		r.Post("/authentication/reset-password", app.requestResetPasswordHandler)
		r.Patch("/authentication/reset-password", app.resetPasswordHandler)

		r.Route("/authentication", func(r chi.Router) {
			r.Post("/user", app.registerUserHandler)
			// Mobile:
			r.Post("/token", app.createTokenHandler)

			// Web:
			r.Post("/token/cookie", app.createTokenCookieHandler)
			r.Get("/session", app.sessionHandler)

		})

		// ---------- Merchant E-COMMERCE ROUTES (written admin on path since api is already integrated on web. might change later) admin  = "Merchant" ----------

		r.Route("/store/admin", func(r chi.Router) {
			r.Use(app.AuthTokenMiddleware)
			r.Use(app.RequireRoleMiddleware(accesscontrol.RoleMerchant))
			r.Get("/payments", app.adminListPaymentsHandler)
			r.Post("/brands", app.createBrandHandler)
			r.Patch("/brands/{brandID}", app.updateBrandHandler)
			r.Delete("/brands/{brandID}", app.deleteBrandHandler)
			r.Post("/categories", app.createCategoryHandler)
			r.Patch("/categories/{categoryID}", app.updateCategoryHandler)
			r.Delete("/categories/{categoryID}", app.deleteCategoryHandler)

			r.Get("/products", app.adminListProductsHandler)
			r.Get("/category/products", app.listProductsHandler)
			r.Post("/products", app.createProductHandler)
			r.Patch("/products/{productID}", app.updateProductHandler)
			r.Post("/products/{productID}/publish", app.publishProductHandler)
			r.Post("/products/images", app.createProductImageHandler)
			r.Get("/products/{productID}/images", app.listProductImagesHandler)
			r.Post("/products/{productID}/images/{imageID}/primary", app.setPrimaryImageHandler)
			r.Patch("/products/images/{id}", app.updateProductImageHandler)
			r.Delete("/products/images/{id}", app.deleteProductImageHandler)
			r.Post("/products/{productID}/images/reorder", app.reorderProductImagesHandler)
			r.Post("/products/variants", app.createVariantHandler)
			r.Patch("/products/variants/{id}", app.updateVariantHandler)
			r.Delete("/products/variants/{id}", app.deleteVariantHandler)
			r.Get("/products/variants", app.listAllVariantsHandler)

			r.Get("/carts", app.adminListCartsHandler)
			r.Get("/carts/{cartID}", app.adminGetCartHandler)
			r.Post("/carts/mark-abandoned", app.adminMarkExpiredCartsHandler)

			// orders admin
			r.Get("/orders", app.adminListOrdersHandler)
			r.Get("/orders/{orderID}", app.adminGetOrderHandler)
			r.Patch("/orders/{orderID}/status", app.adminUpdateOrderStatusHandler)

		})

		// ---------- PUBLIC E-COMMERCE ROUTES ----------
		r.Route("/store", func(r chi.Router) {
			r.Get("/payments/esewa/start", app.esewaStartHandler)
			r.Get("/payments/khalti", app.khaltiReturnHandler)
			r.Get("/payments/esewa/return", app.esewaReturnHandler)
			// ---------- PUBLIC CATALOG ----------
			r.Get("/brands", app.getAllBrandsHandler)
			r.Get("/categories", app.listCategoriesHandler)
			r.Get("/categories/{categoryID}", app.getCategoryByIDHandler)
			r.Get("/categories/tree", app.getCategoryTreeHandler)
			r.Get("/categories/search", app.searchCategoriesHandler)
			r.Get("/categories/search/fts", app.fullTextSearchCategoriesHandler)
			r.Get("/products", app.listProductsHandler)
			r.Get("/products/{productID}", app.getProductByIDHandler)
			r.Get("/products/slug/{slug}", app.getProductDetailHandler)
			r.Get("/{product_id}/variants", app.listVariantsByProductHandler)

			r.Get("/products/search", app.searchProductsHandler)
			r.Get("/products/search/fts", app.fullTextSearchProductsHandler)

			r.Get("/variants/{id}", app.getVariantHandler)

			r.Post("/payments/webhook", app.paymentWebhookHandler)

			// ---------- AUTH-REQUIRED STORE FLOW ----------
			r.Group(func(r chi.Router) {
				r.Use(app.AuthTokenMiddleware)

				r.Get("/cart", app.getCartHandler)
				r.Post("/cart/items", app.addCartItemHandler)
				r.Patch("/cart/items/{itemID}", app.updateCartItemQtyHandler)
				r.Delete("/cart/items/{itemID}", app.removeCartItemHandler)
				r.Delete("/cart", app.clearCartHandler)

				r.Get("/orders", app.listMyOrdersHandler)
				r.Get("/orders/{orderID}", app.getMyOrderHandler)

				r.Post("/checkout", app.checkoutHandler)
				r.Post("/payments/verify", app.verifyPaymentHandler)
			})
		})

		// ---------- SUPERADMIN ROUTES ----------

		r.Route("/superadmin", func(r chi.Router) {
			r.Use(app.AuthTokenMiddleware)
			r.Use(app.RequireRoleMiddleware(accesscontrol.RoleAdmin))
			r.Get("/users", app.adminListUsersHandler)
			r.Get("/users/{userID}", app.AdminUserOverviewHandler)
			r.Get("/{userID}/roles", app.adminGetUserRolesHandler)
			r.Post("/{userID}/roles", app.adminAssignUserRoleHandler)
			r.Delete("/{userID}/roles/{roleID}", app.adminRemoveUserRoleHandler)
			r.Post("/users", app.adminCreateUserHandler)

			r.Get("/venue-requests", app.adminListVenueRequestsHandler)
			r.Post("/venue-requests/{id}/approve", app.adminApproveVenueRequestHandler)
			r.Post("/venue-requests/{id}/reject", app.adminRejectVenueRequestHandler)

			r.Get("/overview", app.adminOverviewHandler)

			r.Get("/app-reviews", app.getAllAppReviewsHandler)
			r.Get("/venues", app.AdminlistVenuesHandler)

		})

	})
	return r
}

func (app *application) run(mux http.Handler, cancel context.CancelFunc) error {
	// Docs
	docs.SwaggerInfo.Version = version
	docs.SwaggerInfo.Host = app.config.apiURL
	docs.SwaggerInfo.BasePath = "/v1"

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
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
		app.logger.Infow("signal caught", "signal", s.String())

		cancel()

		ctx, cancelTimeout := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelTimeout()

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
