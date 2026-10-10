// Package ga4 implements the project's read-only Google Analytics reports.
package ga4

import (
	"errors"
	"time"
)

// ReportKind identifies one of the supported GA4 report types.
type ReportKind string

// GA4 report kinds exposed by the reporting endpoint.
const (
	LandingPages         ReportKind = "landing_pages"
	PagePerformance      ReportKind = "page_performance"
	KeyEvents            ReportKind = "key_events"
	TrafficAcquisition   ReportKind = "traffic_acquisition"
	EcommercePerformance ReportKind = "ecommerce_performance"
	SiteSearch           ReportKind = "site_search"
	AudienceBreakdown    ReportKind = "audience_breakdown"
)

// ReportInput controls the requested GA4 report and its date and paging range.
type ReportInput struct {
	Kind                          ReportKind `json:"kind"`
	StartDate                     string     `json:"startDate,omitempty"`
	EndDate                       string     `json:"endDate,omitempty"`
	Limit                         *int       `json:"limit,omitempty"`
	Offset                        int        `json:"offset,omitempty"`
	Channel                       string     `json:"channel,omitempty"`
	IncludeDate                   bool       `json:"includeDate,omitempty"`
	Breakdown                     string     `json:"breakdown,omitempty"`
	AcquisitionBreakdown          string     `json:"acquisitionBreakdown,omitempty"`
	EcommerceBreakdown            string     `json:"ecommerceBreakdown,omitempty"`
	EcommerceOnlyWithTransactions bool       `json:"ecommerceOnlyWithTransactions,omitempty"`
	AudienceBreakdown             string     `json:"audienceBreakdown,omitempty"`
	ComparePreviousPeriod         bool       `json:"comparePreviousPeriod,omitempty"`
}

// Connection contains the property and OAuth grant selected for a project.
type Connection struct {
	PropertyID, PropertyDisplayName, PropertyTimeZone, PropertyCurrencyCode string
	ConnectedByUserID, GA4AccountID                                         string
}

// APIRequest is one Data API runReport request after server-side validation.
type APIRequest struct {
	DateRanges          []DateRange `json:"dateRanges"`
	Dimensions          []Name      `json:"dimensions"`
	Metrics             []Name      `json:"metrics"`
	DimensionFilter     any         `json:"dimensionFilter,omitempty"`
	MetricFilter        any         `json:"metricFilter,omitempty"`
	Offset              string      `json:"offset"`
	Limit               string      `json:"limit"`
	OrderBys            []OrderBy   `json:"orderBys"`
	KeepEmptyRows       bool        `json:"keepEmptyRows"`
	ReturnPropertyQuota bool        `json:"returnPropertyQuota"`
}

// DateRange is an inclusive GA4 date range in YYYY-MM-DD form.
type DateRange struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// Name names a GA4 dimension or metric.
type Name struct {
	Name string `json:"name"`
}

// OrderBy configures descending ordering for the requested report metric.
type OrderBy struct {
	Metric    *MetricOrder    `json:"metric,omitempty"`
	Dimension *DimensionOrder `json:"dimension,omitempty"`
	Desc      bool            `json:"desc,omitempty"`
}

// MetricOrder identifies the metric used to sort the report.
type MetricOrder struct {
	MetricName string `json:"metricName"`
}

// DimensionOrder identifies a dimension used to order a trend report.
type DimensionOrder struct {
	DimensionName string `json:"dimensionName"`
}

// ProviderResponse is the bounded GA4 Data API response used by normalization.
type ProviderResponse struct {
	DimensionHeaders []Name        `json:"dimensionHeaders"`
	MetricHeaders    []Name        `json:"metricHeaders"`
	Rows             []ProviderRow `json:"rows"`
	RowCount         int           `json:"rowCount"`
	Metadata         *Metadata     `json:"metadata"`
	PropertyQuota    *Quota        `json:"propertyQuota"`
}

// ProviderRow contains positional dimension and metric values from GA4.
type ProviderRow struct {
	DimensionValues []Value `json:"dimensionValues"`
	MetricValues    []Value `json:"metricValues"`
}

// Value is one string-encoded GA4 dimension or metric value.
type Value struct {
	Value string `json:"value"`
}

