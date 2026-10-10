// Package workspace contains migration operations for legacy organizations.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const sharedOrganizationID = "shared-workspace"

var ErrModeDisabled = errors.New("workspace merge is only enabled in cloudflare_access mode")

// Service merges the old per-user Cloudflare Access workspaces into the
// shared workspace created by the legacy auth bootstrap.
type Service struct {
	DB       *pgxpool.Pool
	AuthMode string
}

// Status reports how many old workspaces remain. The migration is intentionally
// hidden outside Cloudflare Access mode.
func (s *Service) Status(ctx context.Context) (int, error) {
	if s.AuthMode != "cloudflare_access" {
		return 0, nil
	}
	var count int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM organization WHERE id LIKE 'delegated-%'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count legacy workspaces: %w", err)
	}
	return count, nil
}

// Merge atomically repoints workspace-owned records, keeps every project,
// merges activation milestones, then removes the emptied legacy organizations.
func (s *Service) Merge(ctx context.Context) (int, error) {
	if s.AuthMode != "cloudflare_access" {
		return 0, ErrModeDisabled
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin workspace merge: %w", err)
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id, name FROM organization WHERE id LIKE 'delegated-%' ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("list legacy workspaces: %w", err)
	}
	type legacy struct{ id, name string }
	var orgs []legacy
	for rows.Next() {
		var item legacy
		if err := rows.Scan(&item.id, &item.name); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan legacy workspace: %w", err)
		}
		orgs = append(orgs, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate legacy workspaces: %w", err)
	}
	rows.Close()
	if len(orgs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		return 0, nil
	}
	ids := make([]string, 0, len(orgs))
	names := make(map[string]string, len(orgs))
	for _, org := range orgs {
		ids = append(ids, org.id)
		names[org.id] = strings.TrimSuffix(org.name, " workspace")
	}
	defaults, err := tx.Query(ctx, `SELECT id, organization_id FROM projects WHERE organization_id = ANY($1) AND name = 'Default' AND domain IS NULL AND archived_at IS NULL`, ids)
	if err != nil {
		return 0, fmt.Errorf("find duplicate default projects: %w", err)
	}
	for defaults.Next() {
		var projectID, orgID string
		if err := defaults.Scan(&projectID, &orgID); err != nil {
			defaults.Close()
			return 0, fmt.Errorf("scan default project: %w", err)
		}
		label := strings.TrimSpace(names[orgID])
		if label == "" {
			label = "imported"
		}
		if _, err := tx.Exec(ctx, `UPDATE projects SET name=$1 WHERE id=$2`, "Default ("+label+")", projectID); err != nil {
			defaults.Close()
			return 0, fmt.Errorf("rename imported default project: %w", err)
		}
	}
	if err := defaults.Err(); err != nil {
		defaults.Close()
		return 0, fmt.Errorf("iterate default projects: %w", err)
	}
	defaults.Close()
	for _, table := range []string{"projects", "user_onboarding_answers", "gsc_connections", "ga4_connections"} {
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET organization_id=$1 WHERE organization_id = ANY($2)`, sharedOrganizationID, ids); err != nil {
			return 0, fmt.Errorf("repoint %s to shared workspace: %w", table, err)
		}
	}
	var firstAuthorized, firstToolCall *string
	activationRows, err := tx.Query(ctx, `SELECT first_mcp_authorized_at, first_mcp_tool_call_at FROM organization_activation_state WHERE organization_id = ANY($1)`, append(ids, sharedOrganizationID))
	if err != nil {
		return 0, fmt.Errorf("read workspace activation milestones: %w", err)
	}
	for activationRows.Next() {
		var authorized, toolCall *string
		if err := activationRows.Scan(&authorized, &toolCall); err != nil {
			activationRows.Close()
			return 0, fmt.Errorf("scan activation milestones: %w", err)
		}
		firstAuthorized = earlier(firstAuthorized, authorized)
		firstToolCall = earlier(firstToolCall, toolCall)
	}
	if err := activationRows.Err(); err != nil {
		activationRows.Close()
		return 0, fmt.Errorf("iterate activation milestones: %w", err)
	}
	activationRows.Close()
	if firstAuthorized != nil || firstToolCall != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO organization_activation_state (organization_id,first_mcp_authorized_at,first_mcp_tool_call_at) VALUES ($1,$2,$3) ON CONFLICT (organization_id) DO UPDATE SET first_mcp_authorized_at=EXCLUDED.first_mcp_authorized_at,first_mcp_tool_call_at=EXCLUDED.first_mcp_tool_call_at`, sharedOrganizationID, firstAuthorized, firstToolCall); err != nil {
			return 0, fmt.Errorf("save merged activation milestones: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM organization WHERE id = ANY($1)`, ids); err != nil {
		return 0, fmt.Errorf("delete migrated workspaces: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit workspace merge: %w", err)
	}
	return len(orgs), nil
}

func earlier(current, candidate *string) *string {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate < *current {
		return candidate
	}
	return current
}
