package ga4

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/google"
	"github.com/toufiqqureshi/seomarine/backend/internal/gsc"
)

type testRow struct{ err error }

func (r testRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	values := []string{"properties/123", "Property", "America/Los_Angeles", "USD", "owner-1", "account-1"}
	for i, d := range dest {
		*(d.(*string)) = values[i]
	}
	return nil
}

type testDB struct{ err error }

func (d testDB) QueryRow(context.Context, string, ...any) pgx.Row { return testRow(d) }

type testConnections struct{ err error }

func (c testConnections) GetByProjectID(context.Context, string) (Connection, error) {
	if c.err != nil {
		return Connection{}, c.err
	}
	return Connection{PropertyID: "properties/123", PropertyDisplayName: "Property", PropertyTimeZone: "America/Los_Angeles", PropertyCurrencyCode: "USD", ConnectedByUserID: "owner-1", GA4AccountID: "account-1"}, nil
}

type testGoogle struct {
	response  ProviderResponse
	responses []ProviderResponse
	err       error
	requests  []google.APIRequest
}

type adminGoogle struct {
	requests       []google.APIRequest
	streamResponse any
}

func (g *adminGoogle) DoJSON(_ context.Context, request google.APIRequest) error {
	g.requests = append(g.requests, request)
	var body any
	switch {
	case strings.Contains(request.URL, "/dataStreams?"):
		body = g.streamResponse
		if body == nil {
			body = map[string]any{"dataStreams": []any{
				map[string]any{"name": "properties/123/dataStreams/17", "type": "WEB_DATA_STREAM", "displayName": "Web", "webStreamData": map[string]any{"measurementId": "G-TEST123", "defaultUri": "https://example.test"}},
				map[string]any{"name": "properties/123/dataStreams/22", "type": "ANDROID_APP_DATA_STREAM", "displayName": "Android"},
			}}
		}
	case strings.HasSuffix(request.URL, "/enhancedMeasurementSettings"):
		body = map[string]any{"streamEnabled": true, "siteSearchEnabled": false, "searchQueryParameter": "q"}
	case strings.Contains(request.URL, "/keyEvents?"):
		body = map[string]any{"keyEvents": []any{map[string]any{"eventName": "purchase", "countingMethod": "ONCE_PER_EVENT"}}}
	case strings.Contains(request.URL, "/customDimensions?"):
		body = map[string]any{"customDimensions": []any{map[string]any{"parameterName": "author", "displayName": "Author", "scope": "EVENT"}}}
	case strings.Contains(request.URL, "/customMetrics?"):
		body = map[string]any{"customMetrics": []any{map[string]any{"parameterName": "score", "displayName": "Score", "measurementUnit": "STANDARD", "scope": "EVENT"}}}
	default:
		return errors.New("unexpected Admin API URL")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, request.Response)
}

func reportCode(t *testing.T, err error) string {
	t.Helper()
	var reportFailure *reportError
	if !errors.As(err, &reportFailure) {
		t.Fatalf("error %v is not a report error", err)
	}
	return reportFailure.Code
}

func (g *testGoogle) DoJSON(_ context.Context, r google.APIRequest) error {
	g.requests = append(g.requests, r)
	if g.err != nil {
		return g.err
	}
	response := g.response
	if len(g.responses) > 0 {
		response = g.responses[0]
		g.responses = g.responses[1:]
	}
	*r.Response.(*ProviderResponse) = response
	return nil
}

