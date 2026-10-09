package google

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// TokenCipher encrypts tokens using a versioned AES-256-GCM key ring.
type TokenCipher struct {
	mu        sync.RWMutex
	keys      map[string][]byte
	currentID string
}

// NewTokenCipher requires a 32-byte base64 key and a stable key identifier.
func NewTokenCipher(keyID, encodedKey string) (*TokenCipher, error) {
	if keyID == "" || strings.Contains(keyID, ":") {
		return nil, errors.New("invalid token key id")
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid token encryption key")
	}
	return &TokenCipher{keys: map[string][]byte{keyID: key}, currentID: keyID}, nil
}

// AddDecryptionKey allows old ciphertext to be read during key rotation.
func (c *TokenCipher) AddDecryptionKey(keyID, encodedKey string) error {
	if c == nil || keyID == "" || strings.Contains(keyID, ":") || keyID == c.currentID {
		return errors.New("invalid token key id")
	}
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return errors.New("invalid token encryption key")
	}
	c.mu.Lock()
	c.keys[keyID] = key
	c.mu.Unlock()
	return nil
}

// Encrypt stores a token with a random nonce and the active key identifier.
func (c *TokenCipher) Encrypt(token string) (string, error) {
	if c == nil || token == "" {
		return "", errors.New("invalid token")
	}
	c.mu.RLock()
	key := c.keys[c.currentID]
	c.mu.RUnlock()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("initialize token cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("initialize token encryption: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate token nonce: %w", err)
	}
	ciphertext := aead.Seal(nonce, nonce, []byte(token), []byte(c.currentID))
	return c.currentID + ":" + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt authenticates ciphertext and returns the original token.
func (c *TokenCipher) Decrypt(encoded string) (string, error) {
	if c == nil {
		return "", errors.New("token cipher unavailable")
	}
	keyID, payload, ok := strings.Cut(encoded, ":")
	c.mu.RLock()
	key, exists := c.keys[keyID]
	c.mu.RUnlock()
	if !ok || !exists {
		return "", errors.New("unknown token key")
	}
	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", errors.New("invalid token ciphertext")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.New("invalid token key")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.New("invalid token key")
	}
	if len(data) < aead.NonceSize() {
		return "", errors.New("invalid token ciphertext")
	}
	plaintext, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(keyID))
	if err != nil {
		return "", errors.New("invalid token ciphertext")
	}
	return string(plaintext), nil
}
