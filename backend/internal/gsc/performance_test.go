package gsc

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type performanceConnectionStub struct{ connection Connection }

func (s performanceConnectionStub) GetByProjectID(context.Context, string, string) (Connection, error) {
	return s.connection, nil
}

type performanceClientStub struct {
	mu       sync.Mutex
	requests []SearchRequest
	rows     []SearchRow
	err      error
}

func (s *performanceClientStub) QuerySearchAnalytics(_ context.Context, _ string, request SearchRequest) ([]SearchRow, error) {
	s.mu.Lock()
	s.requests = append(s.requests, request)
	s.mu.Unlock()
	return s.rows, s.err
}

func (s *performanceClientStub) requestSnapshot() []SearchRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SearchRequest(nil), s.requests...)
}

func TestResolveDateRangePresetsAndFloor(t *testing.T) {
	today := time.Date(2026, time.May, 28, 22, 0, 0, 0, time.FixedZone("offset", 7*60*60))
	tests := []struct {
		name  string
		input PerformanceInput
		start string
		end   string
	}{
		{name: "default 28 days stops before data lag", input: PerformanceInput{}, start: "2026-04-27", end: "2026-05-25"},
		{name: "three months clamps month end", input: PerformanceInput{DateRange: last3Months}, start: "2026-02-25", end: "2026-05-25"},
		{name: "explicit start respects history floor", input: PerformanceInput{StartDate: "2020-01-01", EndDate: "2026-05-25"}, start: "2025-01-28", end: "2026-05-25"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveDateRange(test.input, today)
			if err != nil {
				t.Fatal(err)
			}
			if got.StartDate != test.start || got.EndDate != test.end {
				t.Fatalf("range = %+v, want %s..%s", got, test.start, test.end)
			}
		})
	}
}

func TestValidatePerformanceInputAcceptsOnlyThreeAsciiLettersForCountry(t *testing.T) {
	valid := PerformanceInput{Country: "USA"}
	if err := validatePerformanceInput(&valid); err != nil || valid.Country != "usa" {
		t.Fatalf("valid country = %q, %v; want lowercase country code", valid.Country, err)
	}
	for _, country := range []string{"us1", "u$a"} {
		t.Run(country, func(t *testing.T) {
			input := PerformanceInput{Country: country}
			if err := validatePerformanceInput(&input); err == nil {
				t.Fatalf("country %q passed validation", country)
			}
		})
	}
}

