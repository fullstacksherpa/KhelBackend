package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ad represents the ads table structure
type Ad struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	Description  *string   `json:"description"`
	ImageURL     string    `json:"image_url"`
	ImageAlt     *string   `json:"image_alt"`
	Link         *string   `json:"link"`
	Active       bool      `json:"active"`
	DisplayOrder int       `json:"display_order"`
	Impressions  int       `json:"impressions"`
	Clicks       int       `json:"clicks"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateAdRequest represents the request payload for creating an ad
type CreateAdRequest struct {
	Title        string  `json:"title"`
	Description  *string `json:"description"`
	ImageURL     string  `json:"image_url"`
	ImageAlt     *string `json:"image_alt"`
	Link         *string `json:"link"`
	DisplayOrder int     `json:"display_order"`
}

// UpdateAdRequest represents the request payload for updating an ad
type UpdateAdRequest struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	ImageURL     *string `json:"image_url"`
	ImageAlt     *string `json:"image_alt"`
	Link         *string `json:"link"`
	Active       *bool   `json:"active"`
	DisplayOrder *int    `json:"display_order"`
}

type AdsStore struct {
	db *pgxpool.Pool
}

// GetActiveAds returns all active ads ordered by display_order and created_at
func (s *AdsStore) GetActiveAds(ctx context.Context) ([]Ad, error) {
	query := `
		SELECT id, title, description, image_url, image_alt, link, active, 
		       display_order, impressions, clicks, created_at, updated_at
		FROM ads 
		WHERE active = TRUE 
		ORDER BY display_order ASC, created_at DESC
	`

	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active ads: %w", err)
	}
	defer rows.Close()

	var ads []Ad
	for rows.Next() {
		var ad Ad
		err := rows.Scan(
			&ad.ID, &ad.Title, &ad.Description, &ad.ImageURL, &ad.ImageAlt,
			&ad.Link, &ad.Active, &ad.DisplayOrder, &ad.Impressions, &ad.Clicks,
			&ad.CreatedAt, &ad.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ad row: %w", err)
		}
		ads = append(ads, ad)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return ads, nil
}

// GetAllAds returns all ads with pagination for admin dashboard
func (s *AdsStore) GetAllAds(ctx context.Context, limit, offset int) ([]Ad, int, error) {
	//Get total count
	var totalCount int
	countQuery := `SELECT COUNT(*) FROM ads`
	err := s.db.QueryRow(ctx, countQuery).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	//Get ads with pagination

	query := `
	   SELECT id, title, description, image_url, image_alt, link, active,        display_order, impressions, clicks, created_at, updated_at 
	   FROM ads
	   ORDER BY display_order ASC, created_at DESC
	   LIMIT $1 OFFSET $2
	`

	rows, err := s.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query all ads: %w", err)
	}
	defer rows.Close()

	var ads []Ad
	for rows.Next() {
		var ad Ad
		err := rows.Scan(
			&ad.ID, &ad.Title, &ad.Description, &ad.ImageURL, &ad.ImageAlt,
			&ad.Link, &ad.Active, &ad.DisplayOrder, &ad.Impressions, &ad.Clicks,
			&ad.CreatedAt, &ad.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan ad row: %w", err)
		}
		ads = append(ads, ad)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating over rows: %w", err)
	}

	return ads, totalCount, nil
}

// GetAdByID retrieves a single ad by its ID
func (s *AdsStore) GetAdByID(ctx context.Context, id int64) (*Ad, error) {
	query := `
		SELECT id, title, description, image_url, image_alt, link, active, 
		       display_order, impressions, clicks, created_at, updated_at
		FROM ads 
		WHERE id = $1
	`

	var ad Ad
	err := s.db.QueryRow(ctx, query, id).Scan(
		&ad.ID, &ad.Title, &ad.Description, &ad.ImageURL, &ad.ImageAlt,
		&ad.Link, &ad.Active, &ad.DisplayOrder, &ad.Impressions, &ad.Clicks,
		&ad.CreatedAt, &ad.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("ad not found")
		}
		return nil, fmt.Errorf("failed to get ad by ID: %w", err)
	}

	return &ad, nil
}

// CreateAd creates a new ad
func (s *AdsStore) CreateAd(ctx context.Context, req CreateAdRequest) (*Ad, error) {
	query := `
		INSERT INTO ads (title, description, image_url, image_alt, link, display_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, title, description, image_url, image_alt, link, active, 
		         display_order, impressions, clicks, created_at, updated_at
	`

	var ad Ad
	err := s.db.QueryRow(ctx, query,
		req.Title, req.Description, req.ImageURL, req.ImageAlt, req.Link, req.DisplayOrder,
	).Scan(
		&ad.ID, &ad.Title, &ad.Description, &ad.ImageURL, &ad.ImageAlt,
		&ad.Link, &ad.Active, &ad.DisplayOrder, &ad.Impressions, &ad.Clicks,
		&ad.CreatedAt, &ad.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create ad: %w", err)
	}

	return &ad, nil
}

// UpdateAd updates an existing ad with dynamic fields
func (s *AdsStore) UpdateAd(ctx context.Context, id int64, req UpdateAdRequest) (*Ad, error) {
	setParts := []string{}
	args := []interface{}{}
	argIndex := 1

	if req.Title != nil {
		setParts = append(setParts, fmt.Sprintf("title = $%d", argIndex))
		args = append(args, *req.Title)
		argIndex++
	}
	if req.Description != nil {
		setParts = append(setParts, fmt.Sprintf("description = $%d", argIndex))
		args = append(args, *req.Description)
		argIndex++
	}
	if req.ImageURL != nil {
		setParts = append(setParts, fmt.Sprintf("image_url = $%d", argIndex))
		args = append(args, *req.ImageURL)
		argIndex++
	}
	if req.ImageAlt != nil {
		setParts = append(setParts, fmt.Sprintf("image_alt = $%d", argIndex))
		args = append(args, *req.ImageAlt)
		argIndex++
	}
	if req.Link != nil {
		setParts = append(setParts, fmt.Sprintf("link = $%d", argIndex))
		args = append(args, *req.Link)
		argIndex++
	}
	if req.Active != nil {
		setParts = append(setParts, fmt.Sprintf("active = $%d", argIndex))
		args = append(args, *req.Active)
		argIndex++
	}
	if req.DisplayOrder != nil {
		setParts = append(setParts, fmt.Sprintf("display_order = $%d", argIndex))
		args = append(args, *req.DisplayOrder)
		argIndex++
	}

	if len(setParts) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	// Always update updated_at
	setParts = append(setParts, fmt.Sprintf("updated_at = $%d", argIndex))
	args = append(args, time.Now())
	argIndex++

	// Add the ID for WHERE clause
	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE ads 
		SET %s
		WHERE id = $%d
		RETURNING id, title, description, image_url, image_alt, link, active, 
		         display_order, impressions, clicks, created_at, updated_at
	`, strings.Join(setParts, ", "), argIndex)

	var ad Ad
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&ad.ID, &ad.Title, &ad.Description, &ad.ImageURL, &ad.ImageAlt,
		&ad.Link, &ad.Active, &ad.DisplayOrder, &ad.Impressions, &ad.Clicks,
		&ad.CreatedAt, &ad.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("ad not found")
		}
		return nil, fmt.Errorf("failed to update ad: %w", err)
	}

	return &ad, nil
}

