package ga4

import (
	"fmt"
	"math"
	"strconv"
)

func normalize(response ProviderResponse, request APIRequest) (normalizedReport, error) {
	expectedDimensions, expectedMetrics := make([]string, 0, len(request.Dimensions)), make([]string, 0, len(request.Metrics))
	for _, item := range request.Dimensions {
		expectedDimensions = append(expectedDimensions, item.Name)
	}
	for _, item := range request.Metrics {
		expectedMetrics = append(expectedMetrics, item.Name)
	}
	dimensions, metrics := names(response.DimensionHeaders), names(response.MetricHeaders)
	headerlessEmpty := response.DimensionHeaders == nil && response.MetricHeaders == nil && len(response.Rows) == 0
	if !headerlessEmpty && (!sameStrings(dimensions, expectedDimensions) || !sameStrings(metrics, expectedMetrics)) {
		return normalizedReport{}, fmt.Errorf("malformed GA4 report headers")
	}
	restricted := make(map[string]MetricRestriction)
	restrictions := []MetricRestriction{}
	meta := Metadata{}
	if response.Metadata != nil {
		meta = *response.Metadata
	}
	if meta.SchemaRestrictionResponse != nil {
		for _, item := range meta.SchemaRestrictionResponse.ActiveMetricRestrictions {
			restricted[item.MetricName] = item
			restrictions = append(restrictions, item)
		}
	}
	rows := make([]map[string]any, 0, len(response.Rows))
	for _, row := range response.Rows {
		if len(row.DimensionValues) != len(dimensions) || len(row.MetricValues) != len(metrics) {
			return normalizedReport{}, fmt.Errorf("malformed GA4 report row")
		}
		normalized := make(map[string]any, len(dimensions)+len(metrics))
		for index, name := range dimensions {
			normalized[name] = row.DimensionValues[index].Value
		}
		for index, name := range metrics {
			if _, ok := restricted[name]; ok {
				normalized[name] = nil
				continue
			}
			value, err := strconv.ParseFloat(row.MetricValues[index].Value, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return normalizedReport{}, fmt.Errorf("malformed GA4 metric")
			}
			normalized[name] = value
		}
		rows = append(rows, normalized)
	}
	var emptyReason *string
	if meta.EmptyReason != "" {
		emptyReason = &meta.EmptyReason
	}
	limited := meta.DataLossFromOtherRow || meta.SubjectToThresholding || len(meta.SamplingMetadatas) > 0 || len(restricted) > 0
	metadata := reportMetadata{DataLossFromOtherRow: meta.DataLossFromOtherRow, SubjectToThresholding: meta.SubjectToThresholding, Sampling: meta.SamplingMetadatas, RestrictedMetrics: restrictions, EmptyReason: emptyReason, HasLimitedData: limited}
	if metadata.Sampling == nil {
		metadata.Sampling = []Sampling{}
	}
	count := response.RowCount
	if count == 0 && len(rows) > 0 {
		count = len(rows)
	}
	return normalizedReport{Rows: rows, TotalRowCount: count, Metadata: metadata, Quota: response.PropertyQuota}, nil
}

func names(values []Name) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = value.Name
	}
	return result
}
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
