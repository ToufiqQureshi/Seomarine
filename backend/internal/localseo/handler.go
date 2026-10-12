// Package localseo serves local search ranking grid requests.
package localseo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/keywords"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
)

// Data provides the local search data used by ranking grids.
type Data interface {
	keywords.LocalSERPProvider
	keywords.BusinessListingsSearchProvider
	keywords.BusinessInfoProvider
}

// Service computes local ranking grids from provider results.
type Service struct{ Data Data }
type localPoint struct {
	Row          int      `json:"row"`
	Col          int      `json:"col"`
	Latitude     float64  `json:"latitude"`
	Longitude    float64  `json:"longitude"`
	Rank         *float64 `json:"rank"`
	ResultsCount int      `json:"resultsCount"`
	Error        bool     `json:"error,omitempty"`
}
type gridRequest struct {
	Keyword string `json:"keyword"`
	Target  struct {
		CID     string `json:"cid"`
		PlaceID string `json:"placeId"`
		Name    string `json:"name"`
	} `json:"target"`
	Center struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"center"`
	GridSize     int     `json:"gridSize"`
	SpacingKm    float64 `json:"spacingKm"`
	Device       string  `json:"device"`
	LanguageCode string  `json:"languageCode"`
}

// Mount registers the local SEO grid endpoint.
func Mount(mux *http.ServeMux, service *Service, withSession, withProjectAccess func(http.Handler) http.Handler) {
	protect := func(h http.Handler) http.Handler { return withSession(withProjectAccess(h)) }
	mux.Handle("POST /api/v1/projects/{projectId}/local-seo/rank-grid", protect(http.HandlerFunc(service.rankGrid)))
	mux.Handle("POST /api/v1/projects/{projectId}/local-seo/businesses", protect(http.HandlerFunc(service.businesses)))
	mux.Handle("POST /api/v1/projects/{projectId}/local-seo/profile", protect(http.HandlerFunc(service.profile)))
}

func (s *Service) rankGrid(w http.ResponseWriter, r *http.Request) {
	var in gridRequest
	if err := httpx.DecodeJSONLimit(r.Body, 16<<10, &in); err != nil {
		httpx.WriteError(w, 400, "invalid_json", "Request body is invalid.")
		return
	}
	in.Keyword = strings.TrimSpace(in.Keyword)
	in.Target.Name = strings.TrimSpace(in.Target.Name)
	in.Target.CID = strings.TrimSpace(in.Target.CID)
	in.Target.PlaceID = strings.TrimSpace(in.Target.PlaceID)
	if len([]rune(in.Keyword)) < 1 || len([]rune(in.Keyword)) > 120 || (in.Target.CID == "" && in.Target.PlaceID == "" && in.Target.Name == "") || len(in.Target.CID) > 64 || len(in.Target.PlaceID) > 256 || len([]rune(in.Target.Name)) > 200 || in.Center.Latitude == nil || in.Center.Longitude == nil || math.IsNaN(*in.Center.Latitude) || math.IsNaN(*in.Center.Longitude) || *in.Center.Latitude < -90 || *in.Center.Latitude > 90 || *in.Center.Longitude < -180 || *in.Center.Longitude > 180 {
		httpx.WriteError(w, 400, "validation_error", "Keyword, target business and valid center coordinates are required.")
		return
	}
	if in.GridSize == 0 {
		in.GridSize = 3
	}
	if in.GridSize != 3 && in.GridSize != 5 {
		httpx.WriteError(w, 400, "validation_error", "Grid size must be 3 or 5.")
		return
	}
	if in.SpacingKm == 0 {
		in.SpacingKm = 2
	}
	if math.IsNaN(in.SpacingKm) || in.SpacingKm < .25 || in.SpacingKm > 10 {
		httpx.WriteError(w, 400, "validation_error", "Spacing must be between 0.25 and 10 km.")
		return
	}
	if in.Device == "" {
		in.Device = "mobile"
	}
	if in.Device != "mobile" && in.Device != "desktop" {
		httpx.WriteError(w, 400, "validation_error", "Device must be mobile or desktop.")
		return
	}
	if s == nil || s.Data == nil {
		httpx.WriteError(w, 503, "unavailable", "Local SEO data provider is not configured.")
		return
	}
	org, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, 404, "project_not_found", "Project not found.")
		return
	}
	lang := strings.TrimSpace(in.LanguageCode)
	if len(lang) < 2 || len(lang) > 8 {
		lang = "en"
	}
	centerLat, centerLon := *in.Center.Latitude, *in.Center.Longitude
	type cell struct {
		localPoint
		err error
	}
	cells := make([]cell, in.GridSize*in.GridSize)
	middle := float64(in.GridSize-1) / 2
	latStep := in.SpacingKm / 110.574
	cos := math.Max(math.Abs(math.Cos(centerLat*math.Pi/180)), .01)
	lonStep := in.SpacingKm / (111.32 * cos)
	for i := range cells {
		row, col := i/in.GridSize, i%in.GridSize
		cells[i].Row = row
		cells[i].Col = col
		cells[i].Latitude = math.Round((centerLat+(middle-float64(row))*latStep)*1e7) / 1e7
		cells[i].Longitude = math.Round((centerLon+(float64(col)-middle)*lonStep)*1e7) / 1e7
	}
	zoom := rankGridZoom(in.SpacingKm, centerLat)
	for start := 0; start < len(cells); start += 3 {
		end := min(start+3, len(cells))
		var wg sync.WaitGroup
		for i := start; i < end; i++ {
			wg.Add(1)
			go func(ctx context.Context, i int) {
				defer wg.Done()
				c := &cells[i]
				coord := fmt.Sprintf("%.7f,%.7f,%dz", c.Latitude, c.Longitude, zoom)
				items, err := s.Data.LocalSERP(ctx, org, keywords.LocalSERPInput{Keyword: in.Keyword, Coordinate: coord, LanguageCode: lang, SearchType: "maps", Device: in.Device, Depth: 20})
				c.err = err
				if err != nil {
					c.Error = true
					return
				}
				c.ResultsCount = len(items)
				for _, item := range items {
					if matches(item, in.Target.CID, in.Target.PlaceID, in.Target.Name) {
						rank := number(item["rank_absolute"])
						if rank == nil {
							rank = number(item["rank_group"])
						}
						c.Rank = rank
						break
					}
				}
			}(r.Context(), i)
		}
		wg.Wait()
		for i := start; i < end; i++ {
			if isBillingError(cells[i].err) {
				httpx.WriteError(w, http.StatusPaymentRequired, "insufficient_credits", "The data provider reported a billing or credit balance issue. Remaining grid searches were stopped.")
				return
			}
			if isAuthError(cells[i].err) {
				httpx.WriteError(w, http.StatusBadGateway, "provider_auth_failed", "The configured Maps data provider rejected its credentials.")
				return
			}
		}
		if r.Context().Err() != nil {
			httpx.WriteError(w, 499, "request_cancelled", "Request was cancelled.")
			return
		}
	}
	failed, found, top3, top10 := 0, 0, 0, 0
	total := 0.0
	out := make([]localPoint, len(cells))
	for i, c := range cells {
		out[i] = c.localPoint
		if c.Error {
			failed++
			continue
		}
		if c.Rank != nil {
			found++
			total += *c.Rank
			if *c.Rank <= 3 {
				top3++
			}
			if *c.Rank <= 10 {
				top10++
			}
		}
	}
	if failed == len(cells) {
		httpx.WriteError(w, 502, "upstream_error", "Google Maps rank grid could not be completed.")
		return
	}
	var avg *float64
	if found > 0 {
		v := math.Round(total/float64(found)*100) / 100
		avg = &v
	}
	httpx.WriteJSON(w, 200, map[string]any{"grid": out, "summary": map[string]any{"pointsSearched": len(out), "pointsFound": found, "averageRank": avg, "top3Count": top3, "top10Count": top10, "failedPoints": failed}, "estimatedRequests": len(out)})
}

func (s *Service) businesses(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query     string   `json:"query"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
		RadiusKm  float64  `json:"radiusKm"`
	}
	if err := httpx.DecodeJSONLimit(r.Body, 8<<10, &in); err != nil {
		httpx.WriteError(w, 400, "invalid_json", "Request body is invalid.")
		return
	}
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" || len([]rune(in.Query)) > 200 || in.Latitude == nil || in.Longitude == nil || *in.Latitude < -90 || *in.Latitude > 90 || *in.Longitude < -180 || *in.Longitude > 180 {
		httpx.WriteError(w, 400, "validation_error", "Business query and valid coordinates are required.")
		return
	}
	if in.RadiusKm == 0 {
		in.RadiusKm = 10
	}
	if in.RadiusKm < 1 || in.RadiusKm > 100 {
		httpx.WriteError(w, 400, "validation_error", "Radius must be from 1 to 100 km.")
		return
	}
	if s == nil || s.Data == nil {
		httpx.WriteError(w, 503, "unavailable", "Local SEO data provider is not configured.")
		return
	}
	org, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, 404, "project_not_found", "Project not found.")
		return
	}
	provider, ok := s.Data.(keywords.BusinessListingsSearchProvider)
	if !ok {
		httpx.WriteError(w, 503, "unavailable", "Business search is not configured.")
		return
	}
	items, err := provider.BusinessListingsSearch(r.Context(), org, keywords.BusinessListingsSearchInput{Title: in.Query, Coordinate: keywords.FormatBusinessListingsCoordinate(*in.Latitude, *in.Longitude, in.RadiusKm), Limit: 20})
	if err != nil {
		httpx.WriteError(w, 502, "upstream_error", "Business search failed. Try again.")
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	httpx.WriteJSON(w, 200, map[string]any{"businesses": items})
}

