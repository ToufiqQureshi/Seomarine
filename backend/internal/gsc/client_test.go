package gsc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/google"
)

type staticToken struct{}

func (staticToken) AccessToken(context.Context, string, string, string, string) (string, error) {
	return "test", nil
}

func TestQuerySearchAnalyticsValidatesBeforeGoogle(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		segment := strings.TrimSuffix(strings.TrimPrefix(r.URL.EscapedPath(), "/sites/"), "/searchAnalytics/query")
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded != "https://example.com/" {
			t.Errorf("site path = %q, %v", decoded, err)
		}
		var request SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if len(request.DimensionFilterGroups) != 1 || request.DimensionFilterGroups[0].Filters[0].Expression != "brand" {
			t.Errorf("filter group missing: %+v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rows": []map[string]any{{"keys": []string{"brand"}, "clicks": 1, "impressions": 2, "ctr": .5, "position": 4}}})
	}))
	defer server.Close()
	client := Client{API: &google.APIClient{Tokens: staticToken{}, Client: server.Client()}, UserID: "user", BaseURL: server.URL}
	valid := SearchRequest{StartDate: "2026-01-01", EndDate: "2026-01-31", Dimensions: []string{"query"}, RowLimit: 100, Type: "web", DataState: "all", DimensionFilterGroups: []FilterGroup{{GroupType: "and", Filters: []DimensionFilter{{Dimension: "query", Operator: "contains", Expression: "brand"}}}}}
	invalid := valid
	invalid.RowLimit = 1001
	if _, err := client.QuerySearchAnalytics(t.Context(), "https://example.com/", invalid); err == nil || calls != 0 {
		t.Fatalf("invalid request reached provider: %v, calls=%d", err, calls)
	}
	rows, err := client.QuerySearchAnalytics(t.Context(), "https://example.com/", valid)
	if err != nil || len(rows) != 1 || calls != 1 || rows[0].Keys[0] != "brand" {
		t.Fatalf("rows=%+v, err=%v, calls=%d", rows, err, calls)
	}
}
