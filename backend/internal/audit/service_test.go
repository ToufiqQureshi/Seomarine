package audit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestService builds a Service with an in-memory store and a guard whose
// probe client never touches the network.
func newTestService(store Store, scheduler Scheduler, plans PaidPlans, access ManagedAccess, rendering RenderingMeter, hosted bool, env map[string]string) *Service {
	return NewService(ServiceConfig{
		Repository: store,
		Guard: &Guard{
			Resolver: fakeResolver{addrs: map[string][]string{"example.com": {"93.184.216.34"}}},
			Client:   failNetworkClient(),
		},
		Scheduler: scheduler, Plans: plans, Access: access, Rendering: rendering, Hosted: hosted,
		Env:    func(key string) string { return env[key] },
		Logger: discardLogger(),
		Now:    func() time.Time { return time.Unix(0, 0) },
		NewID:  func() string { return "audit-1" },
	})
}

func TestStartAuditSucceeds(t *testing.T) {
	store := newMemoryStore()
	scheduler := &recordingScheduler{}
	service := newTestService(store, scheduler, nil, nil, nil, false, nil)

	result, err := service.StartAudit(context.Background(), StartAuditInput{
		ActorUserID: "user-1", OrganizationID: "org-1", ProjectID: "proj-1",
		StartURL: "example.com", MaxPages: 50, LighthouseStrategy: LighthouseAuto, LimitTier: TierSelfHosted,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AuditID != "audit-1" {
		t.Fatalf("auditId = %q", result.AuditID)
	}
	if len(scheduler.enqueued) != 1 {
		t.Fatalf("enqueued = %d, want 1", len(scheduler.enqueued))
	}
	audit, err := store.GetAuditForProject(context.Background(), "audit-1", "proj-1")
	if err != nil {
		t.Fatalf("audit not stored: %v", err)
	}
	config, ok := ParseAuditConfig(audit.Config)
	if !ok || config.MaxPages != 50 || config.LighthouseStrategy != LighthouseAuto {
		t.Fatalf("config = %+v ok=%v", config, ok)
	}
	if audit.StartURL != "https://example.com/" {
		t.Fatalf("startUrl = %q", audit.StartURL)
	}
	if audit.PagesTotal != 50 || audit.LighthouseTotal != 20 {
		t.Fatalf("reservation = %d/%d", audit.PagesTotal, audit.LighthouseTotal)
	}
}

func TestStartAuditRejectsRenderingWhenUnavailable(t *testing.T) {
	service := newTestService(newMemoryStore(), &recordingScheduler{}, nil, nil, nil, false, nil)
	_, err := service.StartAudit(context.Background(), StartAuditInput{
		ProjectID: "proj-1", StartURL: "example.com", RenderJavaScript: true, LimitTier: TierSelfHosted,
	})
	if !errors.Is(err, ErrRenderingUnavailable) {
		t.Fatalf("error = %v, want ErrRenderingUnavailable", err)
	}
}

func TestStartAuditRejectsRenderingPageLimit(t *testing.T) {
	store := newMemoryStore()
	service := newTestService(store, &recordingScheduler{}, nil, nil, nil, false,
		map[string]string{"AUDIT_BROWSER_RENDERING": "true"})
	_, err := service.StartAudit(context.Background(), StartAuditInput{
		ProjectID: "proj-1", StartURL: "example.com", MaxPages: 2000, RenderJavaScript: true, LimitTier: TierSelfHosted,
	})
	if !errors.Is(err, ErrAuditPageLimitExceeded) {
		t.Fatalf("error = %v, want ErrAuditPageLimitExceeded", err)
	}
}

func TestStartAuditEnforcesFreePageLimit(t *testing.T) {
	service := newTestService(newMemoryStore(), &recordingScheduler{}, nil, nil, nil, false, nil)
	_, err := service.StartAudit(context.Background(), StartAuditInput{
		ProjectID: "proj-1", StartURL: "example.com", MaxPages: 100, LimitTier: TierFree,
	})
	if !errors.Is(err, ErrAuditPageLimitExceeded) {
		t.Fatalf("error = %v, want ErrAuditPageLimitExceeded", err)
	}
}

func TestStartAuditRejectsBlockedTarget(t *testing.T) {
	service := newTestService(newMemoryStore(), &recordingScheduler{}, nil, nil, nil, false, nil)
	_, err := service.StartAudit(context.Background(), StartAuditInput{
		ProjectID: "proj-1", StartURL: "http://127.0.0.1/", LimitTier: TierSelfHosted,
	})
	if !errors.Is(err, ErrCrawlTargetBlocked) {
		t.Fatalf("error = %v, want ErrCrawlTargetBlocked", err)
	}
}

func TestStartAuditCleansUpWhenSchedulingFails(t *testing.T) {
	store := newMemoryStore()
	scheduler := &recordingScheduler{failEnqueue: true}
	service := newTestService(store, scheduler, nil, nil, nil, false, nil)
	_, err := service.StartAudit(context.Background(), StartAuditInput{
		ProjectID: "proj-1", StartURL: "example.com", LimitTier: TierSelfHosted,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(store.audits) != 0 {
		t.Fatalf("the failed audit row must be removed, %v", store.audits)
	}
	if len(scheduler.terminated) != 1 {
		t.Fatalf("expected the scheduled work to be terminated, got %v", scheduler.terminated)
	}
}

func TestStartAuditHoldsRenderingCredits(t *testing.T) {
	store := newMemoryStore()
	meter := &fakeRenderingMeter{}
	service := newTestService(store, &recordingScheduler{}, nil, nil, meter, false,
		map[string]string{"AUDIT_BROWSER_RENDERING": "true"})
	if _, err := service.StartAudit(context.Background(), StartAuditInput{
		ProjectID: "proj-1", StartURL: "example.com", MaxPages: 20, RenderJavaScript: true, LimitTier: TierSelfHosted,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meter.locked != 1 || meter.lastMaxPages != 20 {
		t.Fatalf("meter = %+v", meter)
	}
}

func TestStartAuditReleasesCreditsWhenSchedulingFails(t *testing.T) {
	store := newMemoryStore()
	meter := &fakeRenderingMeter{}
	scheduler := &recordingScheduler{failEnqueue: true}
	service := newTestService(store, scheduler, nil, nil, meter, false,
		map[string]string{"AUDIT_BROWSER_RENDERING": "true"})
	if _, err := service.StartAudit(context.Background(), StartAuditInput{
		ProjectID: "proj-1", StartURL: "example.com", MaxPages: 20, RenderJavaScript: true, LimitTier: TierSelfHosted,
	}); err == nil {
		t.Fatal("expected the scheduling failure to surface")
	}
	if meter.locked != 1 || meter.released != 1 {
		t.Fatalf("expected the hold to be released, meter = %+v", meter)
	}
}

func TestResolveAuditLimitTier(t *testing.T) {
	selfHosted := newTestService(newMemoryStore(), nil, nil, nil, nil, false, nil)
	if tier, err := selfHosted.ResolveAuditLimitTier(context.Background(), "org-1"); err != nil || tier != TierSelfHosted {
		t.Fatalf("tier = %q err = %v", tier, err)
	}

	openWide := newTestService(newMemoryStore(), nil, nil, nil, nil, true, nil)
	if tier, err := openWide.ResolveAuditLimitTier(context.Background(), "org-1"); err != nil || tier != TierFree {
		t.Fatalf("tier = %q err = %v", tier, err)
	}

	paid := newTestService(newMemoryStore(), nil, fakePlans{paid: true}, nil, nil, true, nil)
	if tier, err := paid.ResolveAuditLimitTier(context.Background(), "org-1"); err != nil || tier != TierPaid {
		t.Fatalf("tier = %q err = %v", tier, err)
	}

	denied := newTestService(newMemoryStore(), nil, nil, fakeAccess{allowed: false}, nil, true, nil)
	if _, err := denied.ResolveAuditLimitTier(context.Background(), "org-1"); !errors.Is(err, ErrPaymentRequired) {
		t.Fatalf("error = %v, want ErrPaymentRequired", err)
	}
}

func TestGetStatusAndResultsAndHistory(t *testing.T) {
	store := newMemoryStore()
	config, _ := MarshalAuditConfig(Config{MaxPages: 50, LighthouseStrategy: LighthouseAuto})
	if err := store.CreateAudit(context.Background(), CreateAuditInput{
		ID: "audit-1", ProjectID: "proj-1", StartedByUserID: "user-1", StartURL: "https://example.com/",
		WorkflowInstanceID: "audit-1", Config: config, PagesTotal: 50, LighthouseTotal: 20,
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}
	service := newTestService(store, nil, nil, nil, nil, false, nil)

	status, err := service.GetStatus(context.Background(), "audit-1", "proj-1")
	if err != nil || status.Status != "running" {
		t.Fatalf("status = %+v err = %v", status, err)
	}
	if status.StartedAt != "1970-01-01T00:00:00.000Z" {
		t.Fatalf("startedAt = %q", status.StartedAt)
	}

	if _, err := service.GetResults(context.Background(), "audit-1", "proj-1"); err != nil {
		t.Fatalf("results: %v", err)
	}

	history, err := service.GetHistory(context.Background(), "proj-1")
	if err != nil || len(history) != 1 || !history[0].RanLighthouse {
		t.Fatalf("history = %+v err = %v", history, err)
	}

	// Another project must not see this audit.
	if _, err := service.GetStatus(context.Background(), "audit-1", "proj-2"); !errors.Is(err, ErrAuditNotFound) {
		t.Fatalf("error = %v, want ErrAuditNotFound (tenant isolation)", err)
	}
}

func TestGetResultsRejectsInvalidConfig(t *testing.T) {
	store := newMemoryStore()
	if err := store.CreateAudit(context.Background(), CreateAuditInput{
		ID: "audit-1", ProjectID: "proj-1", StartURL: "https://example.com/", WorkflowInstanceID: "audit-1", Config: "{bad}",
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}
	service := newTestService(store, nil, nil, nil, nil, false, nil)
	if _, err := service.GetResults(context.Background(), "audit-1", "proj-1"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestGetCrawlProgressWithoutRedis(t *testing.T) {
	store := newMemoryStore()
	_ = store.CreateAudit(context.Background(), CreateAuditInput{ID: "audit-1", ProjectID: "proj-1", StartURL: "https://example.com/", WorkflowInstanceID: "audit-1"})
	service := newTestService(store, nil, nil, nil, nil, false, nil)
	entries, err := service.GetCrawlProgress(context.Background(), "audit-1", "proj-1")
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries = %v err = %v", entries, err)
	}
}

func TestRemoveAudit(t *testing.T) {
	store := newMemoryStore()
	_ = store.CreateAudit(context.Background(), CreateAuditInput{ID: "audit-1", ProjectID: "proj-1", StartURL: "https://example.com/", WorkflowInstanceID: "audit-1"})
	scheduler := &recordingScheduler{}
	service := newTestService(store, scheduler, nil, nil, nil, false, nil)
	if err := service.Remove(context.Background(), "audit-1", "proj-1"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(store.audits) != 0 {
		t.Fatalf("audit still stored")
	}
	if len(scheduler.terminated) != 1 {
		t.Fatalf("expected the running audit to be terminated")
	}
}

func TestCapabilities(t *testing.T) {
	disabled := newTestService(newMemoryStore(), nil, nil, nil, nil, false, nil)
	if disabled.Capabilities().CanRenderJavaScript {
		t.Fatal("rendering should be off")
	}
	enabled := newTestService(newMemoryStore(), nil, nil, nil, nil, false, map[string]string{"CONTEXT_API_KEY": "k"})
	if !enabled.Capabilities().CanRenderJavaScript {
		t.Fatal("rendering should be on")
	}
}

func TestStartAuditRejectsBadStrategy(t *testing.T) {
	store := newMemoryStore()
	service := newTestService(store, &recordingScheduler{}, nil, nil, nil, false, nil)
	_, err := service.StartAudit(context.Background(), StartAuditInput{
		ProjectID: "proj-1", StartURL: "example.com", LighthouseStrategy: "sometimes", LimitTier: TierSelfHosted,
	})
	if err == nil {
		t.Fatal("expected an error for an unknown strategy")
	}
	if len(store.audits) != 0 {
		t.Fatal("no audit should be created for an invalid request")
	}
}

func TestFormatISO(t *testing.T) {
	if got := formatISO(time.Unix(0, 0)); got != "1970-01-01T00:00:00.000Z" {
		t.Fatalf("got %q", got)
	}
	if formatISOMillisOpt(nil) != nil {
		t.Fatal("nil time must format as nil")
	}
}
