package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"khel/internal/domain/products"
	"khel/internal/params"
	"log"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ---------- Admin: Brands ----------
func generateSlug(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces and special characters
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = regexp.MustCompile(`^-|-$`).ReplaceAllString(slug, "")

	return slug
}

func isValidSlug(slug string) bool {
	// Alphanumeric and hyphens only, 3-50 chars
	return regexp.MustCompile(`^[a-z0-9-]{3,50}$`).MatchString(slug)
}

// helper: sniff first 512 bytes and reset reader
func sniffMIME(file multipart.File) (string, error) {
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read: %w", err)
	}
	mime := http.DetectContentType(buf[:n])

	// reset so later reads start from byte 0
	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek reset: %w", err)
		}
	}
	return mime, nil
}

func (app *application) createBrandHandler(w http.ResponseWriter, r *http.Request) {
	const maxBytes = 3 * 1024 * 1024 // 3MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	description := strings.TrimSpace(r.FormValue("description"))
	if name == "" {
		app.badRequestResponse(w, r, fmt.Errorf("brand name is required"))
		return
	}
	if slug == "" {
		slug = generateSlug(name)
	}
	if !isValidSlug(slug) {
		app.badRequestResponse(w, r, fmt.Errorf("invalid slug format"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	exists, err := app.store.Products.BrandExistsByNameOrSlug(ctx, name, slug)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("check brand existence: %w", err))
		return
	}
	if exists {
		app.conflictResponse(w, r, fmt.Errorf("brand with name '%s' or slug '%s' already exists", name, slug))
		return
	}

	// --- logo upload (optional) with MIME sniffing ---
	var logoURL string
	if file, _, err := r.FormFile("logo"); err == nil {
		defer file.Close()

		// sniff actual MIME from bytes (don’t trust Content-Type header)
		mime, err := sniffMIME(file)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("sniff mime: %w", err))
			return
		}
		allowed := map[string]bool{
			"image/jpeg": true,
			"image/png":  true,
			"image/webp": true,
		}
		if !allowed[mime] {
			app.badRequestResponse(w, r, fmt.Errorf("invalid image type: %s", mime))
			return
		}

		publicID := fmt.Sprintf("brands/%s_logo_%d", slug, time.Now().UnixNano())

		// upload using the same file reader (we reset it in sniffMIME)
		url, upErr := app.uploadToCloudinaryWithID(file, publicID)
		if upErr != nil {
			app.internalServerError(w, r, fmt.Errorf("upload logo: %w", upErr))
			return
		}
		logoURL = url
	}
	// --------------------------------------------------

	brand := &products.Brand{
		Name: name,
		Slug: &slug,
		Description: func(s string) *string {
			if strings.TrimSpace(s) == "" {
				return nil
			}
			return &s
		}(description),
		LogoURL: func(s string) *string {
			if s == "" {
				return nil
			}
			return &s
		}(logoURL),
	}

	created, err := app.store.Products.CreateBrand(ctx, brand)
	if err != nil {
		if logoURL != "" {
			go func(url string) {
				if delErr := app.deletePhotoFromCloudinary(url); delErr != nil {
					app.logger.Error("cloudinary cleanup failed", "url", url, "err", delErr)
				}
			}(logoURL)
		}
		app.internalServerError(w, r, fmt.Errorf("create brand: %w", err))
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/v1/store/admin/brands/%d", created.ID))
	app.jsonResponse(w, http.StatusCreated, created)
}

