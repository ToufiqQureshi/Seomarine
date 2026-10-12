package keywords

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/dataforseo"
)

const businessListingsSearchPath = "/v3/business_data/business_listings/search/live"

// BusinessListingsSearchInput contains a business listings search request.
type BusinessListingsSearchInput struct {
	Categories []string
	Title      string
	Coordinate string
	IsClaimed  *bool
	Filters    [][]any
	OrderBy    []string
	Limit      int
	Offset     int
}

// BusinessListingsSearchProvider searches for local business listings.
type BusinessListingsSearchProvider interface {
	BusinessListingsSearch(context.Context, string, BusinessListingsSearchInput) ([]map[string]any, error)
}

// BusinessListingsSearch searches DataForSEO local business listings.
func (p DataForSEOProvider) BusinessListingsSearch(ctx context.Context, org string, in BusinessListingsSearchInput) ([]map[string]any, error) {
	body := map[string]any{"location_coordinate": in.Coordinate, "limit": in.Limit}
	if len(in.Categories) > 0 {
		body["categories"] = in.Categories
	}
	if in.Title != "" {
		body["title"] = in.Title
	}
	if in.IsClaimed != nil {
		body["is_claimed"] = *in.IsClaimed
	}
	if len(in.Filters) > 0 {
		var filters []any
		for i, f := range in.Filters {
			if i > 0 {
				filters = append(filters, "and")
			}
			filters = append(filters, f)
		}
		body["filters"] = filters
	}
	if len(in.OrderBy) > 0 {
		body["order_by"] = in.OrderBy
	}
	if in.Offset > 0 {
		body["offset"] = in.Offset
	}
	results, err := p.post(ctx, org, businessListingsSearchPath, body)
	if taskErr, ok := errors.AsType[*dataforseo.TaskError](err); ok && taskErr.StatusCode == 40501 {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return []map[string]any{}, nil
	}
	var response struct {
		Items []map[string]any `json:"items"`
	}
	if err = json.Unmarshal(results[0], &response); err != nil {
		return nil, fmt.Errorf("decode business listings: %w", err)
	}
	if response.Items == nil {
		return []map[string]any{}, nil
	}
	return response.Items, nil
}

// FormatBusinessListingsCoordinate formats a geographic point and radius for DataForSEO.
func FormatBusinessListingsCoordinate(lat, lon, radiusKm float64) string {
	f := func(v float64) string { return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.7f", v), "0"), ".") }
	radius := max(1, int(radiusKm+0.5))
	return fmt.Sprintf("%s,%s,%d", f(lat), f(lon), radius)
}
