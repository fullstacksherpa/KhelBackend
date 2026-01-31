package featured

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"khel/internal/params"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// pg error helpers (kept local to repository)
func isUniqueViolation(err error) (*pgconn.PgError, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr, true
	}
	return nil, false
}

func isFKViolation(err error) (*pgconn.PgError, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return pgErr, true
	}
	return nil, false
}

// ------------------- Collections CRUD -------------------

// CreateCollection inserts into featured_collections (source of truth).
func (r *Repository) CreateCollection(ctx context.Context, req CreateCollectionRequest) (*FeaturedCollection, error) {
	// Default active true if not provided
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	const q = `
INSERT INTO featured_collections (key, title, type, description, is_active, starts_at, ends_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
RETURNING id, key, title, type, description, is_active, starts_at, ends_at, created_at, updated_at;
`
	var out FeaturedCollection
	err := r.db.QueryRow(ctx, q,
		req.Key,
		req.Title,
		req.Type,
		req.Description,
		isActive,
		req.StartsAt,
		req.EndsAt,
	).Scan(
		&out.ID,
		&out.Key,
		&out.Title,
		&out.Type,
		&out.Description,
		&out.IsActive,
		&out.StartsAt,
		&out.EndsAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		if pgErr, ok := isUniqueViolation(err); ok {
			// Most likely featured_collections.key unique
			if strings.Contains(pgErr.ConstraintName, "featured_collections") {
				return nil, ErrDuplicateCollectionKey
			}
			return nil, ErrDuplicateCollectionKey
		}
		return nil, fmt.Errorf("create featured collection: %w", err)
	}

	return &out, nil
}

func (r *Repository) GetCollectionByID(ctx context.Context, id int64) (*FeaturedCollection, error) {
	const q = `
SELECT id, key, title, type, description, is_active, starts_at, ends_at, created_at, updated_at
FROM featured_collections
WHERE id = $1
LIMIT 1;
`
	var out FeaturedCollection
	err := r.db.QueryRow(ctx, q, id).Scan(
		&out.ID,
		&out.Key,
		&out.Title,
		&out.Type,
		&out.Description,
		&out.IsActive,
		&out.StartsAt,
		&out.EndsAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get featured collection by id: %w", err)
	}
	return &out, nil
}

func (r *Repository) UpdateCollection(ctx context.Context, id int64, req UpdateCollectionRequest) (*FeaturedCollection, error) {
	set := make([]string, 0, 8)
	args := make([]any, 0, 10)
	argPos := 1

	// Build partial update safely.
	if req.Key != nil {
		set = append(set, fmt.Sprintf("key = $%d", argPos))
		args = append(args, *req.Key)
		argPos++
	}
	if req.Title != nil {
		set = append(set, fmt.Sprintf("title = $%d", argPos))
		args = append(args, *req.Title)
		argPos++
	}
	if req.Type != nil {
		set = append(set, fmt.Sprintf("type = $%d", argPos))
		args = append(args, *req.Type)
		argPos++
	}
	if req.Description != nil {
		set = append(set, fmt.Sprintf("description = $%d", argPos))
		args = append(args, *req.Description)
		argPos++
	}
	if req.IsActive != nil {
		set = append(set, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *req.IsActive)
		argPos++
	}
	if req.StartsAt != nil {
		set = append(set, fmt.Sprintf("starts_at = $%d", argPos))
		args = append(args, *req.StartsAt)
		argPos++
	}
	if req.EndsAt != nil {
		set = append(set, fmt.Sprintf("ends_at = $%d", argPos))
		args = append(args, *req.EndsAt)
		argPos++
	}

	if len(set) == 0 {
		return nil, ErrNoFieldsToUpdate
	}

	// Where id
	args = append(args, id)

	q := fmt.Sprintf(`
UPDATE featured_collections
SET %s
WHERE id = $%d
RETURNING id, key, title, type, description, is_active, starts_at, ends_at, created_at, updated_at;
`, strings.Join(set, ", "), argPos)

	var out FeaturedCollection
	err := r.db.QueryRow(ctx, q, args...).Scan(
		&out.ID,
		&out.Key,
		&out.Title,
		&out.Type,
		&out.Description,
		&out.IsActive,
		&out.StartsAt,
		&out.EndsAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if _, ok := isUniqueViolation(err); ok {
			return nil, ErrDuplicateCollectionKey
		}
		return nil, fmt.Errorf("update featured collection: %w", err)
	}

	return &out, nil
}

