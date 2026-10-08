// Package httpx provides shared HTTP request and response handling.
package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// ErrPayloadTooLarge means a JSON request exceeded its configured size limit.
var ErrPayloadTooLarge = errors.New("request body exceeds configured limit")

// ErrInvalidPage means the page query value is outside the supported range.
var ErrInvalidPage = errors.New("page must be between 1 and 1000")

// ErrInvalidLimit means the limit query value is outside the supported range.
var ErrInvalidLimit = errors.New("limit must be between 1 and 100")

// Error is the common API error envelope body.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Envelope is the common JSON error shape used throughout the API.
type Envelope struct {
	Error Error `json:"error"`
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
			response := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(response, r)
			if log != nil {
				log.InfoContext(r.Context(), "request complete", "status", response.statusCode(), "duration_ms", time.Since(start).Milliseconds())
			}
		})
	}
}

// Recover turns panics from a handler into a generic API error when the
// response has not started, and logs the panic with its stack for diagnosis.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := &statusRecorder{ResponseWriter: w}
			defer func() {
				if recovered := recover(); recovered != nil {
					if logger != nil {
						logger.ErrorContext(r.Context(), "panic serving request", "panic_type", fmt.Sprintf("%T", recovered), "stack", string(debug.Stack()))
					}
					if !response.wroteHeader {
						WriteError(response, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
					}
				}
			}()
			next.ServeHTTP(response, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.ResponseWriter.WriteHeader(status)
	if status < 100 || status >= 200 || status == http.StatusSwitchingProtocols {
		w.status = status
		w.wroteHeader = true
	}
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusRecorder) statusCode() int {
	if !w.wroteHeader {
		return http.StatusOK
	}
	return w.status
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
