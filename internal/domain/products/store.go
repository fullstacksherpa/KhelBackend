package products

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrBrandNotFound = errors.New("brand not found")

	ErrBrandHasProducts    = errors.New("cannot delete brand with associated products")
	ErrDuplicateBrand      = errors.New("brand with this name or slug already exists")
	ErrCategoryNotFound    = fmt.Errorf("category not found")
	ErrCategoryHasChildren = errors.New("category has child categories")
	ErrCategoryHasProducts = errors.New("category has associated products")
	ErrDuplicateSlug       = errors.New("slug already exists")
	ErrInvalidParent       = errors.New("invalid parent category")
	ErrCircularDependency  = errors.New("circular dependency detected")
)

// Store is the data access abstraction for the products domain.
// Implemented by Repository (which uses pgxpool.Pool).
type Store interface {
	// Transaction helper
	WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error

	// Brands
	CreateBrand(ctx context.Context, b *Brand) (*Brand, error)
	BrandExistsByNameOrSlug(ctx context.Context, name, slug string) (bool, error)
	GetBrandByID(ctx context.Context, id int64) (*Brand, error)
	ListBrandsWithTotal(ctx context.Context, limit, offset int) ([]*Brand, int, error)
	BrandHasProducts(ctx context.Context, id int64) (bool, error)
	UpdateBrand(ctx context.Context, b *Brand) error
	BrandConflictExists(ctx context.Context, name, slug string, excludeID int64) (bool, error)
	DeleteBrand(ctx context.Context, id int64) error

	// Categories
	CreateCategory(ctx context.Context, c *Category) (*Category, error)
	CountCategories(ctx context.Context) (int, error)
	GetCategoryByID(ctx context.Context, id int64) (*Category, error)
	ListCategories(ctx context.Context, limit, offset int) ([]*Category, int, error)
	UpdateCategory(ctx context.Context, c *Category) (*Category, error)
	DeleteCategory(ctx context.Context, id int64) error
	CategoryExistsByNameOrSlug(ctx context.Context, name, slug string) (bool, error)
	GetCategoryStats(ctx context.Context, categoryID int64) (map[string]interface{}, error)
	SearchCategories(ctx context.Context, query string, limit, offset int) ([]*Category, int, error)
	FullTextSearchCategories(ctx context.Context, query string, limit, offset int) ([]*CategoryWithRank, int, error)
	GetCategoryTree(ctx context.Context, includeInactive bool) ([]*CategoryWithChildren, error)

	// Products
	CreateProduct(ctx context.Context, p *Product) (*Product, error)
	GetProductByID(ctx context.Context, id int64) (*Product, error)
	GetProductBySlug(ctx context.Context, slug string) (*Product, error)
	ListProducts(ctx context.Context, limit, offset int) ([]*Product, int, error)
	UpdateProduct(ctx context.Context, p *Product) (*Product, error)
	DeleteProduct(ctx context.Context, id int64) error
	ListProductCards(
		ctx context.Context,
		categorySlug string,
		limit, offset int,
	) ([]*ProductCard, int, error)
	GetProductDetailBySlug(ctx context.Context, slug string) (*ProductDetail, error)
	ListAdminProductCards(ctx context.Context, limit, offset int) ([]*ProductCard, int, error)

	SearchProducts(ctx context.Context, query string, limit, offset int) ([]*Product, int, error)
	FullTextSearchProducts(ctx context.Context, query string, limit, offset int) ([]*ProductWithRank, int, error)

	// Variants
	CreateVariant(ctx context.Context, v *ProductVariant) (*ProductVariant, error)
	GetVariantByID(ctx context.Context, id int64) (*ProductVariant, error)
	ListVariantsByProduct(ctx context.Context, productID int64) ([]*ProductVariant, error)
	UpdateVariant(ctx context.Context, v *ProductVariant) error
	DeleteVariant(ctx context.Context, id int64) error
	ListAllVariants(ctx context.Context, limit, offset int) ([]*ProductVariant, int, error)

	// Product images
	CreateProductImage(ctx context.Context, img *ProductImage) (*ProductImage, error)
	GetProductImageByID(ctx context.Context, id int64) (*ProductImage, error)
	ListProductImagesByProduct(ctx context.Context, productID int64) ([]*ProductImage, error)
	SetPrimaryImage(ctx context.Context, productID, imageID int64) error
	UpdateProductImage(ctx context.Context, img *ProductImage) (*ProductImage, error)
	DeleteProductImage(ctx context.Context, id int64) error
	ReorderProductImages(ctx context.Context, productID int64, orderedIDs []int64) error
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

// ------------------------------------
// Transaction helper
// ------------------------------------
func (r *Repository) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	// Use Rollback with check
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			log.Printf("warning: rollback failed: %v", err)
		}
	}()

	if err := fn(tx); err != nil {
		return fmt.Errorf("tx fn: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// ------------------------------------
// Brands
// ------------------------------------
func (r *Repository) CreateBrand(ctx context.Context, b *Brand) (*Brand, error) {
	query := `
		INSERT INTO brands (name, slug, description, logo_url)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, slug, description, logo_url, created_at, updated_at;
	`
	row := r.db.QueryRow(ctx, query, b.Name, b.Slug, b.Description, b.LogoURL)
	if err := row.Scan(&b.ID, &b.Name, &b.Slug, &b.Description, &b.LogoURL, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create brand: %w", err)
	}
	return b, nil
}

func (r *Repository) BrandConflictExists(ctx context.Context, name, slug string, excludeID int64) (bool, error) {
	query := `
SELECT 1
FROM brands
WHERE (LOWER(name) = LOWER($1) OR slug = $2)
  AND id <> $3
LIMIT 1
`
	var one int
	err := r.db.QueryRow(ctx, query, name, slug, excludeID).Scan(&one)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *Repository) BrandExistsByNameOrSlug(ctx context.Context, name, slug string) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS(
			SELECT 1 FROM brands 
			WHERE name = $1 OR slug = $2
		)
	`
	err := r.db.QueryRow(ctx, query, name, slug).Scan(&exists)
	return exists, err
}

func (r *Repository) GetBrandByID(ctx context.Context, id int64) (*Brand, error) {
	query := `SELECT id, name, slug, description, logo_url, created_at, updated_at FROM brands WHERE id = $1;`
	b := &Brand{}
	if err := r.db.QueryRow(ctx, query, id).
		Scan(&b.ID, &b.Name, &b.Slug, &b.Description, &b.LogoURL, &b.CreatedAt, &b.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get brand: %w", err)
	}
	return b, nil
}

// ListBrandsWithTotal returns a page of brands and the true total.
// It uses COUNT(*) OVER() when rows exist; if the page is beyond the end
// (0 rows returned), it falls back to a separate COUNT(*) to avoid a false total.
func (r *Repository) ListBrandsWithTotal(ctx context.Context, limit, offset int) ([]*Brand, int, error) {
	if limit <= 0 || limit > 30 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}

	const q = `
		SELECT id, name, slug, description, logo_url, created_at, updated_at,
		       COUNT(*) OVER() AS total_count
		FROM brands
		ORDER BY LOWER(name) ASC, id ASC
		LIMIT $1 OFFSET $2;
	`

	rows, err := r.db.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list brands: %w", err)
	}
	defer rows.Close()

	var (
		brands     []*Brand
		totalCount int
	)

	for rows.Next() {
		var b Brand
		var t int
		if err := rows.Scan(&b.ID, &b.Name, &b.Slug, &b.Description, &b.LogoURL, &b.CreatedAt, &b.UpdatedAt, &t); err != nil {
			return nil, 0, fmt.Errorf("scan brand: %w", err)
		}
		if totalCount == 0 {
			totalCount = t
		}
		brands = append(brands, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration: %w", err)
	}

	// Fallback: user paged past the end → no rows, but total may be > 0.
	if len(brands) == 0 && offset > 0 {
		const countQ = `SELECT COUNT(*) FROM brands;`
		if err := r.db.QueryRow(ctx, countQ).Scan(&totalCount); err != nil {
			return nil, 0, fmt.Errorf("count brands: %w", err)
		}
	}

	return brands, totalCount, nil
}

func (r *Repository) UpdateBrand(ctx context.Context, b *Brand) error {
	query := `
		UPDATE brands 
		SET 
			name = COALESCE($1, name),
			slug = COALESCE($2, slug),
			description = COALESCE($3, description),
			logo_url = COALESCE($4, logo_url),
			updated_at = now()
		WHERE id = $5;
	`
	_, err := r.db.Exec(ctx, query,
		b.Name, b.Slug, b.Description, b.LogoURL, b.ID)
	if err != nil {
		return fmt.Errorf("update brand: %w", err)
	}
	return nil
}

func (r *Repository) DeleteBrand(ctx context.Context, id int64) error {
	cmd, err := r.db.Exec(ctx, `DELETE FROM brands WHERE id=$1;`, id)
	if err != nil {
		// 23503 = foreign_key_violation (useful if your schema is RESTRICT)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			// bubble up; handler will map to 409
			return fmt.Errorf("brand has dependent records: %w", err)
		}
		return fmt.Errorf("delete brand: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrBrandNotFound
	}
	return nil
}

func (r *Repository) BrandHasProducts(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE brand_id=$1)`, id).Scan(&exists)
	return exists, err
}

