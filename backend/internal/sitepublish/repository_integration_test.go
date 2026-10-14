package sitepublish

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/contenteditor"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
)

func TestRepositoryKeepsOneConnectionPerProjectAndScopesPublications(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	pool := publishTestPool(ctx, t)
	suffix := strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	organizationID := "publish-test-org-" + suffix
	projectID, otherProjectID := "publish-test-project-"+suffix, "publish-test-other-"+suffix
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Publish test', $1, now())`, organizationID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM organization WHERE id=$1`, organizationID); err != nil {
			t.Errorf("clean up publish fixture: %v", err)
		}
	})
	for _, id := range []string{projectID, otherProjectID} {
		if _, err := pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name) VALUES ($1, $2, $1)`, id, organizationID); err != nil {
			t.Fatal(err)
		}
	}
	repo := Repository{DB: pool}

	if _, err := repo.Connection(ctx, projectID); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("empty project: err = %v, want ErrNotConnected", err)
	}
	first := Connection{ProjectID: projectID, Platform: WordPressPlatform, SiteURL: "https://a.example", APIBase: "https://a.example/wp-json", Username: "u1", SecretCiphertext: "k:one", SiteName: "A", ConnectedBy: "user-1"}
	if err := repo.SaveConnection(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.SiteURL, second.APIBase, second.Username, second.SecretCiphertext = "https://b.example", "https://b.example/?rest_route=", "u2", "k:two"
	if err := repo.SaveConnection(ctx, second); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	got, err := repo.Connection(ctx, projectID)
	if err != nil || got.SiteURL != second.SiteURL || got.APIBase != second.APIBase || got.Username != "u2" || got.SecretCiphertext != "k:two" {
		t.Fatalf("connection after reconnect = %+v, err %v", got, err)
	}
	if _, err := repo.Connection(ctx, otherProjectID); !errors.Is(err, ErrNotConnected) {
		t.Errorf("another project sees this connection: %v", err)
	}

	draft, err := contenteditor.Repository{DB: pool}.Save(ctx, projectID, contenteditor.Input{Title: "T", Content: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	publication := Publication{DraftID: draft.ID, ProjectID: projectID, SiteURL: second.SiteURL, RemoteID: "7", RemoteURL: "https://b.example/?p=7", RemoteStatus: StatusDraft,
		Media: map[string]Media{"https://img.example/a.png": {ID: 3, URL: "https://b.example/a.png"}}}
	if err := repo.SavePublication(ctx, publication); err != nil {
		t.Fatal(err)
	}
	publication.RemoteStatus = StatusPublish
	if err := repo.SavePublication(ctx, publication); err != nil {
		t.Fatalf("save again: %v", err)
	}
	stored, err := repo.Publication(ctx, projectID, draft.ID)
	if err != nil || stored.RemoteID != "7" || stored.RemoteStatus != StatusPublish || stored.Media["https://img.example/a.png"].ID != 3 {
		t.Fatalf("publication = %+v, err %v", stored, err)
	}
	if _, err := repo.Publication(ctx, otherProjectID, draft.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another project reads this publication: %v", err)
	}
	wrongProject := publication
	wrongProject.ProjectID = otherProjectID
	if err := repo.SavePublication(ctx, wrongProject); !errors.Is(err, ErrNotFound) {
		t.Errorf("saving against another project's draft: err = %v, want ErrNotFound", err)
	}

	if err := (contenteditor.Repository{DB: pool}).Delete(ctx, projectID, draft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Publication(ctx, projectID, draft.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("publication outlived its draft: %v", err)
	}
	if err := repo.DeleteConnection(ctx, projectID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteConnection(ctx, projectID); err != nil {
		t.Errorf("deleting twice: %v", err)
	}
	if _, err := repo.Connection(ctx, projectID); !errors.Is(err, ErrNotConnected) {
		t.Errorf("connection after delete: %v", err)
	}
}

func publishTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	pool, err := pgdb.Open(ctx, url)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return pool
}