// DeleteAd deletes an ad by ID
func (s *AdsStore) DeleteAd(ctx context.Context, id int64) error {
	query := "DELETE FROM ads WHERE id = $1"

	cmdTag, err := s.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete ad: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("ad not found")
	}

	return nil
}

// ToggleAdStatus toggles the active status of an ad
func (s *AdsStore) ToggleAdStatus(ctx context.Context, id int64) (*Ad, error) {
	query := `
		UPDATE ads 
		SET active = NOT active, updated_at = NOW()
		WHERE id = $1
		RETURNING id, title, description, image_url, image_alt, link, active, 
		         display_order, impressions, clicks, created_at, updated_at
	`

	var ad Ad
	err := s.db.QueryRow(ctx, query, id).Scan(
		&ad.ID, &ad.Title, &ad.Description, &ad.ImageURL, &ad.ImageAlt,
		&ad.Link, &ad.Active, &ad.DisplayOrder, &ad.Impressions, &ad.Clicks,
		&ad.CreatedAt, &ad.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("ad not found")
		}
		return nil, fmt.Errorf("failed to toggle ad status: %w", err)
	}

	return &ad, nil
}

// IncrementImpressions increments the impressions count for an ad
func (s *AdsStore) IncrementImpressions(ctx context.Context, id int64) error {
	query := "UPDATE ads SET impressions = impressions + 1 WHERE id = $1"

	cmdTag, err := s.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment impressions: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("ad not found")
	}

	return nil
}

