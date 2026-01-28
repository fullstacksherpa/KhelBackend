package users

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"khel/internal/database"

	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	GetByID(context.Context, int64) (*User, error)
	GetByEmail(context.Context, string) (*User, error)
	Create(ctx context.Context, tx pgx.Tx, user *User) error
	CreateAndInvite(ctx context.Context, user *User, token string, exp time.Duration) error
	Activate(context.Context, string) error
	Delete(context.Context, int64) error
	SetProfile(context.Context, string, int64) error
	GetProfileUrl(context.Context, int64) (*string, error)
	UpdateUser(context.Context, int64, map[string]interface{}) error
	SaveRefreshToken(ctx context.Context, userID int64, refreshToken string) error
	DeleteRefreshToken(ctx context.Context, userID int64) error
	GetRefreshToken(ctx context.Context, userID int64) (string, error)
	UpdateResetToken(ctx context.Context, email, resetToken string, resetTokenExpires time.Time) error
	GetByResetToken(ctx context.Context, resetToken string) (*User, error)
	Update(ctx context.Context, user *User) error
	UpdateAndUpload(ctx context.Context, userID int64, updates map[string]interface{}, profilePictureURL *string) error
	ListAdminUsers(ctx context.Context, filters AdminListUsersFilters, limit, offset int) ([]AdminUserRow, int, error)
	GetAdminUserStats(ctx context.Context, userID int64) (*AdminUserStatsRow, error)
	AdminCreateUser(ctx context.Context, user *User) (*User, error)
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

func (r *Repository) GetAdminUserStats(ctx context.Context, userID int64) (*AdminUserStatsRow, error) {
	var s AdminUserStatsRow

	err := r.db.QueryRow(ctx, `
SELECT
  (SELECT COUNT(*) FROM orders   WHERE user_id = $1) AS orders_count,
  (SELECT COUNT(*) FROM bookings WHERE user_id = $1) AS bookings_count,
  (SELECT COUNT(*) FROM game_players WHERE user_id = $1) AS games_count,

  (SELECT COALESCE(SUM(total_cents),0)
   FROM orders
   WHERE user_id = $1 AND payment_status = 'paid') AS total_spent_cents,

  (SELECT MAX(created_at) FROM orders   WHERE user_id = $1) AS last_order_at,
  (SELECT MAX(created_at) FROM bookings WHERE user_id = $1) AS last_booking_at,

  -- last_game_at: latest game start_time the user is in
  (SELECT MAX(g.start_time)
   FROM game_players gp
   JOIN games g ON g.id = gp.game_id
   WHERE gp.user_id = $1) AS last_game_at
`, userID).Scan(
		&s.OrdersCount,
		&s.BookingsCount,
		&s.GamesCount,
		&s.TotalSpentCents,
		&s.LastOrderAt,
		&s.LastBookingAt,
		&s.LastGameAt,
	)

	if err != nil {
		return nil, fmt.Errorf("admin user stats: %w", err)
	}

	return &s, nil
}

func (r *Repository) Create(ctx context.Context, tx pgx.Tx, user *User) error {

	query := `
	  INSERT INTO users (first_name, last_name, password, email, phone) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	err := tx.QueryRow(
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

func (r *Repository) SetProfile(ctx context.Context, url string, userID int64) error {
	query := `UPDATE users SET profile_picture_url = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, url, userID)
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetProfileUrl(ctx context.Context, userID int64) (*string, error) {
	var old pgtype.Text
	query := `SELECT profile_picture_url FROM users WHERE id = $1`

	err := r.db.QueryRow(ctx, query, userID).Scan(&old)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to retrieve profile picture URL: %w", err)
	}

	if !old.Valid {
		return nil, nil // keep NULL
	}
	v := old.String
	return &v, nil
}

