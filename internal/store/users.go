package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type User struct {
	ID                       int64          `json:"id"`
	FirstName                string         `json:"first_name"`
	LastName                 string         `json:"last_name"`
	Email                    string         `json:"email"`
	Phone                    string         `json:"-"` // Sensitive data
	Password                 password       `json:"-"` // Hide password
	ProfilePictureURL        sql.NullString `json:"profile_picture_url"`
	SkillLevel               sql.NullString `json:"skill_level,omitempty"`
	NoOfGames                sql.NullInt16  `json:"no_of_games"`
	RefreshToken             string         `json:"-"` // Sensitive data
	IsEmailVerified          bool           `json:"is_email_verified"`
	EmailVerificationToken   string         `json:"-"` // Sensitive data
	EmailVerificationExpires time.Time      `json:"-"` // Internal use only
	ResetPasswordToken       string         `json:"-"` // Sensitive data
	ResetPasswordExpires     time.Time      `json:"-"` // Internal use only
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
}

// Password struct to store plain text and hash
type password struct {
	text *string `json:"-"` // Hide plaintext password
	hash []byte  `json:"-"` // Hide hashed password
}

type UsersStore struct {
	db *sql.DB
}

func (s *UsersStore) Create(ctx context.Context, user *User) error {
	// TODO: change later password
	// Dummy hashed password (bcrypt hash of "test12345")
	dummyHashedPassword := []byte("$2a$10$K8hURwzST/8JhP8S12vMyuPAZEKYbQfHJpY2P1q2xGmU6T9eyTxlK")
	query := `
	  INSERT INTO users (first_name, last_name, password, email, phone) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at
	`
	err := s.db.QueryRowContext(
		//TODO: change to user.Password.hash on $3
		ctx, query, user.FirstName, user.LastName, dummyHashedPassword, user.Email, user.Phone,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return err
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
