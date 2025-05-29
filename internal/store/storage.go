package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		Create(ctx context.Context, tx pgx.Tx, user *User) error
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
		UpdateAndUpload(ctx context.Context, userID int64, updates map[string]interface{}, profilePictureURL string) error
	}
	Venues interface {
		Create(context.Context, *Venue) error
		UpdateImageURLs(ctx context.Context, venueID int64, urls []string) error
		Delete(ctx context.Context, venueID int64) error
		Update(ctx context.Context, venueID int64, updateData map[string]interface{}) error
		CheckIfVenueExists(context.Context, string, int64) (bool, error)
		RemovePhotoURL(ctx context.Context, venueID int64, photoURL string) error
		AddPhotoURL(ctx context.Context, venueID int64, photoURL string) error
		IsOwner(ctx context.Context, venueID int64, userID int64) (bool, error)
		GetVenueByID(ctx context.Context, venueID int64) (*Venue, error)
		GetOwnedVenueIDs(ctx context.Context, userID int64) ([]int64, error)
		List(ctx context.Context, filter VenueFilter) ([]VenueListing, error)
		GetVenueDetail(ctx context.Context, venueID int64) (*VenueDetail, error)
		GetImageURLs(ctx context.Context, venueID int64) ([]string, error)
		GetVenueInfo(ctx context.Context, venueID int64) (*VenueInfo, error)
	}
	Reviews interface {
		CreateReview(context.Context, *Review) error
		GetReviews(context.Context, int64) ([]Review, error)
		DeleteReview(context.Context, int64, int64) error
		GetReviewStats(context.Context, int64) (int, float64, error)
		IsReviewOwner(ctx context.Context, reviewID int64, userID int64) (bool, error)
		HasReview(ctx context.Context, venueID, userID int64) (bool, error)
	}
	Followers interface {
		Follow(ctx context.Context, followerID, userID int64) error
		Unfollow(ctx context.Context, followerID, userID int64) error
	}
	Games interface {
		GetGames(ctx context.Context, q GameFilterQuery) ([]GameSummary, error)
		Create(ctx context.Context, game *Game) (int64, error)
		GetGameByID(ctx context.Context, gameID int64) (*Game, error)
		CheckRequestExist(ctx context.Context, gameID int64, userID int64) (bool, error)
		AddToGameRequest(ctx context.Context, gameID int64, UserID int64) error
		IsAdminAssistant(ctx context.Context, gameID int64, userID int64) (bool, error)
		IsAdmin(ctx context.Context, gameID, userID int64) (bool, error)
		ToggleMatchFull(ctx context.Context, gameID int64) error
		InsertNewPlayer(ctx context.Context, gameID int64, userID int64) error
		InsertAdminInPlayer(ctx context.Context, gameID int64, userID int64) error
		UpdateRequestStatus(ctx context.Context, gameID, userID int64, status GameRequestStatus) error
		GetJoinRequest(ctx context.Context, gameID, userID int64) (*GameRequest, error)
		GetAllJoinRequests(ctx context.Context, gameID int64) ([]*GameRequestWithUser, error)
		GetPlayerCount(ctx context.Context, gameID int) (int, error)
		GetGamePlayers(ctx context.Context, gameID int64) ([]*User, error)
		AssignAssistant(ctx context.Context, gameID, playerID int64) error
		CancelGame(ctx context.Context, gameID int64) error
		GetGameDetailsWithID(ctx context.Context, gameID int64) (*GameDetails, error)
		GetUpcomingGamesByVenue(ctx context.Context, venueID int64) ([]GameSummary, error)
		GetUpcomingGamesByUser(ctx context.Context, userID int64) ([]GameSummary, error)
	}
	Bookings interface {
		GetPricingSlots(ctx context.Context, venueID int64, dayOfWeek string) ([]PricingSlot, error)
		GetBookingsForDate(ctx context.Context, venueID int64, date time.Time) ([]Interval, error)
		CreateBooking(ctx context.Context, booking *Booking) error
		UpdatePricing(ctx context.Context, p *PricingSlot) error
		CreatePricingSlot(ctx context.Context, p *PricingSlot) error
		GetPendingBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]PendingBooking, error)
		UpdateBookingStatus(ctx context.Context, venueID, bookingID int64, status string) error
		AcceptBooking(ctx context.Context, venueID, bookingID int64) error
		RejectBooking(ctx context.Context, venueID, bookingID int64) error
		GetScheduledBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]ScheduledBooking, error)
		GetBookingsByUser(ctx context.Context, userID int64, filter BookingFilter) ([]UserBooking, error)
	}
	FavoriteVenues interface {
		AddFavorite(ctx context.Context, userID, venueID int64) error
		RemoveFavorite(ctx context.Context, userID, venueID int64) error
		GetFavoritesByUser(ctx context.Context, userID int64) ([]Venue, error)
		GetFavoriteVenueIDsByUser(ctx context.Context, userID int64) (map[int64]struct{}, error)
	}
	ShortlistedGames interface {
		AddShortlist(ctx context.Context, userID, gameID int64) error
		RemoveShortlist(ctx context.Context, userID, gameID int64) error
		GetShortlistedGamesByUser(
			ctx context.Context,
			userID int64,
		) ([]ShortlistedGameDetail, error)
	}
	GameQA interface {
		CreateQuestion(ctx context.Context, question *Question) error
		GetQuestionsByGame(ctx context.Context, gameID int64) ([]Question, error)
		CreateReply(ctx context.Context, reply *Reply) error
		GetRepliesByQuestion(ctx context.Context, questionID int64) ([]Reply, error)
		DeleteQuestion(ctx context.Context, questionID, userID int64) error
		GetQuestionsWithReplies(ctx context.Context, gameID int64) ([]QuestionWithReplies, error)
	}
	AppReviews interface {
		AddReview(ctx context.Context, userID int64, rating int, feedback string) error
		GetAllReviews(ctx context.Context) ([]AppReview, error)
	}
}

func NewStorage(db *pgxpool.Pool) Storage {
	return Storage{
		Users:            &UsersStore{db},
		Venues:           &VenuesStore{db},
		Reviews:          &ReviewStore{db},
		Followers:        &FollowerStore{db},
		Games:            &GameStore{db},
		Bookings:         &BookingStore{db},
		FavoriteVenues:   &FavoriteVenuesStore{db},
		ShortlistedGames: &ShortlistGamesStore{db},
		GameQA:           &QuestionStore{db},
		AppReviews:       &AppReviewStore{db},
	}
}

func withTx(pool *pgxpool.Pool, ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