// ------------------------------------
// Categories
// ------------------------------------
func (r *Repository) CreateCategory(ctx context.Context, c *Category) (*Category, error) {
	if err := validateCategory(c); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check for duplicate slug
	exists, err := r.categorySlugExists(ctx, c.Slug, nil)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("category with slug '%s' already exists", c.Slug)
	}

	// Validate parent exists if provided
	if c.ParentID != nil {
		parent, err := r.GetCategoryByID(ctx, *c.ParentID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, fmt.Errorf("parent category with ID %d not found", *c.ParentID)
		}
	}

	query := `
        INSERT INTO categories (name, slug, parent_id, image_urls, is_active)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, name, slug, parent_id, image_urls, is_active, created_at, updated_at;
    `

	category := &Category{}
	err = r.db.QueryRow(ctx, query, c.Name, c.Slug, c.ParentID, c.ImageURLs, c.IsActive).
		Scan(&category.ID, &category.Name, &category.Slug, &category.ParentID,
			&category.ImageURLs, &category.IsActive, &category.CreatedAt, &category.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}

	return category, nil
}

func (r *Repository) CountCategories(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM categories`).Scan(&n); err != nil {
		return 0, fmt.Errorf("Count categories: %w", err)
	}
	return n, nil
}

func (r *Repository) GetCategoryByID(ctx context.Context, id int64) (*Category, error) {
	query := `
        SELECT id, name, slug, parent_id, image_urls, is_active, created_at, updated_at 
        FROM categories 
        WHERE id = $1;
    `

	category := &Category{}
	err := r.db.QueryRow(ctx, query, id).
		Scan(&category.ID, &category.Name, &category.Slug, &category.ParentID,
			&category.ImageURLs, &category.IsActive, &category.CreatedAt, &category.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCategoryNotFound
		}
		return nil, fmt.Errorf("get category by id: %w", err)
	}

	return category, nil
}

func (r *Repository) ListCategories(ctx context.Context, limit, offset int) ([]*Category, int, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT 
			id, name, slug, parent_id, image_urls, is_active, created_at, updated_at,
			COUNT(*) OVER() AS total_count
		FROM categories 
		ORDER BY id 
		LIMIT $1 OFFSET $2;`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var list []*Category
	var totalCount int

	for rows.Next() {
		var c Category
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug, &c.ParentID, &c.ImageURLs, &c.IsActive,
			&c.CreatedAt, &c.UpdatedAt, &totalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan category: %w", err)
		}
		list = append(list, &c)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return list, totalCount, nil
}

func (r *Repository) UpdateCategory(ctx context.Context, c *Category) (*Category, error) {
	if err := validateCategory(c); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if category exists
	existing, err := r.GetCategoryByID(ctx, c.ID)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		return nil, fmt.Errorf("category with ID %d not found", c.ID)
	}

	// Check for duplicate slug (excluding current category)
	exists, err := r.categorySlugExists(ctx, c.Slug, &c.ID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("category with slug '%s' already exists", c.Slug)
	}

	query := `
        UPDATE categories 
        SET 
            name = COALESCE(NULLIF($1, ''), name),
            slug = COALESCE(NULLIF($2, ''), slug),
            parent_id = COALESCE($3, parent_id),
            image_urls = COALESCE(NULLIF($4, '{}'::text[]), image_urls),
            is_active = COALESCE($5, is_active),
            updated_at = NOW()
        WHERE id = $6
        RETURNING id, name, slug, parent_id, image_urls, is_active, created_at, updated_at;
    `

	updated := &Category{}
	err = r.db.QueryRow(ctx, query, c.Name, c.Slug, c.ParentID, c.ImageURLs,
		c.IsActive, c.ID).
		Scan(&updated.ID, &updated.Name, &updated.Slug, &updated.ParentID,
			&updated.ImageURLs, &updated.IsActive, &updated.CreatedAt, &updated.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("update category: %w", err)
	}

	return updated, nil
}

