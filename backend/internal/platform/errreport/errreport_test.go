package errreport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// recorder is a Sentry transport that keeps events instead of sending them.
type recorder struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (r *recorder) Flush(time.Duration) bool              { return true }
func (r *recorder) FlushWithContext(context.Context) bool { return true }
func (r *recorder) Configure(sentry.ClientOptions)        {}
func (r *recorder) Close()                                {}
func (r *recorder) SendEvent(event *sentry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) sent() []*sentry.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*sentry.Event(nil), r.events...)
}

const testDSN = "https://public@example.invalid/1"

func newReporter(t *testing.T) (*Reporter, *recorder) {
	t.Helper()
	rec := &recorder{}
	reporter, err := New(Options{DSN: testDSN, Environment: "test", Release: "abc123", Transport: rec})
	if err != nil {
		t.Fatal(err)
	}
	if reporter == nil {
		t.Fatal("New returned no reporter for a DSN")
	}
	return reporter, rec
}

func TestNewWithoutDSNReportsNothing(t *testing.T) {
	reporter, err := New(Options{})
	if err != nil || reporter != nil {
		t.Fatalf("New() = %v, %v; want nil, nil", reporter, err)
	}

	var out strings.Builder
	base := slog.NewTextHandler(&out, nil)
	if got := reporter.Handler(base); got != slog.Handler(base) {
		t.Error("Handler wrapped the logger although reporting is off")
	}
	reporter.Flush(time.Millisecond)
}

func TestNewRejectsABadDSN(t *testing.T) {
	if _, err := New(Options{DSN: "not a dsn"}); err == nil {
		t.Fatal("New accepted a malformed DSN")
	}
}

func TestErrorLogsAreReportedAndStillWritten(t *testing.T) {
	reporter, rec := newReporter(t)
	var out strings.Builder
	logger := slog.New(reporter.Handler(slog.NewTextHandler(&out, nil)))

	logger.Info("listening", "addr", ":8080")
	logger.Warn("cache down")
	if n := len(rec.sent()); n != 0 {
		t.Fatalf("%d events for info and warn logs, want 0", n)
	}

	logger.With("service", "billing").WithGroup("req").Error("billing cancel",
		"err", errors.New("razorpay: timeout"),
		slog.Group("org", "id", "org_1"),
		"attempt", 2,
	)

	events := rec.sent()
	if len(events) != 1 {
		t.Fatalf("%d events for one error log, want 1", len(events))
	}
	event := events[0]
	if event.Message != "billing cancel" || event.Level != sentry.LevelError {
		t.Errorf("event = %q at %q, want \"billing cancel\" at error", event.Message, event.Level)
	}
	if len(event.Fingerprint) != 1 || event.Fingerprint[0] != "billing cancel" {
		t.Errorf("fingerprint = %v, want the log message", event.Fingerprint)
	}
	if event.Environment != "test" || event.Release != "abc123" {
		t.Errorf("environment %q, release %q; want test, abc123", event.Environment, event.Release)
	}
	want := map[string]string{
		"service":     "billing",
		"req.err":     "razorpay: timeout",
		"req.org.id":  "org_1",
		"req.attempt": "2",
	}
	for key, value := range want {
		if got := event.Contexts[logContext][key]; got != value {
			t.Errorf("extra[%q] = %v, want %q", key, got, value)
		}
	}
	if len(event.Contexts[logContext]) != len(want) {
		t.Errorf("extra = %v, want exactly %v", event.Contexts[logContext], want)
	}

	for _, line := range []string{"listening", "cache down", "billing cancel"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("log output lost %q:\n%s", line, out.String())
		}
	}
}

func TestLoggersDerivedFromOneHandlerDoNotShareAttributes(t *testing.T) {
	reporter, rec := newReporter(t)
	base := slog.New(reporter.Handler(slog.NewTextHandler(io.Discard, nil))).With("a", "1")
	first := base.With("b", "2")
	second := base.With("c", "3")

	first.Error("first")
	second.Error("second")

	events := rec.sent()
	if len(events) != 2 {
		t.Fatalf("%d events, want 2", len(events))
	}
	if _, leaked := events[0].Contexts[logContext]["c"]; leaked {
		t.Errorf("first logger carries the second's attribute: %v", events[0].Contexts[logContext])
	}
	if _, leaked := events[1].Contexts[logContext]["b"]; leaked {
		t.Errorf("second logger carries the first's attribute: %v", events[1].Contexts[logContext])
	}
}
