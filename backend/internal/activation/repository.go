package activation

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ DB *pgxpool.Pool }

func (r Repository) GetDashboardActivation(ctx context.Context, userID, projectID, organizationID string) (DashboardActivation, error) {
	var result DashboardActivation
	var projectCount int
	var memberCount int
	var pendingInvite bool
	err := r.DB.QueryRow(ctx, `SELECT
		p.domain,
		(SELECT property_display_name FROM ga4_connections WHERE project_id=p.id),
		(SELECT site_url FROM gsc_connections WHERE project_id=p.id),
		(SELECT first_mcp_authorized_at FROM organization_activation_state WHERE organization_id=p.organization_id),
		(SELECT first_mcp_tool_call_at FROM organization_activation_state WHERE organization_id=p.organization_id),
		(SELECT ga4_card_dismissed_at FROM project_activation_state WHERE project_id=p.id),
		(SELECT mcp_card_dismissed_at FROM project_activation_state WHERE project_id=p.id),
		(SELECT competitor_step_clicked_at FROM project_activation_state WHERE project_id=p.id),
		(SELECT keyword_step_clicked_at FROM project_activation_state WHERE project_id=p.id),
		(SELECT count(*) FROM projects WHERE organization_id=p.organization_id AND archived_at IS NULL),
		(SELECT count(*) FROM member WHERE organization_id=p.organization_id),
		EXISTS(SELECT 1 FROM invitation WHERE organization_id=p.organization_id AND status='pending' AND expires_at>now()),
		EXISTS(SELECT 1 FROM audits WHERE project_id=p.id),
		ARRAY(SELECT step FROM dashboard_step_dismissals WHERE user_id=$2 AND project_id=p.id ORDER BY step)
		FROM projects p WHERE p.id=$1 AND p.organization_id=$3 AND p.archived_at IS NULL`, projectID, userID, organizationID).
		Scan(&result.Domain, &result.GA4.PropertyDisplayName, &result.GSC.SiteURL, &result.MCP.AuthorizedAt, &result.MCP.FirstToolCallAt, &result.GA4.CardDismissedAt, &result.MCP.CardDismissedAt, &result.CompetitorClickedAt, &result.KeywordsClickedAt, &projectCount, &memberCount, &pendingInvite, &result.HasAudit, &result.DismissedSteps)
	if err != nil {
		return DashboardActivation{}, fmt.Errorf("read dashboard activation: %w", err)
	}
	result.HasMultipleProjects = projectCount > 1
	result.GA4.Connected = result.GA4.PropertyDisplayName != nil
	result.GSC.Connected = result.GSC.SiteURL != nil
	result.HasTeammate = memberCount > 1 || pendingInvite
	if result.DismissedSteps == nil {
		result.DismissedSteps = []string{}
	}
	return result, nil
}

func (r Repository) MarkClicked(ctx context.Context, project, step, stamp string) error {
	column := "competitor_step_clicked_at"
	if step == "keywords" {
		column = "keyword_step_clicked_at"
	}
	_, err := r.DB.Exec(ctx, `INSERT INTO project_activation_state(project_id,`+column+`,updated_at) VALUES($1,$2,$2) ON CONFLICT(project_id) DO UPDATE SET `+column+`=COALESCE(project_activation_state.`+column+`,$2),updated_at=$2`, project, stamp)
	if err != nil {
		return fmt.Errorf("record activation click: %w", err)
	}
	return nil
}
func (r Repository) DismissGA4(ctx context.Context, project, stamp string) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO project_activation_state(project_id,ga4_card_dismissed_at,updated_at) VALUES($1,$2,$2) ON CONFLICT(project_id) DO UPDATE SET ga4_card_dismissed_at=COALESCE(project_activation_state.ga4_card_dismissed_at,$2),updated_at=$2`, project, stamp)
	if err != nil {
		return fmt.Errorf("dismiss GA4 activation card: %w", err)
	}
	return nil
}
func (r Repository) SetDismissed(ctx context.Context, user, project, step string, dismissed bool) error {
	if dismissed {
		_, err := r.DB.Exec(ctx, `INSERT INTO dashboard_step_dismissals(user_id,project_id,step) VALUES($1,$2,$3) ON CONFLICT(user_id,project_id,step) DO NOTHING`, user, project, step)
		if err != nil {
			return fmt.Errorf("dismiss activation step: %w", err)
		}
		return nil
	}
	if step == "mcp" {
		if _, err := r.DB.Exec(ctx, `UPDATE project_activation_state SET mcp_card_dismissed_at=NULL WHERE project_id=$1`, project); err != nil {
			return fmt.Errorf("clear MCP card dismissal: %w", err)
		}
	}
	if _, err := r.DB.Exec(ctx, `DELETE FROM dashboard_step_dismissals WHERE user_id=$1 AND project_id=$2 AND step=$3`, user, project, step); err != nil {
		return fmt.Errorf("restore activation step: %w", err)
	}
	return nil
}