func (r *Repository) UpdateUser(ctx context.Context, userID int64, updates map[string]interface{}) error {
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

	_, err := r.db.Exec(ctx, query, args...)
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

func (r *Repository) GetByID(ctx context.Context, userID int64) (*User, error) {
	query := `
		SELECT
			id,
			first_name,
			last_name,
			email,
			phone,
			password,
			profile_picture_url,
			skill_level,
			no_of_games,
			is_active,
			created_at,
			updated_at
		FROM users
		WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	user := &User{}

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.FirstName,
		&user.LastName,
		&user.Email,
		&user.Phone,
		&user.Password.hash,
		&user.ProfilePictureURL,
		&user.SkillLevel,
		&user.NoOfGames,
		&user.IsActive,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return user, nil
}

func (r *Repository) CreateAndInvite(ctx context.Context, user *User, token string, invitationExp time.Duration) error {
	return database.WithTx(r.db, ctx, func(tx pgx.Tx) error {
		if err := r.Create(ctx, tx, user); err != nil {
			return err
		}

		if err := r.createUserInvitation(ctx, tx, token, invitationExp, user.ID); err != nil {
			return err
		}

		return nil
	})
}

func (r *Repository) createUserInvitation(ctx context.Context, tx pgx.Tx, token string, exp time.Duration, userID int64) error {
	query := `INSERT INTO user_invitations (token, user_id, expiry) VALUES ($1, $2, $3)`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := tx.Exec(ctx, query, token, userID, time.Now().Add(exp))
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) Activate(ctx context.Context, token string) error {
	return database.WithTx(r.db, ctx, func(tx pgx.Tx) error {
		user, err := r.getUserFromInvitation(ctx, tx, token)
		if err != nil {
			return err
		}

		// ✅ idempotent: already active => success
		if user.IsActive {
			return nil
		}

		// activate
		user.IsActive = true
		if err := r.updateActiveOnly(ctx, tx, user.ID); err != nil {
			return err
		}

		// ❌ do NOT delete invitations here
		return nil
	})
}

func (r *Repository) updateActiveOnly(ctx context.Context, tx pgx.Tx, userID int64) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := tx.Exec(ctx, `UPDATE users SET is_active = TRUE WHERE id = $1`, userID)
	return err
}

func (r *Repository) getUserFromInvitation(ctx context.Context, tx pgx.Tx, token string) (*User, error) {
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
	err := tx.QueryRow(ctx, query, hashToken, time.Now()).Scan(
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

func (r *Repository) update(ctx context.Context, tx pgx.Tx, user *User) error {
	query := `UPDATE users SET  email = $1, is_active = $2 WHERE id = $3`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := tx.Exec(ctx, query, user.Email, user.IsActive, user.ID)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) deleteUserInvitations(ctx context.Context, tx pgx.Tx, userID int64) error {
	query := `DELETE FROM user_invitations WHERE user_id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := tx.Exec(ctx, query, userID)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) Delete(ctx context.Context, userID int64) error {
	return database.WithTx(r.db, ctx, func(tx pgx.Tx) error {
		if err := r.delete(ctx, tx, userID); err != nil {
			return err
		}

		if err := r.deleteUserInvitations(ctx, tx, userID); err != nil {
			return err
		}

		return nil
	})
}

func (r *Repository) delete(ctx context.Context, tx pgx.Tx, id int64) error {
	query := `DELETE FROM users WHERE id = $1`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := tx.Exec(ctx, query, id)
	if err != nil {
		return err
	}

	return nil
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, first_name, phone, email, password, created_at FROM users
		WHERE email = $1 AND is_active = true
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	user := &User{}
	err := r.db.QueryRow(ctx, query, email).Scan(
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

func (r *Repository) SaveRefreshToken(ctx context.Context, userID int64, refreshToken string) error {
	query := `UPDATE users SET refresh_token = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, refreshToken, userID)
	if err != nil {
		return fmt.Errorf("failed to save refresh token: %w", err)
	}
	return nil
}

func (r *Repository) DeleteRefreshToken(ctx context.Context, userID int64) error {
	query := `UPDATE users SET refresh_token = NULL, updated_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete refresh token: %w", err)
	}
	return nil
}

// GetRefreshToken retrieves the refresh token for a specific user from the database.
func (r *Repository) GetRefreshToken(ctx context.Context, userID int64) (string, error) {
	var refreshToken string

	// Query to retrieve the refresh token for the given userID
	query := `SELECT refresh_token FROM users WHERE id = $1`
	err := r.db.QueryRow(ctx, query, userID).Scan(&refreshToken)

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

func (r *Repository) UpdateResetToken(ctx context.Context, email, resetToken string, resetTokenExpires time.Time) error {
	query := `
        UPDATE users
        SET reset_password_token = $1, reset_password_expires = $2
        WHERE email = $3
    `
	_, err := r.db.Exec(ctx, query, resetToken, resetTokenExpires, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return err
		}
		return err
	}
	return nil
}

func (r *Repository) GetByResetToken(ctx context.Context, resetToken string) (*User, error) {
	query := `
        SELECT id, first_name, last_name, email, phone, password, profile_picture_url, skill_level, no_of_games, is_active, reset_password_token, reset_password_expires, created_at, updated_at
        FROM users
        WHERE reset_password_token = $1
    `
	var user User
	err := r.db.QueryRow(ctx, query, resetToken).Scan(
		&user.ID, &user.FirstName, &user.LastName, &user.Email, &user.Phone, &user.Password.hash, &user.ProfilePictureURL, &user.SkillLevel, &user.NoOfGames, &user.IsActive, &user.ResetPasswordToken, &user.ResetPasswordExpires, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}
	return &user, nil
}

// Update updates a user's details in the database.
func (r *Repository) Update(ctx context.Context, user *User) error {
	query := `
        UPDATE users
        SET 
            first_name = $1,
            last_name = $2,
            email = $3,
            phone = $4,
            password = $5,
            profile_picture_url = $6,
            skill_level = $7,
            no_of_games = $8,
            refresh_token = $9,
            is_active = $10,
            reset_password_token = $11,
            reset_password_expires = $12,
            updated_at = $13
        WHERE id = $14
    `
	args := []interface{}{
		user.FirstName,
		user.LastName,
		user.Email,
		user.Phone,
		user.Password.hash, // Use the hashed password
		user.ProfilePictureURL,
		user.SkillLevel,
		user.NoOfGames,
		user.RefreshToken,
		user.IsActive,
		user.ResetPasswordToken,
		user.ResetPasswordExpires,
		time.Now(), // Update the `updated_at` field
		user.ID,
	}

	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}

	return nil
}

// UpdateAndUpload updates arbitrary user fields + profile_picture_url in one TX.
func (r *Repository) UpdateAndUpload(
	ctx context.Context,
	userID int64,
	updates map[string]interface{},
	profilePictureURL *string, // <-- pointer: nil means "leave as-is"
) error {
	return database.WithTx(r.db, ctx, func(tx pgx.Tx) error {
		// 1) update other fields
		if len(updates) > 0 {
			if lvl, ok := updates["skill_level"]; ok {
				valid := map[string]bool{"beginner": true, "intermediate": true, "advanced": true}
				if !valid[lvl.(string)] {
					return fmt.Errorf("invalid skill_level: %s", lvl)
				}
			}

			setClauses := []string{}
			args := []interface{}{}
			i := 1
			for col, val := range updates {
				if !isValidField(col) {
					return fmt.Errorf("invalid field name: %s", col)
				}
				setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, i))
				args = append(args, val)
				i++
			}

			args = append(args, userID)
			query := fmt.Sprintf(
				"UPDATE users SET %s, updated_at = NOW() WHERE id = $%d",
				strings.Join(setClauses, ", "),
				i,
			)
			if _, err := tx.Exec(ctx, query, args...); err != nil {
				return fmt.Errorf("update fields failed: %w", err)
			}
		}

		// 2) update picture only if caller provided it
		if profilePictureURL != nil {
			q2 := `UPDATE users SET profile_picture_url = $1, updated_at = NOW() WHERE id = $2`
			if _, err := tx.Exec(ctx, q2, *profilePictureURL, userID); err != nil {
				return fmt.Errorf("update profile picture failed: %w", err)
			}
		}

		return nil
	})
}

