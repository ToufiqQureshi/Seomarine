package audit

import "math"

// Field data is what real Chrome visitors experienced (the Chrome UX Report,
// CrUX), as opposed to Lighthouse's one simulated visit. It comes from the
// CrUX API when CRUX_API_KEY is set, otherwise from the copy PageSpeed
// Insights still embeds in its response.

// FieldDataStatus says what is known about a page's field data.
type FieldDataStatus string

const (
	// FieldAvailable means metrics were returned.
	FieldAvailable FieldDataStatus = "available"
	// FieldInsufficientData means Google answered that neither the page nor
	// the site has enough Chrome traffic to report. Only a documented "no
	// record" answer sets it, never an error.
	FieldInsufficientData FieldDataStatus = "insufficient_data"
	// FieldUnavailable means the lookup failed (key rejected, quota, timeout,
	// outage). It says nothing about the site's traffic.
	FieldUnavailable FieldDataStatus = "unavailable"
	// FieldNotConfigured means no CrUX key is set and PageSpeed Insights sent
	// no field data, so nothing was looked up.
	FieldNotConfigured FieldDataStatus = "not_configured"
)

// Core Web Vitals assessment results, following PageSpeed Insights: LCP, CLS
// and (when measured) INP must all be good at the 75th percentile. Without
// LCP or CLS no assessment is made.
const (
	CoreWebVitalsPassed      = "passed"
	CoreWebVitalsFailed      = "failed"
	CoreWebVitalsNotAssessed = "not_assessed"
)

// Field metric categories at the 75th percentile, the names PageSpeed
// Insights uses.
const (
	fieldGood             = "FAST"
	fieldNeedsImprovement = "AVERAGE"
	fieldPoor             = "SLOW"
)

// StoredFieldMetric is one field metric at the 75th percentile, with the share
// of visits in each of Google's good / needs improvement / poor bands.
type StoredFieldMetric struct {
	Key string `json:"key"`
	// Percentile is milliseconds, except CLS, which is unitless.
	Percentile       float64 `json:"percentile"`
	Category         string  `json:"category"`
	Good             float64 `json:"good"`
	NeedsImprovement float64 `json:"needsImprovement"`
	Poor             float64 `json:"poor"`
}

// FieldCollectionPeriod is the 28-day window the data covers, as YYYY-MM-DD.
type FieldCollectionPeriod struct {
	FirstDate string `json:"firstDate"`
	LastDate  string `json:"lastDate"`
}

// StoredFieldData is the field data stored with one Lighthouse check.
type StoredFieldData struct {
	Status FieldDataStatus `json:"status"`
	// Source is "crux" or "pagespeed"; empty when nothing was returned.
	Source string `json:"source,omitempty"`
	// Scope is "page" for data about the URL itself and "origin" for the whole
	// site, used when the page has too few visits.
	Scope            string                 `json:"scope,omitempty"`
	CoreWebVitals    string                 `json:"coreWebVitals,omitempty"`
	CollectionPeriod *FieldCollectionPeriod `json:"collectionPeriod,omitempty"`
	Metrics          []StoredFieldMetric    `json:"metrics"`
}

// fieldThreshold is a metric's good and poor boundaries at the 75th
// percentile (developers.google.com/speed/docs/insights/v5/about).
type fieldThreshold struct{ good, poor float64 }

// fieldMetrics lists the stored metrics in display order, with their names in
// PageSpeed Insights and in the CrUX API.
var fieldMetrics = []struct {
	key, pageSpeed, crux string
	threshold            fieldThreshold
}{
	{"lcp", "LARGEST_CONTENTFUL_PAINT_MS", "largest_contentful_paint", fieldThreshold{2500, 4000}},
	{"inp", "INTERACTION_TO_NEXT_PAINT", "interaction_to_next_paint", fieldThreshold{200, 500}},
	{"cls", "CUMULATIVE_LAYOUT_SHIFT_SCORE", "cumulative_layout_shift", fieldThreshold{0.1, 0.25}},
	{"fcp", "FIRST_CONTENTFUL_PAINT_MS", "first_contentful_paint", fieldThreshold{1800, 3000}},
	{"ttfb", "EXPERIMENTAL_TIME_TO_FIRST_BYTE", "experimental_time_to_first_byte", fieldThreshold{800, 1800}},
}

