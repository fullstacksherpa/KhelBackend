package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New sets up a new pgx connection pool
func New(addr string, maxConns int32, maxIdleTime string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(addr)
	if err != nil {
		return nil, err
	}

	// Configure connection limits
	config.MaxConns = maxConns

	// Set max idle time
	duration, err := time.ParseDuration(maxIdleTime)
	if err != nil {
		return nil, err
	}
	config.MaxConnIdleTime = duration

	// This timeout applied to pool initialization, including establishing initial connections to the database. Running the Ping() test. if nay step exceeds the quoted time, the pool fails to start.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbpool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	// Test the connection
	if err := dbpool.Ping(ctx); err != nil {
		dbpool.Close()
		return nil, err
	}

	return dbpool, nil
}
