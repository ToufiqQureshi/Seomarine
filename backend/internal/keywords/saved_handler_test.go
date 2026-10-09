package keywords

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

type indiaMarket struct{}

func (indiaMarket) Get(context.Context, string, string) (market.Pair, error) {
	return market.Pair{LocationCode: 2356, LanguageCode: "hi"}, nil
}

func newSavedServer(t *testing.T) (*httptest.Server, savedFixture) {
	t.Helper()
	f := newSavedFixture(t)
	mux := http.NewServeMux()
	MountSaved(mux, SavedDeps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Service: f.svc, ProjectMarkets: indiaMarket{},
		WithSession: func(next http.Handler) http.Handler { return next },
		WithProjectAccess: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-test")))
			})
		},
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, f
}

func postSaved(t *testing.T, srv *httptest.Server, project, path, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/projects/"+project+"/keywords/saved/"+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s response: %v", path, err)
	}
	return resp.StatusCode, out
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestSavedKeywordFlowOverHTTP(t *testing.T) {
	srv, f := newSavedServer(t)
	p := f.project

	status, body := postSaved(t, srv, p, "save", `{"keywords":["Alpha","beta"],"tags":["Brand"],"metrics":[{"keyword":"alpha","searchVolume":10,"cpc":1.2,"competition":0.3,"keywordDifficulty":20,"intent":"commercial","monthlySearches":[{"year":2026,"month":9,"searchVolume":8}]}]}`)
	if status != 200 || body["success"] != true || len(body["savedKeywordIds"].([]any)) != 2 {
		t.Fatalf("save = %d %v", status, body)
	}
	saved := body["savedKeywordIds"].([]any)
	var code int
	var lang string
	_ = f.pool.QueryRow(context.Background(), `SELECT location_code, language_code FROM go_saved_keywords WHERE id = $1`, saved[0]).Scan(&code, &lang)
	if code != 2356 || lang != "hi" {
		t.Errorf("market = %d/%s, want the project's India/Hindi", code, lang)
	}

	status, body = postSaved(t, srv, p, "list", `{"sort":"searchVolume","order":"desc"}`)
	rows := body["rows"].([]any)
	if status != 200 || len(rows) != 2 || body["totalCount"] != float64(2) || len(body["tags"].([]any)) != 1 {
		t.Fatalf("list = %d %v", status, body)
	}
	first := rows[0].(map[string]any)
	if first["keyword"] != "alpha" || first["searchVolume"] != float64(10) || first["intent"] != "commercial" || len(first["tags"].([]any)) != 1 ||
		len(first["monthlySearches"].([]any)) != 1 || first["fetchedAt"] == nil {
		t.Errorf("first row = %v", first)
	}
	if rows[1].(map[string]any)["searchVolume"] != nil {
		t.Errorf("second row = %v, want null metrics", rows[1])
	}
	if status, body = postSaved(t, srv, p, "export", `{"tagNames":["brand"]}`); status != 200 || len(body["rows"].([]any)) != 2 {
		t.Errorf("export = %d %v", status, body)
	}

	tagID := body["rows"].([]any)[0].(map[string]any)["tags"].([]any)[0].(map[string]any)["id"].(string)
	status, body = postSaved(t, srv, p, "tags/assign", `{"savedKeywordIds":["`+saved[0].(string)+`"],"addTags":["Extra"],"removeTagIds":["`+tagID+`"]}`)
	if status != 200 || body["success"] != true || body["taggedCount"] != float64(1) || body["removedAssignments"] != float64(1) ||
		len(body["addedTags"].([]any)) != 1 || len(body["removedTagIds"].([]any)) != 1 {
		t.Errorf("assign = %d %v", status, body)
	}
	status, body = postSaved(t, srv, p, "tags/update", `{"tagId":"`+tagID+`","color":"sky"}`)
	if tag, _ := body["tag"].(map[string]any); status != 200 || body["success"] != true || tag["color"] != "sky" {
		t.Errorf("recolor = %d %v", status, body)
	}
	if status, body = postSaved(t, srv, p, "tags/update", `{"tagId":"`+tagID+`","color":null}`); body["tag"].(map[string]any)["color"] != nil {
		t.Errorf("null color = %d %v, want the color cleared", status, body)
	}
	if status, body = postSaved(t, srv, p, "tags/update", `{"tagId":"missing","name":"x"}`); status != 200 || body["success"] != false {
		t.Errorf("update of a missing tag = %d %v, want success false", status, body)
	}
	// "Brand" still labels the second keyword, so deleting it is refused with a count.
	status, body = postSaved(t, srv, p, "tags/delete", `{"tagId":"`+tagID+`"}`)
	if status != 409 || errCode(body) != "tag_in_use" || body["assignmentCount"] != float64(1) {
		t.Errorf("delete of a used tag = %d %v, want 409 tag_in_use with the count", status, body)
	}
	if status, body = postSaved(t, srv, p, "tags/delete", `{"tagId":"missing"}`); status != 200 || body["success"] != false {
		t.Errorf("delete of a missing tag = %d %v, want success false", status, body)
	}
	status, body = postSaved(t, srv, p, "remove", `{"savedKeywordIds":["`+saved[0].(string)+`","`+saved[1].(string)+`","nope"]}`)
	if status != 200 || body["success"] != true || body["deletedCount"] != float64(2) {
		t.Errorf("remove = %d %v", status, body)
	}
	if status, body = postSaved(t, srv, p, "tags/delete", `{"tagId":"`+tagID+`"}`); status != 200 || body["success"] != true {
		t.Errorf("delete of the now unused tag = %d %v", status, body)
	}
}

