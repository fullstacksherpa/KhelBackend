package storage

import (
	"khel/internal/domain/ads"
	"khel/internal/domain/appreviews"
	"khel/internal/domain/bookings"
	"khel/internal/domain/followers"
	"khel/internal/domain/gameqa"
	"khel/internal/domain/games"
	"khel/internal/domain/pushtokens"
	"khel/internal/domain/users"
	venuereviews "khel/internal/domain/venuereview"
	"khel/internal/domain/venues"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Container struct {
	Users         users.Store
	Venues        venues.Store
	VenuesReviews venuereviews.Store
	Followers     followers.Store
	Games         games.Store
	Bookings      bookings.Store
	GameQA        gameqa.Store
	AppReviews    appreviews.Store
	PushTokens    pushtokens.Store
	Ads           ads.Store
}

func NewContainer(db *pgxpool.Pool) *Container {
	return &Container{
		Users:         users.NewRepository(db),
		Venues:        venues.NewRepository(db),
		VenuesReviews: venuereviews.NewRepository(db),
		Followers:     followers.NewRepository(db),
		Games:         games.NewRepository(db),
		Bookings:      bookings.NewRepository(db),
		GameQA:        gameqa.NewRepository(db),
		AppReviews:    appreviews.NewRepository(db),
		PushTokens:    pushtokens.NewRepository(db),
		Ads:           ads.NewRepository(db),
	}
}
