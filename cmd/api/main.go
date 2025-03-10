package main

import (
	"expvar"
	"khel/internal/auth"
	"khel/internal/db"
	"khel/internal/mailer"
	"khel/internal/store"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	_ "github.com/lib/pq"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

const version = "0.0.1"

//	@title			Khel API
//	@description	API for Khel, a complete sport application.

//	@contact.name	API Support
//	@contact.url	http://www.swagger.io/support
//	@contact.email	support@swagger.io

//	@license.name	Apache 2.0
//	@license.url	http://www.apache.org/licenses/LICENSE-2.0.html

//	@BasePath					/v1
//	@securityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						Authorization
//	@description

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	// Retrieve and convert maxOpenConns
	maxOpenConnsStr := os.Getenv("DB_MAX_OPEN_CONNS")
	maxOpenConns, err := strconv.Atoi(maxOpenConnsStr)
	if err != nil {
		log.Fatalf("Invalid value for DB_MAX_OPEN_CONNS: %v", err)
	}
	// Retrieve and convert maxIdleConns
	maxIdleConnsStr := os.Getenv("DB_MAX_IDLE_CONNS")
	maxIdleConns, err := strconv.Atoi(maxIdleConnsStr)
	if err != nil {
		log.Fatalf("Invalid value for DB_MAX_IDLE_CONNS: %v", err)
	}

	cfg := config{
		addr:        os.Getenv("ADDR"),
		env:         os.Getenv("ENV"),
		frontendURL: os.Getenv("FRONTEND_URL"),
		apiURL:      os.Getenv("EXTERNAL_URL"),
		db: dbConfig{
			addr:         os.Getenv("DB_ADDR"),
			maxOpenConns: maxOpenConns,
			maxIdleConns: maxIdleConns,
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
	}

	// Logger
	logger := zap.Must(zap.NewProduction()).Sugar()
	defer logger.Sync()

	// Database
	db, err := db.New(
		cfg.db.addr,
		cfg.db.maxOpenConns,
		cfg.db.maxIdleConns,
		cfg.db.maxIdleTime,
	)

	if err != nil {
		logger.Fatal(err)
	}

	defer db.Close()
	logger.Info("database connection pool established")

	//storage
	store := store.NewStorage(db)

	//cloudinary
	cloudinaryUrl := os.Getenv("CLOUDINARY_URL")
	cld, err := cloudinary.NewFromURL(cloudinaryUrl)
	if err != nil {
		logger.Fatal(err)
	}

	// client to send email for activation
	// mailer := mailer.NewSendgrid(cfg.mail.sendGrid.apiKey, cfg.mail.fromEmail)

	mailtrap, err := mailer.NewMailTrapClient(cfg.mail.mailtrap.apiKey, cfg.mail.fromEmail)
	if err != nil {
		logger.Fatal(err)
	}

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
		store:         store,
		cld:           cld,
		mailer:        mailtrap,
		authenticator: jwtAuthenticator,
	}

	//Metrics collected
	expvar.NewString("version").Set(version)
	expvar.Publish("database", expvar.Func(func() any {
		return db.Stats()
	}))
	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))

	mux := app.mount()

	logger.Fatal(app.run(mux))
}