// PATCH /v1/store/admin/brands/{brandID}
func (app *application) updateBrandHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1) Parse path ID
	idStr := chi.URLParam(r, "brandID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid brand ID: %s", idStr))
		return
	}

	// 2) Parse multipart (with size cap)
	const maxBytes = 3 * 1024 * 1024 // 3 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	// 3) Load existing brand
	existing, err := app.store.Products.GetBrandByID(ctx, id)
	if err != nil {
		if errors.Is(err, products.ErrBrandNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// 4) Read optional fields (empty string means "no change")
	inName := strings.TrimSpace(r.FormValue("name"))
	inSlug := strings.TrimSpace(r.FormValue("slug"))
	inDesc := strings.TrimSpace(r.FormValue("description"))

	if inSlug != "" && !isValidSlug(inSlug) {
		app.badRequestResponse(w, r, fmt.Errorf("invalid slug format"))
		return
	}

	// Probe if a file part exists at all
	_, _, probeErr := r.FormFile("logo")
	if inName == "" && inSlug == "" && inDesc == "" && probeErr == http.ErrMissingFile {
		app.badRequestResponse(w, r, fmt.Errorf("at least one field must be provided"))
		return
	} else if probeErr != nil && probeErr != http.ErrMissingFile {
		app.badRequestResponse(w, r, fmt.Errorf("invalid logo part: %v", probeErr))
		return
	}

	// 5) Build the update payload using existing values for fields not provided
	upd := &products.Brand{
		ID:          id,
		Name:        existing.Name,        // keep current by default
		Slug:        existing.Slug,        // pointer — may be nil
		Description: existing.Description, // pointer — may be nil
		LogoURL:     existing.LogoURL,     // pointer — may be nil
	}
	if inName != "" {
		upd.Name = inName
	}
	if inSlug != "" {
		upd.Slug = &inSlug
	}
	if inDesc != "" {
		d := inDesc
		upd.Description = &d
	}

	// 6) Optional logo upload with MIME sniffing
	var newLogoURL string
	var oldLogoURL string
	if existing.LogoURL != nil {
		oldLogoURL = *existing.LogoURL
	}

	if file, _, err := r.FormFile("logo"); err == nil {
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

		// choose slug for Cloudinary folder: prefer updated slug, else existing, else generate from (updated) name
		baseSlug := ""
		switch {
		case upd.Slug != nil && *upd.Slug != "":
			baseSlug = *upd.Slug
		case existing.Slug != nil && *existing.Slug != "":
			baseSlug = *existing.Slug
		default:
			baseSlug = generateSlug(upd.Name)
		}

		publicID := fmt.Sprintf("brands/%s_logo_%d", baseSlug, time.Now().UnixNano())
		url, upErr := app.uploadToCloudinaryWithID(file, publicID)
		if upErr != nil {
			app.internalServerError(w, r, fmt.Errorf("upload logo: %w", upErr))
			return
		}
		newLogoURL = url
		upd.LogoURL = &newLogoURL
	} else if err != http.ErrMissingFile {
		// do nothing; we already validated above
	}

	// 7) Persist (no pre-check; rely on DB unique + 23505 mapping)
	if err := app.store.Products.UpdateBrand(ctx, upd); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			// UNIQUE violation on name/slug
			app.conflictResponse(w, r, fmt.Errorf("brand with same name or slug already exists"))
			if newLogoURL != "" {
				go func(u string) { _ = app.deletePhotoFromCloudinary(u) }(newLogoURL)
			}
			return
		}
		if newLogoURL != "" {
			go func(u string) { _ = app.deletePhotoFromCloudinary(u) }(newLogoURL)
		}
		app.internalServerError(w, r, fmt.Errorf("update brand: %w", err))
		return
	}

	// 8) Clean up old logo AFTER success if replaced
	if newLogoURL != "" && oldLogoURL != "" && oldLogoURL != newLogoURL {
		go func(u string) { _ = app.deletePhotoFromCloudinary(u) }(oldLogoURL)
	}

	// 9) Return fresh row
	updated, err := app.store.Products.GetBrandByID(ctx, id)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("fetch updated brand: %w", err))
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"message": "Brand updated successfully",
		"brand":   updated,
	})
}

