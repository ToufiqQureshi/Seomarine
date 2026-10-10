package activation

import (
	"context"
	"fmt"
	"time"
)

type Store interface {
	GetDashboardActivation(context.Context, string, string, string) (DashboardActivation, error)
	MarkClicked(context.Context, string, string, string) error
	DismissGA4(context.Context, string, string) error
	SetDismissed(context.Context, string, string, string, bool) error
}

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
type DashboardGA4 struct {
	Connected           bool    `json:"connected"`
	PropertyDisplayName *string `json:"propertyDisplayName"`
	CardDismissedAt     *string `json:"cardDismissedAt"`
}
type DashboardGSC struct {
	Connected bool    `json:"connected"`
	SiteURL   *string `json:"siteUrl"`
}
type DashboardMCP struct {
	AuthorizedAt    *string `json:"authorizedAt"`
	FirstToolCallAt *string `json:"firstToolCallAt"`
	CardDismissedAt *string `json:"cardDismissedAt"`
}
type Service struct {
	Store Store
	Now   func() time.Time
}

func (s *Service) GetDashboardActivation(ctx context.Context, userID, projectID, organizationID string) (DashboardActivation, error) {
	if s == nil || s.Store == nil {
		return DashboardActivation{}, fmt.Errorf("dashboard activation store unavailable")
	}
	return s.Store.GetDashboardActivation(ctx, userID, projectID, organizationID)
}

func (s *Service) MarkClicked(ctx context.Context, project, step string) error {
	if step != "competitor" && step != "keywords" {
		return fmt.Errorf("invalid dashboard click step")
	}
	return s.Store.MarkClicked(ctx, project, step, s.stamp())
}
func (s *Service) DismissGA4(ctx context.Context, project string) error {
	return s.Store.DismissGA4(ctx, project, s.stamp())
}
func (s *Service) SetDismissed(ctx context.Context, user, project, step string, dismissed bool) error {
	switch step {
	case "competitor", "keywords", "audit", "mcp", "team", "project":
	default:
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
