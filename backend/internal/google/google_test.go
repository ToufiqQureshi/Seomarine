package google

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSafeCallbackPath(t *testing.T) {
	origin := "https://app.example"
	for _, tc := range []struct{ input, want string }{
		{"https://evil.example/p", "/"},
		{"//evil.example/p", "/"},
		{"https://app.example//evil.example/p", "/"},
		{"/p/1?x=1#h", "/p/1?x=1#h"},
		{"https://app.example/p", "/p"},
	} {
		if got := SafeCallbackPath(tc.input, origin); got != tc.want {
			t.Errorf("SafeCallbackPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPKCE(t *testing.T) {
	verifier, err := CodeVerifier("state", "gsc", "secret")
	if err != nil || verifier == "" {
		t.Fatalf("verifier: %q, %v", verifier, err)
	}
	other, _ := CodeVerifier("state", "ga4", "secret")
	if other == verifier || CodeChallenge(verifier) == "" {
		t.Fatal("pkce binding failed")
	}
}

func TestTokenCipher(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	c, err := NewTokenCipher("v1", key)
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.Encrypt("sensitive-token")
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Encrypt("sensitive-token")
	if err != nil || first == second || strings.Contains(first, "sensitive-token") {
		t.Fatal("encryption must use a fresh nonce and hide plaintext")
	}
	got, err := c.Decrypt(first)
	if err != nil || got != "sensitive-token" {
		t.Fatalf("decrypt: %q, %v", got, err)
	}
	if _, err := c.Decrypt(first + "a"); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}