func (app *application) deleteBrandHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "brandID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid brand ID: %s", idStr))
		return
	}

	// Load once to get logoURL for cleanup (and to 404 early)
	brand, err := app.store.Products.GetBrandByID(ctx, id)
	if err != nil {
		if errors.Is(err, products.ErrBrandNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	hasProducts, err := app.store.Products.BrandHasProducts(ctx, id)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if hasProducts {
		app.conflictResponse(w, r, fmt.Errorf("cannot delete brand: products still reference this brand"))
		return
	}

	// Delete
	if err := app.store.Products.DeleteBrand(ctx, id); err != nil {
		switch {
		case errors.Is(err, products.ErrBrandNotFound):
			app.notFoundResponse(w, r, err)
		default:
			// Map FK violation/guards to 409
			if strings.Contains(err.Error(), "dependent records") {
				app.conflictResponse(w, r, fmt.Errorf("cannot delete brand: dependent records exist"))
				return
			}
			// Unique/other DB errors
			app.internalServerError(w, r, err)
		}
		return
	}

	// Best-effort async Cloudinary cleanup
	if brand.LogoURL != nil && *brand.LogoURL != "" {
		go func(url string) {
			if err := app.deletePhotoFromCloudinary(url); err != nil {
				app.logger.Error("cloudinary delete failed", "brand_id", id, "url", url, "err", err)
			}
		}(*brand.LogoURL)
	}

	// 204 No Content on success
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getAllBrandsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query params for pagination
	q := r.URL.Query()
	pagination := params.ParsePagination(q)

	// Fetch brands and total count in one query
	brands, total, err := app.store.Products.ListBrandsWithTotal(ctx, pagination.Limit, pagination.Offset)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to fetch brands: %w", err))
		return
	}

	// Compute metadata (total pages, has_next, has_prev)
	pagination.ComputeMeta(total)

	// Build response
	response := map[string]interface{}{
		"brands":     brands,
		"pagination": pagination,
	}

	app.jsonResponse(w, http.StatusOK, response)
}

// -------- Admin: Categories ---------

func (app *application) createCategoryHandler(w http.ResponseWriter, r *http.Request) {
	// Parse the multipart form to get brand data and logo
	const maxBytes = 3 * 1024 * 1024 // 3MB for Category logos
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	// Extract brand data from form
	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	parentIDStr := strings.TrimSpace(r.FormValue("parent_id"))
	isActiveStr := strings.TrimSpace(r.FormValue("is_active"))

	// Validate required fields
	if name == "" {
		app.badRequestResponse(w, r, fmt.Errorf("category name is required"))
		return
	}

	// Generate slug if not provided
	if slug == "" {
		slug = generateSlug(name)
	}

	// Validate slug format
	if !isValidSlug(slug) {
		app.badRequestResponse(w, r, fmt.Errorf("invalid slug format"))
		return
	}

	var parentIDPtr *int64

	if parentIDStr != "" {
		if parsedID, err := strconv.ParseInt(parentIDStr, 10, 64); err == nil {
			parentIDPtr = &parsedID
		} else {
			// Handle error - log it and leave as nil, or return error
			log.Printf("Invalid parent_id: %s, error: %v", parentIDStr, err)
			parentIDPtr = nil
		}
	}

	// Check if Category name or slug already exists
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	exists, err := app.store.Products.CategoryExistsByNameOrSlug(ctx, name, slug)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("could not check category existence: %w", err))
		return
	}
	if exists {
		app.conflictResponse(w, r, fmt.Errorf("category with name '%s' or slug '%s' already exists", name, slug))
		return
	}

	var logoURL string

	if file, _, err := r.FormFile("logo"); err == nil {
		defer file.Close()

		// sniff actual MIME from bytes (don’t trust Content-Type header)
		mime, err := sniffMIME(file)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("sniff mime: %w", err))
			return
		}
		allowed := map[string]bool{
			"image/jpeg": true,
			"image/png":  true,
			"image/webp": true,
		}
		if !allowed[mime] {
			app.badRequestResponse(w, r, fmt.Errorf("invalid image type: %s", mime))
			return
		}

		publicID := fmt.Sprintf("category/%s_logo_%d", slug, time.Now().UnixNano())

		// upload using the same file reader (we reset it in sniffMIME)
		url, upErr := app.uploadToCloudinaryWithID(file, publicID)
		if upErr != nil {
			app.internalServerError(w, r, fmt.Errorf("upload logo: %w", upErr))
			return
		}
		logoURL = url
	}

	// --------------------------------------------------

	// Create brand in database
	category := &products.Category{
		Name:      name,
		Slug:      slug,
		ImageURLs: []string{logoURL},
		ParentID:  parentIDPtr,
		IsActive:  strings.ToLower(isActiveStr) == "true",
	}

	CreatedCategory, err := app.store.Products.CreateCategory(ctx, category)
	if err != nil {
		// Clean up uploaded logo if brand creation fails
		if logoURL != "" {
			go func(url string) {
				if err := app.deletePhotoFromCloudinary(url); err != nil {
					app.logger.Error("failed to delete category logo from Cloudinary", "url", url, "err", err)
				}
			}(logoURL)
		}
		app.internalServerError(w, r, fmt.Errorf("failed to create category: %w", err))
		return
	}

	// Respond with created brand
	app.jsonResponse(w, http.StatusCreated, CreatedCategory)
}

