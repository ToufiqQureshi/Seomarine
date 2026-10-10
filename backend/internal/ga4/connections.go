package ga4

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/google"
)

var (
	// ErrManageForbidden means the caller cannot manage project integrations.
	ErrManageForbidden = errors.New("organization integration manage permission required")
	// ErrGrantNotFound means the signed-in user does not own the requested OAuth grant.
	ErrGrantNotFound = errors.New("google Analytics grant not found")
	// ErrPropertyNotFound means Google did not list the property for the selected grant.
	ErrPropertyNotFound = errors.New("google Analytics property not found on selected grant")
	// ErrConnectionsUnavailable means the GA4 setup dependencies are not configured.
	ErrConnectionsUnavailable = errors.New("google Analytics connections are not configured")
)

// PropertyGrant identifies a Google Analytics OAuth grant owned by a user.
type PropertyGrant struct{ AccountID string }

// PropertySummary is a property surfaced in the GA4 setup picker.
type PropertySummary struct {
	PropertyID         string `json:"propertyId"`
	DisplayName        string `json:"displayName"`
	AccountDisplayName string `json:"accountDisplayName"`
	IsSelected         bool   `json:"isSelected"`
}

// PropertyAccount groups discoverable properties and grant health for one OAuth grant.
type PropertyAccount struct {
	AccountID             string            `json:"accountId"`
	Email                 *string           `json:"email"`
	RequiresReconnect     bool              `json:"requiresReconnect"`
	PropertiesUnavailable bool              `json:"propertiesUnavailable"`
	Properties            []PropertySummary `json:"properties"`
}

// PropertyDetails contains canonical metadata read from Google's Admin API.
type PropertyDetails struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	TimeZone     string `json:"timeZone"`
	CurrencyCode string `json:"currencyCode"`
}

// PropertyAdmin contains read-only Google Analytics Admin operations used by setup.
type PropertyAdmin interface {
	ListProperties(context.Context) ([]PropertySummary, error)
	GetProperty(context.Context, string) (PropertyDetails, error)
	UserInfoEmail(context.Context) (string, error)
}

// PropertyAdminFactory creates an Admin API client scoped to one user's grant.
type PropertyAdminFactory func(userID, accountID string) PropertyAdmin

// PropertyConnectionStore reads and changes project property mappings and grants.
type PropertyConnectionStore interface {
	GetByProjectID(context.Context, string, string) (ProjectConnection, error)
	HasGrant(context.Context, string) (bool, error)
	ListGrants(context.Context, string) ([]PropertyGrant, error)
	CanManage(context.Context, string, string, string) (bool, error)
	Upsert(context.Context, string, string, ProjectConnection) (ProjectConnection, error)
	Delete(context.Context, string, string) error
}

// ProjectConnection is the selected GA4 property plus its legacy connection metadata.
type ProjectConnection struct {
	Connection
	ConnectedAccountEmail *string `json:"connectedAccountEmail"`
	CreatedAt             string  `json:"createdAt"`
}

// ConnectionOperations serves the GA4 property picker and project connection lifecycle.
type ConnectionOperations struct {
	Store      PropertyConnectionStore
	NewAdmin   PropertyAdminFactory
	OAuthReady bool
}

// Status returns the selected property and the user's grant and permission state.
func (s *ConnectionOperations) Status(ctx context.Context, userID, organizationID, projectID string) (map[string]any, error) {
	if s == nil || s.Store == nil {
		return nil, ErrConnectionsUnavailable
	}
	connection, err := s.Store.GetByProjectID(ctx, organizationID, projectID)
	if err != nil && !errors.Is(err, ErrConnectionNotFound) {
		return nil, err
	}
	connected := err == nil
	hasGrant, err := s.Store.HasGrant(ctx, userID)
	if err != nil {
		return nil, err
	}
	canManage, err := s.Store.CanManage(ctx, userID, organizationID, projectID)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"connected": connected, "canManage": canManage, "currentUserHasGrant": hasGrant, "googleOAuthConfigured": s.OAuthReady,
		"propertyId": nil, "propertyDisplayName": nil, "propertyTimeZone": nil, "propertyCurrencyCode": nil, "connectedByEmail": nil, "connectedAt": nil}
	if connected {
		result["propertyId"], result["propertyDisplayName"] = connection.PropertyID, connection.PropertyDisplayName
		result["propertyTimeZone"], result["propertyCurrencyCode"] = connection.PropertyTimeZone, connection.PropertyCurrencyCode
		result["connectedByEmail"], result["connectedAt"] = connection.ConnectedAccountEmail, connection.CreatedAt
	}
	return result, nil
}

