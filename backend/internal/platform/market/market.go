// Package market resolves DataForSEO country and language combinations.
package market

import (
	_ "embed"
	"encoding/json"
	"strings"
)

// DefaultLocationCode is the product-wide United States fallback.
const DefaultLocationCode = 2840

// Location is a selectable country and its default keyword-data language.
type Location struct {
	Code          int    `json:"code"`
	Label         string `json:"label"`
	ShortLabel    string `json:"shortLabel"`
	LanguageCode  string `json:"languageCode"`
	GoogleAdsOnly bool   `json:"googleAdsOnly"`
}

// Language is a language supported by DataForSEO's Google SERP API.
type Language struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// Pair identifies the country and language used for one provider request.
type Pair struct {
	LocationCode int
	LanguageCode string
}

type table struct {
	Locations []Location       `json:"locations"`
	Languages []Language       `json:"languages"`
	Multi     map[int][]string `json:"multi"`
}

//go:embed data.json
var rawData []byte

var data = loadTable()

func loadTable() table {
	var result table
	if err := json.Unmarshal(rawData, &result); err != nil {
		panic("invalid generated market data: " + err.Error())
	}
	return result
}

// Locations returns a copy of supported country options.
func Locations() []Location { return append([]Location(nil), data.Locations...) }

// Languages returns a copy of supported SERP language options.
func Languages() []Language { return append([]Language(nil), data.Languages...) }

// Lookup returns a country by location code.
func Lookup(code int) (Location, bool) {
	for _, location := range data.Locations {
		if location.Code == code {
			return location, true
		}
	}
	return Location{}, false
}

// IsSupportedLanguageCode reports whether the SERP API accepts the language.
func IsSupportedLanguageCode(code string) bool {
	for _, language := range data.Languages {
		if language.Code == code {
			return true
		}
	}
	return false
}

// LanguageOptions returns the keyword-data languages served for a country.
func LanguageOptions(code int) []Language {
	codes := data.Multi[code]
	if len(codes) == 0 {
		codes = []string{LanguageCode(code)}
	}
	result := make([]Language, 0, len(codes))
	for _, language := range data.Languages {
		for _, allowed := range codes {
			if language.Code == allowed {
				result = append(result, language)
				break
			}
		}
	}
	return result
}

// LanguageCode returns the country's default language, or English for unknown codes.
func LanguageCode(code int) string {
	if location, ok := Lookup(code); ok {
		return location.LanguageCode
	}
	return "en"
}

// ISOCode returns the country code required by per-country provider endpoints.
func ISOCode(code int) string {
	location, ok := Lookup(code)
	if !ok {
		return "us"
	}
	if location.ShortLabel == "UK" {
		return "gb"
	}
	return strings.ToLower(location.ShortLabel)
}

// FormatLocationLabel normalizes comma spacing and optionally limits segments.
func FormatLocationLabel(name string, maxSegments int) string {
	parts := strings.Split(name, ",")
	if maxSegments > 0 && len(parts) > maxSegments {
		parts = parts[:maxSegments]
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, ", ")
}

// Resolve chooses request overrides or the project's default as one market pair.
func Resolve(request Pair, project Pair) Pair {
	result := project
	if request.LocationCode != 0 {
		result.LocationCode = request.LocationCode
		if result.LocationCode != project.LocationCode {
			result.LanguageCode = LanguageCode(result.LocationCode)
		}
	}
	if request.LanguageCode != "" {
		result.LanguageCode = request.LanguageCode
	}
	return result
}

// IsLabsLocationCode reports whether Labs serves country-level keyword data.
func IsLabsLocationCode(code int) bool {
	location, ok := Lookup(code)
	return ok && !location.GoogleAdsOnly
}

// KeywordDataProvider selects Labs or Google Ads for keyword data.
func KeywordDataProvider(code int) string {
	if location, ok := Lookup(code); ok && location.GoogleAdsOnly {
		return "google_ads"
	}
	return "labs"
}

// IsLanguageServedForLocation checks a Labs pair before a billed provider call.
func IsLanguageServedForLocation(code int, language string) bool {
	if KeywordDataProvider(code) != "labs" {
		return true
	}
	for _, option := range LanguageOptions(code) {
		if option.Code == language {
			return true
		}
	}
	return false
}

// ResolveLabs replaces an unsupported project default with the US market.
func ResolveLabs(request Pair, project Pair) Pair {
	if !IsLabsLocationCode(project.LocationCode) || !IsLanguageServedForLocation(project.LocationCode, project.LanguageCode) {
		project = Pair{DefaultLocationCode, "en"}
	}
	return Resolve(request, project)
}

// ResolveKeywordDataLanguage falls back to the country's default for unsupported languages.
func ResolveKeywordDataLanguage(code int, language string) string {
	for _, option := range LanguageOptions(code) {
		if option.Code == language {
			return language
		}
	}
	return LanguageCode(code)
}
