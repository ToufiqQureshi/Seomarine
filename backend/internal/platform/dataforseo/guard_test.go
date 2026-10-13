package dataforseo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend/spendtest"
)

func TestDoSendsNothingWhenTheGuardRefuses(t *testing.T) {
	for _, refusal := range []error{spend.ErrNoCredits, spend.ErrPaused, errors.New("ledger unreachable")} {
		t.Run(refusal.Error(), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
			defer server.Close()
			recorder := &testRecorder{}
			client := guardedClient(t, server, recorder, &spendtest.Guard{Refuse: refusal}, 0)
			_, err := client.Do(context.Background(), "org", spend.Backlinks, http.MethodPost, "/v3/backlinks/summary/live", []byte(`[{"target":"example.com"}]`), false)
			if !errors.Is(err, refusal) {
				t.Fatalf("Do() error = %v, want %v", err, refusal)
			}
			// Callers that already stop on a provider billing issue (scheduled
			// rank checks, AI search) must stop on a refusal too.
			if !errors.Is(err, ErrBillingIssue) {
				t.Fatalf("Do() error = %v, want it to wrap ErrBillingIssue", err)
			}
			if requests != 0 || len(recorder.snapshot()) != 0 {
				t.Fatalf("a refused request reached the provider: %d requests, %d costs", requests, len(recorder.snapshot()))
			}
		})
	}
}

func TestDoHoldsTheCeilingAndSettlesTheRealCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tasks":[{"path":["v3","backlinks","summary","live"],"cost":0.02},{"path":["v3","backlinks","summary","live"],"cost":0.0201}]}`)
	}))
	defer server.Close()
	guard := &spendtest.Guard{}
	client := guardedClient(t, server, &testRecorder{}, guard, 0)
	body := []byte(`[{"target":"a.com","limit":10},{"target":"b.com","limit":10}]`)
	if _, err := client.Do(context.Background(), "org-1", spend.Backlinks, http.MethodPost, "/v3/backlinks/summary/live", body, false); err != nil {
		t.Fatal(err)
	}
	held := guard.Held()
	if len(held) != 1 || held[0].OrganizationID != "org-1" || held[0].Feature != spend.Backlinks || held[0].EstimateUSD != EstimateUSD("/v3/backlinks/summary/live", body) {
		t.Fatalf("held = %#v", held)
	}
	if settled := guard.Settled(); len(settled) != 1 || settled[0] < 0.04009 || settled[0] > 0.04011 {
		t.Fatalf("settled = %v, want the summed task cost 0.0401", settled)
	}
}

func TestDoReleasesTheHoldWhenTheProviderFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer server.Close()
	guard := &spendtest.Guard{}
	client := guardedClient(t, server, &testRecorder{}, guard, 0)
	if _, err := client.Do(context.Background(), "org", spend.DomainResearch, http.MethodPost, "/v3/dataforseo_labs/google/ranked_keywords/live", []byte(`[{}]`), false); err == nil {
		t.Fatal("Do() succeeded on HTTP 502")
	}
	if settled := guard.Settled(); len(settled) != 1 || settled[0] != 0 {
		t.Fatalf("settled = %v, want one release at zero", settled)
	}
}

func TestDoStillChargesWhenRecordingFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tasks":[{"path":["v3","serp"],"cost":0.01}]}`)
	}))
	defer server.Close()
	guard := &spendtest.Guard{}
	client := guardedClient(t, server, &testRecorder{err: errors.New("db down")}, guard, 0)
	_, err := client.Do(context.Background(), "org", spend.RankTracking, http.MethodPost, "/v3/serp/google/organic/live/advanced", []byte(`[{"keyword":"x","depth":10}]`), false)
	if _, ok := errors.AsType[*CostRecordingError](err); !ok {
		t.Fatalf("Do() error = %v, want CostRecordingError", err)
	}
	if settled := guard.Settled(); len(settled) != 1 || settled[0] != 0.01 {
		t.Fatalf("settled = %v, want the provider's 0.01: the work was billed even though recording failed", settled)
	}
}

type failingSettle struct{}

func (failingSettle) Hold(context.Context, spend.Request) (spend.Hold, error) {
	return failingSettle{}, nil
}
func (failingSettle) Settle(context.Context, float64) error { return errors.New("ledger down") }

func TestDoReportsASettleFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tasks":[{"path":["v3","serp"],"cost":0.002}]}`)
	}))
	defer server.Close()
	client := guardedClient(t, server, &testRecorder{}, failingSettle{}, 0)
	_, err := client.Do(context.Background(), "org-9", spend.RankTracking, http.MethodPost, "/v3/serp/google/organic/live/advanced", []byte(`[{}]`), false)
	if recordingErr, ok := errors.AsType[*CostRecordingError](err); !ok || recordingErr.OrganizationID != "org-9" || recordingErr.Cost.USD != 0.002 {
		t.Fatalf("Do() error = %#v, want the unsettled cost reported", err)
	}
}

func TestDoRequiresASpendFeature(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	guard := &spendtest.Guard{}
	client := guardedClient(t, server, &testRecorder{}, guard, 0)
	if _, err := client.Do(context.Background(), "org", " ", http.MethodGet, "/v3/appendix/status", nil, true); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Do() error = %v, want ErrInvalidRequest", err)
	}
	if len(guard.Held()) != 0 {
		t.Fatal("an invalid request was held")
	}
}
