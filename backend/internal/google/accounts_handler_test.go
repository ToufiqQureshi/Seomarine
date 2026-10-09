package google

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountHandlersRejectInvalidInputBeforeDatabase(t *testing.T) {
	mux := http.NewServeMux()
	MountAccounts(mux, AccountDeps{
		Logger:      slog.Default(),
		WithSession: func(next http.Handler) http.Handler { return next },
	})
	for _, tc := range []struct {
		path string
		body string
	}{
		{"impact", `{"provider":"other","accountId":"id"}`},
		{"impact", `{"provider":"gsc","accountId":""}`},
		{"impact", `{"provider":"gsc","accountId":"id","extra":1}`},
		{"impact", `{"provider":"gsc","accountId":"id"}{}`},
		{"impact", `{"provider":"gsc","accountId":"id","confirmed":true}`},
		{"remove", `{"provider":"ga4","accountId":"id"}`},
		{"remove", `{"provider":"ga4","accountId":"id","confirmed":false}`},
		{"remove", `{"provider":"ga4","accountId":"id","confirmed":true,"extra":1}`},
		{"remove", `{"provider":"ga4","accountId":"` + strings.Repeat("😀", 129) + `","confirmed":true}`},
		{"remove", `{"provider":"ga4","accountId":"` + strings.Repeat("x", 4096) + `","confirmed":true}`},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/google/accounts/"+tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %q: status %d, want 400", tc.path, tc.body, rec.Code)
		}
	}
}
