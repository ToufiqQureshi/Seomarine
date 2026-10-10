package ga4

import "fmt"

var definitions = map[ReportKind]reportDefinition{
	LandingPages:         {dimensions: []string{"hostName", "landingPage"}, metrics: []string{"sessions", "activeUsers", "engagedSessions", "engagementRate", "keyEvents", "sessionKeyEventRate", "transactions", "purchaseRevenue"}, orderMetric: "sessions"},
	PagePerformance:      {dimensions: []string{"hostName", "pagePath"}, metrics: []string{"screenPageViews", "activeUsers", "userEngagementDuration", "keyEvents"}, orderMetric: "screenPageViews"},
	KeyEvents:            {dimensions: []string{"eventName"}, metrics: []string{"keyEvents", "totalUsers"}, orderMetric: "keyEvents"},
	TrafficAcquisition:   {dimensions: []string{"sessionDefaultChannelGroup"}, metrics: []string{"sessions", "activeUsers", "engagedSessions", "engagementRate", "keyEvents", "transactions", "purchaseRevenue"}, orderMetric: "sessions"},
	EcommercePerformance: {dimensions: []string{"itemName", "itemId"}, metrics: []string{"itemsViewed", "itemsAddedToCart", "itemsPurchased", "itemRevenue"}, orderMetric: "itemRevenue"},
	SiteSearch:           {dimensions: []string{"searchTerm"}, metrics: []string{"eventCount", "activeUsers", "sessions", "engagedSessions", "engagementRate"}, orderMetric: "eventCount"},
	AudienceBreakdown:    {dimensions: []string{"deviceCategory"}, metrics: []string{"activeUsers", "sessions", "engagementRate", "keyEvents"}, orderMetric: "activeUsers"},
}

func buildDefinition(i ReportInput) (reportDefinition, error) {
	d, ok := definitions[i.Kind]
	if !ok {
		return reportDefinition{}, fmt.Errorf("unsupported report kind")
	}
	if i.Kind == EcommercePerformance && i.effectiveBreakdown() == "landing_page" {
		d = reportDefinition{dimensions: []string{"hostName", "landingPage"}, metrics: []string{"sessions", "transactions", "purchaseRevenue"}, orderMetric: "purchaseRevenue"}
	}
	switch i.Kind {
	case TrafficAcquisition:
		name := map[string]string{"channel_group": "sessionDefaultChannelGroup", "source_medium": "sessionSourceMedium", "campaign": "sessionCampaignName"}[i.effectiveBreakdown()]
		if name == "" {
			return reportDefinition{}, fmt.Errorf("invalid acquisition breakdown")
		}
		d.dimensions = []string{name}
	case AudienceBreakdown:
		name := map[string]string{"device": "deviceCategory", "country": "country", "new_vs_returning": "newVsReturning"}[i.effectiveBreakdown()]
		if name == "" {
			return reportDefinition{}, fmt.Errorf("invalid audience breakdown")
		}
		d.dimensions = []string{name}
	case KeyEvents:
		if i.effectiveBreakdown() == "event_and_landing_page" {
			d.dimensions = []string{"eventName", "hostName", "landingPage"}
		} else if i.effectiveBreakdown() != "event" {
			return reportDefinition{}, fmt.Errorf("invalid event breakdown")
		}
	case PagePerformance:
		if i.IncludeDate {
			d.dimensions = append(d.dimensions, "date")
		}
	case EcommercePerformance:
		if i.effectiveBreakdown() != "item" && i.effectiveBreakdown() != "landing_page" {
			return reportDefinition{}, fmt.Errorf("invalid ecommerce breakdown")
		}
	}
	return d, nil
}

func buildRequest(i ReportInput, dates DateRange, d reportDefinition, limit, offset int) APIRequest {
	request := APIRequest{DateRanges: []DateRange{dates}, Offset: fmt.Sprint(offset), Limit: fmt.Sprint(limit), KeepEmptyRows: false, ReturnPropertyQuota: true,
		OrderBys: []OrderBy{{Metric: MetricOrder{MetricName: d.orderMetric}, Desc: true}}}
	for _, n := range d.dimensions {
		request.Dimensions = append(request.Dimensions, Name{Name: n})
	}
	for _, n := range d.metrics {
		request.Metrics = append(request.Metrics, Name{Name: n})
	}
	if i.Kind == SiteSearch {
		request.DimensionFilter = map[string]any{"andGroup": map[string]any{"expressions": []any{
			map[string]any{"filter": map[string]any{"fieldName": "eventName", "stringFilter": map[string]any{"matchType": "EXACT", "value": "view_search_results"}}},
			map[string]any{"notExpression": map[string]any{"filter": map[string]any{"fieldName": "searchTerm", "stringFilter": map[string]any{"matchType": "EXACT", "value": "(not set)"}}}},
		}}}
	} else if i.Channel == "organic_search" {
		request.DimensionFilter = map[string]any{"filter": map[string]any{"fieldName": "sessionDefaultChannelGroup", "stringFilter": map[string]any{"matchType": "EXACT", "value": "Organic Search"}}}
	}
	if i.Kind == KeyEvents {
		request.MetricFilter = map[string]any{"filter": map[string]any{"fieldName": "keyEvents", "numericFilter": map[string]any{"operation": "GREATER_THAN", "value": map[string]any{"doubleValue": 0}}}}
	}
	if i.Kind == EcommercePerformance && i.effectiveBreakdown() == "landing_page" && i.EcommerceOnlyWithTransactions {
		request.MetricFilter = map[string]any{"filter": map[string]any{"fieldName": "transactions", "numericFilter": map[string]any{"operation": "GREATER_THAN", "value": map[string]any{"doubleValue": 0}}}}
	}
	return request
}
