package billing

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/razorpay"
)

const (
	testKeyID         = "rzp_test_key"
	testWebhookSecret = "webhook-secret"
	testPlanID        = "plan_pro"
)

var (
	t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	t1 = t0.Add(time.Minute)
	t2 = t0.Add(2 * time.Minute)
)

func TestCheckoutCreatesASubscription(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)

	checkout, err := f.svc.Checkout(ctx, f.org)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}
	if checkout != (Checkout{SubscriptionID: "sub_1", KeyID: testKeyID}) {
		t.Fatalf("Checkout() = %+v", checkout)
	}
	if got := f.lastNotes.Load(); got == nil || (*got)["organization_id"] != f.org {
		t.Fatalf("subscription notes = %v, want the organization id", got)
	}
	f.assertStatus(ctx, t, Status{Plan: PlanFree, Status: "created"})

	// An unpaid subscription is replaced by a fresh one, never reused.
	checkout, err = f.svc.Checkout(ctx, f.org)
	if err != nil || checkout.SubscriptionID != "sub_2" {
		t.Fatalf("second Checkout() = %+v, %v; want the new sub_2", checkout, err)
	}
	f.assertStatus(ctx, t, Status{Plan: PlanFree, Status: "created"})
	if got := f.storedSubscriptionID(ctx, t); got != "sub_2" {
		t.Fatalf("stored subscription = %q, want sub_2", got)
	}
}

func TestCheckoutRefusesALiveSubscription(t *testing.T) {
	ctx := context.Background()
	for _, status := range liveStatuses {
		t.Run(status, func(t *testing.T) {
			f := newFixture(ctx, t)
			f.deliver(ctx, t, f.event("subscription.charged", "sub_paid", status, t1))
			calls := f.calls.Load()

			if _, err := f.svc.Checkout(ctx, f.org); !errors.Is(err, ErrAlreadySubscribed) {
				t.Fatalf("Checkout() error = %v, want ErrAlreadySubscribed", err)
			}
			if f.calls.Load() != calls {
				t.Fatal("Checkout() called Razorpay for an organization that already pays")
			}
		})
	}
}

// After a subscription ends, the organization can subscribe again.
func TestCheckoutAfterTheSubscriptionEnded(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.deliver(ctx, t, f.event("subscription.cancelled", "sub_old", "cancelled", t1))

	checkout, err := f.svc.Checkout(ctx, f.org)
	if err != nil || checkout.SubscriptionID != "sub_1" {
		t.Fatalf("Checkout() = %+v, %v", checkout, err)
	}
	// A late event for the old subscription does not displace the new one.
	f.deliver(ctx, t, f.event("subscription.completed", "sub_old", "completed", t0.Add(30*time.Second)))
	if got := f.storedSubscriptionID(ctx, t); got != "sub_1" {
		t.Fatalf("stored subscription = %q, want sub_1", got)
	}
}

// A subscription that becomes live while checkout talks to Razorpay wins;
// the new one is left unpaid.
func TestCheckoutLosesARaceToAWebhook(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.duringCreate = func() {
		f.deliver(ctx, t, f.event("subscription.activated", "sub_other_tab", "active", t1))
	}

	if _, err := f.svc.Checkout(ctx, f.org); !errors.Is(err, ErrAlreadySubscribed) {
		t.Fatalf("Checkout() error = %v, want ErrAlreadySubscribed", err)
	}
	if got := f.storedSubscriptionID(ctx, t); got != "sub_other_tab" {
		t.Fatalf("stored subscription = %q, want the live sub_other_tab", got)
	}
}

func TestCheckoutReportsRazorpayFailures(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.failWith = http.StatusBadRequest

	_, err := f.svc.Checkout(ctx, f.org)
	var apiErr *razorpay.APIError
	if !errors.Is(err, ErrPaymentProvider) || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("Checkout() error = %v, want ErrPaymentProvider wrapping the 400", err)
	}
	f.assertStatus(ctx, t, Status{Plan: PlanFree, Status: "none"})
}