func TestOrganicOverviewRunsBoundedOrganicReportsAndDiagnostics(t *testing.T) {
	metrics := []Name{{Name: "sessions"}, {Name: "activeUsers"}, {Name: "engagedSessions"}, {Name: "engagementRate"}, {Name: "keyEvents"}, {Name: "transactions"}, {Name: "purchaseRevenue"}}
	values := func(v ...string) []Value {
		out := make([]Value, len(v))
		for i := range v {
			out[i] = Value{Value: v[i]}
		}
		return out
	}
	row := func(values ...string) ProviderRow {
		return ProviderRow{MetricValues: func() []Value { return valuesOf(values) }()}
	}
	current := ProviderResponse{MetricHeaders: metrics, Rows: []ProviderRow{row("100", "80", "60", "0.6", "2", "1", "25")}}
	previous := ProviderResponse{MetricHeaders: metrics, Rows: []ProviderRow{row("90", "70", "50", "0.5", "6", "1", "15")}}
	trend := ProviderResponse{DimensionHeaders: []Name{{Name: "date"}}, MetricHeaders: metrics,
		Rows: []ProviderRow{{DimensionValues: values("20261001"), MetricValues: values("100", "80", "60", "0.6", "2", "1", "25")}}, RowCount: 1}
	googleClient := &testGoogle{responses: []ProviderResponse{current, previous, trend}}
	service := &Service{Connections: testConnections{}, Google: googleClient, Now: func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }}
	got, err := service.GetOrganicOverview(context.Background(), "project-1", OrganicOverviewInput{Trend: "daily"})
	if err != nil {
		t.Fatal(err)
	}
	if len(googleClient.requests) != 3 || len(googleClient.requests[0].Body.(APIRequest).Dimensions) != 0 {
		t.Fatalf("overview requests = %#v", googleClient.requests)
	}
	currentSummary := got["current"].(map[string]any)
	if currentSummary["sessions"] != float64(100) || len(got["diagnostics"].([]any)) != 1 {
		t.Fatalf("overview response = %#v", got)
	}
	firstRequest := googleClient.requests[0]
	body := firstRequest.Body.(APIRequest)
	if body.Limit != "1" || body.DimensionFilter == nil || body.Metrics[6].Name != "purchaseRevenue" {
		t.Fatalf("summary request = %#v", body)
	}
	trendBody := googleClient.requests[2].Body.(APIRequest)
	if trendBody.Limit != "1000" || trendBody.Dimensions[0].Name != "date" || trendBody.OrderBys[0].Dimension.DimensionName != "date" || trendBody.OrderBys[0].Metric != nil {
		t.Fatalf("trend request = %#v", trendBody)
	}
}

