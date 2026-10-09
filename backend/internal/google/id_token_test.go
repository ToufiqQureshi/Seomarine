package google

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIDTokenVerifier(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	keyCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		keyCalls++
		e := big.NewInt(int64(privateKey.E)).Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "test-key", "kty": "RSA", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(e),
		}}})
	}))
	defer server.Close()
	verifier := &IDTokenVerifier{KeysURL: server.URL, Client: server.Client(), Now: func() time.Time { return now }}
	makeToken := func(audience string, expiry int64) string {
		header, _ := json.Marshal(map[string]any{"alg": "RS256", "kid": "test-key"})
		claims, _ := json.Marshal(map[string]any{"iss": "https://accounts.google.com", "aud": audience, "sub": "google-user", "exp": expiry, "iat": now.Add(-time.Minute).Unix()})
		body := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
		hash := sha256.Sum256([]byte(body))
		signature, signErr := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
		if signErr != nil {
			t.Fatal(signErr)
		}
		return body + "." + base64.RawURLEncoding.EncodeToString(signature)
	}
	valid := makeToken("google-client", now.Add(time.Hour).Unix())
	accountID, err := verifier.AccountID(context.Background(), valid, "google-client")
	if err != nil || accountID != "google-user" {
		t.Fatalf("account = %q, %v", accountID, err)
	}
	if _, err := verifier.AccountID(context.Background(), valid, "other-client"); err == nil {
		t.Fatal("wrong audience accepted")
	}
	if _, err := verifier.AccountID(context.Background(), makeToken("google-client", now.Add(-time.Minute).Unix()), "google-client"); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err := verifier.AccountID(context.Background(), valid+"a", "google-client"); err == nil {
		t.Fatal("tampered signature accepted")
	}
	if keyCalls != 1 {
		t.Fatalf("JWK requests = %d, want cached 1", keyCalls)
	}
}
