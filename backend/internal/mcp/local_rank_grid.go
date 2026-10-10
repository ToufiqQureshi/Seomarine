package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/toufiqqureshi/seomarine/backend/internal/keywords"
)

const (
	rankGridDepth       = 20
	rankGridConcurrency = 3
	kmPerDegreeLatitude = 110.574
	kmPerDegreeLongitude = 111.32
	minLongitudeCosine  = 0.01
	rankGridZoomNumeratorKM = 24045
)

type localRankGridPoint struct {
	Row       int     `json:"row"`
	Col       int     `json:"col"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type localRankGridResult struct {
	localRankGridPoint
	Rank         *float64                `json:"rank"`
	ResultsCount *int                    `json:"resultsCount,omitempty"`
	TopResult    *localRankGridTopResult `json:"topResult,omitempty"`
	Error        bool                    `json:"error,omitempty"`
}

type localRankGridTopResult struct {
	Title *string `json:"title"`
	CID   *string `json:"cid"`
}

type localRankGridBusiness struct {
	Title   *string `json:"title"`
	CID     *string `json:"cid"`
	PlaceID *string `json:"placeId"`
}

type localRankGridSummary struct {
	PointsSearched int      `json:"pointsSearched"`
	PointsFound    int      `json:"pointsFound"`
	AverageRank    *float64 `json:"averageRank"`
	Top3Count      int      `json:"top3Count"`
	Top10Count     int      `json:"top10Count"`
}

func getLocalRankGridTool() *tool {
	return &tool{
		Name: "get_local_rank_grid",
		Title: "Get local rank grid",
		Description: "Runs one Google Maps search per point of a bounded grid around a coordinate and reports the target business rank at each point, plus the result count and top result. Charges credits per point.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"keyword":{"type":"string","minLength":1,"maxLength":120},"target":{"type":"object","properties":{"cid":{"type":"string","minLength":1,"maxLength":64},"placeId":{"type":"string","minLength":1,"maxLength":256},"name":{"type":"string","minLength":1,"maxLength":200}},"additionalProperties":false},"center":{"type":"object","properties":{"latitude":{"type":"number","minimum":-90,"maximum":90},"longitude":{"type":"number","minimum":-180,"maximum":180}},"required":["latitude","longitude"],"additionalProperties":false},"gridSize":{"type":"integer","enum":[3,5]},"spacingKm":{"type":"number","minimum":0.25,"maximum":10},"device":{"type":"string","enum":["desktop","mobile"]},"zoom":{"type":"integer","minimum":4,"maximum":18},"languageCode":{"type":"string","minLength":2,"maxLength":8}},"required":["projectId","keyword","target","center"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"grid":{"type":"array","items":{"type":"object"}},"summary":{"type":"object"},"matchedBusiness":{"type":["object","null"]}},"required":["grid","summary","matchedBusiness"],"additionalProperties":true}`),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false},
		Handler: handleGetLocalRankGrid,
	}
}

