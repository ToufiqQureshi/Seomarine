package gsc

import (
	"context"
	"errors"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/google"
)

var (
	// ErrManageForbidden means the caller cannot manage project integrations.
	ErrManageForbidden = errors.New("organization integration manage permission required")
	// ErrGrantNotFound means the current user does not own the requested Google grant.
	ErrGrantNotFound = errors.New("search console Google grant not found")
	// ErrSiteNotFound means the selected grant does not expose the requested property.
	ErrSiteNotFound = errors.New("search console property not found on selected grant")
	// ErrSiteUnverified means Google has not verified the property for this grant.
	ErrSiteUnverified = errors.New("search console property access is unverified")
	// ErrGoogleUnavailable means Google OAuth credentials are not configured.
	ErrGoogleUnavailable = errors.New("search console is not configured")
)

// SearchConsoleClient contains the provider operations used by GSC routes.
type SearchConsoleClient interface {
	QuerySearchAnalytics(context.Context, string, SearchRequest) ([]SearchRow, error)
	ListSites(context.Context) ([]Site, error)
	UserInfoEmail(context.Context) (string, error)
}

// GrantReader reads OAuth grants owned by a user.
type GrantReader interface {
	HasGrant(context.Context, string) (bool, error)
	ListGrants(context.Context, string) ([]Grant, error)
}

// ProjectConnectionManager enforces integration-management permission and changes project mappings.
type ProjectConnectionManager interface {
	CanManage(context.Context, string, string, string) (bool, error)
	Upsert(context.Context, string, string, ProjectProperty) (ProjectProperty, error)
	Delete(context.Context, string, string) error
}

// ProjectPropertyReader reads a selected property within an organization.
type ProjectPropertyReader interface {
	GetByProjectID(context.Context, string, string) (ProjectProperty, error)
}

// ConnectionOperations serves the property-picker and connection-status behavior.
type ConnectionOperations struct {
	Connections ProjectPropertyReader
	Grants      GrantReader
	Manager     ProjectConnectionManager
	NewClient   func(userID, accountID string) SearchConsoleClient
	OAuthReady  bool
}

// ConnectionStatus matches the integration status shown to project members.
type ConnectionStatus struct {
	Connected             bool    `json:"connected"`
	CanManage             bool    `json:"canManage"`
	CurrentUserHasGrant   bool    `json:"currentUserHasGrant"`
	GoogleOAuthConfigured bool    `json:"googleOAuthConfigured"`
	SiteURL               *string `json:"siteUrl"`
	ConnectedByEmail      *string `json:"connectedByEmail"`
	ConnectedAt           *string `json:"connectedAt"`
}

// SiteOption is a property and its selection state for the project.
type SiteOption struct {
	SiteURL         string `json:"siteUrl"`
	PermissionLevel string `json:"permissionLevel"`
	Selectable      bool   `json:"selectable"`
	IsSelected      bool   `json:"isSelected"`
}

// SiteAccount contains all visible properties for one linked Google grant.
type SiteAccount struct {
	AccountID             string       `json:"accountId"`
	Email                 *string      `json:"email"`
	RequiresReconnect     bool         `json:"requiresReconnect"`
	PropertiesUnavailable bool         `json:"propertiesUnavailable"`
	Sites                 []SiteOption `json:"sites"`
}

// GetStatus returns project connection state and current-user integration permissions.
func (s *ConnectionOperations) GetStatus(ctx context.Context, userID, organizationID, projectID string) (ConnectionStatus, error) {
	if s == nil || s.Connections == nil || s.Grants == nil || s.Manager == nil {
		return ConnectionStatus{}, ErrGoogleUnavailable
	}
	connection, err := s.Connections.GetByProjectID(ctx, organizationID, projectID)
	if err != nil && !errors.Is(err, ErrProjectPropertyNotFound) {
		return ConnectionStatus{}, err
	}
	connected := err == nil
	hasGrant, err := s.Grants.HasGrant(ctx, userID)
	if err != nil {
		return ConnectionStatus{}, err
	}
	canManage, err := s.Manager.CanManage(ctx, userID, organizationID, projectID)
	if err != nil {
		return ConnectionStatus{}, err
	}
	status := ConnectionStatus{Connected: connected, CanManage: canManage, CurrentUserHasGrant: hasGrant, GoogleOAuthConfigured: s.OAuthReady}
	if connected {
		status.SiteURL = stringPointer(connection.SiteURL)
		status.ConnectedByEmail = connection.ConnectedAccountEmail
		status.ConnectedAt = stringPointer(connection.CreatedAt)
	}
	return status, nil
}