// ListProperties discovers each of the user's properties and marks the project selection.
func (s *ConnectionOperations) ListProperties(ctx context.Context, userID, organizationID, projectID string) ([]PropertyAccount, error) {
	if s == nil || s.Store == nil {
		return nil, ErrConnectionsUnavailable
	}
	grants, err := s.Store.ListGrants(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(grants) == 0 {
		return []PropertyAccount{}, nil
	}
	if s.NewAdmin == nil {
		return nil, ErrConnectionsUnavailable
	}
	connection, err := s.Store.GetByProjectID(ctx, organizationID, projectID)
	if err != nil && !errors.Is(err, ErrConnectionNotFound) {
		return nil, err
	}
	accounts := make([]PropertyAccount, len(grants))
	for i, grant := range grants {
		account := PropertyAccount{AccountID: grant.AccountID, Properties: []PropertySummary{}}
		admin := s.NewAdmin(userID, grant.AccountID)
		properties, listErr := admin.ListProperties(ctx)
		if listErr != nil {
			account.RequiresReconnect = isGrantFailure(listErr)
			account.PropertiesUnavailable = !account.RequiresReconnect
			accounts[i] = account
			continue
		}
		if email, emailErr := admin.UserInfoEmail(ctx); emailErr == nil && email != "" {
			account.Email = &email
		}
		for _, property := range properties {
			property.IsSelected = err == nil && connection.GA4AccountID == grant.AccountID && connection.PropertyID == property.PropertyID
			account.Properties = append(account.Properties, property)
		}
		accounts[i] = account
	}
	return accounts, nil
}

// SetProperty verifies the grant and property before saving canonical metadata.
func (s *ConnectionOperations) SetProperty(ctx context.Context, userID, organizationID, projectID, accountID, propertyID string) (ProjectConnection, error) {
	if s == nil || s.Store == nil {
		return ProjectConnection{}, ErrConnectionsUnavailable
	}
	if !validPropertyID(propertyID) {
		return ProjectConnection{}, invalid("propertyId must be a canonical GA4 property.")
	}
	allowed, err := s.Store.CanManage(ctx, userID, organizationID, projectID)
	if err != nil {
		return ProjectConnection{}, err
	}
	if !allowed {
		return ProjectConnection{}, ErrManageForbidden
	}
	if s.NewAdmin == nil {
		return ProjectConnection{}, ErrConnectionsUnavailable
	}
	grants, err := s.Store.ListGrants(ctx, userID)
	if err != nil {
		return ProjectConnection{}, err
	}
	owned := false
	for _, grant := range grants {
		owned = owned || grant.AccountID == accountID
	}
	if !owned {
		return ProjectConnection{}, ErrGrantNotFound
	}
	admin := s.NewAdmin(userID, accountID)
	properties, err := admin.ListProperties(ctx)
	if err != nil {
		return ProjectConnection{}, mapConnectionError(err)
	}
	var selected *PropertySummary
	for i := range properties {
		if properties[i].PropertyID == propertyID {
			selected = &properties[i]
			break
		}
	}
	if selected == nil {
		return ProjectConnection{}, ErrPropertyNotFound
	}
	details, err := admin.GetProperty(ctx, propertyID)
	if err != nil {
		return ProjectConnection{}, mapConnectionError(err)
	}
	if details.Name != propertyID || details.DisplayName == "" || details.TimeZone == "" || details.CurrencyCode == "" {
		return ProjectConnection{}, google.ErrInvalidAPIResponse
	}
	var email *string
	if value, emailErr := admin.UserInfoEmail(ctx); emailErr == nil && value != "" {
		email = &value
	}
	return s.Store.Upsert(ctx, organizationID, projectID, ProjectConnection{Connection: Connection{
		PropertyID: details.Name, PropertyDisplayName: details.DisplayName, PropertyTimeZone: details.TimeZone,
		PropertyCurrencyCode: details.CurrencyCode, ConnectedByUserID: userID, GA4AccountID: accountID,
	}, ConnectedAccountEmail: email})
}

// Disconnect removes the selected property after checking the caller's project role.
func (s *ConnectionOperations) Disconnect(ctx context.Context, userID, organizationID, projectID string) error {
	if s == nil || s.Store == nil {
		return ErrConnectionsUnavailable
	}
	allowed, err := s.Store.CanManage(ctx, userID, organizationID, projectID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrManageForbidden
	}
	return s.Store.Delete(ctx, organizationID, projectID)
}

// GooglePropertyAdmin calls the read-only Analytics Admin API with bounded,
// token-aware requests. Account summaries are capped to avoid unbounded paging.
type GooglePropertyAdmin struct {
	API               *google.APIClient
	UserID, AccountID string
}

func (c GooglePropertyAdmin) do(ctx context.Context, path string, dst any) error {
	return c.API.DoJSON(ctx, google.APIRequest{UserID: c.UserID, Provider: "google-analytics", AccountID: c.AccountID,
		Method: http.MethodGet, URL: "https://analyticsadmin.googleapis.com/v1beta/" + path, Response: dst, Retryable: true, MaxResponseBytes: 2 << 20})
}

// ListProperties lists property summaries visible to the scoped Google grant.
func (c GooglePropertyAdmin) ListProperties(ctx context.Context) ([]PropertySummary, error) {
	var all []PropertySummary
	token := ""
	for page := 0; page < 100; page++ {
		query := url.Values{"pageSize": {"200"}}
		if token != "" {
			query.Set("pageToken", token)
		}
		var response struct {
			AccountSummaries []struct {
				DisplayName string `json:"displayName"`
				Properties  []struct {
					Property    string `json:"property"`
					DisplayName string `json:"displayName"`
				} `json:"propertySummaries"`
			} `json:"accountSummaries"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.do(ctx, "accountSummaries?"+query.Encode(), &response); err != nil {
			return nil, err
		}
		for _, account := range response.AccountSummaries {
			for _, property := range account.Properties {
				if !validPropertyID(property.Property) || property.DisplayName == "" || account.DisplayName == "" {
					return nil, google.ErrInvalidAPIResponse
				}
				all = append(all, PropertySummary{PropertyID: property.Property, DisplayName: property.DisplayName, AccountDisplayName: account.DisplayName})
			}
		}
		token = response.NextPageToken
		if token == "" {
			return all, nil
		}
	}
	return nil, errors.New("google Analytics property discovery exceeded 100 pages")
}

// GetProperty returns canonical metadata for a validated GA4 property ID.
func (c GooglePropertyAdmin) GetProperty(ctx context.Context, propertyID string) (PropertyDetails, error) {
	if !validPropertyID(propertyID) {
		return PropertyDetails{}, errors.New("invalid GA4 property ID")
	}
	var result PropertyDetails
	if err := c.do(ctx, propertyID, &result); err != nil {
		return PropertyDetails{}, err
	}
	return result, nil
}

// UserInfoEmail returns the email for the scoped OAuth grant when Google provides it.
func (c GooglePropertyAdmin) UserInfoEmail(ctx context.Context) (string, error) {
	var result struct {
		Email string `json:"email"`
	}
	if err := c.API.DoJSON(ctx, google.APIRequest{UserID: c.UserID, Provider: "google-analytics", AccountID: c.AccountID, Method: http.MethodGet,
		URL: "https://openidconnect.googleapis.com/v1/userinfo", Response: &result, Retryable: true, MaxResponseBytes: 16 << 10}); err != nil {
		return "", err
	}
	if result.Email != "" && !strings.Contains(result.Email, "@") {
		return "", google.ErrInvalidAPIResponse
	}
	return result.Email, nil
}

func isGrantFailure(err error) bool {
	if errors.Is(err, google.ErrGrantUnavailable) {
		return true
	}
	var apiErr google.APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized
}

type connectionProviderError struct {
	Status        int
	Code, Message string
}

func (e connectionProviderError) Error() string { return e.Message }

func mapConnectionError(err error) error {
	if isGrantFailure(err) {
		return connectionProviderError{http.StatusUnauthorized, "ga4_reconnect_required", "The Google Analytics connection has expired or was revoked."}
	}
	var apiErr google.APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests {
		return connectionProviderError{http.StatusTooManyRequests, "ga4_rate_limited", "Google Analytics rate limit reached. Retry shortly."}
	}
	return connectionProviderError{http.StatusBadGateway, "ga4_upstream_unavailable", "Google Analytics is temporarily unavailable."}
}
