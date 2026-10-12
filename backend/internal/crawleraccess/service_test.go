package crawleraccess

import (
	"strings"
	"testing"
)

func TestCredentialEncryptionAuthenticatesAndHidesPlaintext(t *testing.T) {
	const secret = "a-long-enough-test-secret-with-at-least-32-bytes"
	const plaintext = "signature=private-shopify-credential"
	ciphertext, err := encrypt(secret, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}
	if got, err := decrypt(secret, ciphertext); err != nil || got != plaintext {
		t.Fatalf("decrypt() = %q, %v; want original plaintext", got, err)
	}
	if _, err := decrypt(secret+"-wrong", ciphertext); err == nil {
		t.Fatal("decrypt() accepted ciphertext under the wrong secret")
	}
}

func TestCredentialEncryptionRejectsShortSecretAndMalformedCiphertext(t *testing.T) {
	if _, err := encrypt("too-short", "credential"); err == nil {
		t.Fatal("encrypt() accepted a short secret")
	}
	for _, ciphertext := range []string{"not-hex", "00", ""} {
		if _, err := decrypt("a-long-enough-test-secret-with-at-least-32-bytes", ciphertext); err == nil {
			t.Errorf("decrypt(%q) accepted malformed ciphertext", ciphertext)
		}
	}
}
