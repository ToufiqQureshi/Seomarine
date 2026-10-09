package google

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

// LegacyTokenCipher reads Better Auth's XChaCha20-Poly1305 encrypted grants.
// New grants are always written with TokenCipher and AES-GCM.
type LegacyTokenCipher struct {
	secret string
}

// NewLegacyTokenCipher uses the legacy BETTER_AUTH_SECRET to read old grants.
func NewLegacyTokenCipher(secret string) (*LegacyTokenCipher, error) {
	if len(secret) < 32 {
		return nil, errors.New("invalid legacy token secret")
	}
	return &LegacyTokenCipher{secret: secret}, nil
}

// Decrypt reads a bare hexadecimal legacy token; envelope tokens require a
// separately configured versioned Better Auth secret and are rejected.
func (c *LegacyTokenCipher) Decrypt(encoded string) (string, error) {
	if c == nil || encoded == "" || strings.HasPrefix(encoded, "$ba$") {
		return "", errors.New("legacy token unavailable")
	}
	data, err := hex.DecodeString(encoded)
	if err != nil || len(data) < chacha20poly1305.NonceSizeX+chacha20poly1305.Overhead {
		return "", errors.New("invalid legacy token ciphertext")
	}
	key := sha256.Sum256([]byte(c.secret))
	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return "", errors.New("invalid legacy token key")
	}
	plaintext, err := aead.Open(nil, data[:chacha20poly1305.NonceSizeX], data[chacha20poly1305.NonceSizeX:], nil)
	if err != nil {
		return "", errors.New("invalid legacy token ciphertext")
	}
	return string(plaintext), nil
}
