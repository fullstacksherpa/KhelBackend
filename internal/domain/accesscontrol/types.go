package accesscontrol

import "time"

type RoleName string

const (
	RoleAdmin    RoleName = "admin"
	RoleOwner    RoleName = "owner"
	RoleCustomer RoleName = "customer"
	RoleMerchant RoleName = "merchant"
)

type Role struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserRole struct {
	UserID     int64     `json:"user_id"`
	RoleID     int64     `json:"role_id"`
	AssignedAt time.Time `json:"assigned_at"`
}
