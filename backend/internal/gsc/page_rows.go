package gsc

import (
	"context"
	"errors"
	"fmt"
)

// GetPageRows returns up to 1,000 final web-search page rows for a bounded
// date range. It is intended for server-side joins with other project reports.
func (s *Service) GetPageRows(ctx context.Context, organizationID, projectID, startDate, endDate string) (Connection, []SearchRow, error) {
	connection, client, err := s.client(ctx, organizationID, projectID)
	if err != nil {
		return Connection{}, nil, err
	}
	if !validDate(startDate) || !validDate(endDate) || startDate > endDate {
		return Connection{}, nil, validationError("Dates must be valid YYYY-MM-DD values with startDate on or before endDate.")
	}
	request := SearchRequest{StartDate: startDate, EndDate: endDate, Dimensions: []string{"page"}, RowLimit: 1000, Type: "web", DataState: "final", AggregationType: "auto"}
	rows, err := client.QuerySearchAnalytics(ctx, connection.SiteURL, request)
	if err != nil {
		return Connection{}, nil, fmt.Errorf("query Search Console page rows: %w", err)
	}
	if len(rows) > request.RowLimit {
		return Connection{}, nil, errors.New("search console returned too many page rows")
	}
	if rows == nil {
		rows = []SearchRow{}
	}
	return connection, rows, nil
}