func TestMeasurementHealthMapsAdminConfigurationAndFlags(t *testing.T) {
	client := &adminGoogle{}
	service := &Service{Connections: testConnections{}, Google: client}
	got, err := service.GetMeasurementHealth(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	summary := got["summary"].(map[string]any)
	if summary["dataStreamCount"] != 2 || summary["webStreamCount"] != 1 || summary["keyEventCount"] != 1 || summary["customDimensionCount"] != 1 || summary["customMetricCount"] != 1 || summary["issueCount"] != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(client.requests) != 5 || client.requests[0].Method != http.MethodGet || client.requests[0].Provider != "google-analytics" || client.requests[0].AccountID != "account-1" {
		t.Fatalf("Admin API requests = %#v", client.requests)
	}
	if got["issues"].([]string)[0] != "site_search_measurement_disabled" {
		t.Fatalf("issues = %#v", got["issues"])
	}
}

func TestMeasurementHealthRejectsCrossPropertyDataStreamName(t *testing.T) {
	client := &adminGoogle{streamResponse: map[string]any{"dataStreams": []any{map[string]any{
		"name": "properties/999/dataStreams/17", "type": "WEB_DATA_STREAM",
	}}}}
	service := &Service{Connections: testConnections{}, Google: client}
	_, err := service.GetMeasurementHealth(context.Background(), "project-1")
	if reportCode(t, err) != "ga4_malformed_response" || len(client.requests) != 1 {
		t.Fatalf("error = %v; requests = %d", err, len(client.requests))
	}
}

type opportunityGSCStorage struct{ connection gsc.Connection }

func (s opportunityGSCStorage) GetByProjectID(context.Context, string, string) (gsc.Connection, error) {
	return s.connection, nil
}

type opportunityGSCClient struct {
	rows    []gsc.SearchRow
	request gsc.SearchRequest
}

func (c *opportunityGSCClient) QuerySearchAnalytics(_ context.Context, _ string, request gsc.SearchRequest) ([]gsc.SearchRow, error) {
	c.request = request
	return c.rows, nil
}

func TestSearchOpportunitiesJoinsAndScoresOrganicPages(t *testing.T) {
	gscClient := &opportunityGSCClient{rows: []gsc.SearchRow{
		{Keys: []string{"https://example.test/path/?campaign=x"}, Clicks: 10, Impressions: 100, CTR: .1, Position: 5},
		{Keys: []string{"https://example.test/unmatched"}, Clicks: 20, Impressions: 200, CTR: .1, Position: 4},
		{Keys: []string{"https://example.test/ignored"}, Impressions: 900, Position: 21},
	}}
	searchConsole := &gsc.Service{Connections: opportunityGSCStorage{connection: gsc.Connection{SiteURL: "https://example.test/"}},
		NewClient: func(string, string) gsc.SearchClient { return gscClient }}
	metrics := []Name{{Name: "sessions"}, {Name: "activeUsers"}, {Name: "engagedSessions"}, {Name: "engagementRate"}, {Name: "keyEvents"}, {Name: "sessionKeyEventRate"}, {Name: "transactions"}, {Name: "purchaseRevenue"}}
	googleClient := &testGoogle{response: ProviderResponse{DimensionHeaders: []Name{{Name: "hostName"}, {Name: "landingPage"}}, MetricHeaders: metrics,
		Rows: []ProviderRow{{DimensionValues: []Value{{Value: "example.test"}, {Value: "/path/"}}, MetricValues: valuesOf([]string{"50", "40", "30", "0.75", "0", "0", "1", "15"})}}, RowCount: 1}}
	service := &Service{Connections: testConnections{}, Google: googleClient, Now: func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }}
	got, err := service.GetSearchOpportunities(context.Background(), searchConsole, "org-1", "project-1", SearchOpportunityInput{})
	if err != nil {
		t.Fatal(err)
	}
	if gscClient.request.StartDate != "2026-09-10" || gscClient.request.EndDate != "2026-10-07" || gscClient.request.DataState != "final" || gscClient.request.RowLimit != 1000 {
		t.Fatalf("Search Console request = %+v", gscClient.request)
	}
	rows := got["rows"].([]map[string]any)
	if len(rows) != 2 || rows[0]["joinStatus"] != "joined" || rows[0]["normalizedPage"] != "example.test/path" || rows[0]["score"] != float64(100) || rows[1]["score"] != nil {
		t.Fatalf("opportunity rows = %#v", rows)
	}
	scoring := got["scoring"].(map[string]any)
	if scoring["businessValueMetric"] != "engagementRate" || scoring["engagementFallback"] != true {
		t.Fatalf("scoring = %#v", scoring)
	}
	coverage := got["coverage"].(map[string]any)
	if coverage["gscRowsConsidered"] != 3 || coverage["matchedRows"] != 1 || coverage["unmatchedGscRows"] != 1 {
		t.Fatalf("coverage = %#v", coverage)
	}
}

func TestNormalizePageKeyCanonicalizesHostAndDropsQuery(t *testing.T) {
	got, ok := normalizePageKey(" HTTPS://BÜCHER.example:443/path///?q=1#top ")
	if !ok || got != "xn--bcher-kva.example/path" {
		t.Fatalf("normalizePageKey() = %q, %v", got, ok)
	}
	if _, ok := normalizePageKey("(not set)"); ok {
		t.Fatal("placeholder URL was accepted")
	}
}

func valuesOf(values []string) []Value {
	out := make([]Value, len(values))
	for i := range values {
		out[i] = Value{Value: values[i]}
	}
	return out
}

