package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrNotFound          = errors.New("resource not found")
	ErrConflict          = errors.New("resource already exists")
	QueryTimeoutDuration = time.Second * 5
)

type Storage struct {
	Users interface {
		GetByID(context.Context, int64) (*User, error)
		Create(context.Context, *User) error
		SetProfile(context.Context, string, string) error
		GetProfileUrl(context.Context, string) (string, error)
		UpdateUser(context.Context, int64, map[string]interface{}) error
	}
	Venues interface {
		Create(context.Context, *Venue) error
		Update(context.Context, string, map[string]interface{}) error
		CheckIfVenueExists(context.Context, string, int64) (bool, error)
		RemovePhotoURL(context.Context, string, string) error
		AddPhotoURL(context.Context, string, string) error
	}
	Reviews interface {
		CreateReview(context.Context, *Review) error
		GetReviews(context.Context, int64) ([]Review, error)
		DeleteReview(context.Context, int64, int64) error
		GetReviewStats(context.Context, int64) (int, float64, error)
	}
	Followers interface {
		Follow(ctx context.Context, followerID, userID int64) error
		Unfollow(ctx context.Context, followerID, userID int64) error
	}
}

func NewStorage(db *sql.DB) Storage {
	return Storage{
		Users:     &UsersStore{db},
		Venues:    &VenuesStore{db},
		Reviews:   &ReviewStore{db},
		Followers: &FollowerStore{db},
	}
}
