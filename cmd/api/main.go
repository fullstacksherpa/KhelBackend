package main

import (
	"context"
	"expvar"
	"fmt"
	"khel/internal/auth"
	"khel/internal/db"
	"khel/internal/domain/storage"
	"khel/internal/mailer"
	"khel/internal/notifications"
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

var version = "1.7.0"

//	@title			Khel API
//	@description	API for Khel, a complete sport application.

//	@contact.name	fullstacksherpa
//	@contact.url	https://www.fullstacksherpa.tech/
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
	envFile := fmt.Sprintf(".env.%s", env)
	godotenv.Load(envFile)

	hashSalt := os.Getenv("HASHIDS_SALT")
	hd := hashids.NewData()
	hd.Salt = hashSalt
	hd.MinLength = 8
	h, err := hashids.NewWithData(hd)
	if err != nil {
		log.Fatal("Failed to initialize HashID:", err)
	}

	// Retrieve and convert maxOpenConns
	maxOpenConnsStr := os.Getenv("DB_MAX_OPEN_CONNS")
	maxOpenConns, err := strconv.Atoi(maxOpenConnsStr)
	if err != nil {
		log.Fatalf("Invalid value for DB_MAX_OPEN_CONNS: %v", err)
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
			exp:       time.Hour * 24 * 3, //3 days
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
				accessTokenExp:  time.Hour * 24 * 3, // 3 days
				refreshTokenExp: time.Hour * 24 * 9, // 9 days
				iss:             "Khel",
			},
		},
		rateLimiter: LoadRateLimiterConfig(),
	}

	// Logger
	// Create the logger
	logger, err := NewLogger()
	if err != nil {
		fmt.Println("Error creating logger:", err)
		return
	}
	defer logger.Sync()

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

	storeContainer := storage.NewContainer(dbpool)

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

	// Authenticator
	jwtAuthenticator := auth.NewJWTAuthenticator(
		cfg.auth.token.refreshSecret,
		cfg.auth.token.secret,
		cfg.auth.token.iss,
		cfg.auth.token.iss,
	)

	app := &application{
		config:        cfg,
		logger:        logger,
		store:         storeContainer,
		cld:           cld,
		mailer:        mailtrap,
		authenticator: jwtAuthenticator,
		rateLimiter:   rateLimiter,
		push:          sender,
		hashID:        h,
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

	logger.Fatal(app.run(mux, cancel))
}
