package sitepublish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound means the project or draft has no publication.
var ErrNotFound = errors.New("not published")

// Repository stores connections and publications in Postgres.
type Repository struct{ DB *pgxpool.Pool }

// Connection returns the project's website, or ErrNotConnected.
func (r Repository) Connection(ctx context.Context, projectID string) (Connection, error) {
	var c Connection
	err := r.DB.QueryRow(ctx, `SELECT project_id, platform, site_url, api_base, username, secret_ciphertext, site_name, connected_by, created_at
		FROM go_site_connections WHERE project_id=$1`, projectID).
		Scan(&c.ProjectID, &c.Platform, &c.SiteURL, &c.APIBase, &c.Username, &c.SecretCiphertext, &c.SiteName, &c.ConnectedBy, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Connection{}, ErrNotConnected
	}
	if err != nil {
		return Connection{}, fmt.Errorf("load site connection: %w", err)
	}
	return c, nil
}

// SaveConnection stores the project's website, replacing any earlier one.
func (r Repository) SaveConnection(ctx context.Context, c Connection) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO go_site_connections
		(project_id, platform, site_url, api_base, username, secret_ciphertext, site_name, connected_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (project_id) DO UPDATE SET platform=EXCLUDED.platform, site_url=EXCLUDED.site_url,
			api_base=EXCLUDED.api_base, username=EXCLUDED.username, secret_ciphertext=EXCLUDED.secret_ciphertext,
			site_name=EXCLUDED.site_name, connected_by=EXCLUDED.connected_by, created_at=now(), updated_at=now()`,
		c.ProjectID, c.Platform, c.SiteURL, c.APIBase, c.Username, c.SecretCiphertext, c.SiteName, c.ConnectedBy)
	if err != nil {
		return fmt.Errorf("save site connection: %w", err)
	}
	return nil
}

// DeleteConnection forgets the project's website. Deleting a project that
// has none is not an error.
func (r Repository) DeleteConnection(ctx context.Context, projectID string) error {
	if _, err := r.DB.Exec(ctx, `DELETE FROM go_site_connections WHERE project_id=$1`, projectID); err != nil {
		return fmt.Errorf("delete site connection: %w", err)
	}
	return nil
}

// Publication returns where a draft was published, or ErrNotFound.
func (r Repository) Publication(ctx context.Context, projectID, draftID string) (Publication, error) {
	var p Publication
	var media []byte
	err := r.DB.QueryRow(ctx, `SELECT draft_id, project_id, site_url, remote_id, remote_url, remote_status, media, published_at, updated_at
		FROM go_site_publications WHERE project_id=$1 AND draft_id=$2`, projectID, draftID).
		Scan(&p.DraftID, &p.ProjectID, &p.SiteURL, &p.RemoteID, &p.RemoteURL, &p.RemoteStatus, &media, &p.PublishedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Publication{}, ErrNotFound
	}
	if err != nil {
		return Publication{}, fmt.Errorf("load publication: %w", err)
	}
	if err := json.Unmarshal(media, &p.Media); err != nil {
		return Publication{}, fmt.Errorf("read publication media: %w", err)
	}
	return p, nil
}

// SavePublication records where a draft was published. The draft must
// belong to the project; a draft deleted meanwhile is reported as not found.
func (r Repository) SavePublication(ctx context.Context, p Publication) error {
	media, err := json.Marshal(p.Media)
	if err != nil {
		return fmt.Errorf("encode publication media: %w", err)
	}
	tag, err := r.DB.Exec(ctx, `INSERT INTO go_site_publications
		(draft_id, project_id, site_url, remote_id, remote_url, remote_status, media)
		SELECT d.id, d.project_id, $3, $4, $5, $6, $7 FROM go_content_drafts d WHERE d.id=$1 AND d.project_id=$2
		ON CONFLICT (draft_id) DO UPDATE SET site_url=EXCLUDED.site_url, remote_id=EXCLUDED.remote_id,
			remote_url=EXCLUDED.remote_url, remote_status=EXCLUDED.remote_status, media=EXCLUDED.media, updated_at=now()`,
		p.DraftID, p.ProjectID, p.SiteURL, p.RemoteID, p.RemoteURL, p.RemoteStatus, media)
	if err != nil {
		return fmt.Errorf("save publication: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
