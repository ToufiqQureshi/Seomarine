package audit

import (
	"strings"
	"testing"
)

func TestIsAuditRenderingAllowed(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"context key enables", map[string]string{"CONTEXT_API_KEY": "k"}, true},
		{"browser flag enables", map[string]string{"AUDIT_BROWSER_RENDERING": "true"}, true},
		{"nothing enabled", map[string]string{}, false},
		{"flag must be exactly true", map[string]string{"AUDIT_BROWSER_RENDERING": "1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := func(key string) string { return tc.env[key] }
			if got := IsAuditRenderingAllowed(env); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	if IsAuditRenderingAllowed(nil) {
		t.Fatal("a nil env cannot enable rendering")
	}
}

func TestEstimateRenderingCredits(t *testing.T) {
	estimate := EstimateRenderingCredits(100)
	if estimate.Low <= 0 || estimate.High < estimate.Low {
		t.Fatalf("estimate = %+v", estimate)
	}
	// The low end is every page on Cloudflare; the high end adds a Context
	// credit for every page, so it must be strictly larger.
	if estimate.High <= estimate.Low {
		t.Fatalf("high (%d) must exceed low (%d)", estimate.High, estimate.Low)
	}
	zero := EstimateRenderingCredits(0)
	if zero.Low != 0 || zero.High != 0 {
		t.Fatalf("rendering zero pages costs nothing, got %+v", zero)
	}
}

func TestRenderUsageCreditsMonotonic(t *testing.T) {
	low := RenderUsageCredits(RenderUsage{CloudflareAttempts: 10})
	high := RenderUsageCredits(RenderUsage{CloudflareAttempts: 10, ContextCredits: 10})
	if high <= low {
		t.Fatalf("context fallback must cost more: %d vs %d", high, low)
	}
}

func TestRenderingCreditText(t *testing.T) {
	needed := RenderingCreditsNeededText(1000)
	if needed == "" {
		t.Fatal("expected copy")
	}
	// 1000 must be rendered with a thousands separator.
	if !strings.Contains(needed, "1,000") {
		t.Fatalf("expected a thousands separator in %q", needed)
	}
	estimate := RenderingEstimateText(1000)
	if !strings.Contains(estimate, "1,000") || !strings.Contains(estimate, "$") {
		t.Fatalf("estimate = %q", estimate)
	}
}

func TestFormatUSNumber(t *testing.T) {
	cases := map[int]string{
		0: "0", 999: "999", 1000: "1,000", 12345: "12,345", 1000000: "1,000,000",
	}
	for input, want := range cases {
		if got := formatUSNumber(input); got != want {
			t.Errorf("formatUSNumber(%d) = %q, want %q", input, got, want)
		}
	}
}
