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
	"sync/atomic"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/billing"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/domain"
	"github.com/toufiqqureshi/uplumo/backend/internal/kv"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/uplumo/backend/internal/site"
)

// TestPaidLookupsSpendTheWorkspacesCreditsEndToEnd runs a domain lookup
// through the real spend guard, credit ledger and Postgres: without credits
// nothing reaches the provider; with credits the real cost is charged and the
// Billing screen shows where it went.
func TestPaidLookupsSpendTheWorkspacesCreditsEndToEnd(t *testing.T) {
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
	userID, org, project, token := "spend-user-"+id, "spend-org-"+id, "spend-project-"+id, "spend-live-"+id
	t.Cleanup(func() {
		cleanupCtx := context.WithoutCancel(ctx)
		exec(cleanupCtx, t, pool, `DELETE FROM "user" WHERE id = $1`, userID)
		exec(cleanupCtx, t, pool, `DELETE FROM organization WHERE id = $1`, org)
	})
	exec(ctx, t, pool, `INSERT INTO "user" (id,name,email) VALUES ($1,'Asha',$1 || '@example.com')`, userID)
	exec(ctx, t, pool, `INSERT INTO organization (id,name,slug,created_at) VALUES ($1,'Own',$1,now())`, org)
	exec(ctx, t, pool, `INSERT INTO member (id,organization_id,user_id,role,created_at) VALUES ($1,$2,$3,'owner',now())`, "spend-member-"+id, org, userID)
	exec(ctx, t, pool, `INSERT INTO projects (id,organization_id,name,location_code,language_code) VALUES ($1,$2,'Own',2356,'hi')`, project, org)
	exec(ctx, t, pool, `INSERT INTO session (id,token,user_id,active_organization_id,expires_at,updated_at) VALUES ($1,$2,$3,$4,now()+interval '1 hour',now())`, "spend-session-"+id, token, userID, org)

	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		task := map[string]any{"status_code": 20000, "status_message": "Ok.", "cost": 0.01, "path": strings.Split(strings.Trim(r.URL.Path, "/"), "/"), "result": []any{map[string]any{"items": []any{map[string]any{"metrics": map[string]any{"organic": map[string]any{"etv": 100.0, "count": 10.0}}}}}}}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{task}}); err != nil {
			t.Errorf("write DataForSEO response: %v", err)
		}
	}))
	t.Cleanup(provider.Close)

	ledger := billing.NewCreditLedgerService(pool)
	guard, err := billing.NewSpendGuard(pool, ledger, 1e9, true, discardLogger)
	if err != nil {
		t.Fatal(err)
	}
	client, err := dataforseo.NewClient(dataforseo.Options{
		BaseURL: provider.URL, APIKey: base64.StdEncoding.EncodeToString([]byte("fake-login:fake-password")),
		Recorder: dataforseo.NewUsageRecorder(pool), Guard: guard,
	})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := site.New(&url.URL{Scheme: "https", Host: "uplumo.com"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{
		Logger: discardLogger, DB: healthy, Redis: healthy, Auth: auth.NewService(pool, testSecret),
		Domain: domain.NewService(client, rdb, discardLogger), ProjectMarkets: domain.ProjectMarketRepository{DB: pool},
		BillingRoutes: billing.NewReadOnlyService(pool), Credits: ledger, Site: pages,
	})
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "__Secure-better-auth.session_token", Value: sign(token), HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	lookup := func() *httptest.ResponseRecorder {
		return call(http.MethodPost, "/api/v1/projects/"+project+"/domain/overview", `{"domain":"example.com","scope":"domain"}`)
	}

	assertError(t, lookup(), http.StatusPaymentRequired, "credits_exhausted")
	if providerCalls.Load() != 0 {
		t.Fatalf("a workspace without credits reached the provider %d times", providerCalls.Load())
	}

	if err := ledger.AddCredits(ctx, org, 1000, "test grant"); err != nil {
		t.Fatal(err)
	}
	if rec := lookup(); rec.Code != http.StatusOK {
		t.Fatalf("lookup with credits: status %d body %s", rec.Code, rec.Body)
	}
	balance, err := ledger.GetBalance(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	spentCredits := 1000 - balance
	if spentCredits != billing.CreditsForUSD(0.01)*int64(providerCalls.Load()) {
		t.Fatalf("spent %d credits for %d provider tasks at $0.01, want %d each", spentCredits, providerCalls.Load(), billing.CreditsForUSD(0.01))
	}

	rec := call(http.MethodGet, "/api/v1/billing/usage", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("usage status %d body %s", rec.Code, rec.Body)
	}
	var usage struct {
		BalanceCredits int64 `json:"balanceCredits"`
		Spent          []struct {
			Feature string `json:"feature"`
			Credits int64  `json:"credits"`
		} `json:"spent"`
		Costs []struct {
			Feature string `json:"feature"`
			Credits int64  `json:"credits"`
		} `json:"costs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.BalanceCredits != balance || len(usage.Spent) != 1 || usage.Spent[0].Feature != "domain_research" || usage.Spent[0].Credits != spentCredits {
		t.Fatalf("usage = %+v, want %d credits left and %d spent on domain_research", usage, balance, spentCredits)
	}
	if len(usage.Costs) != 9 {
		t.Fatalf("usage lists %d action costs, want 9", len(usage.Costs))
	}
}
