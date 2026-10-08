// Package httpx provides shared HTTP request and response handling.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
)

// ErrPayloadTooLarge means a JSON request exceeded its configured size limit.
var ErrPayloadTooLarge = errors.New("request body exceeds configured limit")

// ErrInvalidPage means the page query value is outside the supported range.
var ErrInvalidPage = errors.New("page must be between 1 and 1000")

// ErrInvalidLimit means the limit query value is outside the supported range.
var ErrInvalidLimit = errors.New("limit must be between 1 and 100")

// userKey is the request-scoped context key for the signed-in user.
type userKey struct{}

// Error is the common API error envelope body.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Envelope is the common JSON error shape used throughout the API.
type Envelope struct {
	Error Error `json:"error"`
}

// WithUser attaches the authenticated user to the request context.
func WithUser(ctx context.Context, user auth.User) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

// UserFromContext returns the user stored on the request context.
func UserFromContext(ctx context.Context) (auth.User, bool) {
	user, ok := ctx.Value(userKey{}).(auth.User)
	return user, ok
}

// RequestLogging wraps the next handler with request-scoped logging metadata.
func RequestLogging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := ""
			if values := r.Header.Values("X-Request-Id"); len(values) == 1 && validRequestID(values[0]) {
				requestID = values[0]
			} else {
				requestID = newRequestID()
			}
			w.Header().Set("X-Request-Id", requestID)
			log := logger
			if logger != nil {
				log = logger.With("request_id", requestID, "method", r.Method, "path", r.URL.Path)
			}
			start := time.Now()
			next.ServeHTTP(w, r)
			if log != nil {
				log.InfoContext(r.Context(), "request complete", "duration_ms", time.Since(start).Milliseconds())
			}
		})
	}
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, c := range value {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.'
		if !valid {
			return false
		}
	}
	return true
}

// ParsePageLimit applies defaults and rejects invalid explicit pagination values.
func ParsePageLimit(values url.Values) (page int, limit int, err error) {
	page = 1
	limit = 10

	if raw, ok := values["page"]; ok {
		if len(raw) != 1 {
			return 0, 0, ErrInvalidPage
		}
		page, err = strconv.Atoi(strings.TrimSpace(raw[0]))
		if err != nil || page < 1 || page > 1000 {
			return 0, 0, ErrInvalidPage
		}
	}
	if raw, ok := values["limit"]; ok {
		if len(raw) != 1 {
			return 0, 0, ErrInvalidLimit
		}
		limit, err = strconv.Atoi(strings.TrimSpace(raw[0]))
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, ErrInvalidLimit
		}
	}
	return page, limit, nil
}

// DecodeJSONLimit reads a JSON request body and enforces a maximum payload size.
func DecodeJSONLimit(r io.Reader, maxBytes int64, v any) error {
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ErrPayloadTooLarge
	}
	if len(body) == 0 {
		return io.EOF
	}
	if err := json.Unmarshal(body, v); err != nil {
		return err
	}
	return nil
}

// WriteJSON writes a JSON response body and sets the correct content type.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Default().Error("write json response", "err", err)
	}
}

// WriteError writes a standard API error payload.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, Envelope{Error: Error{Code: code, Message: message}})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "request" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b[:])
}
