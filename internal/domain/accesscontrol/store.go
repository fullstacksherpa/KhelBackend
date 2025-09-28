package accesscontrol

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	AssignRole(ctx context.Context, userID, roleID int64) error
	RemoveRole(ctx context.Context, userID, roleID int64) error
	GetUserRoles(ctx context.Context, userID int64) ([]Role, error)
	UserHasRole(ctx context.Context, userID int64, roleName string) (bool, error)
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

func (r *Repository) AssignRole(ctx context.Context, userID, roleID int64) error {
	query := `
        INSERT INTO user_roles (user_id, role_id)
        VALUES ($1, $2)
        ON CONFLICT DO NOTHING
    `
	_, err := r.db.Exec(ctx, query, userID, roleID)
	return err
}

func (r *Repository) RemoveRole(ctx context.Context, userID, roleID int64) error {
	query := `DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`
	result, err := r.db.Exec(ctx, query, userID, roleID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("role not found for user_id=%d role_id=%d", userID, roleID)
	}
	return nil
}

func (r *Repository) GetUserRoles(ctx context.Context, userID int64) ([]Role, error) {
	query := `
        SELECT r.id, r.name, r.description, r.created_at, r.updated_at
        FROM roles r
        JOIN user_roles ur ON ur.role_id = r.id
        WHERE ur.user_id = $1
    `
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (r *Repository) UserHasRole(ctx context.Context, userID int64, roleName string) (bool, error) {
	var exists bool
	query := `
        SELECT EXISTS (
            SELECT 1
            FROM user_roles ur
            JOIN roles r ON ur.role_id = r.id
            WHERE ur.user_id = $1 AND r.name = $2
        )
    `
	err := r.db.QueryRow(ctx, query, userID, roleName).Scan(&exists)
	return exists, err
}