func TestBuildDefinitions(t *testing.T) {
	cases := []struct {
		kind      ReportKind
		breakdown string
		dimension string
	}{
		{LandingPages, "landing_page", "hostName"}, {PagePerformance, "page", "hostName"}, {KeyEvents, "event", "eventName"},
		{TrafficAcquisition, "channel_group", "sessionDefaultChannelGroup"}, {EcommercePerformance, "item", "itemName"}, {SiteSearch, "search_term", "searchTerm"}, {AudienceBreakdown, "device", "deviceCategory"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			d, err := buildDefinition(ReportInput{Kind: tc.kind})
			if err != nil {
				t.Fatal(err)
			}
			if d.dimensions[0] != tc.dimension {
				t.Fatalf("dimension = %q, want %q", d.dimensions[0], tc.dimension)
			}
		})
	}
	if _, err := buildDefinition(ReportInput{Kind: TrafficAcquisition, AcquisitionBreakdown: "invalid"}); err == nil {
		t.Fatal("invalid breakdown accepted")
	}
}

func TestConnectionRepositoryMapsMissingRows(t *testing.T) {
	if _, err := (ConnectionRepository{DB: testDB{err: pgx.ErrNoRows}}).GetByProjectID(context.Background(), "project-1"); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("missing connection error = %v", err)
	}
	connection, err := (ConnectionRepository{DB: testDB{}}).GetByProjectID(context.Background(), "project-1")
	if err != nil || connection.PropertyID != "properties/123" {
		t.Fatalf("connection=%#v err=%v", connection, err)
	}
}

func TestNormalizeRestrictedMetricsAndRejectsMalformedValues(t *testing.T) {
	req := APIRequest{Dimensions: []Name{{Name: "landingPage"}}, Metrics: []Name{{Name: "sessions"}, {Name: "purchaseRevenue"}}}
	response := ProviderResponse{DimensionHeaders: []Name{{Name: "landingPage"}}, MetricHeaders: []Name{{Name: "sessions"}, {Name: "purchaseRevenue"}}, Rows: []ProviderRow{{DimensionValues: []Value{{Value: "/"}}, MetricValues: []Value{{Value: "3"}, {Value: "99"}}}}, Metadata: &Metadata{SchemaRestrictionResponse: &RestrictionResponse{ActiveMetricRestrictions: []MetricRestriction{{MetricName: "purchaseRevenue", RestrictedMetricTypes: []string{"COST_DATA"}}}}}}
	normalized, err := normalize(response, req)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Rows[0]["purchaseRevenue"] != nil {
		t.Fatalf("restricted metric = %#v", normalized.Rows[0]["purchaseRevenue"])
	}
	if !normalized.Metadata.HasLimitedData {
		t.Fatal("restriction must mark report as limited")
	}
	response.Rows[0].MetricValues[0].Value = "NaN"
	if _, err := normalize(response, req); err == nil {
		t.Fatal("non-finite metric accepted")
	}
}