func (s *Service) profile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BusinessName string   `json:"businessName"`
		CID          string   `json:"cid"`
		PlaceID      string   `json:"placeId"`
		Latitude     *float64 `json:"latitude"`
		Longitude    *float64 `json:"longitude"`
		RadiusKm     float64  `json:"radiusKm"`
		LanguageCode string   `json:"languageCode"`
	}
	if err := httpx.DecodeJSONLimit(r.Body, 8<<10, &in); err != nil {
		httpx.WriteError(w, 400, "invalid_json", "Request body is invalid.")
		return
	}
	in.BusinessName = strings.TrimSpace(in.BusinessName)
	in.CID = strings.TrimSpace(in.CID)
	in.PlaceID = strings.TrimSpace(in.PlaceID)
	ids := 0
	keyword := ""
	if in.BusinessName != "" {
		ids++
		keyword = in.BusinessName
	}
	if in.CID != "" {
		ids++
		keyword = "cid:" + in.CID
	}
	if in.PlaceID != "" {
		ids++
		keyword = "place_id:" + in.PlaceID
	}
	if ids != 1 || len([]rune(in.BusinessName)) > 200 || len(in.CID) > 64 || len(in.PlaceID) > 256 || in.Latitude == nil || in.Longitude == nil || *in.Latitude < -90 || *in.Latitude > 90 || *in.Longitude < -180 || *in.Longitude > 180 {
		httpx.WriteError(w, 400, "validation_error", "Provide one business identifier and valid coordinates.")
		return
	}
	if in.RadiusKm == 0 {
		in.RadiusKm = 10
	}
	if in.RadiusKm < .2 || in.RadiusKm > 199 {
		httpx.WriteError(w, 400, "validation_error", "Radius must be from 0.2 to 199 km.")
		return
	}
	if s == nil || s.Data == nil {
		httpx.WriteError(w, 503, "unavailable", "Business profile data is not configured.")
		return
	}
	org, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, 404, "project_not_found", "Project not found.")
		return
	}
	profile, err := s.Data.BusinessInfo(r.Context(), org, keywords.BusinessInfoInput{Keyword: keyword, Coordinate: keywords.FormatBusinessDataCoordinate(*in.Latitude, *in.Longitude, in.RadiusKm), LanguageCode: validLanguage(in.LanguageCode)})
	if err != nil {
		httpx.WriteError(w, 502, "upstream_error", "Business profile lookup failed. Try again.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"profile": profile})
}

func validLanguage(v string) string {
	v = strings.TrimSpace(v)
	if len(v) < 2 || len(v) > 8 {
		return "en"
	}
	return v
}

func rankGridZoom(spacing, latitude float64) int {
	cosine := math.Max(math.Abs(math.Cos(latitude*math.Pi/180)), .01)
	zoom := int(math.Floor(math.Log2((24045 * cosine) / spacing)))
	return min(18, max(4, zoom))
}

func isBillingError(err error) bool {
	if errors.Is(err, dataforseo.ErrBillingIssue) {
		return true
	}
	var upstream *dataforseo.HTTPError
	if errors.As(err, &upstream) && upstream.StatusCode == http.StatusPaymentRequired {
		return true
	}
	var task *dataforseo.TaskError
	return errors.As(err, &task) && task.StatusCode == 40502
}

func isAuthError(err error) bool {
	var upstream *dataforseo.HTTPError
	if errors.As(err, &upstream) && (upstream.StatusCode == http.StatusUnauthorized || upstream.StatusCode == http.StatusForbidden) {
		return true
	}
	var task *dataforseo.TaskError
	return errors.As(err, &task) && (task.StatusCode == 40100 || task.StatusCode == 40102)
}

func matches(item map[string]any, cid, placeID, name string) bool {
	if cid != "" && fmt.Sprint(item["cid"]) == cid {
		return true
	}
	if placeID != "" && fmt.Sprint(item["place_id"]) == placeID {
		return true
	}
	title, _ := item["title"].(string)
	return name != "" && strings.Contains(strings.ToLower(title), strings.ToLower(name))
}
func number(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case int:
		f := float64(n)
		return &f
	case json.Number:
		f, e := n.Float64()
		if e == nil {
			return &f
		}
	}
	return nil
}
