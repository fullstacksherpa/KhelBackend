package main

import (
	"context"
	"errors"
	"khel/internal/domain/featured"
	"khel/internal/params"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// -------------------- small helpers --------------------

// parseLimitOffset keeps your existing query style (limit/offset) but fills params.Pagination.
func parseLimitOffset(r *http.Request) (params.Pagination, error) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 10
	if limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v <= 0 {
			return params.Pagination{}, errors.New("invalid limit parameter")
		}
		if v > 100 {
			limit = 100
		} else {
			limit = v
		}
	}

	offset := 0
	if offsetStr != "" {
		v, err := strconv.Atoi(offsetStr)
		if err != nil || v < 0 {
			return params.Pagination{}, errors.New("invalid offset parameter")
		}
		offset = v
	}

	// If your params.Pagination expects Page, compute a best-effort page.
	page := 1
	if limit > 0 {
		page = (offset / limit) + 1
	}

	return params.Pagination{
		Page:   page,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func parsePageLimit(r *http.Request) (params.Pagination, error) {
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	if pageStr != "" {
		v, err := strconv.Atoi(pageStr)
		if err != nil || v <= 0 {
			return params.Pagination{}, errors.New("invalid page parameter")
		}
		page = v
	}

	limit := 20
	if limitStr != "" {
		v, err := strconv.Atoi(limitStr)
		if err != nil || v <= 0 {
			return params.Pagination{}, errors.New("invalid limit parameter")
		}
		if v > 100 {
			limit = 100
		} else {
			limit = v
		}
	}

	offset := (page - 1) * limit

	return params.Pagination{
		Page:   page,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// best-effort refresh: do not fail the request if refresh fails (changes are already committed).
func (app *application) tryRefreshFeaturedCache(ctx context.Context) bool {
	if err := app.store.Featured.RefreshCache(ctx); err != nil {
		app.logger.Errorw("failed to refresh featured cache", "error", err.Error())
		return false
	}
	return true
}

// -------------------- Admin Payloads (JSON) --------------------

// Create Collection Payload
type adminCreateFeaturedCollectionPayload struct {
	Key         string     `json:"key" validate:"required,max=255"`
	Title       string     `json:"title" validate:"required,max=255"`
	Type        string     `json:"type" validate:"required,max=255"`
	Description *string    `json:"description" validate:"omitempty,max=1000"`
	IsActive    *bool      `json:"is_active"`
	StartsAt    *time.Time `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at"`
}

// Update Collection Payload
type adminUpdateFeaturedCollectionPayload struct {
	Key         *string    `json:"key" validate:"omitempty,max=255"`
	Title       *string    `json:"title" validate:"omitempty,max=255"`
	Type        *string    `json:"type" validate:"omitempty,max=255"`
	Description *string    `json:"description" validate:"omitempty,max=1000"`
	IsActive    *bool      `json:"is_active"`
	StartsAt    *time.Time `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at"`
}

// Create Item Payload
type adminCreateFeaturedItemPayload struct {
	Position         int        `json:"position" validate:"min=0"`
	BadgeText        *string    `json:"badge_text" validate:"omitempty,max=255"`
	Subtitle         *string    `json:"subtitle" validate:"omitempty,max=255"`
	DealPriceCents   *int64     `json:"deal_price_cents" validate:"omitempty,min=0"`
	DealPercent      *int       `json:"deal_percent" validate:"omitempty,min=0,max=100"`
	ProductID        *int64     `json:"product_id" validate:"omitempty,gt=0"`
	ProductVariantID *int64     `json:"product_variant_id" validate:"omitempty,gt=0"`
	IsActive         *bool      `json:"is_active"`
	StartsAt         *time.Time `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
}

// Update Item Payload
type adminUpdateFeaturedItemPayload struct {
	Position         *int       `json:"position" validate:"omitempty,min=0"`
	BadgeText        *string    `json:"badge_text" validate:"omitempty,max=255"`
	Subtitle         *string    `json:"subtitle" validate:"omitempty,max=255"`
	DealPriceCents   *int64     `json:"deal_price_cents" validate:"omitempty,min=0"`
	DealPercent      *int       `json:"deal_percent" validate:"omitempty,min=0,max=100"`
	ProductID        *int64     `json:"product_id" validate:"omitempty,gt=0"`
	ProductVariantID *int64     `json:"product_variant_id" validate:"omitempty,gt=0"`
	IsActive         *bool      `json:"is_active"`
	StartsAt         *time.Time `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
}

// -------------------- Admin Handlers (Merchant) --------------------

// AdminListFeaturedCollections godoc
//
//	@Summary		List featured collections (Merchant)
//	@Description	List all featured collections with pagination and optional filters
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Param			limit	query		int						false	"Limit results (default: 10, max: 100)"
//	@Param			offset	query		int						false	"Offset results (default: 0)"
//	@Param			search	query		string					false	"Search by key/title"
//	@Param			type	query		string					false	"Filter by type"
//	@Param			active	query		bool					false	"Filter by active status"
//	@Success		200		{object}	map[string]interface{}	"Collections + pagination"
//	@Failure		400		{object}	error					"Bad Request"
//	@Failure		500		{object}	error					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/collections [get]
func (app *application) adminListFeaturedCollectionsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	p, err := parseLimitOffset(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var filters featured.CollectionFilters

	if s := r.URL.Query().Get("search"); s != "" {
		filters.Search = &s
	}
	if t := r.URL.Query().Get("type"); t != "" {
		filters.Type = &t
	}
	if a := r.URL.Query().Get("active"); a != "" {
		parsed, err := strconv.ParseBool(a)
		if err != nil {
			app.badRequestResponse(w, r, errors.New("invalid active parameter"))
			return
		}
		filters.Active = &parsed
	}

	out, err := app.store.Featured.ListCollections(ctx, p, filters)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, out)
}

// AdminGetFeaturedCollection godoc
//
//	@Summary		Get featured collection by ID (Merchant)
//	@Description	Retrieve a single featured collection by its ID
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Param			collectionID	path		int	true	"Collection ID"
//	@Success		200				{object}	featured.FeaturedCollection
//	@Failure		400				{object}	error	"Bad Request"
//	@Failure		404				{object}	error	"Not Found"
//	@Failure		500				{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/collections/{collectionID} [get]
func (app *application) adminGetFeaturedCollectionHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "collectionID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid collectionID"))
		return
	}

	c, err := app.store.Featured.GetCollectionByID(ctx, id)
	if err != nil {
		if errors.Is(err, featured.ErrNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, c)
}

