package reports

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSharedRawServesSandboxedHTMLOnlyForHostedActiveProject(t *testing.T) {
	token := strings.Repeat("a", 32)
	store := newMemoryStore()
	store.items["report-1"] = Metadata{ID: "report-1", ProjectID: "project-1", ShareToken: &token}
	store.html["report-1"] = "<!doctype html><html><body>report</body></html>"
	d := Deps{Service: &Service{Store: store, Hosted: true}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /s/{token}/raw", d.sharedRaw)
	req := httptest.NewRequest(http.MethodGet, "/s/"+token+"/raw", nil)
	req.Header.Set("Sec-Fetch-Dest", "iframe")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Body.String() != store.html["report-1"] {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	for key, want := range map[string]string{
		"Content-Type":               "text/html; charset=utf-8",
		"Content-Security-Policy":    "sandbox allow-popups allow-popups-to-escape-sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'; object-src 'none'; frame-ancestors 'self'",
		"Cross-Origin-Opener-Policy": "same-origin", "Referrer-Policy": "no-referrer",
		"X-Content-Type-Options": "nosniff", "X-Robots-Tag": "noindex, nofollow",
		"Cache-Control": "public, max-age=0, s-maxage=60",
	} {
		if got := response.Header().Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestSharedRawRedirectsTopLevelAndStripsQueryBeforeLoading(t *testing.T) {
	d := Deps{Service: &Service{Store: newMemoryStore(), Hosted: true}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /s/{token}/raw", d.sharedRaw)
	for _, test := range []struct{ name, target, fetchDest, location string }{
		{"top level", "/s/" + strings.Repeat("a", 32) + "/raw", "document", "/s/" + strings.Repeat("a", 32)},
		{"query", "/s/" + strings.Repeat("a", 32) + "/raw?x=1", "iframe", "/s/" + strings.Repeat("a", 32) + "/raw"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.target, nil)
			req.Header.Set("Sec-Fetch-Dest", test.fetchDest)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, req)
			if response.Code != http.StatusFound || response.Header().Get("Location") != test.location || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("redirect response = %d %v", response.Code, response.Header())
			}
		})
	}
}

func TestSharedRawHidesInvalidRevokedArchivedAndNonHostedReports(t *testing.T) {
	token := strings.Repeat("b", 32)
	store := newMemoryStore()
	store.items["report-1"] = Metadata{ID: "report-1", ProjectID: "project-1", ShareToken: &token}
	for _, test := range []struct {
		name             string
		hosted, archived bool
		requestToken     string
	}{
		{"non-hosted", false, false, token}, {"archived project", true, true, token}, {"invalid token", true, false, "bad"}, {"revoked", true, false, strings.Repeat("c", 32)},
	} {
		t.Run(test.name, func(t *testing.T) {
			store.archived = test.archived
			d := Deps{Service: &Service{Store: store, Hosted: test.hosted}}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /s/{token}/raw", d.sharedRaw)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/s/"+test.requestToken+"/raw", nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", response.Code)
			}
		})
	}
}
