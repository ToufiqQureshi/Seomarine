package billing

import (
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
)

// ActionCost is about what one action of a feature costs, for the Billing
// screen and the hints next to each action. It is a typical figure from
// DataForSEO's and the AI models' published prices: the real cost is charged
// per request, and a request is held at its maximum before it is sent.
type ActionCost struct {
	Feature spend.Feature `json:"feature"`
	// Unit is the action the credits are for, in the customer's words.
	Unit string `json:"unit"`
	// Credits is the typical cost of one unit.
	Credits int64 `json:"credits"`
}

// typicalUSD is the provider cost of one typical action, per feature.
var typicalUSD = []struct {
	feature spend.Feature
	unit    string
	usd     float64
}{
	{spend.RankTracking, "keyword check (top 30, scheduled)", dataforseo.SerpCostUSD(30, true)},
	{spend.KeywordResearch, "keyword search", 0.02},
	{spend.LocalSEO, "local lookup", 0.01},
	{spend.DomainResearch, "domain report tab", 0.02},
	{spend.Backlinks, "backlink report", 0.03},
	{spend.AIVisibility, "AI visibility check", 0.1},
	{spend.AIAssistant, "AI assistant reply", 0.03},
	{spend.AIWriting, "AI writing action", 0.01},
}

// ActionCosts lists the typical cost of every feature that spends credits.
func ActionCosts() []ActionCost {
	out := make([]ActionCost, len(typicalUSD))
	for i, c := range typicalUSD {
		out[i] = ActionCost{Feature: c.feature, Unit: c.unit, Credits: CreditsForUSD(c.usd)}
	}
	return out
}

func featureNames() []string {
	features := spend.Features()
	names := make([]string, len(features))
	for i, f := range features {
		names[i] = string(f)
	}
	return names
}