// AdminCreateFeaturedCollection godoc
//
//	@Summary		Create featured collection (Merchant)
//	@Description	Create a new featured collection (source of truth). Cache refresh is best-effort.
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		adminCreateFeaturedCollectionPayload	true	"Create collection payload"
//	@Success		201		{object}	map[string]interface{}					"Created collection + cache_refreshed"
//	@Failure		400		{object}	error									"Bad Request"
//	@Failure		409		{object}	error									"Conflict (duplicate key)"
//	@Failure		500		{object}	error									"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/collections [post]
func (app *application) adminCreateFeaturedCollectionHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var payload adminCreateFeaturedCollectionPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(&payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	req := featured.CreateCollectionRequest{
		Key:         payload.Key,
		Title:       payload.Title,
		Type:        payload.Type,
		Description: payload.Description,
		IsActive:    payload.IsActive,
		StartsAt:    payload.StartsAt,
		EndsAt:      payload.EndsAt,
	}

	created, err := app.store.Featured.CreateCollection(ctx, req)
	if err != nil {
		if errors.Is(err, featured.ErrDuplicateCollectionKey) {
			app.conflictResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	cacheOK := app.tryRefreshFeaturedCache(ctx)

	app.jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"collection":      created,
		"cache_refreshed": cacheOK,
	})
}

// AdminUpdateFeaturedCollection godoc
//
//	@Summary		Update featured collection (Merchant)
//	@Description	Partially update a featured collection. Cache refresh is best-effort.
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Param			collectionID	path		int										true	"Collection ID"
//	@Param			payload			body		adminUpdateFeaturedCollectionPayload	true	"Update collection payload"
//	@Success		200				{object}	map[string]interface{}					"Updated collection + cache_refreshed"
//	@Failure		400				{object}	error									"Bad Request"
//	@Failure		404				{object}	error									"Not Found"
//	@Failure		409				{object}	error									"Conflict"
//	@Failure		500				{object}	error									"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/collections/{collectionID} [patch]
func (app *application) adminUpdateFeaturedCollectionHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "collectionID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid collectionID"))
		return
	}

	var payload adminUpdateFeaturedCollectionPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(&payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	req := featured.UpdateCollectionRequest{
		Key:         payload.Key,
		Title:       payload.Title,
		Type:        payload.Type,
		Description: payload.Description,
		IsActive:    payload.IsActive,
		StartsAt:    payload.StartsAt,
		EndsAt:      payload.EndsAt,
	}

	updated, err := app.store.Featured.UpdateCollection(ctx, id, req)
	if err != nil {
		if errors.Is(err, featured.ErrNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		if errors.Is(err, featured.ErrNoFieldsToUpdate) {
			app.badRequestResponse(w, r, err)
			return
		}
		if errors.Is(err, featured.ErrDuplicateCollectionKey) {
			app.conflictResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	cacheOK := app.tryRefreshFeaturedCache(ctx)

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"collection":      updated,
		"cache_refreshed": cacheOK,
	})
}

// AdminDeleteFeaturedCollection godoc
//
//	@Summary		Delete featured collection (Merchant)
//	@Description	Deletes a featured collection (items will cascade). Cache refresh is best-effort.
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Param			collectionID	path		int						true	"Collection ID"
//	@Success		200				{object}	map[string]interface{}	"Deleted + cache_refreshed"
//	@Failure		400				{object}	error					"Bad Request"
//	@Failure		404				{object}	error					"Not Found"
//	@Failure		500				{object}	error					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/collections/{collectionID} [delete]
func (app *application) adminDeleteFeaturedCollectionHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "collectionID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid collectionID"))
		return
	}

	if err := app.store.Featured.DeleteCollection(ctx, id); err != nil {
		if errors.Is(err, featured.ErrNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	cacheOK := app.tryRefreshFeaturedCache(ctx)

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"message":         "collection deleted successfully",
		"cache_refreshed": cacheOK,
	})
}

