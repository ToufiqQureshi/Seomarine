package ga4

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/google"
)

const ga4AdminBase = "https://analyticsadmin.googleapis.com"

type dataStreamsResponse struct {
	DataStreams []dataStream `json:"dataStreams"`
}
type dataStream struct {
	Name          string     `json:"name"`
	Type          string     `json:"type"`
	DisplayName   string     `json:"displayName"`
	CreateTime    string     `json:"createTime"`
	UpdateTime    string     `json:"updateTime"`
	WebStreamData *webStream `json:"webStreamData"`
}
type webStream struct {
	MeasurementID string `json:"measurementId"`
	DefaultURI    string `json:"defaultUri"`
}
type enhancedMeasurement struct {
	StreamEnabled           bool   `json:"streamEnabled"`
	ScrollsEnabled          bool   `json:"scrollsEnabled"`
	OutboundClicksEnabled   bool   `json:"outboundClicksEnabled"`
	SiteSearchEnabled       bool   `json:"siteSearchEnabled"`
	VideoEngagementEnabled  bool   `json:"videoEngagementEnabled"`
	FileDownloadsEnabled    bool   `json:"fileDownloadsEnabled"`
	PageChangesEnabled      bool   `json:"pageChangesEnabled"`
	FormInteractionsEnabled bool   `json:"formInteractionsEnabled"`
	SearchQueryParameter    string `json:"searchQueryParameter"`
	URIQueryParameter       string `json:"uriQueryParameter"`
}

