package store

import (
	"context"
	"database/sql"
)

type Storage struct {
	Users interface {
		Create(context.Context, *User) error
		SetProfile(context.Context, string, string) error
		GetProfileUrl(context.Context, string) (string, error)
	}
	Venues interface {
		Create(context.Context, *Venue) error
		Update(context.Context, string, map[string]interface{}) error
		CheckIfVenueExists(context.Context, string, int64) (bool, error)
		RemovePhotoURL(context.Context, string, string) error
		AddPhotoURL(context.Context, string, string) error
	}
}

func NewStorage(db *sql.DB) Storage {
	return Storage{
		Users:  &UsersStore{db},
		Venues: &VenuesStore{db},
	}
}