func (r *Repository) DeleteCategory(ctx context.Context, id int64) error {
	// Check if category exists
	_, err := r.GetCategoryByID(ctx, id)
	if err != nil {
		return err
	}

	// Check if category has children
	hasChildren, err := r.hasChildren(ctx, id)
	if err != nil {
		return err
	}
	if hasChildren {
		return fmt.Errorf("cannot delete category with children")
	}

	result, err := r.db.Exec(ctx, "DELETE FROM categories WHERE id = $1;", id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrCategoryNotFound
	}

	return nil
}

func (r *Repository) CategoryExistsByNameOrSlug(ctx context.Context, name, slug string) (bool, error) {
	var exists bool
	query := `
		SELECT EXISTS(
			SELECT 1 FROM categories 
			WHERE name = $1 OR slug = $2
		)
	`
	err := r.db.QueryRow(ctx, query, name, slug).Scan(&exists)
	return exists, err
}

// Helper methods
func (r *Repository) categorySlugExists(ctx context.Context, slug string, excludeID *int64) (bool, error) {
	var query string
	var args []interface{}

	if excludeID != nil {
		query = "SELECT EXISTS(SELECT 1 FROM categories WHERE slug = $1 AND id != $2)"
		args = []interface{}{slug, *excludeID}
	} else {
		query = "SELECT EXISTS(SELECT 1 FROM categories WHERE slug = $1)"
		args = []interface{}{slug}
	}

	var exists bool
	err := r.db.QueryRow(ctx, query, args...).Scan(&exists)
	return exists, err
}

func (r *Repository) hasChildren(ctx context.Context, parentID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM categories WHERE parent_id = $1)",
		parentID).Scan(&exists)
	return exists, err
}

func validateCategory(c *Category) error {
	if c == nil {
		return fmt.Errorf("category cannot be nil")
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("category name cannot be empty")
	}
	if strings.TrimSpace(c.Slug) == "" {
		return fmt.Errorf("category slug cannot be empty")
	}
	if c.ParentID != nil && *c.ParentID == c.ID {
		return fmt.Errorf("category cannot be its own parent")
	}
	return nil
}

func (r *Repository) GetCategoryStats(ctx context.Context, categoryID int64) (map[string]interface{}, error) {
	query := `
		SELECT 
			(SELECT COUNT(*) FROM products WHERE category_id = $1 AND is_active = true) as product_count,
			(SELECT COUNT(*) FROM categories WHERE parent_id = $1 AND is_active = true) as children_count,
			(SELECT COUNT(*) FROM categories WHERE parent_id = $1) as total_children_count
	`

	stats := make(map[string]interface{})
	var productCount, childrenCount, totalChildrenCount int

	err := r.db.QueryRow(ctx, query, categoryID).Scan(&productCount, &childrenCount, &totalChildrenCount)
	if err != nil {
		return nil, err
	}

	stats["product_count"] = productCount
	stats["active_children_count"] = childrenCount
	stats["total_children_count"] = totalChildrenCount

	return stats, nil
}

func (r *Repository) SearchCategories(ctx context.Context, query string, limit, offset int) ([]*Category, int, error) {
	if query == "" {
		return nil, 0, fmt.Errorf("search query cannot be empty")
	}

	searchQuery := strings.TrimSpace(query)

	// Search only by name using trigram similarity
	searchSQL := `
		SELECT 
			id, name, slug, parent_id, image_urls, is_active, created_at, updated_at,
			COUNT(*) OVER() AS total_count
		FROM categories 
		WHERE 
			(is_active = true) AND
			(
				name % $1 OR                    -- Trigram similarity
				word_similarity($1, name) > 0.4 OR  -- Match whole words
				name ILIKE $1 || '%'             -- Prefix match
			)
		ORDER BY 
			GREATEST(
				similarity($1, name),
				word_similarity($1, name)
			) DESC,
			name ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, searchSQL, searchQuery, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search categories: %w", err)
	}
	defer rows.Close()

	var categories []*Category
	var totalCount int

	for rows.Next() {
		var cat Category
		err := rows.Scan(
			&cat.ID, &cat.Name, &cat.Slug, &cat.ParentID, &cat.ImageURLs,
			&cat.IsActive, &cat.CreatedAt, &cat.UpdatedAt, &totalCount,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan search result: %w", err)
		}
		categories = append(categories, &cat)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return categories, totalCount, nil
}

// Full-text search - advanced text search with ranking
func (r *Repository) FullTextSearchCategories(ctx context.Context, query string, limit, offset int) ([]*CategoryWithRank, int, error) {
	if query == "" {
		return nil, 0, fmt.Errorf("search query cannot be empty")
	}

	searchSQL := `
		SELECT 
			id, name, slug, parent_id, image_urls, is_active, created_at, updated_at,
			COUNT(*) OVER() AS total_count,
			ts_rank_cd(fts, plainto_tsquery('english', $1)) as rank
		FROM categories 
		WHERE fts @@ plainto_tsquery('english', $1)
		ORDER BY rank DESC, name ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, searchSQL, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("full-text search categories: %w", err)
	}
	defer rows.Close()

	var categories []*CategoryWithRank
	var totalCount int

	for rows.Next() {
		var cat CategoryWithRank
		err := rows.Scan(
			&cat.ID, &cat.Name, &cat.Slug, &cat.ParentID, &cat.ImageURLs,
			&cat.IsActive, &cat.CreatedAt, &cat.UpdatedAt, &totalCount, &cat.Rank,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan full-text search result: %w", err)
		}
		categories = append(categories, &cat)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return categories, totalCount, nil
}