// GetMeasurementHealth reads GA4 Admin configuration for the connected property.
func (s *Service) GetMeasurementHealth(ctx context.Context, projectID string) (map[string]any, error) {
	if s == nil || s.Connections == nil || s.Google == nil {
		return nil, reportErr("ga4_unavailable", "Google Analytics reporting is not set up on this server.")
	}
	connection, err := s.Connections.GetByProjectID(ctx, projectID)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			return nil, reportErr("ga4_not_connected", "Google Analytics is not connected for this project.")
		}
		return nil, fmt.Errorf("load GA4 connection: %w", err)
	}
	if !validPropertyID(connection.PropertyID) {
		return nil, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
	}
	base := ga4AdminBase + "/v1beta/" + connection.PropertyID
	var streams dataStreamsResponse
	if err := s.adminGet(ctx, connection, ga4AdminBase+"/v1alpha/"+connection.PropertyID+"/dataStreams?pageSize=200", &streams); err != nil {
		return nil, mapProviderError(err)
	}
	if len(streams.DataStreams) > 200 {
		return nil, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
	}
	if streams.DataStreams == nil {
		streams.DataStreams = []dataStream{}
	}
	webStreams := make([]map[string]any, 0)
	for _, stream := range streams.DataStreams {
		if stream.Name == "" || stream.Type == "" {
			return nil, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
		}
		if !validDataStreamName(connection.PropertyID, stream.Name) {
			return nil, reportErr("ga4_malformed_response", "Google Analytics returned an invalid report.")
		}
		if stream.Type != "WEB_DATA_STREAM" {
			continue
		}
		var settings enhancedMeasurement
		if err := s.adminGet(ctx, connection, ga4AdminBase+"/v1alpha/"+stream.Name+"/enhancedMeasurementSettings", &settings); err != nil {
			return nil, mapProviderError(err)
		}
		var measurementID, defaultURI, createTime, updateTime any
		if stream.WebStreamData != nil {
			measurementID, defaultURI = nullableString(stream.WebStreamData.MeasurementID), nullableString(stream.WebStreamData.DefaultURI)
		}
		createTime, updateTime = nullableString(stream.CreateTime), nullableString(stream.UpdateTime)
		webStreams = append(webStreams, map[string]any{
			"streamId": lastPathPart(stream.Name), "displayName": stream.DisplayName,
			"measurementId": measurementID, "defaultUri": defaultURI, "createTime": createTime, "updateTime": updateTime,
			"enhancedMeasurement": settings,
		})
	}
	var keyEvents struct {
		KeyEvents []map[string]any `json:"keyEvents"`
	}
	var dimensions struct {
		CustomDimensions []map[string]any `json:"customDimensions"`
	}
	var metrics struct {
		CustomMetrics []map[string]any `json:"customMetrics"`
	}
	for _, read := range []struct {
		url    string
		result any
	}{
		{base + "/keyEvents?pageSize=200", &keyEvents},
		{base + "/customDimensions?pageSize=200", &dimensions},
		{base + "/customMetrics?pageSize=200", &metrics},
	} {
		if err := s.adminGet(ctx, connection, read.url, read.result); err != nil {
			return nil, mapProviderError(err)
		}
	}
	issues := []string{}
	webCount := len(webStreams)
	if webCount == 0 {
		issues = append(issues, "no_web_stream")
	}
	streamEnabledCount, siteSearchEnabledCount := 0, 0
	for _, stream := range webStreams {
		settings := stream["enhancedMeasurement"].(enhancedMeasurement)
		if settings.StreamEnabled {
			streamEnabledCount++
			if settings.SiteSearchEnabled {
				siteSearchEnabledCount++
			}
		}
	}
	if webCount > 0 && streamEnabledCount == 0 {
		issues = append(issues, "enhanced_measurement_disabled")
	}
	if webCount > 0 && siteSearchEnabledCount == 0 {
		issues = append(issues, "site_search_measurement_disabled")
	}
	if len(keyEvents.KeyEvents) == 0 {
		issues = append(issues, "no_key_events_configured")
	}
	if keyEvents.KeyEvents == nil {
		keyEvents.KeyEvents = []map[string]any{}
	}
	if dimensions.CustomDimensions == nil {
		dimensions.CustomDimensions = []map[string]any{}
	}
	if metrics.CustomMetrics == nil {
		metrics.CustomMetrics = []map[string]any{}
	}
	otherStreams := make([]map[string]any, 0)
	for _, stream := range streams.DataStreams {
		if stream.Type != "WEB_DATA_STREAM" {
			otherStreams = append(otherStreams, map[string]any{"streamId": lastPathPart(stream.Name), "type": stream.Type, "displayName": stream.DisplayName})
		}
	}
	return map[string]any{
		"status":  "ok",
		"source":  map[string]any{"provider": "google_analytics_admin", "propertyId": connection.PropertyID, "propertyDisplayName": connection.PropertyDisplayName},
		"summary": map[string]any{"dataStreamCount": len(streams.DataStreams), "webStreamCount": webCount, "keyEventCount": len(keyEvents.KeyEvents), "customDimensionCount": len(dimensions.CustomDimensions), "customMetricCount": len(metrics.CustomMetrics), "issueCount": len(issues)},
		"issues":  issues, "webStreams": webStreams, "otherStreams": otherStreams,
		"keyEvents": keyEvents.KeyEvents, "customDefinitions": map[string]any{"dimensions": dimensions.CustomDimensions, "metrics": metrics.CustomMetrics},
	}, nil
}

func (s *Service) adminGet(ctx context.Context, connection Connection, endpoint string, response any) error {
	return s.Google.DoJSON(ctx, google.APIRequest{UserID: connection.ConnectedByUserID, Provider: "google-analytics", AccountID: connection.GA4AccountID, Method: http.MethodGet, URL: endpoint, Response: response, Retryable: true, MaxResponseBytes: 2 << 20})
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func lastPathPart(value string) string {
	if index := strings.LastIndexByte(value, '/'); index >= 0 {
		return value[index+1:]
	}
	return value
}

func validDataStreamName(propertyID, name string) bool {
	prefix := propertyID + "/dataStreams/"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	id := strings.TrimPrefix(name, prefix)
	if id == "" {
		return false
	}
	for _, char := range id {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
