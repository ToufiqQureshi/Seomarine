package sam

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/toufiqqureshi/seomarine/backend/internal/projectcontext"
)

// SessionLoader resolves a chat session to the project, user and organization
// a turn acts for. It returns ErrSessionNotFound for archived sessions, other
// users' sessions and sessions of another project.
type SessionLoader interface {
	Load(ctx context.Context, projectID, userID, sessionID string) (SessionContext, error)
}

// SetTitle names a session, once, from its first message.
func (r Repository) SetTitle(ctx context.Context, sessionID, title string) error {
	if _, err := r.DB.Exec(ctx, `UPDATE sam_sessions SET title=$2,
		updated_at=to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		WHERE id=$1`, sessionID, title); err != nil {
		return fmt.Errorf("set SAM session title: %w", err)
	}
	return nil
}

// Touch moves a session to the top of the list.
func (r Repository) Touch(ctx context.Context, sessionID string) error {
	if _, err := r.DB.Exec(ctx, `UPDATE sam_sessions SET
		updated_at=to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		WHERE id=$1`, sessionID); err != nil {
		return fmt.Errorf("touch SAM session: %w", err)
	}
	return nil
}

// ContextLoader is the Postgres SessionLoader. A user with no membership row
// in the project's organization gets an empty role, which the agent refuses:
// a removed member must lose access on their next message.
type ContextLoader struct {
	DB querier
}

// Load reads the session, its project, the creating user's email and role.
func (l ContextLoader) Load(ctx context.Context, projectID, userID, sessionID string) (SessionContext, error) {
	var sc SessionContext
	err := l.DB.QueryRow(ctx, `SELECT s.id, s.title, s.user_id, u.email,
			p.id, p.name, COALESCE(p.domain, ''), p.location_code, p.language_code, p.organization_id
		FROM sam_sessions s
		JOIN projects p ON p.id = s.project_id
		JOIN "user" u ON u.id = s.user_id
		WHERE s.id=$1 AND s.project_id=$2 AND s.user_id=$3
			AND s.archived_at IS NULL AND p.archived_at IS NULL`, sessionID, projectID, userID).
		Scan(&sc.SessionID, &sc.Title, &sc.UserID, &sc.UserEmail,
			&sc.Project.ID, &sc.Project.Name, &sc.Project.Domain, &sc.Project.LocationCode, &sc.Project.LanguageCode,
			&sc.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionContext{}, ErrSessionNotFound
	}
	if err != nil {
		return SessionContext{}, fmt.Errorf("load SAM session: %w", err)
	}
	err = l.DB.QueryRow(ctx, `SELECT role FROM member WHERE organization_id=$1 AND user_id=$2`, sc.OrganizationID, userID).Scan(&sc.Role)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return SessionContext{}, fmt.Errorf("load SAM membership: %w", err)
	}
	return sc, nil
}

// ProjectMemory renders the project's shared memory from the project-context
// service, the same records the settings UI and MCP clients read and write.
type ProjectMemory struct{ Service *projectcontext.Service }

// Render returns the memory as Markdown and whether intake mode applies.
func (m ProjectMemory) Render(ctx context.Context, projectID string) (string, bool, error) {
	pc, err := m.Service.Get(ctx, projectID)
	if err != nil {
		return "", false, fmt.Errorf("load project context: %w", err)
	}
	return projectcontext.RenderMarkdown(pc), slices.Contains(pc.MissingSections, "business_overview"), nil
}
