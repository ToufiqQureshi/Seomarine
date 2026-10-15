package gsc

import (
	"context"
	"errors"
	"fmt"
)

// GetQueryPageRows returns up to 1,000 query+page pair rows for a bounded
// date range. linksuggest uses it to attach each page's top queries to its
// suggestion reason, the way GetPageRows is used for page totals.
func (s *Service) GetQueryPageRows(ctx context.Context, organizationID, projectID, startDate, endDate string) ([]SearchRow, error) {
	connection, client, err := s.client(ctx, organizationID, projectID)
	if err != nil {
		return nil, err
	}
	if !validDate(startDate) || !validDate(endDate) || startDate > endDate {
		return nil, validationError("Dates must be valid YYYY-MM-DD values with startDate on or before endDate.")
	}
	request := SearchRequest{StartDate: startDate, EndDate: endDate, Dimensions: []string{"query", "page"}, RowLimit: 1000, Type: "web", DataState: "final"}
	rows, err := client.QuerySearchAnalytics(ctx, connection.SiteURL, request)
	if err != nil {
		return nil, fmt.Errorf("query Search Console query-page rows: %w", err)
	}
	if len(rows) > request.RowLimit {
		return nil, errors.New("search console returned too many query-page rows")
	}
	if rows == nil {
		rows = []SearchRow{}
	}
	return rows, nil
}