func (app *application) listCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	pagination := params.ParsePagination(r.URL.Query())
	cats, total, err := app.store.Products.ListCategories(ctx, pagination.Limit, pagination.Offset)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("list categories: %w", err))
		return
	}

	// Empty page fallback for true totals
	if len(cats) == 0 && pagination.Offset > 0 {
		if n, err := app.store.Products.CountCategories(ctx); err == nil {
			total = n
		}
	}

	// Compute pagination metadata
	pagination.ComputeMeta(total)
	// Return response with pagination info
	response := map[string]interface{}{
		"categories": cats,
		"pagination": pagination,
	}

	app.jsonResponse(w, http.StatusOK, response)
}

func (app *application) deleteCategoryHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Extract and validate ID
	idStr := chi.URLParam(r, "categoryID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid category ID: %s", idStr))
		return
	}

	// Check if category exists first (required for getting image URLs)
	existingCategory, err := app.store.Products.GetCategoryByID(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, products.ErrCategoryNotFound):
			app.notFoundResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return
	}

	// Perform deletion from database
	if err := app.store.Products.DeleteCategory(ctx, id); err != nil {
		switch {
		case errors.Is(err, products.ErrCategoryNotFound):
			app.notFoundResponse(w, r, products.ErrCategoryNotFound)
		case errors.Is(err, products.ErrCategoryHasChildren):
			app.badRequestResponse(w, r, err)
		default:
			app.internalServerError(w, r, err)
		}
		return
	}

	// Delete images from Cloudinary (async - don't block response)
	if len(existingCategory.ImageURLs) > 0 {
		go func(imageURLs []string) {

			for _, url := range imageURLs {
				if url != "" {
					if err := app.deletePhotoFromCloudinary(url); err != nil {
						app.logger.Error("failed to delete category image from Cloudinary",
							"url", url,
							"category_id", id,
							"category_name", existingCategory.Name,
							"err", err)
					} else {
						app.logger.Info("successfully deleted category image from Cloudinary",
							"url", url,
							"category_id", id,
							"category_name", existingCategory.Name)
					}
				}
			}
		}(existingCategory.ImageURLs)
	}

	// Log the deletion for audit purposes
	app.logger.Info("category deleted",
		"category_id", id,
		"category_name", existingCategory.Name,
		"image_count", len(existingCategory.ImageURLs),
		"user_id", r.Context().Value("user_id"),
	)

	// Return success response
	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"message": "Category deleted successfully",
		"deleted_category": map[string]interface{}{
			"id":          id,
			"name":        existingCategory.Name,
			"image_count": len(existingCategory.ImageURLs),
		},
	})
}

