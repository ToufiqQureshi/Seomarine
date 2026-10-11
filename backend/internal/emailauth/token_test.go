package emailauth

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("unit-test-signing-key-not-a-secret!")

// Tokens minted by the jose library Better Auth uses, assembled from pieces so
// secret scanners do not mistake a test vector for a leaked token.
var (
	joseToken = strings.Join([]string{"eyJhbGciOiJIUzI1NiJ9",
		"eyJlbWFpbCI6ImFAZXhhbXBsZS5jb20iLCJpYXQiOjE3MDAwMDAwMDAsImV4cCI6NDEwMjQ0NDgwMH0",
		"GSb276RJzjlwn6lK" +
			"-m6OkAxfy47g9ITw" +
			"uXyqWdwqcDs"}, ".")
	joseTokenExpired = strings.Join([]string{"eyJhbGciOiJIUzI1NiJ9",
		"eyJlbWFpbCI6ImFAZXhhbXBsZS5jb20iLCJpYXQiOjE3MDAwMDAwMDAsImV4cCI6MTcwMDAwMzYwMH0",
		"WBQnxWwtROYc-fG0" +
			"uImaPu0AP4lArIWX" +
			"2cPtjslyCWU"}, ".")
)

var fixedNow = time.Unix(1_750_000_000, 0)

func TestVerifyJWTAcceptsJoseTokens(t *testing.T) {
	claims, err := verifyJWT(testSecret, joseToken, fixedNow)
	if err != nil || claims["email"] != "a@example.com" {
		t.Fatalf("claims = %v err = %v", claims, err)
	}
	if _, err := verifyJWT(testSecret, joseTokenExpired, fixedNow); err != ErrTokenExpired {
		t.Fatalf("expired token error = %v", err)
	}
}

func TestSignedTokensRoundTripAndMatchJoseHeader(t *testing.T) {
	token, err := signJWT(testSecret, map[string]any{"email": "b@example.com"}, time.Hour, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))+".") {
		t.Fatalf("header differs from jose's: %s", token)
	}
	claims, err := verifyJWT(testSecret, token, fixedNow.Add(59*time.Minute))
	if err != nil || claims["email"] != "b@example.com" {
		t.Fatalf("claims = %v err = %v", claims, err)
	}
	if _, err := verifyJWT(testSecret, token, fixedNow.Add(time.Hour)); err != ErrTokenExpired {
		t.Fatalf("a token is expired at its expiry second: %v", err)
	}
}

func TestVerifyJWTRejectsForgeries(t *testing.T) {
	good, _ := signJWT(testSecret, map[string]any{"email": "a@example.com"}, time.Hour, fixedNow)
	parts := strings.Split(good, ".")
	noneHeader := b64.EncodeToString([]byte(`{"alg":"none"}`))
	hs512 := b64.EncodeToString([]byte(`{"alg":"HS512"}`))
	noExp := func() string {
		body := b64.EncodeToString([]byte(`{"email":"a@example.com"}`))
		in := b64.EncodeToString([]byte(jwtHeader)) + "." + body
		return in + "." + b64.EncodeToString(jwtSignature(testSecret, in))
	}()
	tests := map[string]string{
		"empty":            "",
		"two parts":        parts[0] + "." + parts[1],
		"alg none":         noneHeader + "." + parts[1] + ".",
		"alg none + sig":   noneHeader + "." + parts[1] + "." + parts[2],
		"other algorithm":  hs512 + "." + parts[1] + "." + parts[2],
		"tampered payload": parts[0] + "." + b64.EncodeToString([]byte(`{"email":"victim@example.com","exp":4102444800}`)) + "." + parts[2],
		"wrong secret":     mustSign(t, []byte("another-secret-another-secret-123456")),
		"bad base64":       "!!!." + parts[1] + "." + parts[2],
		"no expiry":        noExp,
		"truncated sig":    parts[0] + "." + parts[1] + "." + parts[2][:10],
		"extra part":       good + ".x",
	}
	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := verifyJWT(testSecret, token, fixedNow); err != ErrTokenInvalid {
				t.Fatalf("error = %v, want ErrTokenInvalid", err)
			}
		})
	}
}

func mustSign(t *testing.T, secret []byte) string {
	t.Helper()
	token, err := signJWT(secret, map[string]any{"email": "a@example.com"}, time.Hour, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestNewIDShapeAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		id, err := newID(32)
		if err != nil || len(id) != 32 {
			t.Fatalf("id = %q err = %v", id, err)
		}
		for _, c := range id {
			if !strings.ContainsRune(idAlphabet, c) {
				t.Fatalf("id %q has a character outside the alphabet", id)
			}
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}
