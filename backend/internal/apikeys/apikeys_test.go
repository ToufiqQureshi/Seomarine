package apikeys

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestHashKeyMatchesStoredCredentialFormat(t *testing.T) {
	const key = "oseo-secret-key"
	sum := sha256.Sum256([]byte(key))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if got := HashKey(key); got != want {
		t.Fatalf("HashKey() = %q, want %q", got, want)
	}
	if got := HashKey(key); got == key {
		t.Fatal("HashKey() returned the plaintext credential")
	}
}

func TestNewKeyUsesCredentialContractAndUniqueSecrets(t *testing.T) {
	first, err := newKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{first, second} {
		if !strings.HasPrefix(key, Prefix) || len(key) != len(Prefix)+keyLength {
			t.Errorf("generated key shape = %q (length %d)", key[:min(len(key), len(Prefix))], len(key))
		}
		for _, char := range key[len(Prefix):] {
			if !strings.ContainsRune(keyAlphabet, char) {
				t.Errorf("generated key contains unsupported character %q", char)
				break
			}
		}
	}
	if first == second {
		t.Fatal("two generated API keys were identical")
	}
}
