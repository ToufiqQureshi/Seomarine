package aisearch

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestPickLatest(t *testing.T) {
	tests := []struct {
		name  string
		model string
		names []string
		want  string
	}{
		{"pinned model wins while listed", modelChatGPT, []string{"gpt-5.6-luna", "gpt-5.7"}, "gpt-5.6-luna"},
		{"highest plain alias, compared numerically", modelChatGPT, []string{"gpt-5.5", "gpt-5.10", "gpt-5.9", "gpt-5.5-mini", "gpt-5.5-2026-04-23"}, "gpt-5.10"},
		{"a longer version is newer than its prefix", modelChatGPT, []string{"gpt-5", "gpt-5.0.1"}, "gpt-5.0.1"},
		{"5.5 equals 5.5.0, first listed wins", modelChatGPT, []string{"gpt-5.5", "gpt-5.5.0"}, "gpt-5.5"},
		{"claude versions use dashes", modelClaude, []string{"claude-sonnet-4-5", "claude-sonnet-5", "claude-sonnet-4-6", "claude-opus-9"}, "claude-sonnet-5"},
		{"gemini pro only", modelGemini, []string{"gemini-2.5-flash", "gemini-2.5-pro", "gemini-3-pro"}, "gemini-3-pro"},
		{"perplexity has one flagship", modelPerplexity, []string{"sonar", "sonar-reasoning-pro"}, "sonar-reasoning-pro"},
		{"no flagship falls back to the newest listed snapshot", modelChatGPT, []string{"gpt-5.1", "gpt-4o-mini"}, "gpt-5.1"},
		{"nothing known falls back to the newest snapshot", modelClaude, []string{"other"}, "claude-sonnet-5"},
		{"empty catalog", modelGemini, nil, "gemini-2.5-pro"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickLatest(tt.model, tt.names); got != tt.want {
				t.Errorf("pickLatest(%q, %v) = %q, want %q", tt.model, tt.names, got, tt.want)
			}
		})
	}
}

func TestModelCatalogCachesTheCatalogAndFallsBackWhenItFails(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	catalog := svc.provider.models
	clock := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	catalog.now = func() time.Time { return clock }
	const path = "/v3/ai_optimization/claude/llm_responses/models"
	ctx := context.Background()

	if got := catalog.latest(ctx, "org", modelClaude); got != "claude-sonnet-5" {
		t.Fatalf("latest() = %q, want claude-sonnet-5", got)
	}
	if !catalog.known(ctx, "org", modelClaude, "claude-sonnet-5") || catalog.known(ctx, "org", modelClaude, "claude-made-up") {
		t.Error("known() must accept the catalog's names and refuse others")
	}
	if got := len(api.requestsTo(path)); got != 1 {
		t.Errorf("catalog fetches = %d, want 1 (cached)", got)
	}

	clock = clock.Add(catalogTTL + time.Second)
	api.on(path, func(int, string) taskReply { return failReply(50000, "Internal Error.") })
	if got := catalog.latest(ctx, "org", modelClaude); got != "claude-sonnet-5" {
		t.Errorf("latest() during an outage = %q, want the fallback claude-sonnet-5", got)
	}
	// An outage is not cached, so the next call asks again.
	catalog.latest(ctx, "org", modelClaude)
	if got := len(api.requestsTo(path)); got != 3 {
		t.Errorf("catalog fetches = %d, want 3 (initial, expired, retried after failure)", got)
	}
}

func TestModelCatalogUnreachableUsesFallback(t *testing.T) {
	api := newFakeAPI(t)
	svc := newTestService(t, api)
	api.server.Close()
	svc.provider.models.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	if got := svc.provider.models.latest(context.Background(), "org", modelChatGPT); got != "gpt-5.6-luna" {
		t.Errorf("latest() = %q, want the fallback gpt-5.6-luna", got)
	}
}
