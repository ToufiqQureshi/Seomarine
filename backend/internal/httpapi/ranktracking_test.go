package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/domain"
	"github.com/toufiqqureshi/seomarine/backend/internal/keywords"
	"github.com/toufiqqureshi/seomarine/backend/internal/ranktracking"
	"github.com/toufiqqureshi/seomarine/backend/internal/site"
)

func TestRankTrackingIsScopedToTheSignedInOrganizationAndUsesProjectMarket(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(ctx, t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	id := rand.Text()
	userID, stranger, ownOrg, otherOrg := "rt-user-"+id, "rt-stranger-"+id, "rt-own-"+id, "rt-other-"+id
	ownProject, siblingProject, otherProject, token := "rt-own-"+id, "rt-sibling-"+id, "rt-other-"+id, "rt-live-"+id
	t.Cleanup(func() {
		cleanupCtx := context.WithoutCancel(ctx)
		exec(cleanupCtx, t, pool, `DELETE FROM go_rank_tracking_configs WHERE project_id IN ($1,$2,$3)`, ownProject, siblingProject, otherProject)
		exec(cleanupCtx, t, pool, `DELETE FROM "user" WHERE id IN ($1,$2)`, userID, stranger)
		exec(cleanupCtx, t, pool, `DELETE FROM organization WHERE id IN ($1,$2)`, ownOrg, otherOrg)
	})
	exec(ctx, t, pool, `INSERT INTO "user" (id,name,email) VALUES ($1,'Asha',$1 || '@example.com'),($2,'Stranger',$2 || '@example.com')`, userID, stranger)
	exec(ctx, t, pool, `INSERT INTO organization (id,name,slug,created_at) VALUES ($1,'Own',$1,now()),($2,'Other',$2,now())`, ownOrg, otherOrg)
	exec(ctx, t, pool, `INSERT INTO member (id,organization_id,user_id,created_at) VALUES ($1,$2,$3,now()),($4,$5,$6,now())`, "rt-member-"+id, ownOrg, userID, "rt-member-stranger-"+id, otherOrg, stranger)
	exec(ctx, t, pool, `INSERT INTO projects (id,organization_id,name,location_code,language_code) VALUES ($1,$3,'Own',2356,'hi'),($2,$4,'Other',2124,'fr'),($5,$3,'Sibling',2840,'en')`, ownProject, otherProject, ownOrg, otherOrg, siblingProject)
	exec(ctx, t, pool, `INSERT INTO session (id,token,user_id,expires_at) VALUES ($1,$2,$3,now()+interval '1 hour')`, "rt-session-"+id, token, userID)

	pages, err := site.New(&url.URL{Scheme: "https", Host: "seomarine.com"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{
		Logger: discardLogger, DB: healthy, Redis: healthy, Auth: auth.NewService(pool, testSecret),
		RankTracking:   ranktracking.NewService(ranktracking.Store{DB: pool}, ranktracking.Store{DB: pool}, nil),
		ProjectMarkets: domain.ProjectMarketRepository{DB: pool}, Site: pages,
		Upstream: &url.URL{Scheme: "http", Host: "127.0.0.1:1"},
	})
	call := func(project, path, body string, signed bool) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/projects/"+project+"/rank-tracking/"+path, strings.NewReader(body))
		if signed {
			req.AddCookie(&http.Cookie{Name: "__Secure-better-auth.session_token", Value: sign(token)})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	create := `{"domain":"example.com","serpDepth":10}`
	assertError(t, call(ownProject, "configs/create", create, false), http.StatusUnauthorized, "unauthenticated")
	assertError(t, call(otherProject, "configs/create", create, true), http.StatusNotFound, "project_not_found")
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM go_rank_tracking_configs WHERE project_id = $1`, otherProject).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("a refused request wrote %d rows (err %v)", rows, err)
	}

	rec := call(ownProject, "configs/create", create, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body)
	}
	var created struct {
		Config ranktracking.Config `json:"config"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Config.LocationCode != 2356 || created.Config.LanguageCode != "hi" {
		t.Errorf("market = %d/%s, want the project's India/Hindi", created.Config.LocationCode, created.Config.LanguageCode)
	}
	// A config id from another project of the same organization must not be
	// readable through this one.
	assertError(t, call(siblingProject, "keywords/list", `{"configId":"`+created.Config.ID+`"}`, true), http.StatusNotFound, "config_not_found")
}

func TestSavedKeywordsAreScopedToTheSignedInOrganization(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(ctx, t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	id := rand.Text()
	userID, ownOrg, otherOrg := "sk-user-"+id, "sk-own-"+id, "sk-other-"+id
	ownProject, otherProject, token := "sk-own-"+id, "sk-other-"+id, "sk-live-"+id
	t.Cleanup(func() {
		c := context.WithoutCancel(ctx)
		exec(c, t, pool, `DELETE FROM go_saved_keywords WHERE project_id IN ($1,$2)`, ownProject, otherProject)
		exec(c, t, pool, `DELETE FROM "user" WHERE id = $1`, userID)
		exec(c, t, pool, `DELETE FROM organization WHERE id IN ($1,$2)`, ownOrg, otherOrg)
	})
	exec(ctx, t, pool, `INSERT INTO "user" (id,name,email) VALUES ($1,'Asha',$1 || '@example.com')`, userID)
	exec(ctx, t, pool, `INSERT INTO organization (id,name,slug,created_at) VALUES ($1,'Own',$1,now()),($2,'Other',$2,now())`, ownOrg, otherOrg)
	exec(ctx, t, pool, `INSERT INTO member (id,organization_id,user_id,created_at) VALUES ($1,$2,$3,now())`, "sk-member-"+id, ownOrg, userID)
	exec(ctx, t, pool, `INSERT INTO projects (id,organization_id,name,location_code,language_code) VALUES ($1,$3,'Own',2356,'hi'),($2,$4,'Other',2124,'fr')`, ownProject, otherProject, ownOrg, otherOrg)
	exec(ctx, t, pool, `INSERT INTO session (id,token,user_id,expires_at) VALUES ($1,$2,$3,now()+interval '1 hour')`, "sk-session-"+id, token, userID)

	pages, err := site.New(&url.URL{Scheme: "https", Host: "seomarine.com"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{
		Logger: discardLogger, DB: healthy, Redis: healthy, Auth: auth.NewService(pool, testSecret),
		SavedKeywords:  &keywords.SavedService{Store: keywords.SavedRepository{DB: pool}},
		ProjectMarkets: domain.ProjectMarketRepository{DB: pool}, Site: pages,
		Upstream: &url.URL{Scheme: "http", Host: "127.0.0.1:1"},
	})
	call := func(project, path, body string, signed bool) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/projects/"+project+"/keywords/saved/"+path, strings.NewReader(body))
		if signed {
			req.AddCookie(&http.Cookie{Name: "__Secure-better-auth.session_token", Value: sign(token)})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	save := `{"keywords":["alpha"]}`
	assertError(t, call(ownProject, "save", save, false), http.StatusUnauthorized, "unauthenticated")
	assertError(t, call(otherProject, "save", save, true), http.StatusNotFound, "project_not_found")
	if rec := call(ownProject, "save", save, true); rec.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", rec.Code, rec.Body)
	}
	var code int
	var lang string
	if err := pool.QueryRow(ctx, `SELECT location_code, language_code FROM go_saved_keywords WHERE project_id = $1`, ownProject).Scan(&code, &lang); err != nil || code != 2356 || lang != "hi" {
		t.Errorf("market = %d/%s (%v), want the project's India/Hindi", code, lang, err)
	}
	var other int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM go_saved_keywords WHERE project_id = $1`, otherProject).Scan(&other)
	if other != 0 {
		t.Errorf("a refused request saved %d keywords for another organization's project", other)
	}
}
