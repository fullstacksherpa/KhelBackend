package main

import (
	"context"
	"expvar"
	"fmt"
	"khel/internal/auth"
	"khel/internal/db"
	"khel/internal/domain/orders"
	"khel/internal/domain/storage"
	"khel/internal/mailer"
	"khel/internal/notifications"
	"khel/internal/payments"
	"khel/internal/ratelimiter"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/9ssi7/exponent"
	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/speps/go-hashids/v2"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LoadRateLimiterConfig retrieves rate limiter settings from environment variables
func LoadRateLimiterConfig() ratelimiter.Config {
	// Default values
	defaultRequests := 300
	defaultEnabled := false

	// Retrieve request count with error handling
	requestsPerTimeFrame := defaultRequests
	if val, exists := os.LookupEnv("RATELIMITER_REQUESTS_COUNT"); exists {
		if parsedVal, err := strconv.Atoi(val); err == nil {
			requestsPerTimeFrame = parsedVal
		} else {
			fmt.Println("Invalid RATELIMITER_REQUESTS_COUNT, defaulting to", defaultRequests)
		}
	}

	// Retrieve enabled flag with error handling
	enabled := defaultEnabled
	if val, exists := os.LookupEnv("RATE_LIMITER_ENABLED"); exists {
		if parsedVal, err := strconv.ParseBool(val); err == nil {
			enabled = parsedVal
		} else {
			fmt.Println("Invalid RATE_LIMITER_ENABLED, defaulting to", defaultEnabled)
		}
	}

	return ratelimiter.Config{
		RequestsPerTimeFrame: requestsPerTimeFrame,
		TimeFrame:            1 * time.Minute,
		Enabled:              enabled,
	}
}

// NewLogger creates a new zap logger with color.
func NewLogger() (*zap.SugaredLogger, error) {
	// Configure the encoder to be a console encoder with color
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder // This adds color to log levels (INFO, WARN, ERROR)

	// Create a console encoder with the custom configuration
	consoleEncoder := zapcore.NewConsoleEncoder(encoderCfg)

	// Create a log level (you can set your own level here)
	level := zapcore.InfoLevel

	// Use zapcore.NewCore to write logs to standard output (stdout) with color
	core := zapcore.NewCore(consoleEncoder, zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)), level)

	// Create and return a new logger instance
	logger := zap.New(core)

	return logger.Sugar(), nil
}

var version = "2.0.0"

//	@title			Khel API
//	@description	API for Khel, a complete sport application.

//	@contact.name	fullstacksherpa
//	@contact.url	https://khel.gocloudnepal.com/
//	@contact.email	Ongchen10sherpa@gmail.com

//	@license.name	Apache 2.0
//	@license.url	http://www.apache.org/licenses/LICENSE-2.0.html

