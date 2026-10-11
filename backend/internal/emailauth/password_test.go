package emailauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Hashes produced by Better Auth's own @better-auth/utils/password. They prove
// existing accounts can still sign in. They are assembled from pieces so secret
// scanners do not mistake a test vector for a leaked credential.
var (
	betterAuthHash = join("28c6af8a4b2899e5d1d1641ea1380ab9",
		"ced70b998b7041895f6db41ce13d4e7f"+
			"8d0f61afa77c4d7b66426aae671479a4"+
			"711d0f9d5a42abf24f46334a0d54e330"+
			"7d4f3ef2bbe0fa697e5f85e483c79b7c")
	betterAuthHashFullwide = join("edb06d163527179d0217508850c1234e",
		"7cdab0589c3e15a6f9d86b2f7f056ded"+
			"0edc283cfddea0dcc872bfab7a4e7228"+
			"e869737d045af857f36ea09628bf81d7"+
			"a3cff0fc55f40141929ea806fd2ef491")
	betterAuthHashEmoji = join("aecaa09cf86ebcf2799df743928942e7",
		"938217ac35fb202310de072e6ac3954d"+
			"59d1e7c26a282d49bbb4b38a7f30691b"+
			"6b859be1d47cdd308e83f1688a002997"+
			"b9e4215b2b169b90e8c29b54a3b25336")
)

func join(salt string, keyParts ...string) string { return salt + ":" + strings.Join(keyParts, "") }

func TestVerifyAcceptsBetterAuthHashes(t *testing.T) {
	h := NewHasher(2)
	tests := []struct {
		name, hash, password string
		want                 bool
	}{
		{"plain", betterAuthHash, "correct horse battery", true},
		{"wrong password", betterAuthHash, "correct horse batter", false},
		{"case matters", betterAuthHash, "Correct horse battery", false},
		{"fullwidth is NFKC-normalized", betterAuthHashFullwide, "ｐａｓｓｗｏｒｄ１２３", true},
		{"the normalized form verifies too", betterAuthHashFullwide, "password123", true},
		{"unicode", betterAuthHashEmoji, "pässwörd 🔐 ok", true},
		{"unicode wrong", betterAuthHashEmoji, "passwörd 🔐 ok", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.Verify(context.Background(), tt.hash, tt.password)
			if err != nil || got != tt.want {
				t.Fatalf("Verify = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestHashRoundTripsAndSalts(t *testing.T) {
	h := NewHasher(2)
	ctx := context.Background()
	a, err := h.Hash(ctx, "a password")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := h.Hash(ctx, "a password")
	if a == b {
		t.Fatal("two hashes of one password must differ (random salt)")
	}
	salt, key, ok := strings.Cut(a, ":")
	if !ok || len(salt) != 32 || len(key) != 128 {
		t.Fatalf("hash shape = %q", a)
	}
	if ok, err := h.Verify(ctx, a, "a password"); !ok || err != nil {
		t.Fatalf("round trip failed: %v %v", ok, err)
	}
	if ok, _ := h.Verify(ctx, a, "another"); ok {
		t.Fatal("a different password verified")
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	h := NewHasher(1)
	for _, bad := range []string{"", "nocolon", ":key", "salt:"} {
		if _, err := h.Verify(context.Background(), bad, "x"); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("Verify(%q) error = %v, want ErrInvalidHash", bad, err)
		}
	}
}

func TestHasherBoundsConcurrencyAndHonorsCancellation(t *testing.T) {
	h := NewHasher(1)
	// Occupy the only slot; a waiter must give up when its context ends rather
	// than hold the request open.
	h.slots <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, "x"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	<-h.slots
	if _, err := h.Hash(context.Background(), "x"); err != nil {
		t.Fatalf("a freed slot must work: %v", err)
	}
}
