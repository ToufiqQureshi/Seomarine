package main

import (
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/billing"
)

func TestLivePlanGatesRemainActiveWithoutCheckout(t *testing.T) {
	payments := &billing.Service{}
	readOnly := &billing.Service{}
	if got := planGateService("production", nil, readOnly); got != readOnly {
		t.Fatal("live site without checkout must use read-only subscription gates")
	}
	if got := planGateService("production", payments, readOnly); got != payments {
		t.Fatal("live site with checkout must use payment service gates")
	}
	if got := planGateService("development", nil, readOnly); got != nil {
		t.Fatal("self-hosted development retains open feature gates")
	}
}

func TestDashboardOverviewHasNoBacklinkProviderWithoutDataForSEO(t *testing.T) {
	if svc := newDashboardOverview(nil, nil); svc.Backlinks != nil {
		t.Fatal("a nil *backlinks.Service must leave the interface nil, or the dashboard refresh panics")
	}
}