func TestHandleWebhookSignature(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	body := f.event("subscription.activated", "sub_1", "active", t1)
	tests := []struct {
		name      string
		signature string
		wantErr   error
	}{
		{name: "valid", signature: sign(body)},
		{name: "missing", signature: "", wantErr: ErrInvalidSignature},
		{name: "invalid", signature: sign(append(body, ' ')), wantErr: ErrInvalidSignature},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.svc.HandleWebhook(ctx, body, tt.signature, "evt_"+rand.Text())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("HandleWebhook() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandleWebhookRejectsAForgedEventBeforeReadingIt(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	if _, err := f.svc.HandleWebhook(ctx, f.event("subscription.activated", "sub_1", "active", t1), sign([]byte("other")), "evt_1"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("HandleWebhook() error = %v, want ErrInvalidSignature", err)
	}
	f.assertStatus(ctx, t, Status{Plan: PlanFree, Status: "none"})
}

func TestHandleWebhookAppliesSubscriptionState(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	periodEnd := t0.Add(30 * 24 * time.Hour)

	f.deliver(ctx, t, f.eventWithEnd("subscription.activated", "sub_1", "active", t1, &periodEnd))
	f.assertStatus(ctx, t, Status{Plan: PlanPro, Status: "active", CurrentPeriodEnd: &periodEnd})

	f.deliver(ctx, t, f.eventWithEnd("subscription.halted", "sub_1", "halted", t2, &periodEnd))
	f.assertStatus(ctx, t, Status{Plan: PlanFree, Status: "halted", CurrentPeriodEnd: &periodEnd})
}

func TestHandleWebhookIsIdempotent(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	activated := f.event("subscription.activated", "sub_1", "active", t1)
	cancelled := f.event("subscription.cancelled", "sub_1", "cancelled", t2)

	if got := f.deliverAs(ctx, t, "evt_activated-"+f.org, activated); got != WebhookProcessed {
		t.Fatalf("first delivery = %q, want processed", got)
	}
	if got := f.deliverAs(ctx, t, "evt_cancelled-"+f.org, cancelled); got != WebhookProcessed {
		t.Fatalf("cancel = %q, want processed", got)
	}
	// A redelivered event changes nothing, even with a newer timestamp.
	replay := f.event("subscription.activated", "sub_1", "active", t2.Add(time.Hour))
	if got := f.deliverAs(ctx, t, "evt_activated-"+f.org, replay); got != WebhookDuplicate {
		t.Fatalf("replay = %q, want duplicate", got)
	}
	f.assertStatus(ctx, t, Status{Plan: PlanFree, Status: "cancelled"})
}

func TestHandleWebhookConcurrentRedeliveriesApplyOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	body := f.event("subscription.charged", "sub_1", "active", t1)

	results := make(chan WebhookResult, 8)
	for range cap(results) {
		go func() {
			result, err := f.svc.HandleWebhook(ctx, body, sign(body), "evt_concurrent-"+f.org)
			if err != nil {
				t.Errorf("HandleWebhook() error = %v", err)
			}
			results <- result
		}()
	}
	processed := 0
	for range cap(results) {
		if <-results == WebhookProcessed {
			processed++
		}
	}
	if processed != 1 {
		t.Fatalf("%d deliveries processed, want exactly 1", processed)
	}
}

func TestHandleWebhookIgnoresStaleEvents(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.deliver(ctx, t, f.event("subscription.cancelled", "sub_1", "cancelled", t2))
	if got := f.deliverAs(ctx, t, "evt_late-"+f.org, f.event("subscription.charged", "sub_1", "active", t1)); got != WebhookProcessed {
		t.Fatalf("late delivery = %q, want processed (and ignored)", got)
	}
	f.assertStatus(ctx, t, Status{Plan: PlanFree, Status: "cancelled"})
}

// Another subscription of the organization may not displace a live one.
func TestHandleWebhookKeepsTheLiveSubscription(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	f.deliver(ctx, t, f.event("subscription.activated", "sub_live", "active", t1))
	f.deliver(ctx, t, f.event("subscription.cancelled", "sub_other", "cancelled", t2))
	if got := f.storedSubscriptionID(ctx, t); got != "sub_live" {
		t.Fatalf("stored subscription = %q, want sub_live", got)
	}
	f.assertStatus(ctx, t, Status{Plan: PlanPro, Status: "active"})
}

