package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// ---------- REQUEST TYPES ----------

type assignRoleRequest struct {
	RoleID int64 `json:"role_id"` // ID from roles table
}

// AdminAssignUserRole godoc
//
//	@Summary		Assign a role to a user
//	@Description	Assigns a role (by role_id) to the specified user.
//	@Tags			superadmin-role
//	@Accept			json
//	@Produce		json
//	@Param			userID	path		int					true	"User ID"
//	@Param			body	body		assignRoleRequest	true	"Role assignment payload"
//	@Success		200		{object}	map[string]string	"Role assigned successfully"
//	@Failure		400		{object}	error				"Bad Request"
//	@Failure		500		{object}	error				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/superadmin/{userID}/roles [post]
func (app *application) adminAssignUserRoleHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid userID"))
		return
	}

	var in assignRoleRequest
	if err := readJSON(w, r, &in); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if in.RoleID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("Invalid request"))
		return
	}

	if err := app.store.AccessControl.AssignRole(ctx, userID, in.RoleID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{
		"message": "role assigned",
	})
}

// AdminRemoveUserRole godoc
//
//	@Summary		Remove a role from a user
//	@Description	Removes a specific role from a user by role_id.
//	@Tags			superadmin-role
//	@Produce		json
//	@Param			userID	path		int					true	"User ID"
//	@Param			roleID	path		int					true	"Role ID"
//	@Success		200		{object}	map[string]string	"Role removed successfully"
//	@Failure		400		{object}	error				"Bad Request"
//	@Failure		404		{object}	error				"Role not found for user"
//	@Failure		500		{object}	error				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/superadmin/{userID}/roles/{roleID} [delete]
func (app *application) adminRemoveUserRoleHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid userid"))
		return
	}

	roleIDStr := chi.URLParam(r, "roleID")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil || roleID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid roleid"))
		return
	}

	if err := app.store.AccessControl.RemoveRole(ctx, userID, roleID); err != nil {
		// repo returns fmt.Errorf("role not found...") when no row is affected and we can map that to 404
		if strings.Contains(err.Error(), "role not found") {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{
		"message": "role removed",
	})
}

// AdminGetUserRoles godoc
//
//	@Summary		List roles for a user
//	@Description	Returns all roles assigned to the given user.
//	@Tags			superadmin-role
//	@Produce		json
//	@Param			userID	path		int	true	"User ID"
//	@Success		200		{array}		accesscontrol.Role
//	@Failure		400		{object}	error	"Bad Request"
//	@Failure		500		{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/superadmin/{userID}/roles [get]
func (app *application) adminGetUserRolesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid userid"))
		return
	}

	roles, err := app.store.AccessControl.GetUserRoles(ctx, userID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, roles)
}