// Metadata describes sampling, thresholding, restrictions and empty reports.
type Metadata struct {
	DataLossFromOtherRow      bool                 `json:"dataLossFromOtherRow"`
	SubjectToThresholding     bool                 `json:"subjectToThresholding"`
	SamplingMetadatas         []Sampling           `json:"samplingMetadatas"`
	SchemaRestrictionResponse *RestrictionResponse `json:"schemaRestrictionResponse"`
	EmptyReason               string               `json:"emptyReason"`
}

// Sampling records the provider's sampled row counts.
type Sampling struct {
	SamplesReadCount  string `json:"samplesReadCount"`
	SamplingSpaceSize string `json:"samplingSpaceSize"`
}

// RestrictionResponse describes metrics hidden by GA4 schema restrictions.
type RestrictionResponse struct {
	ActiveMetricRestrictions []MetricRestriction `json:"activeMetricRestrictions"`
}

// MetricRestriction names a metric that GA4 restricts for this property.
type MetricRestriction struct {
	MetricName            string   `json:"metricName"`
	RestrictedMetricTypes []string `json:"restrictedMetricTypes"`
}

// QuotaStatus is one reported Google Analytics request quota.
type QuotaStatus struct {
	Consumed  int `json:"consumed"`
	Remaining int `json:"remaining"`
}

// Quota contains the Google Analytics Data API property quota snapshot.
type Quota struct {
	TokensPerDay                          *QuotaStatus `json:"tokensPerDay,omitempty"`
	TokensPerHour                         *QuotaStatus `json:"tokensPerHour,omitempty"`
	ConcurrentRequests                    *QuotaStatus `json:"concurrentRequests,omitempty"`
	ServerErrorsPerProjectPerHour         *QuotaStatus `json:"serverErrorsPerProjectPerHour,omitempty"`
	PotentiallyThresholdedRequestsPerHour *QuotaStatus `json:"potentiallyThresholdedRequestsPerHour,omitempty"`
	TokensPerProjectPerHour               *QuotaStatus `json:"tokensPerProjectPerHour,omitempty"`
}

type reportDefinition struct {
	dimensions, metrics []string
	orderMetric         string
}
type normalizedReport struct {
	Rows          []map[string]any
	TotalRowCount int
	Metadata      reportMetadata
	Quota         *Quota
}
type reportMetadata struct {
	DataLossFromOtherRow  bool                `json:"dataLossFromOtherRow"`
	SubjectToThresholding bool                `json:"subjectToThresholding"`
	Sampling              []Sampling          `json:"sampling"`
	RestrictedMetrics     []MetricRestriction `json:"restrictedMetrics"`
	EmptyReason           *string             `json:"emptyReason"`
	HasLimitedData        bool                `json:"hasLimitedData"`
}
type reportError struct {
	Code, Message     string
	RetryAfterSeconds *int
}

func (e *reportError) Error() string { return e.Message }

// PublicError unwraps the stable user-safe errors for API and MCP callers.
func PublicError(err error) (code, message string, retryAfterSeconds *int, ok bool) {
	var target *reportError
	if !errors.As(err, &target) {
		return "", "", nil, false
	}
	return target.Code, target.Message, target.RetryAfterSeconds, true
}

type dateResolution struct {
	Requested *DateRange `json:"requestedDateRange"`
	Resolved  DateRange  `json:"resolvedDateRange"`
	Warnings  []string   `json:"warnings"`
}

func (i ReportInput) effectiveBreakdown() string {
	switch i.Kind {
	case LandingPages:
		return "landing_page"
	case PagePerformance:
		if i.IncludeDate {
			return "page_and_date"
		}
		return "page"
	case KeyEvents:
		if i.Breakdown == "" {
			return "event"
		}
		return i.Breakdown
	case TrafficAcquisition:
		if i.AcquisitionBreakdown == "" {
			return "channel_group"
		}
		return i.AcquisitionBreakdown
	case EcommercePerformance:
		if i.EcommerceBreakdown == "" {
			return "item"
		}
		return i.EcommerceBreakdown
	case SiteSearch:
		return "search_term"
	case AudienceBreakdown:
		if i.AudienceBreakdown == "" {
			return "device"
		}
		return i.AudienceBreakdown
	default:
		return ""
	}
}

func dayInZone(now time.Time, zone string) (string, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return "", err
	}
	return now.In(loc).Format("2006-01-02"), nil
}