func TestResolveDatesUsesPropertyZoneAndClampsFutureEnd(t *testing.T) {
	got, err := resolveDates(ReportInput{StartDate: "2026-08-01", EndDate: "2026-08-20"}, "America/Los_Angeles", time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.Resolved.EndDate != "2026-08-04" {
		t.Fatalf("end = %s", got.Resolved.EndDate)
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != "end_date_clamped" {
		t.Fatalf("warnings = %#v", got.Warnings)
	}
	if _, err := resolveDates(ReportInput{StartDate: "2026-08-02"}, "UTC", time.Now()); err == nil {
		t.Fatal("half date range accepted")
	}
}

func TestRunReportBuildsGA4RequestAndMapsProviderErrors(t *testing.T) {
	provider := &testGoogle{response: ProviderResponse{DimensionHeaders: []Name{{Name: "hostName"}, {Name: "landingPage"}}, MetricHeaders: []Name{{Name: "sessions"}, {Name: "activeUsers"}, {Name: "engagedSessions"}, {Name: "engagementRate"}, {Name: "keyEvents"}, {Name: "sessionKeyEventRate"}, {Name: "transactions"}, {Name: "purchaseRevenue"}}, Rows: []ProviderRow{{DimensionValues: []Value{{Value: "example.com"}, {Value: "/guide"}}, MetricValues: []Value{{Value: "8"}, {Value: "7"}, {Value: "6"}, {Value: "0.75"}, {Value: "2"}, {Value: "0.2"}, {Value: "1"}, {Value: "19.5"}}}}, RowCount: 1}}
	service := Service{Connections: testConnections{}, Google: provider, Now: func() time.Time { return time.Date(2026, 8, 6, 15, 0, 0, 0, time.UTC) }}
	zero := 0
	_, zeroErr := service.RunReport(context.Background(), "project-1", ReportInput{Kind: LandingPages, Limit: &zero})
	var reportFailure *reportError
	if !errors.As(zeroErr, &reportFailure) || reportFailure.Code != "validation_error" {
		t.Fatalf("explicit zero limit error = %v", zeroErr)
	}
	limit := 10
	result, err := service.RunReport(context.Background(), "project-1", ReportInput{Kind: LandingPages, Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("provider calls = %d", len(provider.requests))
	}
	req := provider.requests[0]
	if req.Provider != "google-analytics" || req.AccountID != "account-1" || req.UserID != "owner-1" {
		t.Fatalf("token scope = %#v", req)
	}
	if req.URL != "https://analyticsdata.googleapis.com/v1beta/properties/123:runReport" {
		t.Fatalf("URL = %s", req.URL)
	}
	apiReq := req.Body.(APIRequest)
	if apiReq.DateRanges[0].StartDate != "2026-07-09" || apiReq.DateRanges[0].EndDate != "2026-08-05" {
		t.Fatalf("default dates = %#v", apiReq.DateRanges)
	}
	if result["status"] != "ok" {
		t.Fatalf("status = %#v", result["status"])
	}
	rows := result["rows"].([]map[string]any)
	if rows[0]["purchaseRevenue"] != 19.5 {
		t.Fatalf("rows = %#v", rows)
	}
	if got := reportCode(t, mapProviderError(google.APIError{Status: http.StatusTooManyRequests})); got != "ga4_quota_exhausted" {
		t.Fatalf("mapped code = %s", got)
	}
	if got := reportCode(t, mapProviderError(errors.New("offline"))); got != "ga4_upstream_unavailable" {
		t.Fatalf("mapped code = %s", got)
	}
}

func TestReportHandlerRejectsUnknownFields(t *testing.T) {
	service := &Service{Connections: testConnections{}, Google: &testGoogle{}}
	withSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: "user-1"})))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{Logger: slog.Default(), Service: service, WithSession: withSession, WithProjectAccess: func(next http.Handler) http.Handler { return next }})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects/project-1/ga4/reports/run", strings.NewReader(`{"kind":"landing_pages","unknown":true}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewHandlersRejectUnknownFields(t *testing.T) {
	service := &Service{Connections: testConnections{}, Google: &testGoogle{}}
	withSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: "user-1"})))
		})
	}
	withProject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1")))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{Logger: slog.Default(), Service: service, WithSession: withSession, WithProjectAccess: withProject})
	for _, test := range []struct{ path, body string }{
		{"/api/v1/projects/project-1/ga4/overview/organic", `{"trend":"monthly"}`},
		{"/api/v1/projects/project-1/ga4/overview/organic", `{"startDate":null}`},
		{"/api/v1/projects/project-1/ga4/measurement-health", `{"unexpected":true}`},
		{"/api/v1/projects/project-1/ga4/search-opportunities", `{"unexpected":true}`},
		{"/api/v1/projects/project-1/ga4/search-opportunities", `{"limit":null}`},
	} {
		t.Run(test.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, test.path, strings.NewReader(test.body))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
