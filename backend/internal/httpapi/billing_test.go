package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/billing"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/razorpay"
)

const testWebhookSecret = "webhook-secret"

func TestBillingEndToEnd(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(ctx, t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	id := rand.Text()
	org, owner, member, loner := "org-"+id, "owner-"+id, "member-"+id, "loner-"+id
	subID := "sub_" + id
	var razorpayDown atomic.Bool
	fakeRazorpay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if razorpayDown.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":%q,"status":"created","current_end":null,"created_at":%d}`, subID, time.Now().Add(-time.Hour).Unix())
	}))
	defer fakeRazorpay.Close()

	newHandler := func(svc *billing.Service) http.Handler {
		return NewHandler(Deps{
			Logger:   discardLogger,
			DB:       healthy,
			Redis:    healthy,
			Auth:     auth.NewService(pool, testSecret),
			Billing:  svc,
			Upstream: &url.URL{Scheme: "http", Host: "127.0.0.1:1"},
		})
	}
	handler := newHandler(billing.NewService(pool, razorpay.NewClient(fakeRazorpay.URL, "rzp_test_key", "key-secret"), "plan_pro", testWebhookSecret))

	t.Cleanup(func() {
		cleanupCtx := context.WithoutCancel(ctx)
		exec(cleanupCtx, t, pool, `DELETE FROM "user" WHERE id IN ($1, $2, $3)`, owner, member, loner)
		exec(cleanupCtx, t, pool, `DELETE FROM organization WHERE id = $1`, org)
	})
	exec(ctx, t, pool, `INSERT INTO "user" (id, name, email) VALUES ($1, 'Owner', $1 || '@example.com'),
		($2, 'Member', $2 || '@example.com'), ($3, 'Loner', $3 || '@example.com')`, owner, member, loner)
	exec(ctx, t, pool, `INSERT INTO organization (id, name, slug, created_at) VALUES ($1, 'Acme', $1, now())`, org)
	exec(ctx, t, pool, `INSERT INTO member (id, organization_id, user_id, role, created_at) VALUES
		($1, $3, $4, 'owner', now()), ($2, $3, $5, 'member', now())`, "m-owner-"+id, "m-member-"+id, org, owner, member)
	exec(ctx, t, pool, `INSERT INTO session (id, token, user_id, active_organization_id, expires_at) VALUES
		($1, $1, $2, $5, now() + interval '1 hour'), ($3, $3, $4, $5, now() + interval '1 hour'),
		($6, $6, $7, NULL, now() + interval '1 hour')`,
		"s-owner-"+id, owner, "s-member-"+id, member, org, "s-loner-"+id, loner)

	as := func(h http.Handler, sessionToken, method, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, method, target, nil)
		req.AddCookie(&http.Cookie{Name: "better-auth.session_token", Value: sign(sessionToken)})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	webhook := func(h http.Handler, body, signature, eventID string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/webhooks/razorpay", strings.NewReader(body))
		if signature != "" {
			req.Header.Set("X-Razorpay-Signature", signature)
		}
		req.Header.Set("X-Razorpay-Event-Id", eventID)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	ownerToken, memberToken, lonerToken := "s-owner-"+id, "s-member-"+id, "s-loner-"+id

	t.Run("billing off answers 503", func(t *testing.T) {
		off := newHandler(nil)
		assertError(t, as(off, ownerToken, http.MethodGet, "/api/v1/billing/status"), http.StatusServiceUnavailable, "billing_unavailable")
		assertError(t, as(off, ownerToken, http.MethodPost, "/api/v1/billing/checkout"), http.StatusServiceUnavailable, "billing_unavailable")
		assertError(t, webhook(off, "{}", hmacHex("{}"), "evt"), http.StatusServiceUnavailable, "billing_unavailable")
	})

	t.Run("a session without an organization is refused", func(t *testing.T) {
		assertError(t, as(handler, lonerToken, http.MethodGet, "/api/v1/billing/status"), http.StatusForbidden, "no_organization")
		assertError(t, as(handler, lonerToken, http.MethodPost, "/api/v1/billing/checkout"), http.StatusForbidden, "no_organization")
	})

	t.Run("a plain member sees the plan but cannot check out", func(t *testing.T) {
		assertJSON(t, as(handler, memberToken, http.MethodGet, "/api/v1/billing/status"), http.StatusOK,
			`{"plan":"free","status":"none","currentPeriodEnd":null}`)
		assertError(t, as(handler, memberToken, http.MethodPost, "/api/v1/billing/checkout"), http.StatusForbidden, "forbidden")
	})

	t.Run("a Razorpay failure is a bad gateway", func(t *testing.T) {
		razorpayDown.Store(true)
		defer razorpayDown.Store(false)
		assertError(t, as(handler, ownerToken, http.MethodPost, "/api/v1/billing/checkout"), http.StatusBadGateway, "payment_provider_error")
	})

	t.Run("the owner checks out and the webhook activates the plan", func(t *testing.T) {
		assertJSON(t, as(handler, ownerToken, http.MethodPost, "/api/v1/billing/checkout"), http.StatusOK,
			fmt.Sprintf(`{"subscriptionId":%q,"keyId":"rzp_test_key"}`, subID))

		periodEnd := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
		event := fmt.Sprintf(`{"entity":"event","event":"subscription.activated","contains":["subscription"],
			"payload":{"subscription":{"entity":{"id":%q,"status":"active","current_end":%d,"notes":{"organization_id":%q}}}},
			"created_at":%d}`, subID, periodEnd.Unix(), org, time.Now().Unix())
		eventID := "evt_" + id

		assertError(t, webhook(handler, event, "", eventID), http.StatusUnauthorized, "invalid_signature")
		assertError(t, webhook(handler, event, hmacHex(event+" "), eventID), http.StatusUnauthorized, "invalid_signature")
		assertJSON(t, webhook(handler, event, hmacHex(event), eventID), http.StatusOK, `{"status":"processed"}`)
		assertJSON(t, webhook(handler, event, hmacHex(event), eventID), http.StatusOK, `{"status":"duplicate"}`)

		assertJSON(t, as(handler, memberToken, http.MethodGet, "/api/v1/billing/status"), http.StatusOK,
			`{"plan":"pro","status":"active","currentPeriodEnd":"2026-11-07T00:00:00Z"}`)
		assertError(t, as(handler, ownerToken, http.MethodPost, "/api/v1/billing/checkout"), http.StatusConflict, "already_subscribed")
	})

	// A database failure is a 500, never the 502 of a Razorpay failure, and
	// a webhook that could not be stored is a 500 so Razorpay redelivers it.
	t.Run("database failures are internal errors", func(t *testing.T) {
		closed, err := pgxpool.New(ctx, pool.Config().ConnString())
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		closed.Close()
		broken := newHandler(billing.NewService(closed, razorpay.NewClient(fakeRazorpay.URL, "rzp_test_key", "key-secret"), "plan_pro", testWebhookSecret))
		assertError(t, as(broken, ownerToken, http.MethodGet, "/api/v1/billing/status"), http.StatusInternalServerError, "internal")
		assertError(t, as(broken, ownerToken, http.MethodPost, "/api/v1/billing/checkout"), http.StatusInternalServerError, "internal")
		event := fmt.Sprintf(`{"event":"subscription.charged","payload":{"subscription":{"entity":{"id":%q,"status":"active","notes":{"organization_id":%q}}}},"created_at":1}`, subID, org)
		assertError(t, webhook(broken, event, hmacHex(event), "evt_broken_"+id), http.StatusInternalServerError, "internal")
	})

	t.Run("bad webhooks are refused", func(t *testing.T) {
		assertJSON(t, webhook(handler, `{"event":"payment.captured"}`, hmacHex(`{"event":"payment.captured"}`), "evt_other_"+id), http.StatusOK, `{"status":"ignored"}`)
		assertError(t, webhook(handler, "not json", hmacHex("not json"), "evt_bad_"+id), http.StatusBadRequest, "invalid_event")
		huge := strings.Repeat(" ", (256<<10)+1)
		assertError(t, webhook(handler, huge, hmacHex(huge), "evt_huge_"+id), http.StatusRequestEntityTooLarge, "payload_too_large")
	})
}

// assertJSON checks the status and the exact JSON body, which is the
// contract the frontend reads.
func assertJSON(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantBody string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, wantStatus, rec.Body)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != wantBody {
		t.Fatalf("body = %s\nwant   %s", got, wantBody)
	}
}

func hmacHex(body string) string {
	mac := hmac.New(sha256.New, []byte(testWebhookSecret))
	_, _ = io.WriteString(mac, body)
	return hex.EncodeToString(mac.Sum(nil))
}
