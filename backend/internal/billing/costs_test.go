package billing

import (
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

func TestRankTrackingTypicalCostUsesThePublishedTop30QueuedRate(t *testing.T) {
	for _, cost := range ActionCosts() {
		if cost.Feature == spend.RankTracking {
			if cost.Credits != CreditsForUSD(dataforseo.SerpCostUSD(30, true)) {
				t.Fatalf("rank tracking typical cost = %d credits, want the published top-30 Standard rate", cost.Credits)
			}
			return
		}
	}
	t.Fatal("rank tracking typical cost is missing")
}
