package aisearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
)

type fakePlans struct {
	paid bool
	err  error
	asks []string
}

func (f *fakePlans) HasPaidPlan(_ context.Context, organizationID string) (bool, error) {
	f.asks = append(f.asks, organizationID)
	return f.paid, f.err
}

// route mounts the AI search routes the way httpapi does, with a fresh project
// organization put in the context by a stand-in for the access middleware. A
// fresh organization keeps Redis cache entries from leaking between tests.
func route(t *testing.T, svc *Service, plans PaidPlans) (http.Handler, string) {
	t.Helper()
	org := newOrg()
	mux := http.NewServeMux()
	Mount(mux, Deps{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Service:     svc,
		Plans:       plans,
		WithSession: func(next http.Handler) http.Handler { return next },
		WithProjectAccess: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), org)))
			})
		},
	})
	return mux, org
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/p1/ai-search/"+path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestBrandLookupRequestValidation(t *testing.T) {
	long := strings.Repeat("a", 251)
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"not JSON", `nope`, "The request is not valid JSON."},
		{"missing query", `{}`, "query must be 1 to 250 characters."},
		{"blank query", `{"query":"   "}`, "query must be 1 to 250 characters."},
		{"query too long", `{"query":"` + long + `"}`, "query must be 1 to 250 characters."},
		{"too many competitors", `{"query":"a.com","competitors":["a","b","c","d","e","f"]}`, "competitors must have at most 5 entries."},
		{"blank competitor", `{"query":"a.com","competitors":[" "]}`, "each competitor must be 1 to 250 characters."},
		{"unknown scope", `{"query":"a.com","scope":"everything"}`, "scope must be exact_url, subfolder, domain or subdomains."},
		{"zero location", `{"query":"a.com","locationCode":0}`, "locationCode must be a positive number."},
		{"one letter language", `{"query":"a.com","languageCode":"e"}`, "languageCode must be 2 to 8 characters."},
		{"scope that does not fit the query", `{"query":"acme.com","scope":"subfolder"}`, "Add a path to use Subfolder (e.g. example.com/blog)"},
		{"too large", `{"query":"a.com","pad":"` + strings.Repeat("x", maxBody) + `"}`, "The request is too large."},
	}
	api := newFakeAPI(t)
	h, _ := route(t, newTestService(t, api), nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(h, "brand-lookup", tt.body)
			var got struct{ Error struct{ Message string } }
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("response %q is not an error envelope: %v", rec.Body, err)
			}
			if rec.Code != http.StatusBadRequest || got.Error.Message != tt.wantMsg {
				t.Errorf("got %d %q, want 400 %q", rec.Code, got.Error.Message, tt.wantMsg)
			}
		})
	}
	if api.total() != 0 {
		t.Errorf("the provider was called %d times for invalid requests", api.total())
	}
}

func TestPromptExplorerRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"missing prompt", `{"models":["claude"]}`, "prompt must be 1 to 500 characters."},
		{"prompt too long", `{"prompt":"` + strings.Repeat("a", 501) + `","models":["claude"]}`, "prompt must be 1 to 500 characters."},
		{"no models", `{"prompt":"hi","models":[]}`, "models must have 1 to 4 entries."},
		{"five models", `{"prompt":"hi","models":["claude","claude","claude","claude","claude"]}`, "models must have 1 to 4 entries."},
		{"unknown model", `{"prompt":"hi","models":["llama"]}`, "models may only be chat_gpt, claude, gemini or perplexity."},
		{"blank brand", `{"prompt":"hi","models":["claude"],"highlightBrand":"  "}`, "highlightBrand must be 1 to 250 characters."},
		{"lowercase country", `{"prompt":"hi","models":["claude"],"webSearchCountryCode":"us"}`, "webSearchCountryCode must be an ISO 3166-1 alpha-2 country code in capitals."},
		{"made-up country", `{"prompt":"hi","models":["claude"],"webSearchCountryCode":"XX"}`, "webSearchCountryCode must be an ISO 3166-1 alpha-2 country code in capitals."},
	}
	api := newFakeAPI(t)
	h, _ := route(t, newTestService(t, api), nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(h, "prompt-explorer", tt.body)
			var got struct{ Error struct{ Message string } }
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("response %q is not an error envelope: %v", rec.Body, err)
			}
			if rec.Code != http.StatusBadRequest || got.Error.Message != tt.wantMsg {
				t.Errorf("got %d %q, want 400 %q", rec.Code, got.Error.Message, tt.wantMsg)
			}
		})
	}
	if api.total() != 0 {
		t.Errorf("the provider was called %d times for invalid requests", api.total())
	}
}

