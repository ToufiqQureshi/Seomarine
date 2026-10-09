package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/domain"
	"github.com/toufiqqureshi/seomarine/backend/internal/kv"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/site"
)

func TestDomainEndToEndAttributesPaidProviderTaskToAuthorizedProjectOrganization(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(ctx, t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rdb, err := kv.Open(ctx, testRedisURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rdb.Close(); err != nil {
			t.Errorf("close redis: %v", err)
		}
	})
	id := rand.Text()
	userID, stranger, ownOrg, otherOrg := "domain-user-"+id, "domain-stranger-"+id, "domain-own-"+id, "domain-other-"+id
	ownProject, otherProject, token := "domain-own-"+id, "domain-other-"+id, "domain-live-"+id
	t.Cleanup(func() {
		cleanupCtx := context.WithoutCancel(ctx)
		exec(cleanupCtx, t, pool, `DELETE FROM "user" WHERE id IN ($1,$2)`, userID, stranger)
		exec(cleanupCtx, t, pool, `DELETE FROM organization WHERE id IN ($1,$2)`, ownOrg, otherOrg)
	})
	exec(ctx, t, pool, `INSERT INTO "user" (id,name,email) VALUES ($1,'Asha',$1 || '@example.com'),($2,'Stranger',$2 || '@example.com')`, userID, stranger)
	exec(ctx, t, pool, `INSERT INTO organization (id,name,slug,created_at) VALUES ($1,'Own',$1,now()),($2,'Other',$2,now())`, ownOrg, otherOrg)
	exec(ctx, t, pool, `INSERT INTO member (id,organization_id,user_id,created_at) VALUES ($1,$2,$3,now()),($4,$5,$6,now())`, "domain-member-"+id, ownOrg, userID, "domain-member-stranger-"+id, otherOrg, stranger)
	exec(ctx, t, pool, `INSERT INTO projects (id,organization_id,name,location_code,language_code) VALUES ($1,$3,'Own',2356,'hi'),($2,$4,'Other',2124,'fr')`, ownProject, otherProject, ownOrg, otherOrg)
	if _, err := (domain.ProjectMarketRepository{DB: pool}).Get(ctx, ownOrg, otherProject); !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("org A read org B market: %v", err)
	}
	exec(ctx, t, pool, `INSERT INTO session (id,token,user_id,expires_at) VALUES ($1,$2,$3,now()+interval '1 hour')`, "domain-session-"+id, token, userID)
	providerMarkets := make(chan map[string]any, 1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tasks []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&tasks); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		if len(tasks) == 1 {
			providerMarkets <- tasks[0]
		}
		task := map[string]any{"status_code": 20000, "status_message": "Ok.", "cost": 0.01, "path": strings.Split(strings.Trim(r.URL.Path, "/"), "/"), "result": []any{map[string]any{"items": []any{map[string]any{"metrics": map[string]any{"organic": map[string]any{"etv": 100.0, "count": 10.0}}}}}}}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{task}}); err != nil {
			t.Errorf("write DataForSEO response: %v", err)
		}
	}))
	t.Cleanup(provider.Close)
	client, err := dataforseo.NewClient(dataforseo.Options{BaseURL: provider.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("fake-login:fake-password")), Recorder: dataforseo.NewUsageRecorder(pool)})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := site.New(&url.URL{Scheme: "https", Host: "seomarine.com"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{Logger: discardLogger, DB: healthy, Redis: healthy, Auth: auth.NewService(pool, testSecret), Domain: domain.NewService(client, rdb, discardLogger), ProjectMarkets: domain.ProjectMarketRepository{DB: pool}, Site: pages, Upstream: &url.URL{Scheme: "http", Host: "127.0.0.1:1"}})
	lookup := func(project string, signed bool) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/projects/"+project+"/domain/overview", strings.NewReader(`{"domain":"example.com","scope":"domain"}`))
		if signed {
			req.AddCookie(&http.Cookie{Name: "__Secure-better-auth.session_token", Value: sign(token)})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	usage := func(org string) int {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM go_dataforseo_usage WHERE organization_id=$1`, org).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	assertError(t, lookup(ownProject, false), http.StatusUnauthorized, "unauthenticated")
	assertError(t, lookup(otherProject, true), http.StatusNotFound, "project_not_found")
	if usage(ownOrg)+usage(otherOrg) != 0 {
		t.Fatal("a refused project access was billed")
	}
	rec := lookup(ownProject, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("lookup status=%d body=%s", rec.Code, rec.Body)
	}
	providerMarket := <-providerMarkets
	if providerMarket["location_code"] != float64(2356) || providerMarket["language_code"] != "hi" {
		t.Errorf("provider market = %v, want authorized project's India/Hindi", providerMarket)
	}
	if got := usage(ownOrg); got != 1 {
		t.Errorf("authorized organization usage=%d, want 1", got)
	}
	if got := usage(otherOrg); got != 0 {
		t.Errorf("other organization usage=%d, want 0", got)
	}
}
