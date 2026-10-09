package ranktracking

import (
	"math"
	"regexp"
)

// Provider prices in USD for one SERP request, per DataForSEO's published rates.
const (
	liveBasePageUSD    = 0.002
	liveExtraPageUSD   = 0.0015
	queuedBasePageUSD  = 0.0006
	queuedExtraPageUSD = 0.00045
)

// Limits shared with the legacy app. They keep provider spend bounded.
const (
	MaxKeywordsPerConfig = 1000
	MaxKeywordUnits      = 200 // UTF-16 code units, the legacy limit
	MaxConfigsPerProject = 500
	MaxTasksPerPost      = 100 // DataForSEO accepts at most 100 queued tasks per task_post
)

// Credit pricing carried over from the legacy app. Seomarine's own credit model
// is an owner decision, so these stay in one place until it is made.
const (
	costMarkup       = 1.28
	creditsPerUSD    = 1000
	usdRoundingScale = 100000
)

// Method is how a rank check reaches the provider.
type Method string

// Live is the instant endpoint used for manual checks; Queued is the cheaper
// task queue used for scheduled checks.
const (
	Live   Method = "live"
	Queued Method = "queued"
)

// Estimate is a pre-flight cost. Credits are summed per provider call because
// metering rounds each call on its own.
type Estimate struct {
	CostUSD     float64
	CostCredits int
}

// serpOperator matches advanced search operators. Google Organic bills these at
// five times the price when the keyword contains one anywhere. A false match
// only over-holds credits.
var serpOperator = regexp.MustCompile(`(?i)(allinanchor|allintext|allintitle|allinurl|cache|define|definition|filetype|id|inanchor|info|intext|intitle|inurl|link|site):`)

// SerpMultiplier is 5 for keywords with a search operator, else 1.
func SerpMultiplier(keyword string) int {
	if serpOperator.MatchString(keyword) {
		return 5
	}
	return 1
}

// DevicesCount is the number of SERP requests per keyword.
func DevicesCount(devices string) int {
	if devices == "both" {
		return 2
	}
	return 1
}

// CostPerSerp is the provider cost of one request at the given depth (10-100).
func CostPerSerp(depth int, method Method) float64 {
	extraPages := float64(depth/10 - 1)
	if method == Queued {
		return queuedBasePageUSD + extraPages*queuedExtraPageUSD
	}
	return liveBasePageUSD + extraPages*liveExtraPageUSD
}

// EstimateCheck prices one full check of the keywords on the given devices.
// Live checks make one provider call per keyword/device pair; queued checks post
// up to MaxTasksPerPost pairs per call. Summing once and rounding once would
// understate what is actually charged.
func EstimateCheck(keywords []string, devices string, depth int, method Method) Estimate {
	perDevice := DevicesCount(devices)
	multipliers := make([]int, 0, len(keywords)*perDevice)
	for _, kw := range keywords {
		for range perDevice {
			multipliers = append(multipliers, SerpMultiplier(kw))
		}
	}
	chunk := 1
	if method == Queued {
		chunk = MaxTasksPerPost
	}
	var est Estimate
	for start := 0; start < len(multipliers); start += chunk {
		sum := 0
		for _, m := range multipliers[start:min(start+chunk, len(multipliers))] {
			sum += m
		}
		raw := float64(sum) * CostPerSerp(depth, method)
		est.CostUSD += markupUSD(raw)
		est.CostCredits += int(math.Ceil(markupUSD(raw) * creditsPerUSD))
	}
	est.CostUSD = roundUSD(est.CostUSD)
	return est
}

// ScheduledEstimate adds the recurring monthly cost of a scheduled config.
type ScheduledEstimate struct {
	Estimate
	Interval           Interval
	ChecksPerMonth     int
	MonthlyCostUSD     float64
	MonthlyCostCredits int
}

// EstimateScheduled prices a scheduled check, which uses the queued method.
func EstimateScheduled(keywords []string, devices string, depth int, interval Interval) ScheduledEstimate {
	per := EstimateCheck(keywords, devices, depth, Queued)
	checks := 1
	switch interval {
	case Daily:
		checks = 30
	case Weekly:
		checks = 4
	}
	return ScheduledEstimate{
		Estimate:           per,
		Interval:           interval,
		ChecksPerMonth:     checks,
		MonthlyCostUSD:     per.CostUSD * float64(checks),
		MonthlyCostCredits: per.CostCredits * checks,
	}
}

func markupUSD(raw float64) float64 { return roundUSD(raw * costMarkup) }

func roundUSD(v float64) float64 { return math.Round(v*usdRoundingScale) / usdRoundingScale }
