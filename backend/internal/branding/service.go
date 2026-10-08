package branding

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service is the branding use cases. Authorization is the caller's job: the
// handlers gate edits on the organization role, and the report routes have
// already authorized the report whose organization they pass in.
type Service struct {
	repo repository
	now  func() time.Time
}

// NewService returns a Service backed by db.
func NewService(db *pgxpool.Pool) *Service {
	return &Service{repo: repository{db: db}, now: time.Now}
}

// Get returns organizationID's branding, and false when it has none.
func (s *Service) Get(ctx context.Context, organizationID string) (Branding, bool, error) {
	return s.repo.get(ctx, organizationID)
}

// Save writes organizationID's branding.
func (s *Service) Save(ctx context.Context, organizationID string, in Input) error {
	return s.repo.upsert(ctx, organizationID, in, s.now().UTC())
}

// Reset returns organizationID to the default product branding.
func (s *Service) Reset(ctx context.Context, organizationID string) error {
	return s.repo.delete(ctx, organizationID)
}

// formatTimestamp renders a timestamp the way the legacy repository did,
// JavaScript's Date#toISOString with millisecond precision.
func formatTimestamp(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000Z")
}
