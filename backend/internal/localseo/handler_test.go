package localseo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/keywords"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/dataforseo"
)

type fakeData struct {
	calls   atomic.Int32
	fail    bool
	billing bool
}

func (f *fakeData) LocalSERP(_ context.Context, org string, in keywords.LocalSERPInput) ([]map[string]any, error) {
	if org != "org-1" || in.Depth != 20 || in.SearchType != "maps" {
		return nil, errors.New("bad provider arguments")
	}
	f.calls.Add(1)
	if f.billing {
		return nil, dataforseo.ErrBillingIssue
	}
	if f.fail {
		return nil, errors.New("provider unavailable")
	}
	return []map[string]any{{"title": "Harbor Dental", "cid": "target", "rank_absolute": float64(2)}}, nil
}
func (f *fakeData) BusinessListingsSearch(context.Context, string, keywords.BusinessListingsSearchInput) ([]map[string]any, error) {
	return nil, nil
}
func (f *fakeData) BusinessInfo(context.Context, string, keywords.BusinessInfoInput) (map[string]any, error) {
	return map[string]any{"title": "Harbor Dental"}, nil
}

func TestRankGridBoundsWorkAndSummarizesMatches(t *testing.T) {
	provider := &fakeData{}
	s := &Service{Data: provider}
	body := `{"keyword":"dentist","target":{"cid":"target"},"center":{"latitude":19.07,"longitude":72.87},"gridSize":3,"spacingKm":1,"device":"mobile"}`
	r := httptest.NewRequestWithContext(context.Background(), "POST", "/", strings.NewReader(body))
	r = r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1"))
	w := httptest.NewRecorder()
	s.rankGrid(w, r)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Grid    []localPoint `json:"grid"`
		Summary struct {
			PointsSearched int      `json:"pointsSearched"`
			PointsFound    int      `json:"pointsFound"`
			AverageRank    *float64 `json:"averageRank"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Grid) != 9 || provider.calls.Load() != 9 || got.Summary.PointsSearched != 9 || got.Summary.PointsFound != 9 || got.Summary.AverageRank == nil || *got.Summary.AverageRank != 2 {
		t.Fatalf("unexpected grid result: %+v calls=%d", got, provider.calls.Load())
	}
	if got.Grid[0].Latitude <= got.Grid[6].Latitude {
		t.Fatal("grid rows should descend from north to south")
	}
}

func TestRankGridRejectsInvalidInputBeforeProviderCall(t *testing.T) {
	provider := &fakeData{}
	s := &Service{Data: provider}
	for _, body := range []string{
		`{"keyword":"dentist","target":{"cid":"target"},"center":{"latitude":91,"longitude":0}}`,
		`{"keyword":"dentist","target":{"cid":"target"},"center":{"longitude":0}}`,
		`{"keyword":"dentist","target":{"cid":"target"},"center":{"latitude":0,"longitude":0},"gridSize":4}`,
		`{"keyword":"dentist","target":{},"center":{"latitude":0,"longitude":0}}`,
	} {
		r := httptest.NewRequestWithContext(context.Background(), "POST", "/", strings.NewReader(body))
		r = r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1"))
		w := httptest.NewRecorder()
		s.rankGrid(w, r)
		if w.Code != 400 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("invalid requests reached paid provider %d times", provider.calls.Load())
	}
}

func TestRankGridAllProviderFailuresReturnGatewayError(t *testing.T) {
	provider := &fakeData{fail: true}
	s := &Service{Data: provider}
	r := httptest.NewRequestWithContext(context.Background(), "POST", "/", strings.NewReader(`{"keyword":"dentist","target":{"name":"Harbor Dental"},"center":{"latitude":0,"longitude":0},"gridSize":3}`))
	r = r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1"))
	w := httptest.NewRecorder()
	s.rankGrid(w, r)
	if w.Code != 502 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if provider.calls.Load() != 9 {
		t.Fatalf("calls=%d", provider.calls.Load())
	}
}

func TestRankGridStopsAfterProviderBillingFailure(t *testing.T) {
	provider := &fakeData{billing: true}
	s := &Service{Data: provider}
	r := httptest.NewRequestWithContext(context.Background(), "POST", "/", strings.NewReader(`{"keyword":"dentist","target":{"name":"Harbor Dental"},"center":{"latitude":0,"longitude":0},"gridSize":5}`))
	r = r.WithContext(auth.WithProjectOrganization(r.Context(), "org-1"))
	w := httptest.NewRecorder()
	s.rankGrid(w, r)
	if w.Code != 402 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if provider.calls.Load() > 3 {
		t.Fatalf("billing failure should stop subsequent grid batches; provider calls=%d", provider.calls.Load())
	}
}