func (r *Repository) DeleteCollection(ctx context.Context, id int64) error {
	const q = `
DELETE FROM featured_collections
WHERE id = $1
RETURNING id;
`
	var deletedID int64
	if err := r.db.QueryRow(ctx, q, id).Scan(&deletedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("delete featured collection: %w", err)
	}
	return nil
}

func (r *Repository) ListCollections(ctx context.Context, p params.Pagination, f CollectionFilters) (*CollectionList, error) {
	// Use batch: count + list in one round-trip.
	b := &pgx.Batch{}

	where := []string{"1=1"}
	args := []any{}
	argPos := 1

	if f.Search != nil && strings.TrimSpace(*f.Search) != "" {
		where = append(where, fmt.Sprintf("(key ILIKE $%d OR title ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+strings.TrimSpace(*f.Search)+"%")
		argPos++
	}
	if f.Type != nil && strings.TrimSpace(*f.Type) != "" {
		where = append(where, fmt.Sprintf("type = $%d", argPos))
		args = append(args, strings.TrimSpace(*f.Type))
		argPos++
	}
	if f.Active != nil {
		where = append(where, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *f.Active)
		argPos++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM featured_collections %s;`, whereSQL)
	listQ := fmt.Sprintf(`
SELECT id, key, title, type, description, is_active, starts_at, ends_at, created_at, updated_at
FROM featured_collections
%s
ORDER BY created_at DESC
LIMIT $%d OFFSET $%d;
`, whereSQL, argPos, argPos+1)

	b.Queue(countQ, args...)
	b.Queue(listQ, append(args, p.Limit, p.Offset)...)

	br := r.db.SendBatch(ctx, b)
	defer br.Close()

	var total int
	if err := br.QueryRow().Scan(&total); err != nil {
		return nil, fmt.Errorf("scan featured collections count: %w", err)
	}

	// Compute pagination meta using your helper.
	p.ComputeMeta(total)

	rows, err := br.Query()
	if err != nil {
		return nil, fmt.Errorf("query featured collections list: %w", err)
	}
	defer rows.Close()

	out := make([]FeaturedCollection, 0, p.Limit)
	for rows.Next() {
		var c FeaturedCollection
		if err := rows.Scan(
			&c.ID, &c.Key, &c.Title, &c.Type, &c.Description,
			&c.IsActive, &c.StartsAt, &c.EndsAt, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan featured collection row: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("featured collections rows error: %w", err)
	}

	return &CollectionList{
		Collections: out,
		Pagination:  ToPaginationInfo(p),
	}, nil
}

// ------------------- Items CRUD -------------------

// CreateItem inserts into featured_items.
func (r *Repository) CreateItem(ctx context.Context, req CreateItemRequest) (*FeaturedItem, error) {
	// Validate business rule (DB also checks, but we want clean API errors)
	if req.ProductID == nil && req.ProductVariantID == nil {
		return nil, errors.New("either product_id or product_variant_id is required")
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	const q = `
INSERT INTO featured_items (
  collection_id, position, badge_text, subtitle, deal_price_cents, deal_percent,
  product_id, product_variant_id, is_active, starts_at, ends_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING
  id, collection_id, position, badge_text, subtitle, deal_price_cents, deal_percent,
  product_id, product_variant_id, is_active, starts_at, ends_at, created_at, updated_at;
`
	var out FeaturedItem
	err := r.db.QueryRow(ctx, q,
		req.CollectionID,
		req.Position,
		req.BadgeText,
		req.Subtitle,
		req.DealPriceCents,
		req.DealPercent,
		req.ProductID,
		req.ProductVariantID,
		isActive,
		req.StartsAt,
		req.EndsAt,
	).Scan(
		&out.ID,
		&out.CollectionID,
		&out.Position,
		&out.BadgeText,
		&out.Subtitle,
		&out.DealPriceCents,
		&out.DealPercent,
		&out.ProductID,
		&out.ProductVariantID,
		&out.IsActive,
		&out.StartsAt,
		&out.EndsAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		if pgErr, ok := isUniqueViolation(err); ok {
			// uq_featured_items_collection_position
			if strings.Contains(pgErr.ConstraintName, "uq_featured_items_collection_position") {
				return nil, ErrDuplicateItemPosition
			}
			return nil, ErrDuplicateItemPosition
		}
		if _, ok := isFKViolation(err); ok {
			// Could be invalid collection_id, product_id, or product_variant_id
			return nil, fmt.Errorf("invalid reference id: %w", err)
		}
		return nil, fmt.Errorf("create featured item: %w", err)
	}

	return &out, nil
}

func (r *Repository) GetItemByID(ctx context.Context, id int64) (*FeaturedItem, error) {
	const q = `
SELECT
  id, collection_id, position, badge_text, subtitle, deal_price_cents, deal_percent,
  product_id, product_variant_id, is_active, starts_at, ends_at, created_at, updated_at
FROM featured_items
WHERE id = $1
LIMIT 1;
`
	var out FeaturedItem
	err := r.db.QueryRow(ctx, q, id).Scan(
		&out.ID,
		&out.CollectionID,
		&out.Position,
		&out.BadgeText,
		&out.Subtitle,
		&out.DealPriceCents,
		&out.DealPercent,
		&out.ProductID,
		&out.ProductVariantID,
		&out.IsActive,
		&out.StartsAt,
		&out.EndsAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrItemNotFound
		}
		return nil, fmt.Errorf("get featured item by id: %w", err)
	}
	return &out, nil
}

func (r *Repository) UpdateItem(ctx context.Context, id int64, req UpdateItemRequest) (*FeaturedItem, error) {
	set := make([]string, 0, 10)
	args := make([]any, 0, 12)
	argPos := 1

	if req.Position != nil {
		set = append(set, fmt.Sprintf("position = $%d", argPos))
		args = append(args, *req.Position)
		argPos++
	}
	if req.BadgeText != nil {
		set = append(set, fmt.Sprintf("badge_text = $%d", argPos))
		args = append(args, *req.BadgeText)
		argPos++
	}
	if req.Subtitle != nil {
		set = append(set, fmt.Sprintf("subtitle = $%d", argPos))
		args = append(args, *req.Subtitle)
		argPos++
	}
	if req.DealPriceCents != nil {
		set = append(set, fmt.Sprintf("deal_price_cents = $%d", argPos))
		args = append(args, *req.DealPriceCents)
		argPos++
	}
	if req.DealPercent != nil {
		set = append(set, fmt.Sprintf("deal_percent = $%d", argPos))
		args = append(args, *req.DealPercent)
		argPos++
	}
	if req.ProductID != nil {
		set = append(set, fmt.Sprintf("product_id = $%d", argPos))
		args = append(args, *req.ProductID)
		argPos++
	}
	if req.ProductVariantID != nil {
		set = append(set, fmt.Sprintf("product_variant_id = $%d", argPos))
		args = append(args, *req.ProductVariantID)
		argPos++
	}
	if req.IsActive != nil {
		set = append(set, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *req.IsActive)
		argPos++
	}
	if req.StartsAt != nil {
		set = append(set, fmt.Sprintf("starts_at = $%d", argPos))
		args = append(args, *req.StartsAt)
		argPos++
	}
	if req.EndsAt != nil {
		set = append(set, fmt.Sprintf("ends_at = $%d", argPos))
		args = append(args, *req.EndsAt)
		argPos++
	}

	if len(set) == 0 {
		return nil, ErrNoFieldsToUpdate
	}

	args = append(args, id)

	q := fmt.Sprintf(`
UPDATE featured_items
SET %s
WHERE id = $%d
RETURNING
  id, collection_id, position, badge_text, subtitle, deal_price_cents, deal_percent,
  product_id, product_variant_id, is_active, starts_at, ends_at, created_at, updated_at;
`, strings.Join(set, ", "), argPos)

	var out FeaturedItem
	err := r.db.QueryRow(ctx, q, args...).Scan(
		&out.ID,
		&out.CollectionID,
		&out.Position,
		&out.BadgeText,
		&out.Subtitle,
		&out.DealPriceCents,
		&out.DealPercent,
		&out.ProductID,
		&out.ProductVariantID,
		&out.IsActive,
		&out.StartsAt,
		&out.EndsAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrItemNotFound
		}
		if pgErr, ok := isUniqueViolation(err); ok {
			if strings.Contains(pgErr.ConstraintName, "uq_featured_items_collection_position") {
				return nil, ErrDuplicateItemPosition
			}
			return nil, ErrDuplicateItemPosition
		}
		return nil, fmt.Errorf("update featured item: %w", err)
	}

	// Optional sanity: ensure we didn't end up with both refs NULL
	if out.ProductID == nil && out.ProductVariantID == nil {
		return nil, errors.New("invalid state: both product_id and product_variant_id are null")
	}

	return &out, nil
}

func (r *Repository) DeleteItem(ctx context.Context, id int64) error {
	const q = `
DELETE FROM featured_items
WHERE id = $1
RETURNING id;
`
	var deletedID int64
	if err := r.db.QueryRow(ctx, q, id).Scan(&deletedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrItemNotFound
		}
		return fmt.Errorf("delete featured item: %w", err)
	}
	return nil
}

func (r *Repository) ListItemsByCollection(ctx context.Context, collectionID int64, p params.Pagination, f ItemFilters) (*ItemList, error) {
	b := &pgx.Batch{}

	where := []string{"collection_id = $1"}
	args := []any{collectionID}
	argPos := 2

	if f.Active != nil {
		where = append(where, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *f.Active)
		argPos++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM featured_items %s;`, whereSQL)
	listQ := fmt.Sprintf(`
SELECT
  id, collection_id, position, badge_text, subtitle, deal_price_cents, deal_percent,
  product_id, product_variant_id, is_active, starts_at, ends_at, created_at, updated_at
FROM featured_items
%s
ORDER BY position ASC, id ASC
LIMIT $%d OFFSET $%d;
`, whereSQL, argPos, argPos+1)

	b.Queue(countQ, args...)
	b.Queue(listQ, append(args, p.Limit, p.Offset)...)

	br := r.db.SendBatch(ctx, b)
	defer br.Close()

	var total int
	if err := br.QueryRow().Scan(&total); err != nil {
		return nil, fmt.Errorf("scan featured items count: %w", err)
	}

	p.ComputeMeta(total)

	rows, err := br.Query()
	if err != nil {
		return nil, fmt.Errorf("query featured items list: %w", err)
	}
	defer rows.Close()

	out := make([]FeaturedItem, 0, p.Limit)
	for rows.Next() {
		var it FeaturedItem
		if err := rows.Scan(
			&it.ID,
			&it.CollectionID,
			&it.Position,
			&it.BadgeText,
			&it.Subtitle,
			&it.DealPriceCents,
			&it.DealPercent,
			&it.ProductID,
			&it.ProductVariantID,
			&it.IsActive,
			&it.StartsAt,
			&it.EndsAt,
			&it.CreatedAt,
			&it.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan featured item row: %w", err)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("featured items rows error: %w", err)
	}

	return &ItemList{
		CollectionID: collectionID,
		Items:        out,
		Pagination:   ToPaginationInfo(p),
	}, nil
}
