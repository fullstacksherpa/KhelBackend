package storage

import (
	"context"
	"khel/internal/domain/accesscontrol"
	"khel/internal/domain/ads"
	"khel/internal/domain/appreviews"
	"khel/internal/domain/bookings"
	"khel/internal/domain/carts"
	"khel/internal/domain/followers"
	"khel/internal/domain/gameqa"
	"khel/internal/domain/games"
	"khel/internal/domain/orders"
	"khel/internal/domain/paymentsrepo"
	"khel/internal/domain/products"
	"khel/internal/domain/pushtokens"
	"khel/internal/domain/users"
	venuereviews "khel/internal/domain/venuereview"
	"khel/internal/domain/venues"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Sales struct {
	Carts    carts.Store
	Orders   orders.Store
	Payments paymentsrepo.Store
	PayLogs  *paymentsrepo.LogsRepository
}

type Container struct {
	pool          *pgxpool.Pool // IMPORTANT: set the pool so WithSalesTx works
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
	AccessControl accesscontrol.Store
	Products      products.Store
	Sales         Sales
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
		AccessControl: accesscontrol.NewRepository(db),
		Products:      products.NewRepository(db),
		Sales: Sales{
			Carts:    carts.NewRepository(db),
			Orders:   orders.NewRepository(db),
			Payments: paymentsrepo.NewRepository(db),
			PayLogs:  paymentsrepo.NewLogsRepository(db),
		},
	}
}

// SalesTx is a temporary, tx-scoped set of repos for atomic units of work.
type SalesTx struct {
	Carts    carts.Store
	Orders   orders.Store
	Payments paymentsrepo.Store
	PayLogs  *paymentsrepo.LogsRepository
}

// WithSalesTx runs a sales unit-of-work atomically.
func (c *Container) WithSalesTx(ctx context.Context, fn func(s *SalesTx) error) error {
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	defer tx.Rollback(ctx)
	if err != nil {
		return err
	}
	s := &SalesTx{
		Carts:    carts.NewRepository(tx),
		Orders:   orders.NewRepository(tx),
		Payments: paymentsrepo.NewRepository(tx),
		PayLogs:  paymentsrepo.NewLogsRepository(tx),
	}
	if err := fn(s); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