func (app *application) updateCategoryHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "categoryID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid category ID: %s", idStr))
		return
	}

	const maxBytes = 3 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("failed to parse form: %w", err))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	// Load existing
	existing, err := app.store.Products.GetCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, products.ErrCategoryNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// Inputs
	inName := strings.TrimSpace(r.FormValue("name"))
	inSlug := strings.TrimSpace(r.FormValue("slug"))
	parentIDStr := strings.TrimSpace(r.FormValue("parent_id"))
	isActiveStr := strings.TrimSpace(r.FormValue("is_active"))
	replaceLogo := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("replace_logo")), "true")

	if inSlug != "" && !isValidSlug(inSlug) {
		app.badRequestResponse(w, r, fmt.Errorf("invalid slug format"))
		return
	}

	// Build update from existing (PATCH semantics)
	upd := &products.Category{
		ID:        id,
		Name:      existing.Name,
		Slug:      existing.Slug,
		ParentID:  existing.ParentID, // nil means "no change" with current DAL
		ImageURLs: append([]string(nil), existing.ImageURLs...),
		IsActive:  existing.IsActive,
	}
	if inName != "" {
		upd.Name = inName
	}
	if inSlug != "" {
		upd.Slug = inSlug
	}

	if parentIDStr != "" {
		pid, err := strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil || pid <= 0 {
			app.badRequestResponse(w, r, fmt.Errorf("invalid parent_id"))
			return
		}
		upd.ParentID = &pid
	}
	if isActiveStr != "" {
		upd.IsActive = strings.EqualFold(isActiveStr, "true")
	}

	// Track Cloudinary URLs for cleanup decisions
	var (
		newLogoURL       string
		oldURLsToDelete  []string // filled only when replace_logo=true
		hadExistingLogos = len(existing.ImageURLs) > 0
	)

	// Optional logo upload
	if file, _, err := r.FormFile("logo"); err == nil {
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

		baseSlug := upd.Slug
		if baseSlug == "" {
			baseSlug = generateSlug(upd.Name)
		}

		publicID := fmt.Sprintf("category/%s_logo_%d", baseSlug, time.Now().UnixNano())
		url, upErr := app.uploadToCloudinaryWithID(file, publicID)
		if upErr != nil {
			app.internalServerError(w, r, fmt.Errorf("upload logo: %w", upErr))
			return
		}
		newLogoURL = url

		if replaceLogo {
			// schedule deletion of all previous URLs (after success)
			if hadExistingLogos {
				oldURLsToDelete = append([]string(nil), existing.ImageURLs...)
			}
			upd.ImageURLs = []string{newLogoURL}
		} else {
			// append behavior
			if len(upd.ImageURLs) == 0 {
				upd.ImageURLs = []string{newLogoURL}
			} else {
				upd.ImageURLs = append(upd.ImageURLs, newLogoURL)
			}
		}
	} else if err != http.ErrMissingFile && err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid logo part: %v", err))
		return
	}

	// Persist; rely on UNIQUE(slug) => 23505
	updated, err := app.store.Products.UpdateCategory(ctx, upd)
	if err != nil {
		// rollback newly-uploaded url to avoid orphaning
		if newLogoURL != "" {
			go func(u string) {
				if delErr := app.deletePhotoFromCloudinary(u); delErr != nil {
					app.logger.Error("rollback: failed to delete uploaded category image", "url", u, "err", delErr)
				}
			}(newLogoURL)
		}
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			app.conflictResponse(w, r, fmt.Errorf("category with same slug already exists"))
			return
		}
		app.internalServerError(w, r, fmt.Errorf("update category: %w", err))
		return
	}

	// After a successful UPDATE:
	// If we replaced logos, delete old Cloudinary images async
	if len(oldURLsToDelete) > 0 {
		go func(urls []string) {
			for _, u := range urls {
				if u == "" {
					continue
				}
				if delErr := app.deletePhotoFromCloudinary(u); delErr != nil {
					app.logger.Error("failed to delete old category image from Cloudinary", "url", u, "category_id", id, "err", delErr)
				} else {
					app.logger.Info("deleted old category image from Cloudinary", "url", u, "category_id", id)
				}
			}
		}(oldURLsToDelete)
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"message":  "Category updated successfully",
		"category": updated,
	})
}

func (app *application) getCategoryByIDHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "categoryID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid category ID: %s", idStr))
		return
	}

	category, err := app.store.Products.GetCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, products.ErrCategoryNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// Get category statistics (products count, children count)
	stats, err := app.store.Products.GetCategoryStats(ctx, id)
	if err != nil {
		app.logger.Error("failed to get category stats", "err", err)
	}

	response := map[string]interface{}{
		"category": category,
		"stats":    stats,
	}

	app.jsonResponse(w, http.StatusOK, response)
}

// GET /categories/search?q=electronics&page=1&limit=20
func (app *application) searchCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	query := r.URL.Query().Get("q")
	if query == "" {
		app.badRequestResponse(w, r, fmt.Errorf("search query is required"))
		return
	}

	// Parse pagination only
	pagination := params.ParsePagination(r.URL.Query())

	// Use simple search (no filters)
	categories, total, err := app.store.Products.SearchCategories(ctx, query, pagination.Limit, pagination.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	pagination.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"categories":  categories,
		"pagination":  pagination,
		"query":       query,
		"search_type": "basic",
	})
}

