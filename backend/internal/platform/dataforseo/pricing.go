package dataforseo

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
)

// Price ceilings per task in USD, from DataForSEO's published prices. They are
// what a task can cost at most, because they are held as credit before the
// request is sent; the real price is charged afterwards and the rest is
// released. A path this table does not know is held at unknownTaskUSD, so a
// new endpoint costs a customer a large hold rather than costing Uplumo money.
const (
	serpLivePageUSD     = 0.002
	serpLiveExtraUSD    = 0.002
	contentParsePageUSD = 0.000125
	serpQueuedPageUSD   = 0.0006
	serpQueuedExtraUSD  = 0.00045
	serpPriorityFactor  = 2
	serpOperatorFactor  = 5
	// DataForSEO Labs rates increased by 20% on July 1, 2026.
	labsTaskUSD          = 0.012
	labsRowUSD           = 0.00012
	labsDefaultRows      = 1000
	backlinksTaskUSD     = 0.02
	backlinksRowUSD      = 0.00003
	backlinksDefaultRows = 1000
	googleAdsTaskUSD     = 0.1
	aiOptimizationUSD    = 0.5
	lighthouseTaskUSD    = 0.01
	businessTaskUSD      = 0.05
	businessPerTenUSD    = 0.0015
	unknownTaskUSD       = 1.0
	defaultSerpDepth     = 100
)

// serpOperator matches advanced search operators, which Google SERP endpoints
// bill at five times the price.
var serpOperator = regexp.MustCompile(`(?i)(allinanchor|allintext|allintitle|allinurl|cache|define|definition|filetype|id|inanchor|info|intext|intitle|inurl|link|site):`)

// freePaths are lookups DataForSEO does not charge for.
var freePaths = []string{"/v3/appendix/", "/v3/serp/google/locations", "/v3/business_data/business_listings/categories"}

// EstimateUSD returns the most a request to path with body can cost.
func EstimateUSD(path string, body []byte) float64 {
	for _, free := range freePaths {
		if strings.HasPrefix(path, free) {
			return 0
		}
	}
	if strings.HasSuffix(path, "/models") {
		return 0
	}
	tasks := taskBodies(body)
	total := 0.0
	for _, task := range tasks {
		total += taskCeiling(path, task)
	}
	return math.Round(total*1e6) / 1e6
}

// taskBodies splits a request body into its tasks. DataForSEO takes a JSON
// array of tasks; anything else is one task.
func taskBodies(body []byte) []map[string]any {
	var tasks []map[string]any
	if err := json.Unmarshal(body, &tasks); err == nil && len(tasks) > 0 {
		return tasks
	}
	return []map[string]any{{}}
}

func taskCeiling(path string, task map[string]any) float64 {
	switch {
	case strings.HasPrefix(path, "/v3/serp/"):
		return serpCeiling(path, task)
	case strings.HasPrefix(path, "/v3/dataforseo_labs/"):
		return labsTaskUSD + labsRowUSD*rows(task, labsDefaultRows)
	case strings.HasPrefix(path, "/v3/backlinks/"):
		return backlinksTaskUSD + backlinksRowUSD*rows(task, backlinksDefaultRows)
	case strings.HasPrefix(path, "/v3/keywords_data/"):
		return googleAdsTaskUSD
	case strings.HasPrefix(path, "/v3/ai_optimization/"):
		return aiOptimizationUSD
	case strings.HasPrefix(path, "/v3/on_page/content_parsing/live"):
		return contentParsePageUSD
	case strings.HasPrefix(path, "/v3/on_page/lighthouse"):
		return lighthouseTaskUSD
	case strings.HasPrefix(path, "/v3/business_data/"):
		return businessTaskUSD + businessPerTenUSD*math.Ceil(number(task, "depth", 0)/10)
	}
	return unknownTaskUSD
}

func serpCeiling(path string, task map[string]any) float64 {
	pages := math.Max(1, math.Ceil(number(task, "depth", defaultSerpDepth)/10))
	price := serpLivePageUSD + (pages-1)*serpLiveExtraUSD
	if strings.Contains(path, "/task_post") {
		price = serpQueuedPageUSD + (pages-1)*serpQueuedExtraUSD
		if number(task, "priority", 1) >= 2 {
			price *= serpPriorityFactor
		}
	}
	if keyword, _ := task["keyword"].(string); serpOperator.MatchString(keyword) {
		price *= serpOperatorFactor
	}
	return price
}

// rows is the result limit a task asked for, or fallback when it set none.
func rows(task map[string]any, fallback float64) float64 {
	return number(task, "limit", fallback)
}

func number(task map[string]any, key string, fallback float64) float64 {
	value, ok := task[key].(float64)
	if !ok || value <= 0 || math.IsInf(value, 0) {
		return fallback
	}
	return value
}