// AdminListFeaturedItems godoc
//
//	@Summary		List featured items in a collection (Merchant)
//	@Description	List items inside a collection ordered by position
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Param			collectionID	path		int		true	"Collection ID"
//	@Param			limit			query		int		false	"Limit results (default: 10, max: 100)"
//	@Param			offset			query		int		false	"Offset results (default: 0)"
//	@Param			active			query		bool	false	"Filter by item active status"
//	@Success		200				{object}	featured.ItemList
//	@Failure		400				{object}	error	"Bad Request"
//	@Failure		500				{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/collections/{collectionID}/items [get]
func (app *application) adminListFeaturedItemsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	collectionIDStr := chi.URLParam(r, "collectionID")
	collectionID, err := strconv.ParseInt(collectionIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid collectionID"))
		return
	}

	p, err := parseLimitOffset(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var filters featured.ItemFilters
	if a := r.URL.Query().Get("active"); a != "" {
		parsed, err := strconv.ParseBool(a)
		if err != nil {
			app.badRequestResponse(w, r, errors.New("invalid active parameter"))
			return
		}
		filters.Active = &parsed
	}

	out, err := app.store.Featured.ListItemsByCollection(ctx, collectionID, p, filters)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, out)
}

// AdminCreateFeaturedItem godoc
//
//	@Summary		Create featured item (Merchant)
//	@Description	Add a new item inside a collection. Cache refresh is best-effort.
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Param			collectionID	path		int								true	"Collection ID"
//	@Param			payload			body		adminCreateFeaturedItemPayload	true	"Create item payload"
//	@Success		201				{object}	map[string]interface{}			"Created item + cache_refreshed"
//	@Failure		400				{object}	error							"Bad Request"
//	@Failure		409				{object}	error							"Conflict (duplicate position)"
//	@Failure		500				{object}	error							"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/collections/{collectionID}/items [post]
func (app *application) adminCreateFeaturedItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	collectionIDStr := chi.URLParam(r, "collectionID")
	collectionID, err := strconv.ParseInt(collectionIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid collectionID"))
		return
	}

	var payload adminCreateFeaturedItemPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if err := Validate.Struct(&payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if payload.ProductID == nil && payload.ProductVariantID == nil {
		app.badRequestResponse(w, r, errors.New("either product_id or product_variant_id is required"))
		return
	}

	req := featured.CreateItemRequest{
		CollectionID:     collectionID,
		Position:         payload.Position,
		BadgeText:        payload.BadgeText,
		Subtitle:         payload.Subtitle,
		DealPriceCents:   payload.DealPriceCents,
		DealPercent:      payload.DealPercent,
		ProductID:        payload.ProductID,
		ProductVariantID: payload.ProductVariantID,
		IsActive:         payload.IsActive,
		StartsAt:         payload.StartsAt,
		EndsAt:           payload.EndsAt,
	}

	created, err := app.store.Featured.CreateItem(ctx, req)
	if err != nil {
		if errors.Is(err, featured.ErrDuplicateItemPosition) {
			app.conflictResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	cacheOK := app.tryRefreshFeaturedCache(ctx)

	app.jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"item":            created,
		"cache_refreshed": cacheOK,
	})
}

