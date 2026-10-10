package sam

import (
	"context"
	"errors"
	"time"
)

// ErrSessionNotFound hides sessions that are archived, owned by another user,
// or belong to a different project.
var ErrSessionNotFound = errors.New("SAM session not found")

// SessionStore contains the operations required by the session registry API.
type SessionStore interface {
	ListForProject(context.Context, string, string) ([]Session, error)
	Create(context.Context, string, string) (Session, error)
	Archive(context.Context, string, string, string, string) (bool, error)
}

// Service applies session registry behavior over the existing SAM session table.
type Service struct {
	Store SessionStore
	Now   func() time.Time
}

// List returns the signed-in user's active sessions in a project.
func (s *Service) List(ctx context.Context, projectID, userID string) ([]Session, error) {
	return s.Store.ListForProject(ctx, projectID, userID)
}

// Create starts a registry session for the signed-in project member.
func (s *Service) Create(ctx context.Context, projectID, userID string) (Session, error) {
	return s.Store.Create(ctx, projectID, userID)
}

// Archive soft-deletes a session after scoping it to the project and owner.
func (s *Service) Archive(ctx context.Context, projectID, userID, sessionID string) error {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	archivedAt := now.Format("2006-01-02T15:04:05.000Z")
	updated, err := s.Store.Archive(ctx, projectID, userID, sessionID, archivedAt)
	if err != nil {
		return err
	}
	if !updated {
		return ErrSessionNotFound
	}
	return nil
}
