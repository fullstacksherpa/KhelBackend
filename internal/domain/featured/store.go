package featured

import (
	"context"
	"errors"
	"fmt"
	"time"

	"khel/internal/params"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("featured collection not found")
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

// GetHomeCollections returns all collections with up to 10 items each. It query featured_collections_cache so data might be stale. be sharp
// Fast because SQL does the "top N per group" using window functions.
func (r *Repository) GetHomeCollections(ctx context.Context) ([]CollectionPreview, error) {
	const q = `
WITH ranked AS (
  SELECT
    collection_key,
    collection_title,
    collection_type,
    collection_description,

    item_id,
    product_id,
    product_name,
    product_slug,
    variant_id,
    variant_price_cents,
    image_url,
    deal_price_cents,
    deal_percent,
    badge_text,
    subtitle,
    position,

    COUNT(*) OVER (PARTITION BY collection_key) AS total_items,
    MIN(cached_at) OVER (PARTITION BY collection_key) AS cached_at,
    ROW_NUMBER() OVER (PARTITION BY collection_key ORDER BY position ASC) AS rn
  FROM featured_collections_cache
)
SELECT
  collection_key,
  collection_title,
  collection_type,
  collection_description,

  item_id,
  product_id,
  product_name,
  product_slug,
  variant_id,
  variant_price_cents,
  image_url,
  deal_price_cents,
  deal_percent,
  badge_text,
  subtitle,
  position,

  total_items,
  cached_at
FROM ranked
WHERE rn <= 10
ORDER BY collection_type ASC, collection_key ASC, position ASC;
`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query featured home collections: %w", err)
	}
	defer rows.Close()

	collections := make(map[string]*CollectionPreview)
	order := make([]string, 0, 8)

	for rows.Next() {
		var (
			key   string
			title string
			typ   string
			desc  *string

			item CollectionItem

			totalItems int
			cachedAt   time.Time
		)

		err := rows.Scan(
			&key,
			&title,
			&typ,
			&desc,

			&item.ItemID,
			&item.ProductID,
			&item.ProductName,
			&item.ProductSlug,
			&item.VariantID,
			&item.PriceCents,
			&item.ImageURL,
			&item.DealPriceCents,
			&item.DealPercent,
			&item.BadgeText,
			&item.Subtitle,
			&item.Position,

			&totalItems,
			&cachedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan featured home row: %w", err)
		}

		if _, ok := collections[key]; !ok {
			collections[key] = &CollectionPreview{
				Key:         key,
				Title:       title,
				Type:        typ,
				Description: desc,
				TotalItems:  totalItems,
				Items:       make([]CollectionItem, 0, 10),
				CachedAt:    cachedAt,
			}
			order = append(order, key)
		}

		collections[key].Items = append(collections[key].Items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("featured home rows error: %w", err)
	}

	out := make([]CollectionPreview, 0, len(order))
	for _, k := range order {
		out = append(out, *collections[k])
	}
	return out, nil
}

// GetCollectionItems returns a single collection with paginated items. t query featured_collections_cache so data might be stale.
// Uses your params.Pagination struct (ParsePagination + ComputeMeta).
func (r *Repository) GetCollectionItems(
	ctx context.Context,
	collectionKey string,
	p params.Pagination,
) (*CollectionDetail, error) {
	// Batch queries reduce latency (one round-trip in most pgx configs).
	b := &pgx.Batch{}

	// Collection info: use featured_collections as source of truth.
	// NOTE: This does NOT depend on MV refresh timing (good).
	const infoQ = `
SELECT key, title, type, description
FROM featured_collections
WHERE key = $1
  AND is_active = TRUE
  AND (starts_at IS NULL OR starts_at <= now())
  AND (ends_at   IS NULL OR ends_at   >  now())
LIMIT 1;
`
	b.Queue(infoQ, collectionKey)

	// Total count from MV (reflects last refresh).
	const countQ = `
SELECT COUNT(*)
FROM featured_collections_cache
WHERE collection_key = $1;
`
	b.Queue(countQ, collectionKey)

	// Items page from MV.
	const itemsQ = `
SELECT
  item_id,
  product_id,
  product_name,
  product_slug,
  variant_id,
  variant_price_cents,
  image_url,
  deal_price_cents,
  deal_percent,
  badge_text,
  subtitle,
  position,
  cached_at
FROM featured_collections_cache
WHERE collection_key = $1
ORDER BY position ASC
LIMIT $2 OFFSET $3;
`
	b.Queue(itemsQ, collectionKey, p.Limit, p.Offset)

	br := r.db.SendBatch(ctx, b)
	defer br.Close()

	// --- info ---
	var info CollectionInfo
	if err := br.QueryRow().Scan(&info.Key, &info.Title, &info.Type, &info.Description); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan collection info: %w", err)
	}

	// --- count ---
	var total int
	if err := br.QueryRow().Scan(&total); err != nil {
		return nil, fmt.Errorf("scan total count: %w", err)
	}

	// Compute pagination metadata using your helper.
	p.ComputeMeta(total)

	// --- items ---
	rows, err := br.Query()
	if err != nil {
		return nil, fmt.Errorf("query items batch: %w", err)
	}
	defer rows.Close()

	items := make([]CollectionItem, 0, p.Limit)
	var cachedAt time.Time

	for rows.Next() {
		var it CollectionItem
		var rowCachedAt time.Time

		if err := rows.Scan(
			&it.ItemID,
			&it.ProductID,
			&it.ProductName,
			&it.ProductSlug,
			&it.VariantID,
			&it.PriceCents,
			&it.ImageURL,
			&it.DealPriceCents,
			&it.DealPercent,
			&it.BadgeText,
			&it.Subtitle,
			&it.Position,
			&rowCachedAt,
		); err != nil {
			return nil, fmt.Errorf("scan collection item: %w", err)
		}

		cachedAt = rowCachedAt
		items = append(items, it)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("items rows error: %w", err)
	}

	return &CollectionDetail{
		Collection: info,
		Pagination: PaginationInfo{
			Page:       p.Page,
			Limit:      p.Limit,
			Offset:     p.Offset,
			TotalItems: p.Total,
			TotalPages: p.TotalPages,
			HasNext:    p.HasNext,
			HasPrev:    p.HasPrev,
		},
		Items:    items,
		CachedAt: cachedAt,
	}, nil
}

// RefreshCache refreshes the MV for fast app reads.
// IMPORTANT: REFRESH ... CONCURRENTLY cannot run inside a transaction.
func (r *Repository) RefreshCache(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `REFRESH MATERIALIZED VIEW CONCURRENTLY featured_collections_cache`)
	if err != nil {
		return fmt.Errorf("refresh featured_collections_cache: %w", err)
	}
	return nil
}
