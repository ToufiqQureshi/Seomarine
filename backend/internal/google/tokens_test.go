package google

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTokenExchangeValidatesResponseAndDoesNotRetryCode(t *testing.T) {
	calls := 0
	status := http.StatusOK
	body := `{"access_token":"access","expires_in":3600}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.FormValue("grant_type") != "authorization_code" || r.FormValue("code") != "code" {
			t.Errorf("unexpected token exchange: %s, %v", r.Method, r.Form)
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()
	cipher, err := NewTokenCipher("v1", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	svc := TokenService{Client: server.Client(), TokenURL: server.URL, ClientID: "client", ClientSecret: "secret", Cipher: cipher}
	values := url.Values{"grant_type": {"authorization_code"}, "code": {"code"}}
	token, err := svc.exchange(context.Background(), values)
	if err != nil || token.AccessToken != "access" {
		t.Fatalf("token = %+v, %v", token, err)
	}
	status = http.StatusBadRequest
	if _, err := svc.exchange(context.Background(), values); err == nil {
		t.Fatal("rejected code accepted")
	}
	if calls != 2 {
		t.Fatalf("token endpoint calls = %d, want 2 explicit requests", calls)
	}
	body = `{"access_token":"","expires_in":3600}`
	status = http.StatusOK
	if _, err := svc.exchange(context.Background(), values); err == nil {
		t.Fatal("empty access token accepted")
	}
}