// AdminUpdateFeaturedItem godoc
//
//	@Summary		Update featured item (Merchant)
//	@Description	Partially update a featured item. Cache refresh is best-effort.
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Param			itemID	path		int								true	"Item ID"
//	@Param			payload	body		adminUpdateFeaturedItemPayload	true	"Update item payload"
//	@Success		200		{object}	map[string]interface{}			"Updated item + cache_refreshed"
//	@Failure		400		{object}	error							"Bad Request"
//	@Failure		404		{object}	error							"Not Found"
//	@Failure		409		{object}	error							"Conflict"
//	@Failure		500		{object}	error							"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/items/{itemID} [patch]
func (app *application) adminUpdateFeaturedItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	itemIDStr := chi.URLParam(r, "itemID")
	itemID, err := strconv.ParseInt(itemIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid itemID"))
		return
	}

	var payload adminUpdateFeaturedItemPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if err := Validate.Struct(&payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	req := featured.UpdateItemRequest{
		Position:         payload.Position,
		BadgeText:        payload.BadgeText,
		Subtitle:         payload.Subtitle,
		DealPriceCents:   payload.DealPriceCents,
		DealPercent:      payload.DealPercent,
		ProductID:        payload.ProductID,
		ProductVariantID: payload.ProductVariantID,
		IsActive:         payload.IsActive,
		StartsAt:         payload.StartsAt,
		EndsAt:           payload.EndsAt,
	}

	updated, err := app.store.Featured.UpdateItem(ctx, itemID, req)
	if err != nil {
		if errors.Is(err, featured.ErrItemNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		if errors.Is(err, featured.ErrNoFieldsToUpdate) {
			app.badRequestResponse(w, r, err)
			return
		}
		if errors.Is(err, featured.ErrDuplicateItemPosition) {
			app.conflictResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	cacheOK := app.tryRefreshFeaturedCache(ctx)

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"item":            updated,
		"cache_refreshed": cacheOK,
	})
}

// AdminDeleteFeaturedItem godoc
//
//	@Summary		Delete featured item (Merchant)
//	@Description	Deletes a featured item. Cache refresh is best-effort.
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Param			itemID	path		int						true	"Item ID"
//	@Success		200		{object}	map[string]interface{}	"Deleted + cache_refreshed"
//	@Failure		400		{object}	error					"Bad Request"
//	@Failure		404		{object}	error					"Not Found"
//	@Failure		500		{object}	error					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/items/{itemID} [delete]
func (app *application) adminDeleteFeaturedItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	itemIDStr := chi.URLParam(r, "itemID")
	itemID, err := strconv.ParseInt(itemIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid itemID"))
		return
	}

	if err := app.store.Featured.DeleteItem(ctx, itemID); err != nil {
		if errors.Is(err, featured.ErrItemNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	cacheOK := app.tryRefreshFeaturedCache(ctx)

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"message":         "item deleted successfully",
		"cache_refreshed": cacheOK,
	})
}

// AdminRefreshFeaturedCache godoc
//
//	@Summary		Refresh featured collections cache (Merchant)
//	@Description	Refreshes the materialized view featured_collections_cache (CONCURRENTLY)
//	@Tags			Merchant
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string	"Cache refreshed"
//	@Failure		500	{object}	error				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/featured/refresh-cache [post]
func (app *application) adminRefreshFeaturedCacheHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := app.store.Featured.RefreshCache(ctx); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "featured cache refreshed"})
}

// -------------------- Public App Handlers --------------------

// GetHomeFeaturedCollections godoc
//
//	@Summary		Get home featured collections
//	@Description	Retrieves all active featured collections with up to 10 items each (from cache table/MV; may be slightly stale)
//	@Tags			Featured
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Home rails collections"
//	@Failure		500	{object}	error					"Internal Server Error"
//	@Router			/store/featured/home [get]
func (app *application) getHomeFeaturedCollectionsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	collections, err := app.store.Featured.GetHomeCollections(ctx)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"collections": collections,
	})
}

// GetFeaturedCollectionItems godoc
//
//	@Summary		Get featured collection items
//	@Description	Retrieves a featured collection by key and paginated items (from cache/MV; may be slightly stale)
//	@Tags			Featured
//	@Accept			json
//	@Produce		json
//	@Param			collectionKey	path		string	true	"Collection key"
//	@Param			page			query		int		false	"Page number (default: 1)"
//	@Param			limit			query		int		false	"Limit per page (default: 20, max: 100)"
//	@Success		200				{object}	featured.CollectionDetail
//	@Failure		400				{object}	error	"Bad Request"
//	@Failure		404				{object}	error	"Not Found"
//	@Failure		500				{object}	error	"Internal Server Error"
//	@Router			/store/featured/collections/{collectionKey} [get]
func (app *application) getFeaturedCollectionItemsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	collectionKey := chi.URLParam(r, "collectionKey")
	if collectionKey == "" {
		app.badRequestResponse(w, r, errors.New("collectionKey is required"))
		return
	}

	p, err := parsePageLimit(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	out, err := app.store.Featured.GetCollectionItems(ctx, collectionKey, p)
	if err != nil {
		if errors.Is(err, featured.ErrNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, out)
}
