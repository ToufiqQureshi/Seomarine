// Package gsc provides the Search Console API and project connection routes.
package gsc

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/google"
)

// Site is a Search Console property returned by sites.list.
type Site struct {
	SiteURL         string `json:"siteUrl"`
	PermissionLevel string `json:"permissionLevel"`
}

// SearchRow is one row of Search Console searchAnalytics.query.
type SearchRow struct {
	Keys        []string `json:"keys,omitempty"`
	Clicks      float64  `json:"clicks"`
	Impressions float64  `json:"impressions"`
	CTR         float64  `json:"ctr"`
	Position    float64  `json:"position"`
}

// DimensionFilter restricts one Search Console dimension.
type DimensionFilter struct {
	Dimension  string `json:"dimension"`
	Operator   string `json:"operator"`
	Expression string `json:"expression"`
}

// FilterGroup combines search analytics filters with AND or OR.
type FilterGroup struct {
	GroupType string            `json:"groupType"`
	Filters   []DimensionFilter `json:"filters"`
}

// SearchRequest is a validated Search Console analytics query.
type SearchRequest struct {
	StartDate             string        `json:"startDate"`
	EndDate               string        `json:"endDate"`
	Dimensions            []string      `json:"dimensions,omitempty"`
	DimensionFilterGroups []FilterGroup `json:"dimensionFilterGroups,omitempty"`
	RowLimit              int           `json:"rowLimit,omitempty"`
	StartRow              int           `json:"startRow,omitempty"`
	Type                  string        `json:"type,omitempty"`
	DataState             string        `json:"dataState,omitempty"`
	AggregationType       string        `json:"aggregationType,omitempty"`
}

// Client calls first-party Search Console REST endpoints for a user's grant.
type Client struct {
	API           *google.APIClient
	UserID        string
	AccountID     string
	BaseURL       string
	InspectionURL string
	UserInfoURL   string
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "https://www.googleapis.com/webmasters/v3"
}

func (c *Client) inspectionURL() string {
	if c.InspectionURL != "" {
		return c.InspectionURL
	}
	return "https://searchconsole.googleapis.com/v1/urlInspection/index:inspect"
}

func (c *Client) userInfoURL() string {
	if c.UserInfoURL != "" {
		return c.UserInfoURL
	}
	return "https://openidconnect.googleapis.com/v1/userinfo"
}

// ListSites returns all properties visible to the user's Google grant.
func (c *Client) ListSites(ctx context.Context) ([]Site, error) {
	var response struct {
		SiteEntry []Site `json:"siteEntry"`
	}
	if err := c.API.DoJSON(ctx, google.APIRequest{UserID: c.UserID, Provider: "gsc", AccountID: c.AccountID, Method: http.MethodGet, URL: c.baseURL() + "/sites", Response: &response, Retryable: true}); err != nil {
		return nil, err
	}
	if len(response.SiteEntry) > 10_000 {
		return nil, errors.New("too many search console sites")
	}
	if response.SiteEntry == nil {
		return []Site{}, nil
	}
	return response.SiteEntry, nil
}

