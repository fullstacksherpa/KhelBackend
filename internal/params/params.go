package params

import (
	"math"
	"net/url"
	"strconv"
	"strings"
)

// URL: /products?page=2&limit=30
// → ParsePagination() → Pagination{Limit:30, Page:2, Offset:30}
// → SQL: SELECT ... LIMIT 30 OFFSET 30
// → DB returns data + total count
// → ComputeMeta(total) → fills TotalPages, HasNext, etc.
// → JSON response with products + pagination metadata
// Pagination holds pagination info and computed metadata.
type Pagination struct {
	Limit      int  `json:"limit"`       // items per page
	Offset     int  `json:"offset"`      // SQL OFFSET value
	Page       int  `json:"page"`        // Current Page number
	Total      int  `json:"total"`       //Total item in database
	TotalPages int  `json:"total_pages"` //Total pages available
	HasNext    bool `json:"has_next"`
	HasPrev    bool `json:"has_prev"`
}

// ParsePagination parses ?limit=...&page=... safely.  Careful key are case sensitive
func ParsePagination(q url.Values) Pagination {
	p := Pagination{
		Limit: 15, // default
		Page:  1,
	}

	// --- Parse limit ---
	if limitStr := strings.TrimSpace(q.Get("limit")); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			switch {
			case limit <= 0:
				p.Limit = 15
			case limit > 30:
				p.Limit = 30
			default:
				p.Limit = limit
			}
		}
	}

	// --- Parse page ---
	if pageStr := strings.TrimSpace(q.Get("page")); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			p.Page = page
		}
	}

	// --- Calculate offset ---
	p.Offset = (p.Page - 1) * p.Limit
	return p
}

// ComputeMeta updates pagination after fetching total count.
func (p *Pagination) ComputeMeta(total int) {
	p.Total = total
	if p.Limit > 0 {
		p.TotalPages = int(math.Ceil(float64(total) / float64(p.Limit)))
	}
	p.HasPrev = p.Page > 1
	p.HasNext = (p.Page * p.Limit) < total
}
