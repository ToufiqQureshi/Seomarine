package sam

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
)

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Session is one active SAM conversation in the project session list.
type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Repository reads and writes the existing SAM session registry.
type Repository struct{ DB querier }

// ListForProject returns the caller's active sessions, newest first.
func (r Repository) ListForProject(ctx context.Context, projectID, userID string) ([]Session, error) {
	rows, err := r.DB.Query(ctx, `SELECT id, title, created_at, updated_at
		FROM sam_sessions
		WHERE project_id=$1 AND user_id=$2 AND archived_at IS NULL
		ORDER BY updated_at DESC, id DESC`, projectID, userID)
	if err != nil {
		return nil, fmt.Errorf("query SAM sessions: %w", err)
	}
	sessions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Session, error) {
		var session Session
		err := row.Scan(&session.ID, &session.Title, &session.CreatedAt, &session.UpdatedAt)
		return session, err
	})
	if err != nil {
		return nil, fmt.Errorf("read SAM sessions: %w", err)
	}
	if sessions == nil {
		sessions = []Session{}
	}
	return sessions, nil
}

// Create inserts a session into the existing registry table.
func (r Repository) Create(ctx context.Context, projectID, userID string) (Session, error) {
	id, err := ids.New()
	if err != nil {
		return Session{}, err
	}
	var session Session
	err = r.DB.QueryRow(ctx, `INSERT INTO sam_sessions (id, project_id, user_id)
		VALUES ($1, $2, $3)
		RETURNING id, title, created_at, updated_at`, id, projectID, userID).
		Scan(&session.ID, &session.Title, &session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return Session{}, fmt.Errorf("create SAM session: %w", err)
	}
	return session, nil
}

// Archive hides a caller-owned session in its project without deleting its transcript.
func (r Repository) Archive(ctx context.Context, projectID, userID, sessionID, archivedAt string) (bool, error) {
	tag, err := r.DB.Exec(ctx, `UPDATE sam_sessions SET archived_at=$4
		WHERE id=$1 AND project_id=$2 AND user_id=$3 AND archived_at IS NULL`, sessionID, projectID, userID, archivedAt)
	if err != nil {
		return false, fmt.Errorf("archive SAM session: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