func TestHandleWebhookIgnoresEventsItDoesNotHandle(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "unknown event", body: f.event("subscription.paused", "sub_1", "paused", t1)},
		{name: "payment event", body: []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_1"}}},"created_at":1}`)},
		{name: "subscription without notes", body: fmt.Appendf(nil,
			`{"event":"subscription.activated","payload":{"subscription":{"entity":{"id":"sub_1","status":"active","notes":[]}}},"created_at":%d}`, t1.Unix())},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := f.deliverAs(ctx, t, "evt_"+rand.Text(), tt.body); got != WebhookIgnored {
				t.Fatalf("HandleWebhook() = %q, want ignored", got)
			}
			f.assertStatus(ctx, t, Status{Plan: PlanFree, Status: "none"})
		})
	}
}

// An organization deleted after subscribing gets no row and the delivery
// succeeds, so Razorpay does not retry it forever.
func TestHandleWebhookForADeletedOrganization(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	gone := "org-deleted-" + rand.Text()
	body := eventFor(gone, "subscription.activated", "sub_1", "active", t1, nil)
	if got := f.deliverAs(ctx, t, "evt_"+rand.Text(), body); got != WebhookProcessed {
		t.Fatalf("HandleWebhook() = %q, want processed", got)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM go_billing_subscriptions WHERE organization_id = $1`, gone).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows for the deleted organization = %d, %v", n, err)
	}
}

func TestHandleWebhookRejectsMalformedEvents(t *testing.T) {
	ctx := context.Background()
	f := newFixture(ctx, t)
	tests := []struct {
		name    string
		body    []byte
		eventID string
	}{
		{name: "not JSON", body: []byte("event=subscription.activated"), eventID: "evt_1-" + f.org},
		{name: "no event id", body: f.event("subscription.activated", "sub_1", "active", t1)},
		{name: "no subscription", body: []byte(`{"event":"subscription.activated","payload":{},"created_at":1}`), eventID: "evt_1-" + f.org},
		{name: "unknown status", body: f.event("subscription.activated", "sub_1", "trialing", t1), eventID: "evt_1-" + f.org},
		{name: "no subscription id", body: f.event("subscription.activated", "", "active", t1), eventID: "evt_1-" + f.org},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.svc.HandleWebhook(ctx, tt.body, sign(tt.body), tt.eventID); !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("HandleWebhook() error = %v, want ErrInvalidEvent", err)
			}
		})
	}
	// A rejected event is not recorded, so a corrected redelivery applies.
	if got := f.deliverAs(ctx, t, "evt_1-"+f.org, f.event("subscription.activated", "sub_1", "active", t1)); got != WebhookProcessed {
		t.Fatalf("delivery after rejections = %q, want processed", got)
	}
}

func TestServiceReportsDatabaseErrors(t *testing.T) {
	f := newFixture(context.Background(), t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := f.event("subscription.activated", "sub_1", "active", t1)

	if _, err := f.svc.Status(ctx, f.org); err == nil {
		t.Error("Status() succeeded on a canceled context")
	}
	if _, err := f.svc.Checkout(ctx, f.org); err == nil || errors.Is(err, ErrPaymentProvider) {
		t.Errorf("Checkout() error = %v, want a database error", err)
	}
	if _, err := f.svc.HandleWebhook(ctx, body, sign(body), "evt_1"); err == nil || errors.Is(err, ErrInvalidEvent) {
		t.Errorf("HandleWebhook() error = %v, want a database error", err)
	}
}

type fixture struct {
	svc  *Service
	pool *pgxpool.Pool
	org  string
	// calls counts subscription creations at the fake Razorpay.
	calls     atomic.Int32
	lastNotes atomic.Pointer[map[string]string]
	// failWith makes the fake Razorpay answer with this status.
	failWith int
	// duringCreate runs while the fake Razorpay handles a creation.
	duringCreate func()
}

// newFixture migrates the test database, creates an organization (deleting
// it at the end cascades to its subscription) and a fake Razorpay that
// numbers the subscriptions it creates sub_1, sub_2, ...
func newFixture(ctx context.Context, t *testing.T) *fixture {
	t.Helper()
	pool, err := database.Open(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(pool.Close)
	schema, err := os.ReadFile("../database/testdata/legacy_schema.sql")
	if err != nil {
		t.Fatalf("read legacy schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	f := &fixture{pool: pool, org: "org-" + rand.Text()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.failWith != 0 {
			w.WriteHeader(f.failWith)
			_, _ = io.WriteString(w, `{"error":{"code":"BAD_REQUEST_ERROR","description":"plan not found"}}`)
			return
		}
		var req struct {
			Notes map[string]string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		f.lastNotes.Store(&req.Notes)
		n := f.calls.Add(1)
		if f.duringCreate != nil {
			f.duringCreate()
		}
		_, _ = fmt.Fprintf(w, `{"id":"sub_%d","status":"created","current_end":null,"created_at":%d}`, n, t0.Unix())
	}))
	t.Cleanup(srv.Close)
	f.svc = NewService(pool, razorpay.NewClient(srv.URL, testKeyID, "key-secret"), testPlanID, testWebhookSecret)

	if _, err := pool.Exec(ctx, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Acme', $1, now())`, f.org); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.WithoutCancel(ctx), `DELETE FROM organization WHERE id = $1`, f.org); err != nil {
			t.Errorf("delete organization: %v", err)
		}
	})
	return f
}

