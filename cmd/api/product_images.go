package main

import (
	"context"
	"fmt"
	"khel/internal/domain/products"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func (app *application) createProductImageHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	const maxBytes = 8 * 1024 * 1024 // 8MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}

	productIDStr := r.FormValue("product_id")
	variantIDStr := r.FormValue("product_variant_id")
	isPrimaryStr := r.FormValue("is_primary")
	sortOrderStr := r.FormValue("sort_order")
	alt := strings.TrimSpace(r.FormValue("alt"))

	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil || productID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid product_id"))
		return
	}

	var variantID *int64
	if variantIDStr != "" {
		if vID, err := strconv.ParseInt(variantIDStr, 10, 64); err == nil {
			variantID = &vID
		}
	}

	isPrimary := strings.ToLower(isPrimaryStr) == "true"
	sortOrder := 0
	if sortOrderStr != "" {
		if v, err := strconv.Atoi(sortOrderStr); err == nil {
			sortOrder = v
		}
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("image file is required"))
		return
	}
	defer file.Close()

	mime, err := sniffMIME(file)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("sniff mime: %w", err))
		return
	}
	allowed := map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}
	if !allowed[mime] {
		app.badRequestResponse(w, r, fmt.Errorf("invalid image type: %s", mime))
		return
	}

	publicID := fmt.Sprintf("products/%d/%d_%d", productID, time.Now().Unix(), rand.Intn(9999))
	imageURL, err := app.uploadToCloudinaryWithID(file, publicID)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to upload image: %w", err))
		return
	}

	img := &products.ProductImage{
		ProductID:        productID,
		ProductVariantID: variantID,
		URL:              imageURL,
		Alt:              &alt,
		IsPrimary:        isPrimary,
		SortOrder:        sortOrder,
	}

	created, err := app.store.Products.CreateProductImage(ctx, img)
	if err != nil {
		// cleanup failed upload
		go app.deletePhotoFromCloudinary(imageURL)
		app.internalServerError(w, r, fmt.Errorf("failed to save image: %w", err))
		return
	}

	app.jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"message": "Image uploaded successfully",
		"image":   created,
	})
}

func (app *application) listProductImagesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	productIDStr := chi.URLParam(r, "productID")
	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil || productID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid product ID"))
		return
	}

	list, err := app.store.Products.ListProductImagesByProduct(ctx, productID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"images": list,
	})
}

func (app *application) setPrimaryImageHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	productIDStr := chi.URLParam(r, "productID")
	imageIDStr := chi.URLParam(r, "imageID")

	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil || productID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid product ID"))
		return
	}
	imageID, err := strconv.ParseInt(imageIDStr, 10, 64)
	if err != nil || imageID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid image ID"))
		return
	}

	if err := app.store.Products.SetPrimaryImage(ctx, productID, imageID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{
		"message": "Primary image set successfully",
	})
}

func (app *application) updateProductImageHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid image ID"))
		return
	}

	var payload struct {
		Alt       *string `json:"alt"`
		IsPrimary *bool   `json:"is_primary"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	existing, err := app.store.Products.GetProductImageByID(ctx, id)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if existing == nil {
		app.notFoundResponse(w, r, fmt.Errorf("image not found"))
		return
	}

	update := &products.ProductImage{
		ID:        id,
		Alt:       payload.Alt,
		IsPrimary: existing.IsPrimary,
		SortOrder: existing.SortOrder,
		URL:       existing.URL,
	}

	if payload.IsPrimary != nil {
		update.IsPrimary = *payload.IsPrimary
	}
	if payload.SortOrder != nil {
		update.SortOrder = *payload.SortOrder
	}

	updated, err := app.store.Products.UpdateProductImage(ctx, update)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"message": "Image updated successfully",
		"image":   updated,
	})
}

func (app *application) deleteProductImageHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid image ID"))
		return
	}

	img, err := app.store.Products.GetProductImageByID(ctx, id)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if img == nil {
		app.notFoundResponse(w, r, fmt.Errorf("image not found"))
		return
	}

	if err := app.store.Products.DeleteProductImage(ctx, id); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Async Cloudinary cleanup
	go func(url string) {
		if err := app.deletePhotoFromCloudinary(url); err != nil {
			app.logger.Error("failed to delete product image from Cloudinary", "url", url, "err", err)
		}
	}(img.URL)

	app.jsonResponse(w, http.StatusOK, map[string]string{
		"message": "Image deleted successfully",
	})
}

func (app *application) reorderProductImagesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	productIDStr := chi.URLParam(r, "productID")
	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil || productID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid product ID"))
		return
	}

	var payload struct {
		OrderedIDs []int64 `json:"ordered_ids"`
	}
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if len(payload.OrderedIDs) == 0 {
		app.badRequestResponse(w, r, fmt.Errorf("ordered_ids cannot be empty"))
		return
	}

	if err := app.store.Products.ReorderProductImages(ctx, productID, payload.OrderedIDs); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{
		"message": "Product images reordered successfully",
	})
}