// @BasePath					/v1
// @securityDefinitions.apikey	ApiKeyAuth
// @in							header
// @name						Authorization
// @description
func main() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	if os.Getenv("DOCKER") == "" && env == "development" {
		_ = godotenv.Load(".env.development")
	}

	hashSalt := os.Getenv("HASHIDS_SALT")
	hd := hashids.NewData()
	hd.Salt = hashSalt
	hd.MinLength = 8
	h, err := hashids.NewWithData(hd)
	if err != nil {
		log.Fatal("Failed to initialize HashID:", err)
	}

	// Retrieve and convert maxOpenConns
	maxOpenConns := 10
	if v := os.Getenv("DB_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			maxOpenConns = n
		} else {
			log.Fatalf("Invalid DB_MAX_OPEN_CONNS: %v", err)
		}
	}

	cfg := config{
		addr:        os.Getenv("ADDR"),
		env:         os.Getenv("ENV"),
		frontendURL: os.Getenv("FRONTEND_URL"),
		apiURL:      os.Getenv("EXTERNAL_URL"),
		db: dbConfig{
			addr:         os.Getenv("DB_ADDR"),
			maxOpenConns: maxOpenConns,
			maxIdleTime:  os.Getenv("DB_MAX_IDLE_TIME"),
		},
		mail: mailConfig{
			exp:       time.Hour * 24 * 1, //1 days
			fromEmail: os.Getenv("SENDGRID_FROM_EMAIL"),
			mailtrap: mailTrapConfig{
				apiKey: os.Getenv("MAILTRAP_API_KEY"),
			},
		},
		auth: authConfig{
			basic: basicConfig{
				user: os.Getenv("AUTH_BASIC_USER"),
				pass: os.Getenv("AUTH_BASIC_PASS"),
			},
			token: tokenConfig{
				refreshSecret:   os.Getenv("AUTH_TOKEN_REFRESH_SECRET"),
				secret:          os.Getenv("AUTH_TOKEN_SECRET"),
				accessTokenExp:  time.Hour * 24 * 1, // 1 days
				refreshTokenExp: time.Hour * 24 * 2, // 2 days
				iss:             "Khel",
			},
		},
		rateLimiter: LoadRateLimiterConfig(),
		payment: paymentConfig{
			Esewa: esewaConfig{
				MerchantID: os.Getenv("ESEWA_MERCHANT_ID"),
				SecretKey:  os.Getenv("ESEWA_SECRET_KEY"),
				SuccessURL: os.Getenv("ESEWA_SUCCESS_URL"),
				FailureURL: os.Getenv("ESEWA_FAILURE_URL"),
			},
			Khalti: khaltiConfig{
				SecretKey:  os.Getenv("KHALTI_SECRET_KEY"),
				ReturnURL:  os.Getenv("KHALTI_RETURN_URL"),
				WebsiteURL: os.Getenv("KHALTI_WEBSITE_URL"),
			},
		},
		turnstile: turnstileConfig{
			secretKey:        os.Getenv("TURNSTILE_SECRET_KEY"),
			expectedHostname: os.Getenv("TURNSTILE_EXPECTED_HOSTNAME"),
		},
	}

	// Logger
	// Create the logger
	logger, err := NewLogger()
	if err != nil {
		fmt.Println("Error creating logger:", err)
		return
	}
	defer logger.Sync()

	//Unique Order Number Generator with userid embedded
	orderSecret := os.Getenv("ORDER_NUMBER_SECRET")
	if orderSecret == "" {
		logger.Fatal("ORDER_NUMBER_SECRET is required")
	}

	orderGen := orders.NewOrderNumberGenerator(orderSecret)

	// Database
	dbpool, err := db.New(
		cfg.db.addr,
		int32(cfg.db.maxOpenConns), // convert to int32 if needed
		cfg.db.maxIdleTime,
	)

	if err != nil {
		logger.Fatal(err)
	}

	defer dbpool.Close()
	logger.Info("database connection pool established")

	//storage

	storeContainer := storage.NewContainer(dbpool, orderGen)

	//cloudinary
	cloudinaryUrl := os.Getenv("CLOUDINARY_URL")
	cld, err := cloudinary.NewFromURL(cloudinaryUrl)
	if err != nil {
		logger.Fatal(err)
	}

	//expo push message (notification)

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	client := exponent.NewClient(
		exponent.WithHttpClient(httpClient),
	)

	//wrap it with your adapter

	sender := notifications.NewExpoAdapter(client)

	// client to send email for activation
	// mailer := mailer.NewSendgrid(cfg.mail.sendGrid.apiKey, cfg.mail.fromEmail)

	mailtrap, err := mailer.NewMailTrapClient(cfg.mail.mailtrap.apiKey, cfg.mail.fromEmail)
	if err != nil {
		logger.Fatal(err)
	}

	// Rate limiter
	rateLimiter := ratelimiter.NewFixedWindowLimiter(
		cfg.rateLimiter.RequestsPerTimeFrame,
		cfg.rateLimiter.TimeFrame,
	)

	// 5 req/min per IP
	venueReqLimiter := ratelimiter.NewFixedWindowLimiter(5, 1*time.Minute)

	// Authenticator
	jwtAuthenticator := auth.NewJWTAuthenticator(
		cfg.auth.token.refreshSecret,
		cfg.auth.token.secret,
		cfg.auth.token.iss,
		cfg.auth.token.iss,
	)

	isProd := os.Getenv("APP_ENV") == "prod"

	// Payments
	pm := payments.NewPaymentManager()

	pm.RegisterGateway("esewa",
		payments.NewEsewaAdapter(
			cfg.payment.Esewa.MerchantID,
			cfg.payment.Esewa.SecretKey,
			cfg.payment.Esewa.SuccessURL,
			cfg.payment.Esewa.FailureURL,
			isProd,
		),
	)

	pm.RegisterGateway("khalti",
		payments.NewKhaltiAdapter(
			cfg.payment.Khalti.SecretKey,
			cfg.payment.Khalti.ReturnURL,
			cfg.payment.Khalti.WebsiteURL,
			isProd,
		),
	)

	app := &application{
		config:              cfg,
		logger:              logger,
		store:               storeContainer,
		cld:                 cld,
		mailer:              mailtrap,
		authenticator:       jwtAuthenticator,
		rateLimiter:         rateLimiter,
		venueRequestLimiter: venueReqLimiter,
		push:                sender,
		hashID:              h,
		payments:            pm,
	}

	//Metrics collected http://localhost:8080/v1/debug/vars
	expvar.NewString("version").Set(version)
	expvar.Publish("database", expvar.Func(func() any {
		return dbpool.Stat()
	}))
	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app.markCompletedGamesEvery30Mins(ctx)

	mux := app.mount()

	if err := app.run(mux, cancel); err != nil {
		logger.Fatal(err)
	}
}
