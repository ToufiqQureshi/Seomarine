// Package errreport sends error logs to Sentry, so a failure on production
// reaches a person instead of waiting in the logs. Handler panics arrive the
// same way: httpx.Recover logs them at error level.
package errreport

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/getsentry/sentry-go"
)

// Options configure a Reporter.
type Options struct {
	// DSN is the Sentry project address. Empty turns reporting off.
	DSN string
	// Environment separates production from local runs in Sentry.
	Environment string
	// Release is the deployed commit, when known.
	Release string
	// Transport replaces the HTTP delivery. Tests set it.
	Transport sentry.Transport
}

// Reporter reports to one Sentry project. A nil Reporter reports nothing, so
// callers need no check of their own when reporting is off.
type Reporter struct {
	hub *sentry.Hub
}

// New returns a Reporter, or nil when opts.DSN is empty.
func New(opts Options) (*Reporter, error) {
	if opts.DSN == "" {
		return nil, nil
	}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:         opts.DSN,
		Environment: opts.Environment,
		Release:     opts.Release,
		Transport:   opts.Transport,
	})
	if err != nil {
		return nil, fmt.Errorf("configure error reporting: %w", err)
	}
	return &Reporter{hub: sentry.NewHub(client, sentry.NewScope())}, nil
}

// Handler returns a handler that writes every record to next and also
// reports the records at error level.
func (r *Reporter) Handler(next slog.Handler) slog.Handler {
	if r == nil {
		return next
	}
	return slog.NewMultiHandler(next, logHandler{hub: r.hub})
}

// Flush waits up to timeout for queued reports to be delivered. Call it
// before the process exits.
func (r *Reporter) Flush(timeout time.Duration) {
	if r != nil {
		r.hub.Flush(timeout)
	}
}

// logContext is the name the log attributes appear under in a Sentry event.
const logContext = "log"

// logHandler turns an error-level record into a Sentry event.
type logHandler struct {
	hub    *sentry.Hub
	attrs  []slog.Attr
	prefix string
}

func (h logHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (h logHandler) Handle(_ context.Context, record slog.Record) error {
	event := sentry.NewEvent()
	event.Level = sentry.LevelError
	event.Logger = "slog"
	event.Timestamp = record.Time
	event.Message = record.Message
	// Log messages are fixed strings, so one message is one issue however
	// the error text behind it varies.
	event.Fingerprint = []string{record.Message}
	fields := make(sentry.Context, len(h.attrs)+record.NumAttrs())
	for _, a := range h.attrs {
		addAttr(fields, "", a)
	}
	record.Attrs(func(a slog.Attr) bool {
		addAttr(fields, h.prefix, a)
		return true
	})
	event.Contexts[logContext] = fields
	h.hub.CaptureEvent(event)
	return nil
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := h
	// Clipped, so two loggers derived from this one never share a backing array.
	next.attrs = slices.Clip(h.attrs)
	for _, a := range attrs {
		a.Key = h.prefix + a.Key
		next.attrs = append(next.attrs, a)
	}
	return next
}

func (h logHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := h
	next.prefix = h.prefix + name + "."
	return next
}

// addAttr writes a under prefix into fields as text, flattening groups.
func addAttr(fields sentry.Context, prefix string, a slog.Attr) {
	value := a.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		if a.Key != "" {
			prefix += a.Key + "."
		}
		for _, member := range value.Group() {
			addAttr(fields, prefix, member)
		}
		return
	}
	if a.Key == "" {
		return
	}
	fields[prefix+a.Key] = value.String()
}
