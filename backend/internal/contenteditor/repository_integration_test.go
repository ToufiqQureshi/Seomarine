package contenteditor

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
)

func TestRepositoryDraftLifecycleIsProjectScoped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := contentTestPool(ctx, t)
	suffix := strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	organizationID := "content-test-org-" + suffix
	projectID := "content-test-project-" + suffix
	otherProjectID := "content-test-other-project-" + suffix
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Content test', $1, now())`, organizationID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM organization WHERE id=$1`, organizationID); err != nil {
			t.Errorf("clean up content editor fixture: %v", err)
		}
	})
	for _, id := range []string{projectID, otherProjectID} {
		if _, err := pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name) VALUES ($1, $2, $1)`, id, organizationID); err != nil {
			t.Fatal(err)
		}
	}
	repo := Repository{DB: pool}
	input := Input{Title: "  Technical SEO guide  ", TargetKeyword: " technical SEO ", Slug: "technical-seo-guide", MetaDescription: "A useful guide", Content: "Draft body"}
	created, err := repo.Save(ctx, projectID, input)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if created.ID == "" || created.ProjectID != projectID || created.Title != "Technical SEO guide" || !created.UpdatedAt.After(created.CreatedAt.Add(-time.Second)) {
		t.Fatalf("created draft has unexpected fields: %+v", created)
	}
	input.ID = created.ID
	input.Title = "Updated SEO guide"
	updated, err := repo.Save(ctx, projectID, input)
	if err != nil || updated.Title != input.Title {
		t.Fatalf("update draft: got %+v, err %v", updated, err)
	}
	if _, err := repo.Save(ctx, otherProjectID, input); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project update error = %v, want ErrNotFound", err)
	}
	drafts, err := repo.List(ctx, projectID)
	if err != nil || len(drafts) != 1 || drafts[0].ID != created.ID || drafts[0].Title != input.Title {
		t.Fatalf("list drafts = %+v, err %v", drafts, err)
	}
	if loaded, err := repo.Get(ctx, projectID, created.ID); err != nil || loaded.Content != input.Content {
		t.Fatalf("get draft = %+v, err %v", loaded, err)
	}
	otherDrafts, err := repo.List(ctx, otherProjectID)
	if err != nil || len(otherDrafts) != 0 {
		t.Fatalf("other project drafts = %+v, err %v", otherDrafts, err)
	}
	if err := repo.Delete(ctx, otherProjectID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project delete error = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, projectID, created.ID); err != nil {
		t.Fatalf("delete draft: %v", err)
	}
	if loaded, err := repo.Get(ctx, projectID, created.ID); !errors.Is(err, ErrNotFound) || loaded.ID != "" {
		t.Fatalf("read deleted draft = %+v, %v; want ErrNotFound", loaded, err)
	}
	if loaded, err := repo.Get(ctx, otherProjectID, created.ID); !errors.Is(err, ErrNotFound) || loaded.ID != "" {
		t.Fatalf("cross-project read = %+v, %v; want ErrNotFound", loaded, err)
	}
	if _, err := repo.Save(ctx, "missing-project", Input{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("create against missing project error = %v, want ErrNotFound", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO go_content_drafts (id, project_id) SELECT 'cap-' || n, $1 FROM generate_series(1, $2) n`, projectID, MaxDraftsPerProject); err != nil {
		t.Fatalf("seed draft limit: %v", err)
	}
	if _, err := repo.Save(ctx, projectID, Input{Title: "Over the limit"}); !errors.Is(err, ErrDraftLimit) {
		t.Fatalf("create beyond project draft limit error = %v, want ErrDraftLimit", err)
	}
}

func TestRepositoryDraftAuthorIsProjectScoped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := contentTestPool(ctx, t)
	suffix := strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	organizationID := "author-test-org-" + suffix
	projectID, otherProjectID := "author-test-p-"+suffix, "author-test-p2-"+suffix
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'A', $1, now())`, organizationID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM organization WHERE id=$1`, organizationID)
	})
	for _, id := range []string{projectID, otherProjectID} {
		if _, err := pool.Exec(ctx, `INSERT INTO projects (id, organization_id, name) VALUES ($1, $2, $1)`, id, organizationID); err != nil {
			t.Fatal(err)
		}
	}
	var authorID string
	if err := pool.QueryRow(ctx, `INSERT INTO go_content_authors (id, project_id, name)
		VALUES ($1, $2, 'Aman') RETURNING id`, "author-"+suffix, projectID).Scan(&authorID); err != nil {
		t.Fatal(err)
	}
	repo := Repository{DB: pool}
	var validation *ValidationError
	// Another project's author can never sign this project's draft.
	if _, err := repo.Save(ctx, otherProjectID, Input{Title: "T", AuthorID: authorID}); !errors.As(err, &validation) {
		t.Fatalf("cross-project author error = %v, want a ValidationError", err)
	}
	created, err := repo.Save(ctx, projectID, Input{
		Title: "T", AuthorID: authorID, LastReviewedDate: "2026-10-15",
	})
	if err != nil {
		t.Fatalf("save with author: %v", err)
	}
	if created.AuthorID != authorID || created.LastReviewedAt == nil || created.LastReviewedAt.Format("2006-01-02") != "2026-10-15" {
		t.Fatalf("author fields lost: %+v", created)
	}
	var badDate *ValidationError
	if _, err := repo.Save(ctx, projectID, Input{Title: "T", LastReviewedDate: "15/10/2026"}); !errors.As(err, &badDate) {
		t.Fatalf("malformed review date error = %v, want a ValidationError", err)
	}
	// Deleting the author clears the draft's author_id, never the draft.
	if _, err := pool.Exec(ctx, `DELETE FROM go_content_authors WHERE id=$1`, authorID); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.Get(ctx, projectID, created.ID); err != nil || got.ID != created.ID || got.AuthorID != "" {
		t.Fatalf("draft after author delete = %+v, %v", got, err)
	}
}

func TestValidateDraftRejectsOversizedFields(t *testing.T) {
	tests := []struct {
		name  string
		input Input
	}{
		{name: "title", input: Input{Title: strings.Repeat("x", MaxTitleLength+1)}},
		{name: "keyword", input: Input{TargetKeyword: strings.Repeat("x", MaxKeywordLength+1)}},
		{name: "slug", input: Input{Slug: strings.Repeat("x", MaxSlugLength+1)}},
		{name: "meta description", input: Input{MetaDescription: strings.Repeat("x", MaxMetaLength+1)}},
		{name: "content", input: Input{Content: strings.Repeat("x", MaxContentLength+1)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validate(tc.input); err == nil {
				t.Fatal("validate() accepted an oversized field")
			}
		})
	}
}

func contentTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
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
