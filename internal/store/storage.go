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
		SetProfile(context.Context, string, int64) error
		GetProfileUrl(context.Context, int64) (string, error)
		UpdateUser(context.Context, int64, map[string]interface{}) error
		SaveRefreshToken(ctx context.Context, userID int64, refreshToken string) error
		DeleteRefreshToken(ctx context.Context, userID int64) error
		GetRefreshToken(ctx context.Context, userID int64) (string, error)
		UpdateResetToken(ctx context.Context, email, resetToken string, resetTokenExpires time.Time) error
		GetByResetToken(ctx context.Context, resetToken string) (*User, error)
		Update(ctx context.Context, user *User) error
	}
	Venues interface {
		Create(context.Context, *Venue) error
		Update(ctx context.Context, venueID int64, updateData map[string]interface{}) error
		CheckIfVenueExists(context.Context, string, int64) (bool, error)
		RemovePhotoURL(ctx context.Context, venueID int64, photoURL string) error
		AddPhotoURL(ctx context.Context, venueID int64, photoURL string) error
		IsOwner(ctx context.Context, venueID int64, userID int64) (bool, error)
		GetVenueByID(ctx context.Context, venueID int64) (*Venue, error)
	}
	Reviews interface {
		CreateReview(context.Context, *Review) error
		GetReviews(context.Context, int64) ([]Review, error)
		DeleteReview(context.Context, int64, int64) error
		GetReviewStats(context.Context, int64) (int, float64, error)
		IsReviewOwner(ctx context.Context, reviewID int64, userID int64) (bool, error)
	}
	Followers interface {
		Follow(ctx context.Context, followerID, userID int64) error
		Unfollow(ctx context.Context, followerID, userID int64) error
	}
	Games interface {
		GetGames(ctx context.Context, q GameFilterQuery) ([]GameWithVenue, error)
		Create(context.Context, *Game) error
		GetGameByID(ctx context.Context, gameID int64) (*Game, error)
		CheckRequestExist(ctx context.Context, gameID int64, userID int64) (bool, error)
		AddToGameRequest(ctx context.Context, gameID int64, UserID int64) error
		IsAdminAssistant(ctx context.Context, gameID int64, userID int64) (bool, error)
		IsAdmin(ctx context.Context, gameID, userID int64) (bool, error)
		SetMatchFull(ctx context.Context, gameID int64) error
		InsertNewPlayer(ctx context.Context, gameID int64, userID int64) error
		UpdateRequestStatus(ctx context.Context, gameID, userID int64, status GameRequestStatus) error
		GetJoinRequest(ctx context.Context, gameID, userID int64) (*GameRequest, error)
		GetPlayerCount(ctx context.Context, gameID int) (int, error)
		GetGamePlayers(ctx context.Context, gameID int64) ([]*User, error)
		AssignAssistant(ctx context.Context, gameID, playerID int64) error
		CancelGame(ctx context.Context, gameID int64) error
	}
	Bookings interface {
		GetPricingSlots(ctx context.Context, venueID int64, dayOfWeek string) ([]PricingSlot, error)
		GetBookingsForDate(ctx context.Context, venueID int64, date time.Time) ([]Interval, error)
		CreateBooking(ctx context.Context, booking *Booking) error
		UpdatePricing(ctx context.Context, p *PricingSlot) error
	}
}

func NewStorage(db *sql.DB) Storage {
	return Storage{
		Users:     &UsersStore{db},
		Venues:    &VenuesStore{db},
		Reviews:   &ReviewStore{db},
		Followers: &FollowerStore{db},
		Games:     &GameStore{db},
		Bookings:  &BookingStore{db},
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
