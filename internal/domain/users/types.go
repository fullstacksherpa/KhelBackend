package users

import (
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound             = errors.New("resource not found")
	ErrConflict             = errors.New("resource already exists")
	ErrDuplicateEmail       = errors.New("a user with that email already exists")
	ErrDuplicatePhoneNumber = errors.New("a user with that phone number already exists")
	QueryTimeoutDuration    = time.Second * 5
)

type User struct {
	ID                   int64          `json:"id"`
	FirstName            string         `json:"first_name"`
	LastName             string         `json:"last_name"`
	Email                string         `json:"email"`
	Phone                string         `json:"phone"`
	Password             password       `json:"-"` // Hide password
	ProfilePictureURL    sql.NullString `json:"profile_picture_url" swaggertype:"string"`
	SkillLevel           sql.NullString `json:"skill_level" swaggertype:"string"`
	NoOfGames            sql.NullInt16  `json:"no_of_games" swaggertype:"integer"`
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