func (r *Repository) GetCategoryTree(ctx context.Context, includeInactive bool) ([]*CategoryWithChildren, error) {
	query := `
		WITH RECURSIVE category_tree AS (
			SELECT 
				id, name, slug, parent_id, image_urls, is_active, created_at, updated_at,
				0 as level,
				ARRAY[id] as path
			FROM categories 
			WHERE parent_id IS NULL
			UNION ALL
			SELECT 
				c.id, c.name, c.slug, c.parent_id, c.image_urls, c.is_active, 
				c.created_at, c.updated_at,
				ct.level + 1,
				ct.path || c.id
			FROM categories c
			INNER JOIN category_tree ct ON c.parent_id = ct.id
		)
		SELECT * FROM category_tree
		WHERE is_active = true OR $1 = true
		ORDER BY path
	`

	rows, err := r.db.Query(ctx, query, includeInactive)
	if err != nil {
		return nil, fmt.Errorf("get category tree: %w", err)
	}
	defer rows.Close()

	// Store all categories in a flat slice first
	var flatCategories []*CategoryWithChildren
	for rows.Next() {
		var cat CategoryWithChildren
		var path []int64

		err := rows.Scan(
			&cat.ID, &cat.Name, &cat.Slug, &cat.ParentID, &cat.ImageURLs,
			&cat.IsActive, &cat.CreatedAt, &cat.UpdatedAt, &cat.Level, &path,
		)
		if err != nil {
			return nil, fmt.Errorf("scan category tree: %w", err)
		}
		cat.Path = path
		flatCategories = append(flatCategories, &cat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Build hierarchical tree
	return r.buildCategoryTree(flatCategories), nil
}

// buildCategoryTree converts flat categories into hierarchical structure
func (r *Repository) buildCategoryTree(categories []*CategoryWithChildren) []*CategoryWithChildren {
	// Create a map for quick lookup
	categoryMap := make(map[int64]*CategoryWithChildren)

	// First pass: add all categories to map
	for _, cat := range categories {
		categoryMap[cat.ID] = cat
	}

	// Second pass: build tree structure
	var rootCategories []*CategoryWithChildren
	for _, cat := range categories {
		if cat.ParentID == nil {
			// This is a root category
			rootCategories = append(rootCategories, cat)
		} else {
			// This is a child category, find its parent
			if parent, exists := categoryMap[*cat.ParentID]; exists {
				parent.Children = append(parent.Children, cat)
			}
		}
	}

	return rootCategories
}

// ------------------------------------
// Products
// ------------------------------------
func (r *Repository) CreateProduct(ctx context.Context, p *Product) (*Product, error) {

	// Validate required fields
	if err := validateProduct(p); err != nil {
		return nil, err
	}

	// Validate foreign keys exist
	if p.CategoryID != nil {
		exists, err := r.categoryExists(ctx, *p.CategoryID)
		if err != nil {
			return nil, fmt.Errorf("validate category: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("category with ID %d does not exist", *p.CategoryID)
		}
	}

	if p.BrandID != nil {
		exists, err := r.brandExists(ctx, *p.BrandID)
		if err != nil {
			return nil, fmt.Errorf("validate brand: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("brand with ID %d does not exist", *p.BrandID)
		}
	}

	if exists, err := r.productSlugExists(ctx, p.Slug, nil); err != nil {
		return nil, err
	} else if exists {
		return nil, fmt.Errorf("product with slug '%s' already exists", p.Slug)
	}

	query := `
		INSERT INTO products (name, slug, description, category_id, brand_id, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, slug, description, category_id, brand_id, is_active, created_at, updated_at;
	`
	row := r.db.QueryRow(ctx, query, p.Name, p.Slug, p.Description, p.CategoryID, p.BrandID, p.IsActive)
	if err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.CategoryID, &p.BrandID, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}
	return p, nil
}

func (r *Repository) GetProductByID(ctx context.Context, id int64) (*Product, error) {
	query := `SELECT id, name, slug, description, category_id, brand_id, is_active, created_at, updated_at FROM products WHERE id=$1;`
	p := &Product{}
	if err := r.db.QueryRow(ctx, query, id).
		Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.CategoryID, &p.BrandID, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get product: %w", err)
	}
	return p, nil
}

// ListProducts returns paginated products and total count for UI pagination
// Note: This performs two queries - one for data and one for count
// For large datasets, consider caching the count or using estimated counts
func (r *Repository) ListProducts(ctx context.Context, limit, offset int) (products []*Product, totalCount int, err error) {
	// Fetch the current page of products
	rows, err := r.db.Query(ctx, `
        SELECT id, name, slug, description, category_id, brand_id, is_active, created_at, updated_at
        FROM products
        ORDER BY id DESC
        LIMIT $1 OFFSET $2
    `, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	products = make([]*Product, 0, limit) // Pre-allocate for performance
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.CategoryID, &p.BrandID, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, &p)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows iteration: %w", err)
	}

	// Get total count for pagination UI (showing "Showing 1-10 of 150 products")
	err = r.db.QueryRow(ctx, `SELECT COUNT(*) FROM products`).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	return products, totalCount, nil
}

func (r *Repository) UpdateProduct(ctx context.Context, p *Product) (*Product, error) {

	if p.ID == 0 {
		return nil, fmt.Errorf("product ID is required")
	}

	// Check if product exists
	existing, err := r.GetProductByID(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("product with ID %d not found", p.ID)
	}

	// Validate foreign keys if changing
	if p.CategoryID != nil && *p.CategoryID != 0 {
		if exists, err := r.categoryExists(ctx, *p.CategoryID); err != nil {
			return nil, err
		} else if !exists {
			return nil, fmt.Errorf("category with ID %d does not exist", *p.CategoryID)
		}
	}

	if p.Slug != "" && p.Slug != existing.Slug {
		if exists, err := r.productSlugExists(ctx, p.Slug, &p.ID); err != nil {
			return nil, err
		} else if exists {
			return nil, fmt.Errorf("product with slug '%s' already exists", p.Slug)
		}
	}

	query := `
		UPDATE products 
		SET name=$1, slug=$2, description=$3, category_id=$4, brand_id=$5, is_active=$6, updated_at=now()
		WHERE id=$7 RETURNING id, name, slug, description,
        category_id, brand_id, is_active, created_at, updated_at;
	`
	updated := &Product{}
	err = r.db.QueryRow(ctx, query,
		p.Name, p.Slug, p.Description,
		p.CategoryID, p.BrandID, p.IsActive, p.ID,
	).Scan(
		&updated.ID, &updated.Name, &updated.Slug, &updated.Description,

		&updated.CategoryID, &updated.BrandID, &updated.IsActive, &updated.CreatedAt, &updated.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}

	return updated, nil
}

func (r *Repository) DeleteProduct(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM products WHERE id=$1;`, id)
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	return nil
}

func (r *Repository) GetProductBySlug(ctx context.Context, slug string) (*Product, error) {
	query := `SELECT id, name, slug, description, 
                     category_id, brand_id, is_active,
                     created_at, updated_at
              FROM products WHERE slug = $1;`

	product := &Product{}
	err := r.db.QueryRow(ctx, query, slug).Scan(
		&product.ID, &product.Name, &product.Slug, &product.Description, &product.CategoryID, &product.BrandID, &product.IsActive, &product.CreatedAt, &product.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get product by slug: %w", err)
	}

	return product, nil
}

func (r *Repository) categoryExists(ctx context.Context, categoryID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1 AND is_active = true)",
		categoryID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) brandExists(ctx context.Context, brandID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM brands WHERE id = $1)",
		brandID,
	).Scan(&exists)
	return exists, err
}

func validateProduct(p *Product) error {
	if p == nil {
		return fmt.Errorf("product cannot be nil")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("product name cannot be empty")
	}
	if strings.TrimSpace(p.Slug) == "" {
		return fmt.Errorf("product slug cannot be empty")
	}
	return nil
}

func (r *Repository) productSlugExists(ctx context.Context, slug string, excludeID *int64) (bool, error) {
	var query string
	var args []interface{}

	if excludeID != nil {
		query = "SELECT EXISTS(SELECT 1 FROM products WHERE slug = $1 AND id != $2)"
		args = []interface{}{slug, *excludeID}
	} else {
		query = "SELECT EXISTS(SELECT 1 FROM products WHERE slug = $1)"
		args = []interface{}{slug}
	}

	var exists bool
	err := r.db.QueryRow(ctx, query, args...).Scan(&exists)
	return exists, err
}

// ------------------------------------
// Variants
// ------------------------------------
func (r *Repository) CreateVariant(ctx context.Context, v *ProductVariant) (*ProductVariant, error) {
	attrJSON, err := json.Marshal(v.Attributes)
	if err != nil {
		return nil, fmt.Errorf("marshal attributes: %w", err)
	}

	query := `
		INSERT INTO product_variants (product_id, price_cents, attributes, is_active)
		VALUES ($1, $2, $3, $4)
		RETURNING id, product_id, price_cents, attributes, is_active, created_at, updated_at;
	`
	row := r.db.QueryRow(ctx, query, v.ProductID, v.PriceCents, attrJSON, v.IsActive)
	var attrData []byte
	if err := row.Scan(&v.ID, &v.ProductID, &v.PriceCents, &attrData, &v.IsActive, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create variant: %w", err)
	}
	if err := json.Unmarshal(attrData, &v.Attributes); err != nil {
		return nil, fmt.Errorf("unmarshal attributes: %w", err)
	}
	return v, nil
}

func (r *Repository) GetVariantByID(ctx context.Context, id int64) (*ProductVariant, error) {
	query := `SELECT id, product_id, price_cents, attributes, is_active, created_at, updated_at FROM product_variants WHERE id=$1;`
	v := &ProductVariant{}
	var attrData []byte
	if err := r.db.QueryRow(ctx, query, id).
		Scan(&v.ID, &v.ProductID, &v.PriceCents, &attrData, &v.IsActive, &v.CreatedAt, &v.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get variant: %w", err)
	}
	json.Unmarshal(attrData, &v.Attributes)
	return v, nil
}

func (r *Repository) ListVariantsByProduct(ctx context.Context, productID int64) ([]*ProductVariant, error) {
	query := `SELECT id, product_id, price_cents, attributes, is_active, created_at, updated_at FROM product_variants WHERE product_id=$1;`
	rows, err := r.db.Query(ctx, query, productID)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	defer rows.Close()

	var list []*ProductVariant
	for rows.Next() {
		var v ProductVariant
		var attrData []byte
		if err := rows.Scan(&v.ID, &v.ProductID, &v.PriceCents, &attrData, &v.IsActive, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(attrData, &v.Attributes)
		list = append(list, &v)
	}
	return list, nil
}

func (r *Repository) UpdateVariant(ctx context.Context, v *ProductVariant) error {
	attrJSON, err := json.Marshal(v.Attributes)
	if err != nil {
		return fmt.Errorf("marshal attributes: %w", err)
	}

	query := `
		UPDATE product_variants 
		SET price_cents=$1, attributes=$2, is_active=$3, updated_at=now()
		WHERE id=$4;
	`
	_, err = r.db.Exec(ctx, query, v.PriceCents, attrJSON, v.IsActive, v.ID)
	if err != nil {
		return fmt.Errorf("update variant: %w", err)
	}
	return nil
}

func (r *Repository) DeleteVariant(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM product_variants WHERE id=$1;`, id)
	if err != nil {
		return fmt.Errorf("delete variant: %w", err)
	}
	return nil
}