// ListSites lists each of the caller's grants and marks the selected project property.
func (s *ConnectionOperations) ListSites(ctx context.Context, userID, organizationID, projectID string) ([]SiteAccount, error) {
	if s == nil || s.Grants == nil || s.Connections == nil || s.NewClient == nil {
		return nil, ErrGoogleUnavailable
	}
	grants, err := s.Grants.ListGrants(ctx, userID)
	if err != nil {
		return nil, err
	}
	connection, err := s.Connections.GetByProjectID(ctx, organizationID, projectID)
	if err != nil && !errors.Is(err, ErrProjectPropertyNotFound) {
		return nil, err
	}
	connected := err == nil
	accounts := make([]SiteAccount, len(grants))
	for i, grant := range grants {
		account := SiteAccount{AccountID: grant.AccountID, Sites: []SiteOption{}}
		client := s.NewClient(userID, grant.AccountID)
		sites, listErr := client.ListSites(ctx)
		if listErr != nil {
			if isGrantFailure(listErr) {
				account.RequiresReconnect = true
			} else {
				account.PropertiesUnavailable = true
			}
			accounts[i] = account
			continue
		}
		if email, emailErr := client.UserInfoEmail(ctx); emailErr == nil && email != "" {
			account.Email = stringPointer(email)
		}
		for _, site := range sites {
			account.Sites = append(account.Sites, SiteOption{SiteURL: site.SiteURL, PermissionLevel: site.PermissionLevel, Selectable: site.PermissionLevel != "siteUnverifiedUser"})
		}
		accounts[i] = account
	}
	legacyCandidates := map[string]int{}
	if connected && connection.GSCAccountID == nil {
		for i, account := range accounts {
			for _, site := range account.Sites {
				if site.SiteURL == connection.SiteURL {
					legacyCandidates[account.AccountID] = i
					break
				}
			}
		}
	}
	for i := range accounts {
		for j := range accounts[i].Sites {
			site := &accounts[i].Sites[j]
			if !connected || site.SiteURL != connection.SiteURL {
				continue
			}
			if connection.GSCAccountID != nil {
				site.IsSelected = *connection.GSCAccountID == accounts[i].AccountID
			} else {
				_, legacyMatch := legacyCandidates[accounts[i].AccountID]
				site.IsSelected = legacyMatch && len(legacyCandidates) == 1
			}
		}
	}
	return accounts, nil
}

// SetSite verifies ownership and grant membership before saving a property.
func (s *ConnectionOperations) SetSite(ctx context.Context, userID, organizationID, projectID, accountID, siteURL string) (ProjectProperty, error) {
	if err := s.canManage(ctx, userID, organizationID, projectID); err != nil {
		return ProjectProperty{}, err
	}
	if s.NewClient == nil || s.Grants == nil || s.Manager == nil {
		return ProjectProperty{}, ErrGoogleUnavailable
	}
	grants, err := s.Grants.ListGrants(ctx, userID)
	if err != nil {
		return ProjectProperty{}, err
	}
	found := false
	for _, grant := range grants {
		found = found || grant.AccountID == accountID
	}
	if !found {
		return ProjectProperty{}, ErrGrantNotFound
	}
	client := s.NewClient(userID, accountID)
	sites, err := client.ListSites(ctx)
	if err != nil {
		return ProjectProperty{}, mapConnectionProviderError(err)
	}
	var matching *Site
	for i := range sites {
		if sites[i].SiteURL == siteURL {
			matching = &sites[i]
			break
		}
	}
	if matching == nil {
		return ProjectProperty{}, ErrSiteNotFound
	}
	if matching.PermissionLevel == "siteUnverifiedUser" {
		return ProjectProperty{}, ErrSiteUnverified
	}
	var email *string
	if value, emailErr := client.UserInfoEmail(ctx); emailErr == nil && value != "" {
		email = stringPointer(value)
	}
	return s.Manager.Upsert(ctx, organizationID, projectID, ProjectProperty{SiteURL: siteURL, ConnectedByUserID: userID, GSCAccountID: stringPointer(accountID), ConnectedAccountEmail: email})
}

// Disconnect removes the selected property after checking the project role.
func (s *ConnectionOperations) Disconnect(ctx context.Context, userID, organizationID, projectID string) error {
	if err := s.canManage(ctx, userID, organizationID, projectID); err != nil {
		return err
	}
	if s.Manager == nil {
		return ErrGoogleUnavailable
	}
	return s.Manager.Delete(ctx, organizationID, projectID)
}

func (s *ConnectionOperations) canManage(ctx context.Context, userID, organizationID, projectID string) error {
	if s == nil || s.Manager == nil {
		return ErrGoogleUnavailable
	}
	allowed, err := s.Manager.CanManage(ctx, userID, organizationID, projectID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrManageForbidden
	}
	return nil
}

func isGrantFailure(err error) bool {
	if errors.Is(err, google.ErrGrantUnavailable) {
		return true
	}
	var apiErr google.APIError
	return errors.As(err, &apiErr) && (apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden)
}

func mapConnectionProviderError(err error) error {
	if isGrantFailure(err) {
		return ConnectionProviderError{Status: http.StatusUnauthorized, Code: "gsc_reconnect_required", Message: "The Search Console connection has expired or was revoked."}
	}
	var apiErr google.APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusTooManyRequests {
		return ConnectionProviderError{Status: http.StatusTooManyRequests, Code: "gsc_rate_limited", Message: "Search Console rate limit reached. Retry shortly."}
	}
	return ConnectionProviderError{Status: http.StatusBadGateway, Code: "gsc_upstream_unavailable", Message: "Search Console is temporarily unavailable."}
}

// ConnectionProviderError carries a safe public response for Google failures.
type ConnectionProviderError struct {
	Status  int
	Code    string
	Message string
}

func (e ConnectionProviderError) Error() string { return e.Message }

func stringPointer(value string) *string { return &value }
