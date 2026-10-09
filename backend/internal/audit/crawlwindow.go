package audit

// CrawlWindowLimits bounds the rolling fetch-concurrency window for a crawl.
type CrawlWindowLimits struct {
	// Initial is the window size a chunk starts with.
	Initial int
	// Min and Max bound every adjustment.
	Min int
	Max int
	// BudgetBytes is the total HTML the in-flight window may buffer at once.
	// Production audits died with exceededMemory when a fast site let the
	// window grow unchecked.
	BudgetBytes int
}

// CrawlWindow is the default window for a fresh chunk.
var CrawlWindow = CrawlWindowLimits{Initial: 2, Min: 1, Max: 2, BudgetBytes: 8 * 1024 * 1024}

// RetryCrawlWindow is the window for a chunk whose earlier attempt died
// mid-crawl (almost always exceededMemory on a heavy-page site). The retry is
// the chunk's last attempt, so it must not re-run the exact memory profile that
// just killed it.
var RetryCrawlWindow = CrawlWindowLimits{Initial: 1, Min: 1, Max: 1, BudgetBytes: 4 * 1024 * 1024}

const (
	slowResponseMs = 10_000
	fastResponseMs = 1_500
	// minAssumedPageBytes is the floor for the observed page size so tiny-page
	// sites cannot void the byte bound.
	minAssumedPageBytes = 64 * 1024
	// growthMinSample: growth requires a full-size sample. The first persist
	// sub-batch is small (so the byte bound reacts to heavy pages early), and a
	// handful of fast pages proves too little to widen the window.
	growthMinSample = 25
)

// ClampCrawlWindow clamps size into the window limits. Mirrors clampCrawlWindow.
func ClampCrawlWindow(size int, limits CrawlWindowLimits) int {
	return min(max(size, limits.Min), limits.Max)
}

// AdjustCrawlWindow adapts the window to the last persisted sub-batch: it
// shrinks on trouble (errors, blocks, very slow responses), grows only on a
// clean, mostly fast, full-size batch, and is always capped so the batch's
// average page size times the window stays inside the in-flight byte budget.
// Mirrors adjustCrawlWindow.
func AdjustCrawlWindow(windowSize int, recent []CrawledPageResult, limits CrawlWindowLimits) int {
	if len(recent) == 0 {
		return windowSize
	}
	troubled := 0
	for _, page := range recent {
		if page.FetchClass != FetchOK ||
			// A 429 the retries recovered from still says we are crawling
			// faster than the site allows.
			page.RateLimited ||
			page.ResponseTimeMs >= slowResponseMs {
			troubled++
		}
	}
	next := windowSize
	if troubled*3 >= len(recent) {
		next = max(limits.Min, windowSize/2)
	} else {
		fast := 0
		for _, page := range recent {
			if page.FetchClass == FetchOK && page.ResponseTimeMs <= fastResponseMs {
				fast++
			}
		}
		if troubled == 0 && fast*2 >= len(recent) && len(recent) >= growthMinSample {
			next = min(limits.Max, windowSize+5)
		}
	}

	totalBytes := 0
	for _, page := range recent {
		totalBytes += page.HTMLBytes
	}
	avgPageBytes := max(totalBytes/len(recent), minAssumedPageBytes)
	byteBound := max(limits.Min, limits.BudgetBytes/avgPageBytes)
	return min(next, byteBound)
}