func (r *Repository) ListAllVariants(ctx context.Context, limit, offset int) ([]*ProductVariant, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	query := `
	SELECT 
		id, 
		product_id, 
		price_cents, 
		attributes, 
		is_active, 
		created_at, 
		updated_at, 
		COUNT(*) OVER() AS total
	FROM product_variants
	ORDER BY id
	LIMIT $1 OFFSET $2;
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list variants: %w", err)
	}
	defer rows.Close()

	var variants []*ProductVariant
	total := 0

	for rows.Next() {
		var v ProductVariant
		var attrData []byte
		var t int
		if err := rows.Scan(
			&v.ID,
			&v.ProductID,
			&v.PriceCents,
			&attrData,
			&v.IsActive,
			&v.CreatedAt,
			&v.UpdatedAt,
			&t,
		); err != nil {
			return nil, 0, err
		}
		total = t
		if err := json.Unmarshal(attrData, &v.Attributes); err != nil {
			return nil, 0, fmt.Errorf("unmarshal attributes: %w", err)
		}
		variants = append(variants, &v)
	}

	return variants, total, nil
}

// ------------------------------------
// Product_images
// ------------------------------------

// CreateProductImage inserts a new image and returns the saved entity.
// If isPrimary is true, it will set this image as primary inside a transaction
// (clearing any other primary for the same product).
func (r *Repository) CreateProductImage(ctx context.Context, img *ProductImage) (*ProductImage, error) {
	if img == nil {
		return nil, fmt.Errorf("image is nil")
	}
	// Use tx if we need to flip primary flag atomically
	if img.IsPrimary {
		var created *ProductImage
		err := r.WithTx(ctx, func(tx pgx.Tx) error {
			// clear existing primary
			if _, err := tx.Exec(ctx, `UPDATE product_images SET is_primary = false WHERE product_id = $1 AND is_primary = true`, img.ProductID); err != nil {
				return fmt.Errorf("clear existing primary: %w", err)
			}
			// insert new row
			q := `
				INSERT INTO product_images (product_id, product_variant_id, url, alt, is_primary, sort_order)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id, product_id, product_variant_id, url, alt, is_primary, sort_order, created_at, updated_at
			`
			row := tx.QueryRow(ctx, q, img.ProductID, img.ProductVariantID, img.URL, img.Alt, img.IsPrimary, img.SortOrder)
			created = &ProductImage{}
			if err := row.Scan(&created.ID, &created.ProductID, &created.ProductVariantID, &created.URL, &created.Alt, &created.IsPrimary, &created.SortOrder, &created.CreatedAt, &created.UpdatedAt); err != nil {
				return fmt.Errorf("insert product_image: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return created, nil
	}

	// simple insert (no tx)
	q := `
		INSERT INTO product_images (product_id, product_variant_id, url, alt, is_primary, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, product_id, product_variant_id, url, alt, is_primary, sort_order, created_at, updated_at
	`
	created := &ProductImage{}
	if err := r.db.QueryRow(ctx, q, img.ProductID, img.ProductVariantID, img.URL, img.Alt, img.IsPrimary, img.SortOrder).
		Scan(&created.ID, &created.ProductID, &created.ProductVariantID, &created.URL, &created.Alt, &created.IsPrimary, &created.SortOrder, &created.CreatedAt, &created.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create product_image: %w", err)
	}
	return created, nil
}

// GetProductImageByID fetches a single image by id.
func (r *Repository) GetProductImageByID(ctx context.Context, id int64) (*ProductImage, error) {
	q := `SELECT id, product_id, product_variant_id, url, alt, is_primary, sort_order, created_at, updated_at FROM product_images WHERE id = $1`
	img := &ProductImage{}
	if err := r.db.QueryRow(ctx, q, id).
		Scan(&img.ID, &img.ProductID, &img.ProductVariantID, &img.URL, &img.Alt, &img.IsPrimary, &img.SortOrder, &img.CreatedAt, &img.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get product_image: %w", err)
	}
	return img, nil
}

// ListProductImagesByProduct lists images for a product ordered by primary desc, sort_order asc, created_at asc
func (r *Repository) ListProductImagesByProduct(ctx context.Context, productID int64) ([]*ProductImage, error) {
	q := `
		SELECT id, product_id, product_variant_id, url, alt, is_primary, sort_order, created_at, updated_at
		FROM product_images
		WHERE product_id = $1
		ORDER BY is_primary DESC, sort_order ASC, created_at ASC
	`
	rows, err := r.db.Query(ctx, q, productID)
	if err != nil {
		return nil, fmt.Errorf("list product_images: %w", err)
	}
	defer rows.Close()

	var out []*ProductImage
	for rows.Next() {
		var img ProductImage
		if err := rows.Scan(&img.ID, &img.ProductID, &img.ProductVariantID, &img.URL, &img.Alt, &img.IsPrimary, &img.SortOrder, &img.CreatedAt, &img.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan product_image: %w", err)
		}
		out = append(out, &img)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return out, nil
}

// SetPrimaryImage marks the given image id as primary for its product. It clears other primaries atomically.
func (r *Repository) SetPrimaryImage(ctx context.Context, productID, imageID int64) error {
	return r.WithTx(ctx, func(tx pgx.Tx) error {
		// Ensure the image belongs to the product (safe-guard)
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_images WHERE id=$1 AND product_id=$2)`, imageID, productID).Scan(&exists); err != nil {
			return fmt.Errorf("check image ownership: %w", err)
		}
		if !exists {
			return fmt.Errorf("image %d not found for product %d", imageID, productID)
		}

		if _, err := tx.Exec(ctx, `UPDATE product_images SET is_primary = false WHERE product_id = $1 AND is_primary = true`, productID); err != nil {
			return fmt.Errorf("clear primary: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE product_images SET is_primary = true, updated_at = now() WHERE id = $1`, imageID); err != nil {
			return fmt.Errorf("set primary: %w", err)
		}
		return nil
	})
}