func TestBuildSearchAnalyticsRequestUsesConjunctiveDimensionFilters(t *testing.T) {
	filter := PerformanceInput{
		StartDate: "2026-04-01", EndDate: "2026-04-30", Device: "MOBILE", Country: "usa",
		PageFilter:  &TextFilter{Operator: "contains", Expression: "https://example.test/blog"},
		QueryFilter: &TextFilter{Operator: "equals", Expression: "search term"},
	}
	request, err := BuildSearchAnalyticsRequest(filter, []string{"query", "page"}, 51, 50, time.Date(2026, time.May, 28, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if request.StartDate != "2026-04-01" || request.EndDate != "2026-04-30" || request.StartRow != 50 || request.RowLimit != 51 || request.Type != "web" || request.DataState != "all" {
		t.Fatalf("unexpected request: %+v", request)
	}
	if len(request.DimensionFilterGroups) != 1 || request.DimensionFilterGroups[0].GroupType != "and" || len(request.DimensionFilterGroups[0].Filters) != 4 {
		t.Fatalf("filters were not sent as one AND group: %+v", request.DimensionFilterGroups)
	}
}

func TestSearchPerformanceShaping(t *testing.T) {
	totals := sumSearchTotals([]SearchRow{{Clicks: 3, Impressions: 10, Position: 2}, {Clicks: 1, Impressions: 30, Position: 6}})
	if totals.Clicks != 4 || totals.Impressions != 40 || totals.CTR != .1 || totals.Position != 5 {
		t.Fatalf("totals = %+v", totals)
	}
	rows := buildStrikingDistanceRows([]SearchRow{
		{Keys: []string{"term", "worse"}, Position: 9, Impressions: 50},
		{Keys: []string{"term", "best"}, Position: 7, Impressions: 20},
		{Keys: []string{"tie", "first"}, Position: 10, Impressions: 10},
		{Keys: []string{"tie", "more"}, Position: 10, Impressions: 11},
		{Keys: []string{"top", "page"}, Position: 4, Impressions: 100},
	}, 100)
	if len(rows) != 2 || rows[0].Query != "term" || rows[0].Page != "best" || rows[1].Page != "more" {
		t.Fatalf("striking distance rows = %+v", rows)
	}
}

func TestGetTableUsesOneExtraRowAndKeepsUnknownTotalWhenPastEnd(t *testing.T) {
	client := &performanceClientStub{rows: []SearchRow{{Keys: []string{"a"}}, {Keys: []string{"b"}}, {Keys: []string{"c"}}}}
	service := &Service{
		Connections: performanceConnectionStub{connection: Connection{SiteURL: "sc-domain:example.test", ConnectedByUserID: "user"}},
		NewClient:   func(string, string) SearchClient { return client },
		Now:         func() time.Time { return time.Date(2026, time.May, 28, 0, 0, 0, 0, time.UTC) },
	}
	input := TableInput{PerformanceInput: PerformanceInput{}, Dimension: "query", Page: OptionalPageInt{Value: 2, Present: true}, PageSize: OptionalPageInt{Value: 25, Present: true}}
	result, err := service.GetTable(context.Background(), "org", "project", input)
	if err != nil {
		t.Fatal(err)
	}
	requests := client.requestSnapshot()
	if requests[0].RowLimit != 26 || requests[0].StartRow != 25 {
		t.Fatalf("request = %+v", requests[0])
	}
	if result["totalCount"] != 28 || result["hasNextPage"] != false {
		t.Fatalf("result = %+v", result)
	}

	client.rows = nil
	result, err = service.GetTable(context.Background(), "org", "project", input)
	if err != nil {
		t.Fatal(err)
	}
	if result["totalCount"] != nil {
		t.Fatalf("past-end totalCount = %#v, want null", result["totalCount"])
	}
}

func TestGetPerformanceBuildsLegacyReportAndOmitsCountryFilterForCountryRows(t *testing.T) {
	client := &performanceClientStub{rows: []SearchRow{{Keys: []string{"term", "https://example.test/"}, Clicks: 2, Impressions: 10, CTR: .2, Position: 6}}}
	service := &Service{
		Connections: performanceConnectionStub{connection: Connection{SiteURL: "https://example.test/", ConnectedByUserID: "user"}},
		NewClient:   func(string, string) SearchClient { return client },
		Now:         func() time.Time { return time.Date(2026, time.May, 28, 12, 0, 0, 0, time.UTC) },
	}
	result, err := service.GetPerformance(context.Background(), "org", "project", PerformanceInput{DateRange: last7Days, Country: "usa"})
	if err != nil {
		t.Fatal(err)
	}
	requests := client.requestSnapshot()
	if len(requests) != 4 {
		t.Fatalf("got %d provider requests, want 4", len(requests))
	}
	currentFound, previousFound, countryFound := false, false, false
	for _, request := range requests {
		rangeValue := request.StartDate + ".." + request.EndDate
		switch {
		case rangeValue == "2026-05-18..2026-05-25" && len(request.DimensionFilterGroups) == 1:
			currentFound = true
		case rangeValue == "2026-05-10..2026-05-17":
			previousFound = true
		case len(request.Dimensions) == 1 && request.Dimensions[0] == "country":
			countryFound = true
		}
	}
	if !currentFound || !previousFound || !countryFound {
		t.Fatalf("report requests did not include expected ranges/breakdown: %+v", requests)
	}
	for _, request := range requests {
		if len(request.Dimensions) == 1 && request.Dimensions[0] == "country" && len(request.DimensionFilterGroups) != 0 {
			t.Fatalf("country breakdown must omit country filter: %+v", request.DimensionFilterGroups)
		}
	}
	if result["connected"] != true {
		t.Fatalf("result = %+v", result)
	}
}

func TestDecodePerformanceRejectsNonObjectUnknownFieldsAndTrailingJSON(t *testing.T) {
	tests := []string{`null`, `[]`, `{"unknown":true}`, `{} {}`, `{"device":null}`, `{"pageFilter":{"operator":null,"expression":"x"}}`}
	for _, body := range tests {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(body))
			var input PerformanceInput
			if err := decodePerformance(httptest.NewRecorder(), req, &input); err == nil {
				t.Fatal("expected invalid request")
			}
		})
	}
}

func TestTableInputRejectsNullAndNonPositivePageValues(t *testing.T) {
	for _, body := range []string{`{"dimension":"query","page":null}`, `{"dimension":"query","page":0}`, `{"dimension":"query","pageSize":0}`} {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(body))
			var input TableInput
			if err := decodePerformance(httptest.NewRecorder(), req, &input); err != nil {
				if strings.Contains(body, "null") {
					return
				}
				t.Fatal(err)
			}
			if input.Page.Present && input.Page.Value < 1 || input.PageSize.Present && input.PageSize.Value < 1 {
				return
			}
			t.Fatal("expected invalid paging value")
		})
	}
}

func TestMapProviderErrors(t *testing.T) {
	if got := mapProviderError(errors.New("opaque transport detail")); got == nil {
		t.Fatal("expected error")
	}
}
