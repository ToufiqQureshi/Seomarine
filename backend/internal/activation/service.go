package activation

import (
	"context"
	"fmt"
	"time"
)

// Store supplies activation state and persistence operations.
type Store interface {
	// GetDashboardActivation returns activation state for a project dashboard.
	GetDashboardActivation(context.Context, string, string, string) (DashboardActivation, error)
	// MarkClicked records that a user clicked an activation step.
	MarkClicked(context.Context, string, string, string) error
	// DismissGA4 records that the GA4 activation card was dismissed.
	DismissGA4(context.Context, string, string) error
	// SetDismissed records whether a user dismissed an activation step.
	SetDismissed(context.Context, string, string, string, bool) error
}

// DashboardActivation is the data shown by the project activation dashboard.
type DashboardActivation struct {
	Domain              *string      `json:"domain"`
	GA4                 DashboardGA4 `json:"ga4"`
	GSC                 DashboardGSC `json:"gsc"`
	MCP                 DashboardMCP `json:"mcp"`
	CompetitorClickedAt *string      `json:"competitorClickedAt"`
	KeywordsClickedAt   *string      `json:"keywordsClickedAt"`
	HasAudit            bool         `json:"hasAudit"`
	HasMultipleProjects bool         `json:"hasMultipleProjects"`
	HasTeammate         bool         `json:"hasTeammate"`
	DismissedSteps      []string     `json:"dismissedSteps"`
}

// DashboardGA4 contains Google Analytics activation state.
type DashboardGA4 struct {
	Connected           bool    `json:"connected"`
	PropertyDisplayName *string `json:"propertyDisplayName"`
	CardDismissedAt     *string `json:"cardDismissedAt"`
}

// DashboardGSC contains Search Console activation state.
type DashboardGSC struct {
	Connected bool    `json:"connected"`
	SiteURL   *string `json:"siteUrl"`
}

// DashboardMCP contains MCP activation state.
type DashboardMCP struct {
	AuthorizedAt    *string `json:"authorizedAt"`
	FirstToolCallAt *string `json:"firstToolCallAt"`
	CardDismissedAt *string `json:"cardDismissedAt"`
}

// Service reads and updates project activation state.
type Service struct {
	Store Store
	Now   func() time.Time
}

// GetDashboardActivation returns activation state for a project dashboard.
func (s *Service) GetDashboardActivation(ctx context.Context, userID, projectID, organizationID string) (DashboardActivation, error) {
	if s == nil || s.Store == nil {
		return DashboardActivation{}, fmt.Errorf("dashboard activation store unavailable")
	}
	return s.Store.GetDashboardActivation(ctx, userID, projectID, organizationID)
}

// MarkClicked records that a user clicked an activation step.
func (s *Service) MarkClicked(ctx context.Context, project, step string) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("dashboard activation store unavailable")
	}
	if !validClickStep(step) {
		return fmt.Errorf("invalid dashboard click step")
	}
	return s.Store.MarkClicked(ctx, project, step, s.stamp())
}

// DismissGA4 records that the GA4 activation card was dismissed.
func (s *Service) DismissGA4(ctx context.Context, project string) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("dashboard activation store unavailable")
	}
	return s.Store.DismissGA4(ctx, project, s.stamp())
}

// SetDismissed records whether a user dismissed an activation step.
func (s *Service) SetDismissed(ctx context.Context, user, project, step string, dismissed bool) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("dashboard activation store unavailable")
	}
	if !validSetupStep(step) {
		return fmt.Errorf("invalid dashboard setup step")
	}
	return s.Store.SetDismissed(ctx, user, project, step, dismissed)
}
func (s *Service) stamp() string {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	return now.UTC().Format("2006-01-02T15:04:05.000Z")
}
