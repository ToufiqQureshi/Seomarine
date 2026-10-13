// Package plans is the server-authoritative Uplumo plan catalog.
package plans

type Definition struct {
	ID                string
	Name              string
	PriceINR          int
	Sites             int
	MonthlyCreditsUSD float64
	PaidSEOData       bool
	AIVisibility      bool
	Features          []string
}

const (
	Free   = "free"
	Basic  = "basic"
	Pro    = "pro"
	Agency = "agency"
)

var catalog = []Definition{
	{ID: Free, Name: "Free", PriceINR: 0, Sites: 1, Features: []string{
		"1 website", "Uplumo Analytics and AI referral traffic",
	}},
	{ID: Basic, Name: "Basic", PriceINR: 899, Sites: 1, Features: []string{
		"1 website", "Everything in Free", "Connect Google Search Console and Google Analytics", "Search queries and page performance",
		"Saved keywords and project context", "No paid SEO-data or AI/LLM provider calls",
	}},
	{ID: Pro, Name: "Pro", PriceINR: 1899, Sites: 5, MonthlyCreditsUSD: 3, PaidSEOData: true, AIVisibility: true, Features: []string{
		"Up to 5 websites", "Everything in Basic", "Keyword research, rank tracking, domain and backlink data",
		"Technical site audits with rendered-page analysis",
		"AI visibility across ChatGPT, Perplexity, Gemini, AI Overviews and Claude", "AI writing assistant",
		"$3 monthly usage credits for AI writing and rendered-page audits",
	}},
	{ID: Agency, Name: "Agency", PriceINR: 4999, Sites: 25, MonthlyCreditsUSD: 10, PaidSEOData: true, AIVisibility: true, Features: []string{
		"Up to 25 websites", "Everything in Pro", "White-label reports with your agency branding", "$10 monthly usage credits for AI writing and rendered-page audits",
	}},
}

func Catalog() []Definition {
	result := make([]Definition, len(catalog))
	for i, definition := range catalog {
		result[i] = definition
		result[i].Features = append([]string(nil), definition.Features...)
	}
	return result
}

func Get(id string) (Definition, bool) {
	for _, definition := range catalog {
		if definition.ID == id {
			copy := definition
			copy.Features = append([]string(nil), definition.Features...)
			return copy, true
		}
	}
	return Definition{}, false
}

func PaidProviderAccess(id string) bool {
	definition, ok := Get(id)
	return ok && definition.PaidSEOData
}
