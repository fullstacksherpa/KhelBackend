package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDuplicateEmail       = errors.New("a user with that email already exists")
	ErrDuplicatePhoneNumber = errors.New("a user with that phone number already exists")
)

type User struct {
	ID                   int64          `json:"id"`
	FirstName            string         `json:"first_name"`
	LastName             string         `json:"last_name"`
	Email                string         `json:"email"`
	Phone                string         `json:"-"` // Sensitive data
	Password             password       `json:"-"` // Hide password
	ProfilePictureURL    sql.NullString `json:"profile_picture_url"`
	SkillLevel           sql.NullString `json:"skill_level,"`
	NoOfGames            sql.NullInt16  `json:"no_of_games"`
	RefreshToken         string         `json:"-"` // Sensitive data
	IsActive             bool           `json:"is_active"`
	ResetPasswordToken   string         `json:"-"` // Sensitive data
	ResetPasswordExpires time.Time      `json:"-"` // Internal use only
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// Password struct to store plain text and hash
type password struct {
	text *string `json:"-"` // Hide plaintext password
	hash []byte  `json:"-"` // Hide hashed password
}

func (p *password) Set(text string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(text), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	p.text = &text
	p.hash = hash

	return nil
}

func (p *password) Compare(text string) error {
	return bcrypt.CompareHashAndPassword(p.hash, []byte(text))
}

type UsersStore struct {
	db *sql.DB
}

func (s *UsersStore) Create(ctx context.Context, tx *sql.Tx, user *User) error {

	query := `
	  INSERT INTO users (first_name, last_name, password, email, phone) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	err := tx.QueryRowContext(
		ctx, query, user.FirstName, user.LastName, user.Password.hash, user.Email, user.Phone,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		switch {
		//TODO: check unique constraint in db
		case err.Error() == `pq: duplicate key value violates unique constraint "users_email_key"`:
			return ErrDuplicateEmail
		case err.Error() == `pq: duplicate key value violates unique constraint "users_phone_key"`:
			return ErrDuplicatePhoneNumber
		default:
			return err
		}
	}
	return nil
}

func (s *UsersStore) SetProfile(ctx context.Context, url string, userID string) error {
	query := `UPDATE users SET profile_picture_url = $1 WHERE id = $2`
	_, err := s.db.ExecContext(ctx, query, url, userID)
	if err != nil {
		return err
	}
	return nil
}

func (s *UsersStore) GetProfileUrl(ctx context.Context, userID string) (string, error) {
	var oldProfilePictureURL string
	query := `SELECT profile_picture_url FROM users WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&oldProfilePictureURL)
	if err != nil {
		if err == sql.ErrNoRows {
			// Handle the case where no rows are returned (user not found)
			return "", fmt.Errorf("user not found")
		}
		// Handle other database errors
		return "", fmt.Errorf("failed to retrieve profile picture URL: %v", err)
	}
	return oldProfilePictureURL, nil
}

func (s *UsersStore) UpdateUser(ctx context.Context, userID int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return fmt.Errorf("no fields to update")
	}

	// Build query dynamically based on provided fields
	setClauses := []string{}
	args := []interface{}{}
	argCounter := 1

	for field, value := range updates {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", field, argCounter))
		args = append(args, value)
		argCounter++
	}
	args = append(args, userID)

	query := fmt.Sprintf("UPDATE users SET %s, updated_at = NOW() WHERE id = $%d",
		strings.Join(setClauses, ", "), argCounter)

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

func (s *UsersStore) GetByID(ctx context.Context, userID int64) (*User, error) {
	query := `
	   SELECT users.id, first_name, last_name, email, password, profile_picture_url, skill_level, created_at, no_of_games
	   FROM users
	   WHERE users.id = $1
	`
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	user := &User{}
	err := s.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.FirstName,
		&user.LastName,
		&user.Email,
		&user.Password.hash,
		&user.ProfilePictureURL,
		&user.SkillLevel,
		&user.CreatedAt,
		&user.NoOfGames,
	)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return nil, ErrNotFound
		default:
			return nil, err
		}
	}
	return user, nil
}

func (s *UsersStore) CreateAndInvite(ctx context.Context, user *User, token string, invitationExp time.Duration) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		if err := s.Create(ctx, tx, user); err != nil {
			return err
		}

		if err := s.createUserInvitation(ctx, tx, token, invitationExp, user.ID); err != nil {
			return err
		}

		return nil
	})
}

func (s *UsersStore) createUserInvitation(ctx context.Context, tx *sql.Tx, token string, exp time.Duration, userID int64) error {
	query := `INSERT INTO user_invitations (token, user_id, expiry) VALUES ($1, $2, $3)`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := tx.ExecContext(ctx, query, token, userID, time.Now().Add(exp))
	if err != nil {
		return err
	}

	return nil
}

func (s *UsersStore) Activate(ctx context.Context, token string) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		// 1. find the user that this token belongs to
		user, err := s.getUserFromInvitation(ctx, tx, token)
		if err != nil {
			return err
		}

		// 2. update the user
		user.IsActive = true
		if err := s.update(ctx, tx, user); err != nil {
			return err
		}

		// 3. clean the invitations
		if err := s.deleteUserInvitations(ctx, tx, user.ID); err != nil {
			return err
		}

		return nil
	})
}

func (s *UsersStore) getUserFromInvitation(ctx context.Context, tx *sql.Tx, token string) (*User, error) {
	query := `
		SELECT u.id, u.first_name, u.last_name, u.email, u.created_at, u.is_active
		FROM users u
		JOIN user_invitations ui ON u.id = ui.user_id
		WHERE ui.token = $1 AND ui.expiry > $2
	`

	hash := sha256.Sum256([]byte(token))
	hashToken := hex.EncodeToString(hash[:])

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	user := &User{}
	err := tx.QueryRowContext(ctx, query, hashToken, time.Now()).Scan(
		&user.ID,
		&user.FirstName,
		&user.LastName,
		&user.Email,
		&user.CreatedAt,
		&user.IsActive,
	)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return nil, ErrNotFound
		default:
			return nil, err
		}
	}

	return user, nil
}

func (s *UsersStore) update(ctx context.Context, tx *sql.Tx, user *User) error {
	query := `UPDATE users SET  email = $1, is_active = $2 WHERE id = $3`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := tx.ExecContext(ctx, query, user.Email, user.IsActive, user.ID)
	if err != nil {
		return err
	}

	return nil
}

func (s *UsersStore) deleteUserInvitations(ctx context.Context, tx *sql.Tx, userID int64) error {
	query := `DELETE FROM user_invitations WHERE user_id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := tx.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}

	return nil
}
