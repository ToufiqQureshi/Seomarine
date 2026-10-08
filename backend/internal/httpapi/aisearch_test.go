package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/aisearch"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/kv"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/site"
)

// TestAISearchEndToEnd proves a lookup is authorized by session and project
// and that its provider spend lands on the project's organization.
func TestAISearchEndToEnd(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(ctx, t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rdb, err := kv.Open(ctx, testRedisURL(t))
	if err != nil {
		t.Fatalf("open redis: %v", err)
	}
	t.Cleanup(func() {
		if err := rdb.Close(); err != nil {
			t.Errorf("close redis: %v", err)
		}
	})

	id := rand.Text()
	userID, stranger, ownOrg, otherOrg := "user-"+id, "stranger-"+id, "org-own-"+id, "org-other-"+id
	ownProject, otherProject, token := "own-"+id, "other-"+id, "live-"+id
	t.Cleanup(func() {
		cleanupCtx := context.WithoutCancel(ctx)
		exec(cleanupCtx, t, pool, `DELETE FROM "user" WHERE id IN ($1, $2)`, userID, stranger)
		exec(cleanupCtx, t, pool, `DELETE FROM organization WHERE id IN ($1, $2)`, ownOrg, otherOrg)
	})
	exec(ctx, t, pool, `INSERT INTO "user" (id, name, email) VALUES ($1, 'Asha', $1 || '@example.com'), ($2, 'Stranger', $2 || '@example.com')`, userID, stranger)
	exec(ctx, t, pool, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Own', $1, now()), ($2, 'Other', $2, now())`, ownOrg, otherOrg)
	exec(ctx, t, pool, `INSERT INTO member (id, organization_id, user_id, created_at) VALUES ($1, $2, $3, now()), ($4, $5, $6, now())`,
		"member-"+id, ownOrg, userID, "member-stranger-"+id, otherOrg, stranger)
	exec(ctx, t, pool, `INSERT INTO projects (id, organization_id, name) VALUES ($1, $3, 'Own'), ($2, $4, 'Other')`, ownProject, otherProject, ownOrg, otherOrg)
	exec(ctx, t, pool, `INSERT INTO session (id, token, user_id, expires_at) VALUES ($1, $2, $3, now() + interval '1 hour')`, "s-"+id, token, userID)

	// A provider that answers every call with an empty, billed result.
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		task := map[string]any{
			"status_code": 20000, "status_message": "Ok.", "cost": 0.01,
			"path": strings.Split(strings.Trim(r.URL.Path, "/"), "/"), "result": []any{map[string]any{"items": []any{}, "total": map[string]any{}}},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{task}}); err != nil {
			t.Errorf("write provider response: %v", err)
		}
	}))
	t.Cleanup(provider.Close)
	client, err := dataforseo.NewClient(dataforseo.Options{
		BaseURL:  provider.URL,
		APIKey:   base64.StdEncoding.EncodeToString([]byte("login:password")),
		Recorder: dataforseo.NewUsageRecorder(pool),
	})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := site.New(&url.URL{Scheme: "https", Host: "seomarine.com"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(Deps{
		Logger:   discardLogger,
		DB:       healthy,
		Redis:    healthy,
		Auth:     auth.NewService(pool, testSecret),
		AISearch: aisearch.NewService(client, rdb, discardLogger),
		Site:     pages,
		Upstream: &url.URL{Scheme: "http", Host: "127.0.0.1:1"},
	})
	lookup := func(projectID string, signedIn bool) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/projects/"+projectID+"/ai-search/brand-lookup", strings.NewReader(`{"query":"acme.com"}`))
		if signedIn {
			req.AddCookie(&http.Cookie{Name: "__Secure-better-auth.session_token", Value: sign(token)})
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	usage := func(org string) (rows int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM go_dataforseo_usage WHERE organization_id = $1`, org).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}

	assertError(t, lookup(ownProject, false), http.StatusUnauthorized, "unauthenticated")
	assertError(t, lookup(otherProject, true), http.StatusNotFound, "project_not_found")
	if usage(ownOrg)+usage(otherOrg) != 0 {
		t.Fatal("a refused request was billed")
	}

	rec := lookup(ownProject, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, body %s", rec.Code, rec.Body)
	}
	// Three calls for each of the two platforms, all billed to the project's organization.
	if got := usage(ownOrg); got != 6 {
		t.Errorf("usage rows for the project's organization = %d, want 6", got)
	}
	if got := usage(otherOrg); got != 0 {
		t.Errorf("usage rows for another organization = %d, want 0", got)
	}
}