func TestPromptExplorerDedupesModelsAndDefaultsToWebSearch(t *testing.T) {
	api := newFakeAPI(t)
	h, _ := route(t, newTestService(t, api), nil)

	rec := post(h, "prompt-explorer", `{"prompt":" hi ","models":["claude","claude"]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if calls := len(api.requestsTo(claudeResponsePath)); calls != 1 {
		t.Errorf("claude calls = %d, want 1 for a repeated model", calls)
	}
	var body []map[string]any
	if err := json.Unmarshal([]byte(api.requestsTo(claudeResponsePath)[0].body), &body); err != nil {
		t.Fatal(err)
	}
	if body[0]["web_search"] != true || body[0]["user_prompt"] != "hi" {
		t.Errorf("request = %v, want web_search true and the trimmed prompt", body[0])
	}
}

func TestResponsesKeepTheShapeTheWebAppReads(t *testing.T) {
	api := newFakeAPI(t)
	h, _ := route(t, newTestService(t, api), nil)

	lookup := post(h, "brand-lookup", `{"query":"acme.com"}`)
	var result map[string]any
	if err := json.Unmarshal(lookup.Body.Bytes(), &result); err != nil {
		t.Fatalf("brand lookup body %s: %v", lookup.Body, err)
	}
	for _, field := range []string{"query", "detectedTargetType", "resolvedTarget", "scope", "aggregatesAreDomainLevel", "fetchedAt", "hasData",
		"totalMentions", "totalAiSearchVolume", "perPlatform", "shareOfVoice", "topPages", "topQueries", "monthlyVolume"} {
		if _, ok := result[field]; !ok {
			t.Errorf("brand lookup lacks %q", field)
		}
	}
	if !strings.HasSuffix(result["fetchedAt"].(string), "Z") || result["scope"] != "subdomains" {
		t.Errorf("fetchedAt = %v, scope = %v", result["fetchedAt"], result["scope"])
	}

	prompt := post(h, "prompt-explorer", `{"prompt":"hi","models":["chat_gpt"],"webSearchCountryCode":"US"}`)
	var run struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(prompt.Body.Bytes(), &run); err != nil {
		t.Fatalf("prompt body %s: %v", prompt.Body, err)
	}
	success := run.Results[0]
	for _, field := range []string{"status", "model", "modelName", "text", "citations", "fanOutQueries", "brandMentioned", "outputTokens", "webSearch", "webSearchCountryCode"} {
		if _, ok := success[field]; !ok {
			t.Errorf("prompt success lacks %q", field)
		}
	}
	if success["status"] != "success" || success["webSearchCountryCode"] != "US" || success["brandMentioned"] != nil {
		t.Errorf("success = %v", success)
	}

	unsupported := post(h, "prompt-explorer", `{"prompt":"hi","models":["gemini"],"webSearchCountryCode":"US"}`)
	var failed struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(unsupported.Body.Bytes(), &failed); err != nil {
		t.Fatal(err)
	}
	failure := failed.Results[0]
	if failure["status"] != "error" || failure["errorCode"] != "UNSUPPORTED_COUNTRY" || failure["message"] == "" {
		t.Errorf("error result = %v, want status error with an errorCode and message", failure)
	}
	if _, has := failure["text"]; has {
		t.Errorf("error result carries success fields: %v", failure)
	}
}

func TestPlanGate(t *testing.T) {
	tests := []struct {
		name     string
		plans    *fakePlans
		wantCode int
		wantCall bool
	}{
		{"paid plan passes", &fakePlans{paid: true}, http.StatusOK, true},
		{"free plan is refused", &fakePlans{}, http.StatusPaymentRequired, false},
		{"a failing plan lookup fails closed", &fakePlans{err: errors.New("database down")}, http.StatusInternalServerError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(t)
			h, org := route(t, newTestService(t, api), tt.plans)

			rec := post(h, "brand-lookup", `{"query":"acme.com"}`)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tt.wantCode, rec.Body)
			}
			if (api.total() > 0) != tt.wantCall {
				t.Errorf("provider calls = %d, want calls: %v", api.total(), tt.wantCall)
			}
			if len(tt.plans.asks) != 1 || tt.plans.asks[0] != org {
				t.Errorf("plan checked for %v, want the project's organization once", tt.plans.asks)
			}
		})
	}
}

func TestNoPlanGateLetsEveryoneIn(t *testing.T) {
	api := newFakeAPI(t)
	h, _ := route(t, newTestService(t, api), nil)
	if rec := post(h, "brand-lookup", `{"query":"acme.com"}`); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 without billing configured", rec.Code)
	}
}

func TestRoutesAnswer503WithoutAProviderKey(t *testing.T) {
	h, _ := route(t, nil, nil)
	for _, path := range []string{"brand-lookup", "prompt-explorer"} {
		if rec := post(h, path, `{}`); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s status = %d, want 503", path, rec.Code)
		}
	}
}

func TestProviderBillingIssueIsHiddenFromTheUser(t *testing.T) {
	api := newFakeAPI(t)
	api.on(aggregatedPath, func(int, string) taskReply { return failReply(40200, "Payment Required. Balance is $0.00") })
	h, _ := route(t, newTestService(t, api), nil)

	rec := post(h, "brand-lookup", `{"query":"acme.com"}`)

	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "Balance") {
		t.Errorf("got %d %s, want 503 without the provider's account details", rec.Code, rec.Body)
	}
}

func TestHandlerWithoutProjectOrganizationFailsClosed(t *testing.T) {
	api := newFakeAPI(t)
	mux := http.NewServeMux()
	Mount(mux, Deps{
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		Service:           newTestService(t, api),
		WithSession:       func(next http.Handler) http.Handler { return next },
		WithProjectAccess: func(next http.Handler) http.Handler { return next }, // forgot to set the organization
	})

	rec := post(mux, "brand-lookup", `{"query":"acme.com"}`)

	if rec.Code != http.StatusInternalServerError || api.total() != 0 {
		t.Errorf("got %d with %d provider calls, want 500 and none", rec.Code, api.total())
	}
}

func TestHandlerStaysSilentWhenTheClientWentAway(t *testing.T) {
	api := newFakeAPI(t)
	h, _ := route(t, newTestService(t, api), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/projects/p1/ai-search/brand-lookup", strings.NewReader(`{"query":"acme.com"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want nothing for a canceled request", rec.Body)
	}
}
