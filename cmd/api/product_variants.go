package main

import (
	"fmt"
	"khel/internal/domain/products"
	"khel/internal/params"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// GetVariantByID (user)
func (app *application) getVariantHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid variant id"))
		return
	}

	variant, err := app.store.Products.GetVariantByID(ctx, id)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if variant == nil || !variant.IsActive {
		app.notFoundResponse(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, variant)
}

// List all variants for a given product (user)
func (app *application) listVariantsByProductHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	productIDStr := chi.URLParam(r, "product_id")
	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid product id"))
		return
	}

	// ✅ query param: include_inactive=true to return ALL variants (admin use)
	includeInactive := false
	if v := strings.TrimSpace(r.URL.Query().Get("include_inactive")); v != "" {
		// accepts: true/false/1/0
		b, err := strconv.ParseBool(v)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("invalid include_inactive"))
			return
		}
		includeInactive = b
	}

	variants, err := app.store.Products.ListVariantsByProduct(ctx, productID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// default: filter only active variants for customers
	filtered := make([]*products.ProductVariant, 0, len(variants))
	if includeInactive {
		filtered = variants
	} else {
		for _, v := range variants {
			if v.IsActive {
				filtered = append(filtered, v)
			}
		}
	}

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"variants": filtered,
		"count":    len(filtered),
	})
}

func (app *application) createVariantHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var input struct {
		ProductID  int64                  `json:"product_id"`
		PriceCents int64                  `json:"price_cents"`
		Attributes map[string]interface{} `json:"attributes"`
		IsActive   bool                   `json:"is_active"`
	}

	if err := readJSON(w, r, &input); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.ProductID == 0 {
		app.badRequestResponse(w, r, fmt.Errorf("product_id is required"))
		return
	}
	if input.PriceCents < 0 {
		app.badRequestResponse(w, r, fmt.Errorf("price_cents must be >= 0"))
		return
	}

	variant := &products.ProductVariant{
		ProductID:  input.ProductID,
		PriceCents: input.PriceCents,
		Attributes: input.Attributes,
		IsActive:   input.IsActive,
	}

	created, err := app.store.Products.CreateVariant(ctx, variant)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to create variant: %w", err))
		return
	}

	app.jsonResponse(w, http.StatusCreated, created)
}

func (app *application) updateVariantHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid variant id"))
		return
	}

	existing, err := app.store.Products.GetVariantByID(ctx, id)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if existing == nil {
		app.notFoundResponse(w, r, err)
		return
	}

	var input struct {
		PriceCents *int64                 `json:"price_cents,omitempty"`
		Attributes map[string]interface{} `json:"attributes,omitempty"`
		IsActive   *bool                  `json:"is_active,omitempty"`
	}
	if err := readJSON(w, r, &input); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.PriceCents != nil {
		existing.PriceCents = *input.PriceCents
	}
	if input.Attributes != nil {
		existing.Attributes = input.Attributes
	}
	if input.IsActive != nil {
		existing.IsActive = *input.IsActive
	}

	if err := app.store.Products.UpdateVariant(ctx, existing); err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to update variant: %w", err))
		return
	}

	app.jsonResponse(w, http.StatusOK, existing)
}

func (app *application) deleteVariantHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid variant id"))
		return
	}

	if err := app.store.Products.DeleteVariant(ctx, id); err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to delete variant: %w", err))
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "variant deleted"})
}

// for admin, it show all variants even notActive
func (app *application) listAllVariantsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pagination := params.ParsePagination(r.URL.Query())

	variants, total, err := app.store.Products.ListAllVariants(ctx, pagination.Limit, pagination.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	pagination.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"variants":   variants,
		"pagination": pagination,
	})
}