// categorize grades a 75th-percentile value against a threshold.
func (t fieldThreshold) categorize(value float64) string {
	switch {
	case value <= t.good:
		return fieldGood
	case value <= t.poor:
		return fieldNeedsImprovement
	default:
		return fieldPoor
	}
}

// assessCoreWebVitals applies Google's rule to the stored metrics.
func assessCoreWebVitals(metrics []StoredFieldMetric) string {
	categories := map[string]string{}
	for _, metric := range metrics {
		categories[metric.Key] = metric.Category
	}
	lcp, hasLCP := categories["lcp"]
	cls, hasCLS := categories["cls"]
	if !hasLCP || !hasCLS {
		return CoreWebVitalsNotAssessed
	}
	inp, hasINP := categories["inp"]
	if lcp == fieldGood && cls == fieldGood && (!hasINP || inp == fieldGood) {
		return CoreWebVitalsPassed
	}
	return CoreWebVitalsFailed
}

// availableFieldData wraps metrics returned by a source, or returns nil when
// none of the metrics we show were there.
func availableFieldData(source, scope string, metrics []StoredFieldMetric, period *FieldCollectionPeriod) *StoredFieldData {
	if len(metrics) == 0 {
		return nil
	}
	return &StoredFieldData{
		Status: FieldAvailable, Source: source, Scope: scope,
		CoreWebVitals: assessCoreWebVitals(metrics), CollectionPeriod: period, Metrics: metrics,
	}
}

// rawFieldMetric is one CrUX metric as PageSpeed Insights embeds it.
type rawFieldMetric struct {
	Percentile    *float64 `json:"percentile"`
	Distributions []struct {
		Proportion float64 `json:"proportion"`
	} `json:"distributions"`
}

// rawLoadingExperience is a CrUX block of a PageSpeed Insights response.
// OriginFallback is set when Google put the whole site's data in the page
// block because the page itself has too few visits.
type rawLoadingExperience struct {
	Metrics        map[string]rawFieldMetric `json:"metrics"`
	OriginFallback bool                      `json:"origin_fallback"`
}

// pageSpeedFieldData keeps PageSpeed Insights' embedded field data: the page
// block, else the separate origin block. It returns nil when neither has
// metrics; PageSpeed Insights is dropping this data, so its absence is not
// read as "insufficient traffic".
func pageSpeedFieldData(page, origin *rawLoadingExperience) *StoredFieldData {
	source := page
	scope := "page"
	if source == nil || len(source.Metrics) == 0 {
		source, scope = origin, "origin"
	} else if source.OriginFallback {
		scope = "origin"
	}
	if source == nil {
		return nil
	}
	metrics := []StoredFieldMetric{}
	for _, name := range fieldMetrics {
		raw, ok := source.Metrics[name.pageSpeed]
		if !ok || raw.Percentile == nil || math.IsNaN(*raw.Percentile) {
			continue
		}
		value := *raw.Percentile
		if name.key == "cls" {
			// PageSpeed Insights reports CLS multiplied by 100.
			value /= 100
		}
		metric := StoredFieldMetric{Key: name.key, Percentile: value, Category: name.threshold.categorize(value)}
		if len(raw.Distributions) == 3 {
			metric.Good = raw.Distributions[0].Proportion
			metric.NeedsImprovement = raw.Distributions[1].Proportion
			metric.Poor = raw.Distributions[2].Proportion
		}
		metrics = append(metrics, metric)
	}
	return availableFieldData("pagespeed", scope, metrics, nil)
}
