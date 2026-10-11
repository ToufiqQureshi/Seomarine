package emailauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/scrypt"
	"golang.org/x/text/unicode/norm"
)

// Better Auth hashes passwords with scrypt at these parameters. They are part of
// the stored hash format ("<salt hex>:<key hex>"), so changing one locks every
// existing user out.
const (
	scryptN      = 16384
	scryptR      = 16
	scryptP      = 1
	scryptKeyLen = 64
	saltBytes    = 16
)

// ErrInvalidHash means a stored hash is not in "<salt>:<key>" form.
var ErrInvalidHash = errors.New("invalid password hash")

// Hasher hashes and verifies passwords. One scrypt costs about 32 MiB, so the
// number running at once is bounded; a burst of sign-ins queues instead of
// exhausting memory.
type Hasher struct{ slots chan struct{} }

// NewHasher returns a Hasher that runs at most concurrency scrypts at a time.
func NewHasher(concurrency int) *Hasher {
	return &Hasher{slots: make(chan struct{}, max(1, concurrency))}
}

func (h *Hasher) derive(ctx context.Context, password, salt string) ([]byte, error) {
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// The salt is the hex text itself, not the bytes it encodes, and the password
	// is NFKC-normalized: both match Better Auth, and both are needed for existing
	// hashes to verify.
	key, err := scrypt.Key([]byte(norm.NFKC.String(password)), []byte(salt), scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return nil, fmt.Errorf("derive password key: %w", err)
	}
	return key, nil
}

// Hash returns a new "<salt hex>:<key hex>" hash of password.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	raw := make([]byte, saltBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	salt := hex.EncodeToString(raw)
	key, err := h.derive(ctx, password, salt)
	if err != nil {
		return "", err
	}
	return salt + ":" + hex.EncodeToString(key), nil
}

// Verify reports whether password matches hash. The comparison is constant time.
func (h *Hasher) Verify(ctx context.Context, hash, password string) (bool, error) {
	salt, want, ok := strings.Cut(hash, ":")
	if !ok || salt == "" || want == "" {
		return false, ErrInvalidHash
	}
	key, err := h.derive(ctx, password, salt)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(key)), []byte(want)) == 1, nil
}
