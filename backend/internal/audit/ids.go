package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SHA256Hex returns the hex-encoded SHA-256 digest of text.
func SHA256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// DeterministicAuditRowID derives a stable 36-character id from its parts.
// Crawl and Lighthouse persistence happens inside retryable workflow steps, so
// deriving ids from stable content (audit id + URL + ...) combined with an
// insert-or-update makes those writes idempotent across retries: a partially
// written batch is completed on the next attempt instead of duplicated under
// fresh random ids. Mirrors deterministicAuditRowId (first 36 hex chars).
func DeterministicAuditRowID(parts ...string) string {
	digest := SHA256Hex(strings.Join(parts, "|"))
	return digest[:36]
}
