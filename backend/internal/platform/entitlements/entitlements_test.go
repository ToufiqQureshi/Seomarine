package entitlements

import (
	"context"
	"errors"
	"testing"
)

func TestAllowAllAllowsAnyOrganizationAndResource(t *testing.T) {
	checker := AllowAll{}
	for _, input := range [][2]string{{"org-a", "keyword"}, {"org-b", "audit"}, {"", ""}} {
		if err := checker.Check(context.Background(), input[0], input[1]); err != nil {
			t.Fatalf("Check(%q, %q) error = %v", input[0], input[1], err)
		}
	}
}

func TestAllowAllStillHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (AllowAll{}).Check(ctx, "org", "resource"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Check() error = %v, want context.Canceled", err)
	}
}
