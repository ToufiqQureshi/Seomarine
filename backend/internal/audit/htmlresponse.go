package audit

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// MaxHTMLBytes is the most HTML the crawler reads from one response.
const MaxHTMLBytes = 1024 * 1024

// ReadTextUpTo reads at most maxBytes of a response body and decodes it as
// UTF-8, cancelling the read once the bound is reached. It mirrors readTextUpTo:
// the returned string is the decoded prefix of the body, never more than
// maxBytes of UTF-8 input.
func ReadTextUpTo(response *http.Response, maxBytes int) (string, error) {
	if response == nil || response.Body == nil {
		return "", nil
	}
	if maxBytes <= 0 {
		return "", nil
	}
	buffer := make([]byte, 0, 32*1024)
	chunk := make([]byte, 32*1024)
	for len(buffer) < maxBytes {
		remaining := maxBytes - len(buffer)
		readSize := min(len(chunk), remaining)
		n, err := response.Body.Read(chunk[:readSize])
		if n > 0 {
			buffer = append(buffer, chunk[:n]...)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return decodeUTF8Prefix(buffer), err
		}
	}
	if len(buffer) >= maxBytes {
		// Cancel the rest of the body so the connection can be reused cleanly.
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 0))
	}
	return decodeUTF8Prefix(buffer), nil
}

// decodeUTF8Prefix decodes bytes as UTF-8, replacing invalid sequences and
// trimming a trailing partial rune so the result is valid UTF-8.
func decodeUTF8Prefix(buffer []byte) string {
	if utf8.Valid(buffer) {
		return string(buffer)
	}
	var builder strings.Builder
	builder.Grow(len(buffer))
	for len(buffer) > 0 {
		r, size := utf8.DecodeRune(buffer)
		if r == utf8.RuneError && size == 1 {
			break
		}
		builder.WriteRune(r)
		buffer = buffer[size:]
	}
	return builder.String()
}

// TruncateToBytes returns the longest prefix of text whose UTF-8 encoding is at
// most maxBytes, mirroring truncateToBytes.
func TruncateToBytes(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	if maxBytes <= 0 {
		return ""
	}
	cut := maxBytes
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}
	return text[:cut]
}