// UpdateProductImage partial update using COALESCE. To clear a nullable field (alt) explicitly, pass &"" and handle in service layer.
func (r *Repository) UpdateProductImage(ctx context.Context, img *ProductImage) (*ProductImage, error) {
	if img == nil {
		return nil, fmt.Errorf("img nil")
	}
	q := `
		UPDATE product_images
		SET
			url = COALESCE(NULLIF($1, ''), url),
			alt = COALESCE($2, alt),
			is_primary = COALESCE($3, is_primary),
			sort_order = COALESCE($4, sort_order),
			updated_at = now()
		WHERE id = $5
		RETURNING id, product_id, product_variant_id, url, alt, is_primary, sort_order, created_at, updated_at
	`
	// We intentionally pass empty string for url if not provided (NULLIF handles it)
	var alt interface{} = img.Alt
	if img.Alt == nil {
		alt = nil
	}
	var isPrimary interface{} = nil
	var sortOrder interface{} = nil
	// We only overwrite is_primary if caller provided a value different from its zero default.
	isPrimary = img.IsPrimary // we always pass a boolean; COALESCE will accept it. If you want "not provided" support, use *bool.
	sortOrder = img.SortOrder // similar note: if 0 is ambiguous, consider *int

	updated := &ProductImage{}
	if err := r.db.QueryRow(ctx, q, img.URL, alt, isPrimary, sortOrder, img.ID).
		Scan(&updated.ID, &updated.ProductID, &updated.ProductVariantID, &updated.URL, &updated.Alt, &updated.IsPrimary, &updated.SortOrder, &updated.CreatedAt, &updated.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update product_image: %w", err)
	}
	return updated, nil
}

// DeleteProductImage deletes an image. Consider deleting remote asset in Cloudinary asynchronously.
func (r *Repository) DeleteProductImage(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM product_images WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete product_image: %w", err)
	}
	return nil
}

