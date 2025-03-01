package db

import (
	"context"
	"database/sql"
	"time"
)

func New(addr string, maxOpenConns, maxIdleConns int, maxIdleTime string) (*sql.DB, error) {
	db, err := sql.Open("postgres", addr)
	if err != nil {
		return nil, err
	}

	//setting connection pool to db
	db.SetMaxOpenConns(maxOpenConns)

	duration, err := time.ParseDuration(maxIdleTime)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxIdleTime(duration)
	db.SetMaxIdleConns(maxIdleConns)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	//defer is called when the function exits always.

	// cancel() immediately marks the context as canceled and releases any resource associated with the context. it also notifies any goroutines listening to ctx.Done() to stop theri work. cancel() ensures proper cleanup even if the request completes early.

	defer cancel()

	//checks if the database connection is alive, if the database does not respond within 5 second, it return an error. if the database is reachable, it returns db.

	if err = db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}