func TestSavedKeywordRequestValidation(t *testing.T) {
	srv, f := newSavedServer(t)
	many := `"` + strings.Repeat(`a","`, 500) + `a"`
	tests := []struct{ name, path, body string }{
		{"unknown field", "list", `{"nope":1}`},
		{"not json", "list", `nope`},
		{"two values", "list", `{}{}`},
		{"empty body", "list", ``},
		{"no keywords", "save", `{"keywords":[]}`},
		{"empty keyword", "save", `{"keywords":["a",""]}`},
		{"too many keywords", "save", `{"keywords":[` + many + `]}`},
		{"bad language", "save", `{"keywords":["a"],"languageCode":"x"}`},
		{"unsupported language", "save", `{"keywords":["a"],"languageCode":"zz"}`},
		{"unsupported location", "save", `{"keywords":["a"],"locationCode":999999}`},
		{"bad tag mode", "save", `{"keywords":["a"],"tagMode":"merge"}`},
		{"replace without tags", "save", `{"keywords":["a"],"tagMode":"replace"}`},
		{"blank tag", "save", `{"keywords":["a"],"tags":["  "]}`},
		{"long tag", "save", `{"keywords":["a"],"tags":["` + strings.Repeat("x", 65) + `"]}`},
		{"competition above 1", "save", `{"keywords":["a"],"metrics":[{"keyword":"a","competition":1.5}]}`},
		{"difficulty above 100", "save", `{"keywords":["a"],"metrics":[{"keyword":"a","keywordDifficulty":101}]}`},
		{"negative volume", "save", `{"keywords":["a"],"metrics":[{"keyword":"a","searchVolume":-1}]}`},
		{"unknown intent", "save", `{"keywords":["a"],"metrics":[{"keyword":"a","intent":"vibes"}]}`},
		{"bad month", "save", `{"keywords":["a"],"metrics":[{"keyword":"a","monthlySearches":[{"year":2026,"month":13,"searchVolume":1}]}]}`},
		{"metric without keyword", "save", `{"keywords":["a"],"metrics":[{"searchVolume":1}]}`},
		{"page size", "list", `{"pageSize":10}`},
		{"page zero", "list", `{"page":-1}`},
		{"sort", "list", `{"sort":"bogus"}`},
		{"order", "list", `{"order":"up"}`},
		{"negative volume limit", "list", `{"minVolume":-1}`},
		{"difficulty limit", "list", `{"maxDifficulty":101}`},
		{"empty include term", "list", `{"includeTerms":[" "]}`},
		{"export has no paging", "export", `{"page":1}`},
		{"no ids to remove", "remove", `{"savedKeywordIds":[]}`},
		{"id too long", "remove", `{"savedKeywordIds":["` + strings.Repeat("x", 65) + `"]}`},
		{"assign without ids", "tags/assign", `{"savedKeywordIds":[],"addTags":["a"]}`},
		{"assign without work", "tags/assign", `{"savedKeywordIds":["x"]}`},
		{"update needs something", "tags/update", `{"tagId":"x"}`},
		{"update color", "tags/update", `{"tagId":"x","color":"purple"}`},
		{"update without id", "tags/update", `{"name":"x"}`},
		{"delete without id", "tags/delete", `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if status, body := postSaved(t, srv, f.project, tt.path, tt.body); status != 400 || errCode(body) != "invalid_request" {
				t.Errorf("%s %s = %d %v, want 400 invalid_request", tt.path, tt.body, status, body)
			}
		})
	}
	if status, body := postSaved(t, srv, f.project, "tags/update", `{"tagId":"x","name":"y"}`); status != 200 || body["success"] != false {
		t.Errorf("valid update of nothing = %d %v", status, body)
	}
}

func TestSavedKeywordTagNameCollisionOverHTTP(t *testing.T) {
	srv, f := newSavedServer(t)
	f.save(t, func(in *SaveInput) { in.Tags = []string{"One", "Two"} })
	tags := f.list(t, nil).Tags
	status, body := postSaved(t, srv, f.project, "tags/update", `{"tagId":"`+tags[1].ID+`","name":"one"}`)
	if status != 409 || errCode(body) != "tag_exists" {
		t.Errorf("rename onto another tag = %d %v, want 409 tag_exists", status, body)
	}
}

func TestSavedKeywordsWithoutServiceAreUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	pass := func(next http.Handler) http.Handler { return next }
	MountSaved(mux, SavedDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), WithSession: pass, WithProjectAccess: pass})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/projects/p/keywords/saved/list", strings.NewReader(`{}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestSavedKeywordListDefaultsAndTagModes(t *testing.T) {
	srv, f := newSavedServer(t)
	p := f.project
	keywords := make([]string, 60)
	for i := range keywords {
		keywords[i] = "keyword " + strings.Repeat("a", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	kw, _ := json.Marshal(keywords)
	if status, body := postSaved(t, srv, p, "save", `{"keywords":`+string(kw)+`,"tags":["One"]}`); status != 200 {
		t.Fatalf("save = %d %v", status, body)
	}
	_, body := postSaved(t, srv, p, "list", `{}`)
	if len(body["rows"].([]any)) != 50 || body["totalCount"] != float64(60) {
		t.Errorf("default page = %d rows of %v, want 50 of 60", len(body["rows"].([]any)), body["totalCount"])
	}
	_, body = postSaved(t, srv, p, "list", `{"sort":"keyword","pageSize":100}`)
	rows := body["rows"].([]any)
	if first, last := rows[0].(map[string]any)["keyword"].(string), rows[len(rows)-1].(map[string]any)["keyword"].(string); first < last {
		t.Errorf("default order = %q first, %q last; want descending", first, last)
	}
	_, body = postSaved(t, srv, p, "list", `{"sort":"keyword","order":"asc","pageSize":100}`)
	rows = body["rows"].([]any)
	if first, last := rows[0].(map[string]any)["keyword"].(string), rows[len(rows)-1].(map[string]any)["keyword"].(string); first > last {
		t.Errorf("ascending order = %q first, %q last", first, last)
	}

	one := keywords[0]
	tagsOf := func() int {
		_, body := postSaved(t, srv, p, "list", `{"search":"`+one+`","pageSize":100}`)
		for _, r := range body["rows"].([]any) {
			if row := r.(map[string]any); row["keyword"] == one {
				return len(row["tags"].([]any))
			}
		}
		t.Fatalf("keyword %q not found", one)
		return 0
	}
	postSaved(t, srv, p, "save", `{"keywords":["`+one+`"],"tags":["Two"]}`)
	if got := tagsOf(); got != 2 {
		t.Errorf("a second save without tagMode left %d tags, want 2 (append is the default)", got)
	}
	postSaved(t, srv, p, "save", `{"keywords":["`+one+`"],"tags":["Three"],"tagMode":"replace"}`)
	if got := tagsOf(); got != 1 {
		t.Errorf("replace left %d tags, want 1", got)
	}
}