// ReorderProductImages sets sort_order for given image IDs in the provided order slice.
// The caller provides orderedIDs in desired display order. This runs in a single tx.
func (r *Repository) ReorderProductImages(ctx context.Context, productID int64, orderedIDs []int64) error {
	if len(orderedIDs) == 0 {
		return fmt.Errorf("no image IDs provided for reordering")
	}
	return r.WithTx(ctx, func(tx pgx.Tx) error {
		for idx, id := range orderedIDs {
			// ensure id belongs to product
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM product_images WHERE id = $1 AND product_id = $2)`, id, productID).Scan(&exists); err != nil {
				return fmt.Errorf("check belongs: %w", err)
			}
			if !exists {
				return fmt.Errorf("image %d does not belong to product %d", id, productID)
			}
			if _, err := tx.Exec(ctx, `UPDATE product_images SET sort_order = $1, updated_at = now() WHERE id = $2`, idx, id); err != nil {
				return fmt.Errorf("update sort_order: %w", err)
			}
		}
		return nil
	})
}

// ListProductCards returns a page of “product cards” with min active price and primary image.
// If categorySlug is non-empty, it includes the subtree of that category.
// total is a true total; we run a separate COUNT(*) which is cheap & accurate.
func (r *Repository) ListProductCards(
	ctx context.Context,
	categorySlug string,
	limit, offset int,
) ([]*ProductCard, int, error) {

	if limit <= 0 || limit > 30 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}

	// Build category filter by slug (include descendants)
	// If no slug provided, subtree is effectively all categories.
	catCTE := `
      WITH RECURSIVE cat_subtree AS (
        SELECT id, slug FROM categories WHERE ($1 = '' OR slug = $1)
        UNION ALL
        SELECT c.id, c.slug
        FROM categories c
        INNER JOIN cat_subtree cs ON c.parent_id = cs.id
      )
    `

	// Data query: brand/category names, primary image via LATERAL, min active price
	dataSQL := catCTE + `
      SELECT
        p.id, p.name, COALESCE(p.slug, ''), p.description,
        p.category_id, c.name AS category_name,
        p.brand_id, b.name AS brand_name,
        p.is_active, p.created_at, p.updated_at,
        mp.min_price_cents,
        img.url AS primary_image_url
      FROM products p
      LEFT JOIN brands b     ON b.id = p.brand_id
      LEFT JOIN categories c ON c.id = p.category_id
      LEFT JOIN LATERAL (
          SELECT MIN(v.price_cents) AS min_price_cents
          FROM product_variants v
          WHERE v.product_id = p.id AND v.is_active = true
      ) mp ON true
      LEFT JOIN LATERAL (
          SELECT i.url
          FROM product_images i
          WHERE i.product_id = p.id
          ORDER BY i.is_primary DESC, i.sort_order ASC, i.id ASC
          LIMIT 1
      ) img ON true
      WHERE ($1 = '' OR p.category_id IN (SELECT id FROM cat_subtree))
      ORDER BY p.id DESC
      LIMIT $2 OFFSET $3;
    `
	rows, err := r.db.Query(ctx, dataSQL, categorySlug, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list product cards: %w", err)
	}
	defer rows.Close()

	cards := make([]*ProductCard, 0, limit)
	for rows.Next() {
		var (
			pc         ProductCard
			desc       sql.NullString
			catName    sql.NullString
			brandName  sql.NullString
			primaryURL sql.NullString
			minPrice   sql.NullInt64
			slug       string
		)
		if err := rows.Scan(
			&pc.ID, &pc.Name, &slug, &desc,
			&pc.CategoryID, &catName,
			&pc.BrandID, &brandName,
			&pc.IsActive, &pc.CreatedAt, &pc.UpdatedAt,
			&minPrice,
			&primaryURL,
		); err != nil {
			return nil, 0, fmt.Errorf("scan product card: %w", err)
		}
		pc.Slug = slug
		if desc.Valid {
			s := desc.String
			pc.Description = &s
		}
		if catName.Valid {
			s := catName.String
			pc.CategoryName = &s
		}
		if brandName.Valid {
			s := brandName.String
			pc.BrandName = &s
		}
		if primaryURL.Valid {
			s := primaryURL.String
			pc.PrimaryImageURL = &s
		}
		if minPrice.Valid {
			v := minPrice.Int64
			pc.MinPriceCents = &v
		}
		cards = append(cards, &pc)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}

	// Count
	countSQL := catCTE + `
      SELECT COUNT(*)
      FROM products p
      WHERE ($1 = '' OR p.category_id IN (SELECT id FROM cat_subtree));
    `
	var total int
	if err := r.db.QueryRow(ctx, countSQL, categorySlug).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	return cards, total, nil
}

func (r *Repository) GetProductDetailBySlug(ctx context.Context, slug string) (*ProductDetail, error) {
	// 1) product + brand + category
	pSQL := `
      SELECT p.id, p.name, p.slug, p.description, p.category_id, p.brand_id, p.is_active, p.created_at, p.updated_at,
             b.id, b.name, b.slug, b.description, b.logo_url, b.created_at, b.updated_at,
             c.id, c.name, c.slug, c.parent_id, c.image_urls, c.is_active, c.created_at, c.updated_at
      FROM products p
      LEFT JOIN brands b     ON b.id = p.brand_id
      LEFT JOIN categories c ON c.id = p.category_id
      WHERE p.slug = $1 AND p.is_active = true
      LIMIT 1;
    `
	var (
		p                   Product
		b                   Brand
		c                   Category
		bNullID             sql.NullInt64
		cNullID             sql.NullInt64
		bSlug, bDesc, bLogo sql.NullString
		cSlug               sql.NullString
		cParent             sql.NullInt64
		cImgURLs            []string
	)
	row := r.db.QueryRow(ctx, pSQL, slug)
	if err := row.Scan(
		&p.ID, &p.Name, &p.Slug, &p.Description, &p.CategoryID, &p.BrandID, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		&bNullID, &b.Name, &bSlug, &bDesc, &bLogo, &b.CreatedAt, &b.UpdatedAt,
		&cNullID, &c.Name, &cSlug, &cParent, &cImgURLs, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("product detail: %w", err)
	}
	if bNullID.Valid {
		b.ID = bNullID.Int64
		if bSlug.Valid {
			s := bSlug.String
			b.Slug = &s
		}
		if bDesc.Valid {
			s := bDesc.String
			b.Description = &s
		}
		if bLogo.Valid {
			s := bLogo.String
			b.LogoURL = &s
		}
	} else {
		// no brand
		b = Brand{}
	}
	if cNullID.Valid {
		c.ID = cNullID.Int64
		if cSlug.Valid {
			c.Slug = cSlug.String
		}
		if cParent.Valid {
			pid := cParent.Int64
			c.ParentID = &pid
		}
		c.ImageURLs = cImgURLs
	} else {
		c = Category{}
	}

	// 2) active variants (ordered by price asc)
	vSQL := `
      SELECT id, product_id, price_cents, attributes, is_active, created_at, updated_at
      FROM product_variants
      WHERE product_id = $1 AND is_active = true
      ORDER BY price_cents ASC, id ASC;
    `
	vRows, err := r.db.Query(ctx, vSQL, p.ID)
	if err != nil {
		return nil, fmt.Errorf("variants: %w", err)
	}
	defer vRows.Close()
	vars := make([]*ProductVariant, 0, 8)
	for vRows.Next() {
		var v ProductVariant
		var attr []byte
		if err := vRows.Scan(&v.ID, &v.ProductID, &v.PriceCents, &attr, &v.IsActive, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan variant: %w", err)
		}
		_ = json.Unmarshal(attr, &v.Attributes)
		vars = append(vars, &v)
	}
	if err := vRows.Err(); err != nil {
		return nil, fmt.Errorf("variants rows: %w", err)
	}

	// 3) images (primary first)
	imgs, err := r.ListProductImagesByProduct(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("images: %w", err)
	}

	return &ProductDetail{
		Product: &p,
		Brand: func() *Brand {
			if bNullID.Valid {
				return &b
			}
			return nil
		}(),
		Category: func() *Category {
			if cNullID.Valid {
				return &c
			}
			return nil
		}(),
		Variants: vars,
		Images:   imgs,
	}, nil
}

// List admin products with counts (variants_count, images_count)
func (r *Repository) ListAdminProductCards(ctx context.Context, limit, offset int) ([]*ProductCard, int, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	dataSQL := `
      SELECT
        p.id, p.name, COALESCE(p.slug, ''), p.description,
        p.category_id, c.name AS category_name,
        p.brand_id, b.name AS brand_name,
        p.is_active, p.created_at, p.updated_at,
        COALESCE(v_cnt.cnt, 0) AS variants_count,
        COALESCE(i_cnt.cnt, 0) AS images_count
      FROM products p
      LEFT JOIN brands b     ON b.id = p.brand_id
      LEFT JOIN categories c ON c.id = p.category_id
      LEFT JOIN LATERAL (
          SELECT COUNT(*) AS cnt FROM product_variants v WHERE v.product_id = p.id
      ) v_cnt ON true
      LEFT JOIN LATERAL (
          SELECT COUNT(*) AS cnt FROM product_images i WHERE i.product_id = p.id
      ) i_cnt ON true
      ORDER BY p.id DESC
      LIMIT $1 OFFSET $2;
    `
	rows, err := r.db.Query(ctx, dataSQL, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("admin list products: %w", err)
	}
	defer rows.Close()

	type adminCard struct {
		ProductCard
		VariantsCount int `json:"variants_count"`
		ImagesCount   int `json:"images_count"`
	}

	out := make([]*ProductCard, 0, limit)
	for rows.Next() {
		var (
			pc                         ProductCard
			desc                       sql.NullString
			catName, brandName         sql.NullString
			slug                       string
			variantsCount, imagesCount int
		)
		if err := rows.Scan(
			&pc.ID, &pc.Name, &slug, &desc,
			&pc.CategoryID, &catName,
			&pc.BrandID, &brandName,
			&pc.IsActive, &pc.CreatedAt, &pc.UpdatedAt,
			&variantsCount, &imagesCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan admin product card: %w", err)
		}
		pc.Slug = slug
		if desc.Valid {
			s := desc.String
			pc.Description = &s
		}
		if catName.Valid {
			s := catName.String
			pc.CategoryName = &s
		}
		if brandName.Valid {
			s := brandName.String
			pc.BrandName = &s
		}
		// We’ll attach counts in handler if needed
		out = append(out, &pc)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM products`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}
	return out, total, nil
}

func (r *Repository) SearchProducts(ctx context.Context, query string, limit, offset int) ([]*Product, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	q := `
SELECT id, name, slug, description, category_id, brand_id, is_active, created_at, updated_at,
       COUNT(*) OVER() AS total
FROM products
WHERE is_active = true
  AND (name ILIKE '%' || $1 || '%' OR description ILIKE '%' || $1 || '%')
ORDER BY id DESC
LIMIT $2 OFFSET $3;
`
	rows, err := r.db.Query(ctx, q, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search products: %w", err)
	}
	defer rows.Close()

	var products []*Product
	var total int
	for rows.Next() {
		var p Product
		var t int
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Slug, &p.Description, &p.CategoryID, &p.BrandID, &p.IsActive,
			&p.CreatedAt, &p.UpdatedAt, &t,
		); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		if total == 0 {
			total = t
		}
		products = append(products, &p)
	}

	return products, total, nil
}

func (r *Repository) FullTextSearchProducts(ctx context.Context, query string, limit, offset int) ([]*ProductWithRank, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	q := `
WITH ranked AS (
  SELECT
    id, name, slug, description, category_id, brand_id, is_active,
    created_at, updated_at,
    ts_rank_cd(fts, plainto_tsquery('english', $1)) AS rank
  FROM products
  WHERE fts @@ plainto_tsquery('english', $1)
)
SELECT *,
       COUNT(*) OVER() AS total
FROM ranked
ORDER BY rank DESC
LIMIT $2 OFFSET $3;
`
	rows, err := r.db.Query(ctx, q, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("fts search products: %w", err)
	}
	defer rows.Close()

	var list []*ProductWithRank
	var total int
	for rows.Next() {
		var p ProductWithRank
		var t int
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Slug, &p.Description,
			&p.CategoryID, &p.BrandID, &p.IsActive,
			&p.CreatedAt, &p.UpdatedAt, &p.Rank, &t,
		); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		if total == 0 {
			total = t
		}
		list = append(list, &p)
	}
	return list, total, nil
}