func (app *application) fullTextSearchCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	query := r.URL.Query().Get("q")
	if query == "" {
		app.badRequestResponse(w, r, fmt.Errorf("search query is required"))
		return
	}

	pagination := params.ParsePagination(r.URL.Query())

	categories, total, err := app.store.Products.FullTextSearchCategories(ctx, query, pagination.Limit, pagination.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	pagination.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"categories":  categories,
		"pagination":  pagination,
		"query":       query,
		"search_type": "full_text", // Indicate this is full-text search
	})
}

func (app *application) getCategoryTreeHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	includeInactive := strings.EqualFold(r.URL.Query().Get("include_inactive"), "true")

	tree, err := app.store.Products.GetCategoryTree(ctx, includeInactive)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{"tree": tree})
}

func (app *application) listProductsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pg := params.ParsePagination(r.URL.Query())
	categorySlug := strings.TrimSpace(r.URL.Query().Get("category_slug"))

	items, total, err := app.store.Products.ListProductCards(ctx, categorySlug, pg.Limit, pg.Offset)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("list products: %w", err))
		return
	}
	pg.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"products":   items,
		"pagination": pg,
		"filters":    map[string]any{"category_slug": categorySlug},
	})
}

func (app *application) getProductDetailHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	if strings.TrimSpace(slug) == "" {
		app.badRequestResponse(w, r, fmt.Errorf("slug is required"))
		return
	}

	detail, err := app.store.Products.GetProductDetailBySlug(ctx, slug)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if detail == nil || !detail.Product.IsActive {
		app.notFoundResponse(w, r, fmt.Errorf("product not found"))
		return
	}

	app.jsonResponse(w, http.StatusOK, detail)
}

func (app *application) adminListProductsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pg := params.ParsePagination(r.URL.Query())

	items, total, err := app.store.Products.ListAdminProductCards(ctx, pg.Limit, pg.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	pg.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"products":   items,
		"pagination": pg,
	})
}

func (app *application) createProductHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	var in struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		Description *string `json:"description"`
		CategoryID  *int64  `json:"category_id"`
		BrandID     *int64  `json:"brand_id"`
	}
	if err := readJSON(w, r, &in); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if strings.TrimSpace(in.Name) == "" {
		app.badRequestResponse(w, r, fmt.Errorf("name required"))
		return
	}
	if strings.TrimSpace(in.Slug) == "" {
		in.Slug = generateSlug(in.Name)
	}
	if !isValidSlug(in.Slug) {
		app.badRequestResponse(w, r, fmt.Errorf("invalid slug"))
		return
	}

	p := &products.Product{
		Name: in.Name, Slug: in.Slug, Description: in.Description,
		CategoryID: in.CategoryID, BrandID: in.BrandID, IsActive: false, // draft
	}
	created, err := app.store.Products.CreateProduct(ctx, p)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/v1/store/admin/products/%d", created.ID))
	app.jsonResponse(w, http.StatusCreated, created)
}

func (app *application) publishProductHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	id, _ := strconv.ParseInt(chi.URLParam(r, "productID"), 10, 64)

	p, err := app.store.Products.GetProductByID(ctx, id)
	if err != nil || p == nil {
		app.notFoundResponse(w, r, fmt.Errorf("product not found"))
		return
	}

	// sanity checks before publish
	vars, _ := app.store.Products.ListVariantsByProduct(ctx, id)
	imgs, _ := app.store.Products.ListProductImagesByProduct(ctx, id)
	if len(vars) == 0 {
		app.badRequestResponse(w, r, fmt.Errorf("at least one variant required"))
		return
	}
	hasPrimary := false
	for _, im := range imgs {
		if im.IsPrimary {
			hasPrimary = true
			break
		}
	}
	if !hasPrimary {
		app.badRequestResponse(w, r, fmt.Errorf("primary image required"))
		return
	}

	p.IsActive = true
	updated, err := app.store.Products.UpdateProduct(ctx, p)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"message": "published",
		"product": updated,
	})
}
