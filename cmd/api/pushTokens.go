package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// SavePushTokenRequest represents the payload for saving/updating a push token
type SavePushTokenRequest struct {
	Token      string          `json:"token" validate:"required"`
	DeviceInfo json.RawMessage `json:"device_info"`
}

// RemovePushTokenRequest represents the payload for removing a push token
type RemovePushTokenRequest struct {
	Token string `json:"token" validate:"required"`
}

// BulkRemoveTokensRequest represents the payload for bulk token removal
type BulkRemoveTokensRequest struct {
	Tokens []string `json:"tokens" validate:"required,min=1"`
}

// PruneStaleTokensRequest represents the payload for pruning stale tokens
type PruneStaleTokensRequest struct {
	OlderThan string `json:"older_than" validate:"required"`
}

func (p *PruneStaleTokensRequest) Duration() (time.Duration, error) {
	return time.ParseDuration(p.OlderThan)
}

/*
for PruneStaleTokensRequests body type is this

{"older_than": "1680h"}. which mean 70 days
*/

// SavePushToken godoc
//
//	@Summary		Save or update a push notification token
//	@Description	Stores or updates a user's Expo push token along with optional device info
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			payload	body	SavePushTokenRequest	true	"Push token data"
//	@Success		204
//	@Failure		400	{object}	error	"Bad Request"
//	@Failure		401	{object}	error	"Unauthorized"
//	@Failure		500	{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/users/push-tokens [post]
func (app *application) savePushTokenHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	if user == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("unauthorized request"))
		return
	}

	var payload SavePushTokenRequest
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.store.PushTokens.AddOrUpdatePushToken(r.Context(), user.ID, payload.Token, payload.DeviceInfo); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemovePushToken godoc
//
//	@Summary		Remove a push notification token
//	@Description	Deletes a specific push token for the current user
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			payload	body	RemovePushTokenRequest	true	"Token to remove"
//	@Success		204
//	@Failure		400	{object}	error	"Bad Request"
//	@Failure		401	{object}	error	"Unauthorized"
//	@Failure		500	{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/push-tokens [delete]
func (app *application) removePushTokenHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	if user == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("unauthorized request"))
		return
	}

	var payload RemovePushTokenRequest
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.store.PushTokens.RemovePushToken(r.Context(), user.ID, payload.Token); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// BulkRemoveTokens godoc
//
//	@Summary		Bulk remove push notification tokens
//	@Description	Deletes multiple push tokens (admin-only)
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			payload	body	BulkRemoveTokensRequest	true	"Tokens to remove"
//	@Success		204
//	@Failure		400	{object}	error	"Bad Request"
//	@Failure		401	{object}	error	"Unauthorized"
//	@Failure		403	{object}	error	"Forbidden"
//	@Failure		500	{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/users/push-tokens/bulk-remove [post]
func (app *application) bulkRemoveTokensHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	if user == nil {
		app.forbiddenResponse(w, r)
		return
	}

	if user.Email != "ongchen10sherpa@gmail.com" {
		app.forbiddenResponse(w, r)
		return
	}

	var payload BulkRemoveTokensRequest
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.store.PushTokens.RemoveTokensByTokenList(r.Context(), payload.Tokens); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PruneStaleTokens godoc
//
//	@Summary		Prune stale push tokens
//	@Description	Removes push tokens not updated within the specified duration (admin-only)
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			payload	body	PruneStaleTokensRequest	true	"Duration criteria"
//	@Success		204
//	@Failure		400	{object}	error	"Bad Request"
//	@Failure		401	{object}	error	"Unauthorized"
//	@Failure		403	{object}	error	"Forbidden"
//	@Failure		500	{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/users/push-tokens/prune [post]
func (app *application) pruneStaleTokensHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	if user == nil {
		app.forbiddenResponse(w, r)
		return
	}

	if user.Email != "ongchen10sherpa@gmail.com" {
		app.forbiddenResponse(w, r)
		return
	}

	var payload PruneStaleTokensRequest

	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	dur, err := payload.Duration()
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.store.PushTokens.PruneStaleTokens(r.Context(), dur); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
