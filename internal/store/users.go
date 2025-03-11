package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDuplicateEmail       = errors.New("a user with that email already exists")
	ErrDuplicatePhoneNumber = errors.New("a user with that phone number already exists")
)

type User struct {
	ID                   int64      `json:"id"`
	FirstName            string     `json:"first_name"`
	LastName             string     `json:"last_name"`
	Email                string     `json:"email"`
	Phone                string     `json:"-"` // Sensitive data
	Password             password   `json:"-"` // Hide password
	ProfilePictureURL    NullString `json:"profile_picture_url" swaggertype:"string"`
	SkillLevel           NullString `json:"skill_level" swaggertype:"string"`
	NoOfGames            NullInt16  `json:"no_of_games" swaggertype:"integer"`
	RefreshToken         string     `json:"-"` // Sensitive data
	IsActive             bool       `json:"is_active"`
	ResetPasswordToken   string     `json:"-"` // Sensitive data
	ResetPasswordExpires time.Time  `json:"-"` // Internal use only
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
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

func (s *UsersStore) SetProfile(ctx context.Context, url string, userID int64) error {
	query := `UPDATE users SET profile_picture_url = $1 WHERE id = $2`
	_, err := s.db.ExecContext(ctx, query, url, userID)
	if err != nil {
		return err
	}
	return nil
}

func (s *UsersStore) GetProfileUrl(ctx context.Context, userID int64) (string, error) {
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

	// Validate skill_level if it's being updated
	if skillLevel, ok := updates["skill_level"]; ok {
		validSkillLevels := map[string]bool{"beginner": true, "intermediate": true, "advanced": true}
		if !validSkillLevels[skillLevel.(string)] {
			return fmt.Errorf("invalid skill level")
		}
	}

	// Build query dynamically based on provided fields
	setClauses := []string{}
	args := []interface{}{}
	argCounter := 1

	for field, value := range updates {
		// Sanitize field names to prevent SQL injection
		if !isValidField(field) {
			return fmt.Errorf("invalid field name: %s", field)
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", field, argCounter))
		args = append(args, value)
		argCounter++
	}
	args = append(args, userID)

	query := fmt.Sprintf("UPDATE users SET %s, updated_at = NOW() WHERE id = $%d",
		strings.Join(setClauses, ", "), argCounter)

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		log.Printf("Failed to update user: %v", err)
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// Helper function to validate field names
func isValidField(field string) bool {
	validFields := map[string]bool{
		"first_name":  true,
		"last_name":   true,
		"skill_level": true,
		"phone":       true,
	}
	return validFields[field]
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

// later we will use to implement saga design pattern when sending emailWithToken fail
func (s *UsersStore) Delete(ctx context.Context, userID int64) error {
	return withTx(s.db, ctx, func(tx *sql.Tx) error {
		if err := s.delete(ctx, tx, userID); err != nil {
			return err
		}

		if err := s.deleteUserInvitations(ctx, tx, userID); err != nil {
			return err
		}

		return nil
	})
}

func (s *UsersStore) delete(ctx context.Context, tx *sql.Tx, id int64) error {
	query := `DELETE FROM users WHERE id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	return nil
}

func (s *UsersStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, first_name, phone, email, password, created_at FROM users
		WHERE email = $1 AND is_active = true
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	user := &User{}
	err := s.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.FirstName,
		&user.Phone,
		&user.Email,
		&user.Password.hash,
		&user.CreatedAt,
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

func (s *UsersStore) SaveRefreshToken(ctx context.Context, userID int64, refreshToken string) error {
	query := `UPDATE users SET refresh_token = $1, updated_at = NOW() WHERE id = $2`
	_, err := s.db.ExecContext(ctx, query, refreshToken, userID)
	if err != nil {
		return fmt.Errorf("failed to save refresh token: %w", err)
	}
	return nil
}

func (s *UsersStore) DeleteRefreshToken(ctx context.Context, userID int64) error {
	query := `UPDATE users SET refresh_token = NULL, updated_at = NOW() WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete refresh token: %w", err)
	}
	return nil
}

// GetRefreshToken retrieves the refresh token for a specific user from the database.
func (s *UsersStore) GetRefreshToken(ctx context.Context, userID int64) (string, error) {
	var refreshToken string

	// Query to retrieve the refresh token for the given userID
	query := `SELECT refresh_token FROM users WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&refreshToken)

	if err != nil {
		if err == sql.ErrNoRows {
			// No rows returned, which means no refresh token found for the user
			return "", fmt.Errorf("no refresh token found for user %d", userID)
		}
		// Handle other database errors
		return "", fmt.Errorf("failed to retrieve refresh token: %v", err)
	}

	// Return the refresh token
	return refreshToken, nil
}