func (r *Repository) ListAdminUsers(ctx context.Context, filters AdminListUsersFilters, limit, offset int) ([]AdminUserRow, int, error) {
	// Important: role filter uses EXISTS so that:
	// - we filter users by role if provided
	// - but still aggregate ALL roles the user has
	role := filters.Role

	// 1) total count (distinct users)
	countQ := `
		SELECT COUNT(*)
		FROM users u
		WHERE ($1 = '' OR EXISTS (
			SELECT 1
			FROM user_roles ur
			JOIN roles rr ON rr.id = ur.role_id
			WHERE ur.user_id = u.id AND rr.name = $1
		))
	`
	var total int
	if err := r.db.QueryRow(ctx, countQ, role).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// 2) page query with roles aggregation
	// NOTE: ORDER BY created_at desc like admin listings usually do.
	listQ := `
		SELECT
			u.id,
			u.first_name,
			u.last_name,
			u.email,
			u.phone,
			u.profile_picture_url,
			u.skill_level,
			COALESCE(u.no_of_games, 0) AS no_of_games,
			u.is_active,
			u.created_at,
			u.updated_at,
			COALESCE(array_remove(array_agg(r.name ORDER BY r.name), NULL), '{}'::text[]) AS roles
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		WHERE ($1 = '' OR EXISTS (
			SELECT 1
			FROM user_roles ur2
			JOIN roles rr2 ON rr2.id = ur2.role_id
			WHERE ur2.user_id = u.id AND rr2.name = $1
		))
		GROUP BY u.id
		ORDER BY u.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, listQ, role, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	out := make([]AdminUserRow, 0, limit)
	for rows.Next() {
		var (
			u             AdminUserRow
			picNullable   *string
			skillNullable *string
		)

		// Because  DB column types might be nullable, scan into *string for url/skill
		// If your columns are TEXT NULL, pgx can scan into *string directly.
		if err := rows.Scan(
			&u.ID,
			&u.FirstName,
			&u.LastName,
			&u.Email,
			&u.Phone,
			&picNullable,
			&skillNullable,
			&u.NoOfGames,
			&u.IsActive,
			&u.CreatedAt,
			&u.UpdatedAt,
			&u.Roles,
		); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}

		u.ProfilePictureURL = picNullable
		u.SkillLevel = skillNullable

		out = append(out, u)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}

	return out, total, nil
}

func (r *Repository) AdminCreateUser(ctx context.Context, user *User) (*User, error) {
	// Admin-created users are active by default
	user.IsActive = true

	query := `
		INSERT INTO users (
			first_name,
			last_name,
			email,
			phone,
			password,
			profile_picture_url,
			skill_level,
			no_of_games,
			is_active
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING
			id,
			created_at,
			updated_at
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	err := r.db.QueryRow(
		ctx,
		query,
		user.FirstName,
		user.LastName,
		user.Email,
		user.Phone,
		user.Password.hash,
		user.ProfilePictureURL,
		user.SkillLevel,
		user.NoOfGames,
		user.IsActive,
	).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		switch {
		case strings.Contains(err.Error(), "users_email_key"):
			return nil, ErrDuplicateEmail
		case strings.Contains(err.Error(), "users_phone_key"):
			return nil, ErrDuplicatePhoneNumber
		default:
			return nil, fmt.Errorf("admin create user: %w", err)
		}
	}

	return user, nil
}
