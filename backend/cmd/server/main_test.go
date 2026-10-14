package main

import "testing"

func TestDashboardOverviewHasNoBacklinkProviderWithoutDataForSEO(t *testing.T) {
	if svc := newDashboardOverview(nil, nil); svc.Backlinks != nil {
		t.Fatal("a nil *backlinks.Service must leave the interface nil, or the dashboard refresh panics")
	}
}