func (f *fixture) event(name, subID, status string, at time.Time) []byte {
	return eventFor(f.org, name, subID, status, at, nil)
}

func (f *fixture) eventWithEnd(name, subID, status string, at time.Time, periodEnd *time.Time) []byte {
	return eventFor(f.org, name, subID, status, at, periodEnd)
}

// eventFor builds a webhook body shaped like Razorpay's.
func eventFor(org, name, subID, status string, at time.Time, periodEnd *time.Time) []byte {
	var end any
	if periodEnd != nil {
		end = periodEnd.Unix()
	}
	body, err := json.Marshal(map[string]any{
		"entity":     "event",
		"account_id": "acc_test",
		"event":      name,
		"contains":   []string{"subscription"},
		"payload": map[string]any{"subscription": map[string]any{"entity": map[string]any{
			"id": subID, "entity": "subscription", "plan_id": testPlanID, "status": status,
			"current_end": end, "created_at": t0.Unix(), "notes": map[string]string{"organization_id": org},
		}}},
		"created_at": at.Unix(),
	})
	if err != nil {
		panic(err) // a map of plain values always encodes
	}
	return body
}

// deliver applies body as a fresh, correctly signed event and fails the
// test unless it was processed.
func (f *fixture) deliver(ctx context.Context, t *testing.T, body []byte) {
	t.Helper()
	if got := f.deliverAs(ctx, t, "evt_"+rand.Text(), body); got != WebhookProcessed {
		t.Fatalf("HandleWebhook() = %q, want processed", got)
	}
}

func (f *fixture) deliverAs(ctx context.Context, t *testing.T, eventID string, body []byte) WebhookResult {
	t.Helper()
	result, err := f.svc.HandleWebhook(ctx, body, sign(body), eventID)
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	return result
}

func (f *fixture) assertStatus(ctx context.Context, t *testing.T, want Status) {
	t.Helper()
	got, err := f.svc.Status(ctx, f.org)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if got.Plan != want.Plan || got.Status != want.Status ||
		(got.CurrentPeriodEnd == nil) != (want.CurrentPeriodEnd == nil) ||
		(want.CurrentPeriodEnd != nil && !got.CurrentPeriodEnd.Equal(*want.CurrentPeriodEnd)) {
		t.Fatalf("Status() = %+v (period end %v), want %+v (period end %v)", got, got.CurrentPeriodEnd, want, want.CurrentPeriodEnd)
	}
}

func (f *fixture) storedSubscriptionID(ctx context.Context, t *testing.T) string {
	t.Helper()
	var id string
	if err := f.pool.QueryRow(ctx, `SELECT razorpay_subscription_id FROM go_billing_subscriptions WHERE organization_id = $1`, f.org).Scan(&id); err != nil {
		t.Fatalf("load stored subscription: %v", err)
	}
	return id
}

func sign(body []byte) string {
	mac := hmac.New(sha256.New, []byte(testWebhookSecret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// testDatabaseURL returns TEST_DATABASE_URL. CI always sets it, so a
// missing value there fails instead of silently skipping.
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	if os.Getenv("CI") != "" {
		t.Fatal("TEST_DATABASE_URL must be set in CI")
	}
	t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	return ""
}
