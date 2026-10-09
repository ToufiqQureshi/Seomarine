package ga4

import (
	"context"
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
	response ProviderResponse
	err      error
	requests []google.APIRequest
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
	*r.Response.(*ProviderResponse) = g.response
	return nil
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/ga4/reports/run", strings.NewReader(`{"kind":"landing_pages","unknown":true}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
