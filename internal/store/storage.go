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
		GetByEmail(context.Context, string) (*User, error)
		Create(context.Context, *sql.Tx, *User) error
		CreateAndInvite(ctx context.Context, user *User, token string, exp time.Duration) error
		Activate(context.Context, string) error
		Delete(context.Context, int64) error
		SetProfile(context.Context, string, string) error
		GetProfileUrl(context.Context, string) (string, error)
		UpdateUser(context.Context, int64, map[string]interface{}) error
		SaveRefreshToken(ctx context.Context, userID int64, refreshToken string) error
		DeleteRefreshToken(ctx context.Context, userID int64) error
		GetRefreshToken(ctx context.Context, userID int64) (string, error)
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
	Games interface {
		Create(context.Context, *Game) error
		GetGameByID(ctx context.Context, gameID int64) (*Game, error)
		CheckRequestExist(ctx context.Context, gameID int64, userID int64) (bool, error)
		AddToGameRequest(ctx context.Context, gameID int64, UserID int64) error
		IsAdminAssistant(ctx context.Context, gameID int64, userID int64) (bool, error)
		SetMatchFull(ctx context.Context, gameID int64) error
		InsertNewPlayer(ctx context.Context, gameID int64, userID int64) error
		UpdateRequestStatus(ctx context.Context, gameID, userID int64, status GameRequestStatus) error
		GetJoinRequest(ctx context.Context, gameID, userID int64) (*GameRequest, error)
		GetPlayerCount(ctx context.Context, gameID int) (int, error)
		GetGamePlayers(ctx context.Context, gameID int64) ([]*User, error)
	}
}

func NewStorage(db *sql.DB) Storage {
	return Storage{
		Users:     &UsersStore{db},
		Venues:    &VenuesStore{db},
		Reviews:   &ReviewStore{db},
		Followers: &FollowerStore{db},
		Games:     &GameStore{db},
	}
}

func withTx(db *sql.DB, ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