func handleGetLocalRankGrid(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string `json:"projectId"`
		Keyword string `json:"keyword"`
		Target *struct {
			CID string `json:"cid"`
			PlaceID string `json:"placeId"`
			Name string `json:"name"`
		} `json:"target"`
		Center struct {
			Latitude *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
		} `json:"center"`
		GridSize int `json:"gridSize"`
		SpacingKm *float64 `json:"spacingKm"`
		Device string `json:"device"`
		Zoom *int `json:"zoom"`
		LanguageCode string `json:"languageCode"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	if strings.TrimSpace(args.Keyword) == "" || len([]rune(args.Keyword)) > 120 {
		return nil, newAppErrorf("VALIDATION_ERROR", "keyword must contain 1 to 120 characters")
	}
	if args.Target == nil {
		return nil, newAppErrorf("VALIDATION_ERROR", "target must include at least one of cid, placeId, or name")
	}
	args.Target.CID = strings.TrimSpace(args.Target.CID)
	args.Target.PlaceID = strings.TrimSpace(args.Target.PlaceID)
	args.Target.Name = strings.TrimSpace(args.Target.Name)
	if args.Target.CID == "" && args.Target.PlaceID == "" && args.Target.Name == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "target must include at least one of cid, placeId, or name")
	}
	if len(args.Target.CID) > 64 || len(args.Target.PlaceID) > 256 || len([]rune(args.Target.Name)) > 200 {
		return nil, newAppErrorf("VALIDATION_ERROR", "target identifier is too long")
	}
	if args.Center.Latitude == nil || args.Center.Longitude == nil || *args.Center.Latitude < -90 || *args.Center.Latitude > 90 || *args.Center.Longitude < -180 || *args.Center.Longitude > 180 {
		return nil, newAppErrorf("VALIDATION_ERROR", "center coordinates are invalid")
	}
	gridSize := args.GridSize
	if gridSize == 0 {
		gridSize = 3
	}
	if gridSize != 3 && gridSize != 5 {
		return nil, newAppErrorf("VALIDATION_ERROR", "gridSize must be 3 or 5")
	}
	spacing := 2.0
	if args.SpacingKm != nil {
		spacing = *args.SpacingKm
	}
	if spacing < 0.25 || spacing > 10 {
		return nil, newAppErrorf("VALIDATION_ERROR", "spacingKm must be from 0.25 to 10")
	}
	device := strings.TrimSpace(args.Device)
	if device == "" {
		device = "mobile"
	}
	if device != "mobile" && device != "desktop" {
		return nil, newAppErrorf("VALIDATION_ERROR", "device must be mobile or desktop")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.KeywordResearch == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Local rank grid is not configured.")
	}
	provider, ok := env.deps.KeywordResearch.Data.(keywords.LocalSERPProvider)
	if !ok {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Local rank grid is not configured.")
	}
	language := strings.TrimSpace(args.LanguageCode)
	if language == "" {
		language = access.Project.LanguageCode
	}
	if len(language) < 2 || len(language) > 8 {
		return nil, newAppErrorf("VALIDATION_ERROR", "languageCode must be 2 to 8 characters")
	}
	zoom := 0
	if args.Zoom != nil {
		zoom = *args.Zoom
		if zoom < 4 || zoom > 18 {
			return nil, newAppErrorf("VALIDATION_ERROR", "zoom must be from 4 to 18")
		}
	} else {
		zoom = rankGridZoom(spacing, *args.Center.Latitude)
	}

	points := buildLocalRankGridPoints(*args.Center.Latitude, *args.Center.Longitude, gridSize, spacing)
	grid := make([]localRankGridResult, len(points))
	matchedRows := make([]*localRankGridBusiness, len(points))
	var firstErr error
	for start := 0; start < len(points); start += rankGridConcurrency {
		end := min(start+rankGridConcurrency, len(points))
		var wg sync.WaitGroup
		for i := start; i < end; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				point := points[index]
				coordinate := formatRankGridCoordinate(point.Latitude) + "," + formatRankGridCoordinate(point.Longitude) + fmt.Sprintf(",%dz", zoom)
				items, callErr := provider.LocalSERP(ctx, access.Auth.OrganizationID, keywords.LocalSERPInput{
					Keyword: args.Keyword, Coordinate: coordinate, LanguageCode: language,
					SearchType: "maps", Device: device, Depth: rankGridDepth,
				})
				if callErr != nil {
					grid[index] = localRankGridResult{localRankGridPoint: point, Error: true}
					return
				}
				result := localRankGridResult{localRankGridPoint: point}
				count := len(items)
				result.ResultsCount = &count
				if len(items) > 0 {
					result.TopResult = &localRankGridTopResult{
						Title: gridString(items[0], "title"),
						CID: gridString(items[0], "cid"),
					}
				}
				matched := matchLocalRankGridItem(items, args.Target.CID, args.Target.PlaceID, args.Target.Name)
				if matched != nil {
					matchedRows[index] = &localRankGridBusiness{
						Title: gridString(matched, "title"),
						CID: gridString(matched, "cid"),
						PlaceID: gridString(matched, "place_id"),
					}
					result.Rank = gridNumber(matched, "rank_absolute")
					if result.Rank == nil {
						result.Rank = gridNumber(matched, "rank_group")
					}
				}
				grid[index] = result
			}(i)
		}
		wg.Wait()
	}
	for _, point := range grid {
		if point.Error && firstErr == nil {
			firstErr = fmt.Errorf("one or more Google Maps grid searches failed")
		}
	}
	allFailed := len(grid) > 0
	for _, point := range grid {
		if !point.Error {
			allFailed = false
			break
		}
	}
	if allFailed {
		return nil, newAppErrorf("UPSTREAM_ERROR", firstErr.Error())
	}

	found := 0
	top3, top10 := 0, 0
	total := 0.0
	var matchedBusiness *localRankGridBusiness
	for _, point := range grid {
		if point.Rank == nil {
			continue
		}
		rank := *point.Rank
		found++
		total += rank
		if rank <= 3 {
			top3++
		}
		if rank <= 10 {
			top10++
		}
	}
	for _, matched := range matchedRows {
		if matched != nil {
			matchedBusiness = matched
			break
		}
	}
	// A rank grid can rank a location even if some points fail.
	avg := (*float64)(nil)
	if found > 0 {
		value := math.Round(total/float64(found)*100) / 100
		avg = &value
	}
	summary := localRankGridSummary{PointsSearched: len(grid), PointsFound: found, AverageRank: avg, Top3Count: top3, Top10Count: top10}
	lines := []string{
		fmt.Sprintf("Local rank grid for %q (%dx%d, %.2f km spacing, zoom %d, top %d checked).", args.Keyword, gridSize, gridSize, spacing, zoom, rankGridDepth),
		"Rank per point, north at the top (\"–\" = not among the results returned there; check resultsCount and topResult before reading it as outranked, \"x\" = search failed but may still be charged):",
		renderLocalRankGrid(grid, gridSize),
		fmt.Sprintf("- ranked at %d of %d points", summary.PointsFound, summary.PointsSearched),
		fmt.Sprintf("- average rank where found: %s", formatAverageRank(summary.AverageRank)),
		fmt.Sprintf("- top 3 at %d points, top 10 at %d points", summary.Top3Count, summary.Top10Count),
	}
	meta := metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID, nil)}
	return mcpResponse(strings.Join(lines, "\n"), map[string]any{
		"grid": grid, "summary": summary, "matchedBusiness": matchedBusiness,
	}, meta), nil
}

func buildLocalRankGridPoints(latitude, longitude float64, gridSize int, spacing float64) []localRankGridPoint {
	middle := float64(gridSize-1) / 2
	latitudeStep := spacing / kmPerDegreeLatitude
	cosine := math.Max(math.Abs(math.Cos(latitude*math.Pi/180)), minLongitudeCosine)
	longitudeStep := spacing / (kmPerDegreeLongitude * cosine)
	points := make([]localRankGridPoint, 0, gridSize*gridSize)
	for row := 0; row < gridSize; row++ {
		for col := 0; col < gridSize; col++ {
			points = append(points, localRankGridPoint{
				Row: row, Col: col,
				Latitude: math.Round((latitude+(middle-float64(row))*latitudeStep)*1e7) / 1e7,
				Longitude: math.Round((longitude+(float64(col)-middle)*longitudeStep)*1e7) / 1e7,
			})
		}
	}
	return points
}

func rankGridZoom(spacing, latitude float64) int {
	cosine := math.Max(math.Abs(math.Cos(latitude*math.Pi/180)), minLongitudeCosine)
	zoom := int(math.Floor(math.Log2((rankGridZoomNumeratorKM*cosine)/spacing)))
	return min(18, max(4, zoom))
}

func formatRankGridCoordinate(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.7f", value), "0"), ".")
}

func gridString(row map[string]any, key string) *string {
	value, ok := row[key].(string)
	if !ok {
		return nil
	}
	return &value
}

func gridNumber(row map[string]any, key string) *float64 {
	switch value := row[key].(type) {
	case float64:
		return &value
	case int:
		number := float64(value)
		return &number
	case int64:
		number := float64(value)
		return &number
	default:
		return nil
	}
}

func matchLocalRankGridItem(items []map[string]any, cid, placeID, name string) map[string]any {
	name = strings.ToLower(name)
	for _, item := range items {
		if cid != "" && item["cid"] == cid {
			return item
		}
		if placeID != "" && item["place_id"] == placeID {
			return item
		}
	}
	if name != "" {
		for _, item := range items {
			title, ok := item["title"].(string)
			if ok && strings.Contains(strings.ToLower(title), name) {
				return item
			}
		}
	}
	return nil
}

func renderLocalRankGrid(results []localRankGridResult, gridSize int) string {
	lines := make([]string, 0, gridSize)
	for row := 0; row < gridSize; row++ {
		cells := make([]string, 0, gridSize)
		for col := 0; col < gridSize; col++ {
			point := results[row*gridSize+col]
			cell := "–"
			if point.Error {
				cell = "x"
			} else if point.Rank != nil {
				cell = fmt.Sprintf("%g", *point.Rank)
			}
			cells = append(cells, fmt.Sprintf("%2s", cell))
		}
		lines = append(lines, strings.Join(cells, " "))
	}
	return strings.Join(lines, "\n")
}

func formatAverageRank(value *float64) string {
	if value == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f", *value)
}