// IncrementClicks increments the clicks count for an ad
func (s *AdsStore) IncrementClicks(ctx context.Context, id int64) error {
	query := "UPDATE ads SET clicks = clicks + 1 WHERE id = $1"

	cmdTag, err := s.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment clicks: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("ad not found")
	}

	return nil
}

// Analytics represents ads analytics data
type Analytics struct {
	TotalAds         int     `json:"total_ads"`
	ActiveAds        int     `json:"active_ads"`
	TotalImpressions int64   `json:"total_impressions"`
	TotalClicks      int64   `json:"total_clicks"`
	AverageCTR       float64 `json:"average_ctr"`
	TopPerformingAds []Ad    `json:"top_performing_ads"`
}

// GetAdsAnalytics retrieves analytics data for ads
func (s *AdsStore) GetAdsAnalytics(ctx context.Context) (*Analytics, error) {
	analytics := &Analytics{}

	// Get total and active ads count, total impressions and clicks
	statsQuery := `
		SELECT 
			COUNT(*) as total_ads,
			COUNT(CASE WHEN active = TRUE THEN 1 END) as active_ads,
			COALESCE(SUM(impressions), 0) as total_impressions,
			COALESCE(SUM(clicks), 0) as total_clicks
		FROM ads
	`

	err := s.db.QueryRow(ctx, statsQuery).Scan(
		&analytics.TotalAds,
		&analytics.ActiveAds,
		&analytics.TotalImpressions,
		&analytics.TotalClicks,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get ads statistics: %w", err)
	}

	// Calculate average CTR
	if analytics.TotalImpressions > 0 {
		analytics.AverageCTR = float64(analytics.TotalClicks) / float64(analytics.TotalImpressions) * 100
	}

	// Get top performing ads (by CTR)
	topAdsQuery := `
		SELECT id, title, description, image_url, image_alt, link, active, 
		       display_order, impressions, clicks, created_at, updated_at
		FROM ads 
		WHERE impressions > 0
		ORDER BY (CAST(clicks AS FLOAT) / CAST(impressions AS FLOAT)) DESC
		LIMIT 5
	`

	rows, err := s.db.Query(ctx, topAdsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get top performing ads: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ad Ad
		err := rows.Scan(
			&ad.ID, &ad.Title, &ad.Description, &ad.ImageURL, &ad.ImageAlt,
			&ad.Link, &ad.Active, &ad.DisplayOrder, &ad.Impressions, &ad.Clicks,
			&ad.CreatedAt, &ad.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan top performing ad: %w", err)
		}

		analytics.TopPerformingAds = append(analytics.TopPerformingAds, ad)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over top performing ads: %w", err)
	}

	return analytics, nil
}

type DisplayOrderUpdate struct {
	ID           int64
	DisplayOrder int
}

// BulkUpdateDisplayOrder updates display order for multiple ads in a transaction
func (s *AdsStore) BulkUpdateDisplayOrder(ctx context.Context, updates []DisplayOrderUpdate) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, update := range updates {
		query := "UPDATE ads SET display_order = $1, updated_at = NOW() WHERE id = $2"
		_, err := tx.Exec(ctx, query, update.DisplayOrder, update.ID)
		if err != nil {
			return fmt.Errorf("failed to update display order for ad %d: %w", update.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