func validDate(value string) bool {
	if len(value) != 10 {
		return false
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validDimension(value string) bool {
	switch value {
	case "query", "page", "country", "device", "date", "searchAppearance":
		return true
	default:
		return false
	}
}

func validFilter(filter DimensionFilter) bool {
	if !validDimension(filter.Dimension) || filter.Expression == "" || len(filter.Expression) > 1024 {
		return false
	}
	switch filter.Operator {
	case "equals", "notEquals", "contains", "notContains":
		return true
	default:
		return false
	}
}

// ValidateSearchRequest rejects provider-invalid dates, dimensions and paging.
func ValidateSearchRequest(request SearchRequest) error {
	if !validDate(request.StartDate) || !validDate(request.EndDate) || request.StartDate > request.EndDate ||
		request.RowLimit < 1 || request.RowLimit > 1000 || request.StartRow < 0 || request.StartRow > 1_000_000 ||
		len(request.Dimensions) == 0 || len(request.Dimensions) > 6 || len(request.DimensionFilterGroups) > 1 {
		return errors.New("invalid search analytics request")
	}
	for _, dimension := range request.Dimensions {
		if !validDimension(dimension) {
			return errors.New("invalid search dimension")
		}
	}
	for _, group := range request.DimensionFilterGroups {
		if group.GroupType != "and" && group.GroupType != "or" || len(group.Filters) == 0 || len(group.Filters) > 10 {
			return errors.New("invalid search filter group")
		}
		for _, filter := range group.Filters {
			if !validFilter(filter) {
				return errors.New("invalid search filter")
			}
		}
	}
	switch request.Type {
	case "web", "image", "video", "news", "googleNews", "discover":
	default:
		return errors.New("invalid search type")
	}
	if request.DataState != "all" && request.DataState != "final" {
		return errors.New("invalid data state")
	}
	if request.AggregationType != "" && request.AggregationType != "auto" && request.AggregationType != "byPage" && request.AggregationType != "byProperty" {
		return errors.New("invalid aggregation type")
	}
	return nil
}

// QuerySearchAnalytics fetches a capped, validated result for an exact site URL.
func (c *Client) QuerySearchAnalytics(ctx context.Context, siteURL string, request SearchRequest) ([]SearchRow, error) {
	if siteURL == "" || len(siteURL) > 2048 {
		return nil, errors.New("invalid search console site")
	}
	if err := ValidateSearchRequest(request); err != nil {
		return nil, err
	}
	var response struct {
		Rows []SearchRow `json:"rows"`
	}
	endpoint := c.baseURL() + "/sites/" + url.PathEscape(siteURL) + "/searchAnalytics/query"
	if err := c.API.DoJSON(ctx, google.APIRequest{UserID: c.UserID, Provider: "gsc", AccountID: c.AccountID, Method: http.MethodPost, URL: endpoint, Body: request, Response: &response, Retryable: true, MaxResponseBytes: 4 << 20}); err != nil {
		return nil, err
	}
	if len(response.Rows) > request.RowLimit {
		return nil, errors.New("search console returned too many rows")
	}
	if response.Rows == nil {
		return []SearchRow{}, nil
	}
	return response.Rows, nil
}

// UserInfoEmail returns the linked Google account email when available.
func (c *Client) UserInfoEmail(ctx context.Context) (string, error) {
	var response struct {
		Email string `json:"email"`
	}
	if err := c.API.DoJSON(ctx, google.APIRequest{UserID: c.UserID, Provider: "gsc", AccountID: c.AccountID, Method: http.MethodGet, URL: c.userInfoURL(), Response: &response, Retryable: true}); err != nil {
		return "", err
	}
	return response.Email, nil
}

// InspectURL returns Google's inspection result for a URL in a verified site.
func (c *Client) InspectURL(ctx context.Context, siteURL, inspectionURL, languageCode string) (map[string]any, error) {
	parsed, err := url.Parse(inspectionURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || len(inspectionURL) > 2048 || siteURL == "" {
		return nil, errors.New("invalid inspection url")
	}
	body := map[string]string{"siteUrl": siteURL, "inspectionUrl": inspectionURL}
	if languageCode != "" {
		if len(languageCode) > 35 {
			return nil, errors.New("invalid inspection language")
		}
		body["languageCode"] = languageCode
	}
	var response struct {
		InspectionResult map[string]any `json:"inspectionResult"`
	}
	if err := c.API.DoJSON(ctx, google.APIRequest{UserID: c.UserID, Provider: "gsc", AccountID: c.AccountID, Method: http.MethodPost, URL: c.inspectionURL(), Body: body, Response: &response, Retryable: true}); err != nil {
		return nil, err
	}
	return response.InspectionResult, nil
}
