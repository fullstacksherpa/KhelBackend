package storage

import (
	"context"
	"fmt"
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
	"khel/internal/domain/venuerequest"
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
	VenueRequests venuerequest.RequestStore
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
		pool:          db,
		Users:         users.NewRepository(db),
		VenueRequests: venuerequest.NewRepository(db),
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
	if c.pool == nil {
		return fmt.Errorf("storage container pool is nil (did you forget to set pool in NewContainer?)")
	}

	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx) // safe even if already committed
	}()

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
