// Package keywords serves keyword research, saved keywords, and SERP locations.
package keywords

import (
	_ "embed"
	"encoding/json"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// SerpLocation is one canonical DataForSEO city, county, or region.
type SerpLocation struct {
	LocationCode int    `json:"locationCode"`
	LocationName string `json:"locationName"`
	LocationType string `json:"locationType"`
	DisplayLabel string `json:"displayLabel"`
}

type locationSearchTable struct {
	Abbreviations map[string]map[string]string `json:"abbreviations"`
	TypeRanks     map[string]int               `json:"typeRanks"`
}

//go:embed location_search.json
var locationSearchRaw []byte

var locationSearchData = loadLocationSearchTable()

func loadLocationSearchTable() locationSearchTable {
	var table locationSearchTable
	if err := json.Unmarshal(locationSearchRaw, &table); err != nil {
		panic("invalid generated SERP location search data: " + err.Error())
	}
	return table
}

// FoldLocationText lowercases text, removes accents, and collapses whitespace.
func FoldLocationText(value string) string {
	var builder strings.Builder
	for _, char := range norm.NFD.String(value) {
		if unicode.Is(unicode.Mn, char) {
			continue
		}
		builder.WriteRune(unicode.ToLower(char))
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func locationTokens(query, country string) []string {
	tokens := strings.FieldsFunc(FoldLocationText(query), func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	abbreviations := locationSearchData.Abbreviations[strings.ToLower(country)]
	for index := 1; index < len(tokens); index++ {
		if expanded, ok := abbreviations[tokens[index]]; ok {
			tokens[index] = FoldLocationText(expanded)
		}
	}
	return tokens
}

// RankSerpLocations returns at most 50 matching canonical locations in relevance order.
func RankSerpLocations(query string, locations []SerpLocation, country string) []SerpLocation {
	tokens := locationTokens(query, country)
	if len(tokens) == 0 {
		return []SerpLocation{}
	}
	phrase := strings.Join(tokens, " ")
	type scored struct {
		location        SerpLocation
		score, typeRank int
		folded          string
	}
	rows := make([]scored, 0)
	for _, location := range locations {
		folded := FoldLocationText(location.LocationName)
		match := true
		for _, token := range tokens {
			if !strings.Contains(folded, token) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		place, _, _ := strings.Cut(folded, ",")
		place = strings.TrimSpace(place)
		score := 3
		switch {
		case place == phrase:
			score = 0
		case place == tokens[0]:
			score = 1
		case strings.HasPrefix(place, tokens[0]):
			score = 2
		}
		typeRank, ok := locationSearchData.TypeRanks[location.LocationType]
		if !ok {
			typeRank = 9
		}
		rows = append(rows, scored{location, score, typeRank, folded})
	}
	slices.SortFunc(rows, func(a, b scored) int {
		if a.score != b.score {
			return a.score - b.score
		}
		if a.typeRank != b.typeRank {
			return a.typeRank - b.typeRank
		}
		return strings.Compare(a.folded, b.folded)
	})
	if len(rows) > 50 {
		rows = rows[:50]
	}
	result := make([]SerpLocation, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.location)
	}
	return result
}
