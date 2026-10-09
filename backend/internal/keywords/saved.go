package keywords

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

// Limits shared with the legacy app.
const (
	MaxSaveKeywords    = 500
	MaxTagsPerRequest  = 20
	MaxTagNameUnits    = 64
	MaxBatchIDs        = 2000
	MaxRemoveTagIDs    = 50
	MaxListTerms       = 20
	MaxFilterTags      = 50
	DefaultSavedPage   = 50
	MaxSearchTextUnits = 200
)

// Errors the saved keyword service returns.
var (
	ErrTagNotFound = errors.New("tag not found")
	ErrTagExists   = errors.New("a tag with that name already exists")
)

// ValidationError is a message that is safe to show the caller.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// TagInUseError means a tag still labels keywords and so was not deleted.
type TagInUseError struct{ AssignmentCount int }

func (e *TagInUseError) Error() string {
	plural := "s"
	if e.AssignmentCount == 1 {
		plural = ""
	}
	return fmt.Sprintf("Tag is attached to %d keyword%s. Remove the tag from those keywords first.", e.AssignmentCount, plural)
}

// TagColors are the palette keys a tag may use.
var TagColors = []string{"slate", "rose", "amber", "lime", "emerald", "sky", "violet", "fuchsia"}

// Intents a keyword metric may carry.
var intents = []string{"informational", "commercial", "transactional", "navigational", "unknown"}

// SortFields a saved keyword list may use.
var SortFields = []string{"createdAt", "keyword", "searchVolume", "cpc", "competition", "keywordDifficulty", "fetchedAt"}

// PageSizes a saved keyword list may use.
var PageSizes = []int{50, 100, 250}

// Tag labels saved keywords.
type Tag struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	NormalizedName string  `json:"normalizedName"`
	Color          *string `json:"color"`
}

// TagSummary is a tag with the number of keywords it labels.
type TagSummary struct {
	Tag
	KeywordCount int `json:"keywordCount"`
}

// MonthlySearch is one month of search volume.
type MonthlySearch struct {
	Year         int `json:"year"`
	Month        int `json:"month"`
	SearchVolume int `json:"searchVolume"`
}

// Metric is the research data of one keyword.
type Metric struct {
	Keyword           string          `json:"keyword"`
	SearchVolume      *int            `json:"searchVolume"`
	CPC               *float64        `json:"cpc"`
	Competition       *float64        `json:"competition"`
	KeywordDifficulty *int            `json:"keywordDifficulty"`
	Intent            *string         `json:"intent"`
	MonthlySearches   []MonthlySearch `json:"monthlySearches"`
}

// SavedKeyword is a saved keyword with its latest metrics and tags.
type SavedKeyword struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"projectId"`
	Keyword           string          `json:"keyword"`
	LocationCode      int             `json:"locationCode"`
	LanguageCode      string          `json:"languageCode"`
	CreatedAt         string          `json:"createdAt"`
	SearchVolume      *int            `json:"searchVolume"`
	CPC               *float64        `json:"cpc"`
	Competition       *float64        `json:"competition"`
	KeywordDifficulty *int            `json:"keywordDifficulty"`
	Intent            *string         `json:"intent"`
	MonthlySearches   []MonthlySearch `json:"monthlySearches"`
	FetchedAt         *string         `json:"fetchedAt"`
	Tags              []Tag           `json:"tags"`
}

// SaveInput saves keywords for a project in one market.
type SaveInput struct {
	ProjectID    string
	LocationCode int
	LanguageCode string
	Keywords     []string
	Tags         []string
	ReplaceTags  bool
	Metrics      []Metric
}

// ListQuery filters, sorts and pages a project's saved keywords. A zero
// PageSize returns every match, which is what an export needs.
type ListQuery struct {
	ProjectID     string
	Search        string
	IncludeTerms  []string
	ExcludeTerms  []string
	MinVolume     *int
	MaxVolume     *int
	MinCPC        *float64
	MaxCPC        *float64
	MinDifficulty *int
	MaxDifficulty *int
	TagIDs        []string
	TagNames      []string
	Page          int
	PageSize      int
	Sort          string
	Descending    bool
}

// ListResult is one page of saved keywords.
type ListResult struct {
	Rows       []SavedKeyword `json:"rows"`
	TotalCount int            `json:"totalCount"`
	Tags       []TagSummary   `json:"tags"`
}

// TagAssignment reports a tag edit across keywords.
type TagAssignment struct {
	SavedKeywordCount int
	Tags              []Tag
	RemovedCount      int
}

// SavedStore is the storage the saved keyword service needs.
type SavedStore interface {
	Save(ctx context.Context, in SaveInput) ([]string, error)
	List(ctx context.Context, q ListQuery) (ListResult, error)
	AddTags(ctx context.Context, projectID string, savedIDs, tagNames []string) (TagAssignment, error)
	RemoveTags(ctx context.Context, projectID string, savedIDs, tagIDs []string) (TagAssignment, error)
	UpdateTag(ctx context.Context, projectID, tagID string, name *string, color **string) (*Tag, error)
	DeleteTag(ctx context.Context, projectID, tagID string) error
	Remove(ctx context.Context, projectID string, savedIDs []string) (int, error)
}

// SavedService holds the rules for saved keywords and their tags.
type SavedService struct{ Store SavedStore }

// NormalizeKeyword trims and lowercases a keyword so one term is saved once.
func NormalizeKeyword(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// NormalizeTag trims and collapses spaces. The normalized name is the unique key.
func NormalizeTag(s string) (name, normalized string, ok bool) {
	name = strings.Join(strings.Fields(s), " ")
	if name == "" {
		return "", "", false
	}
	return name, strings.ToLower(name), true
}

// NormalizeTags normalizes and dedupes tag names, keeping the first spelling.
func NormalizeTags(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		name, normalized, ok := NormalizeTag(n)
		if !ok || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, name)
	}
	return out
}

func units(s string) int { return len(utf16.Encode([]rune(s))) }

// Save stores keywords and any metrics and tags that came with them, then
// returns the ids of exactly those keywords, in the order given.
func (s *SavedService) Save(ctx context.Context, in SaveInput) ([]string, error) {
	var keywords []string
	seen := map[string]bool{}
	for _, k := range in.Keywords {
		if n := NormalizeKeyword(k); n != "" && !seen[n] {
			seen[n] = true
			keywords = append(keywords, n)
		}
	}
	if len(keywords) == 0 {
		return nil, ValidationError("Add at least one keyword.")
	}
	tags := NormalizeTags(in.Tags)
	if in.ReplaceTags && len(tags) == 0 {
		return nil, ValidationError("Replacement tags are required when tagMode is replace.")
	}
	// A metric counts only for a keyword being saved. When two metrics name the
	// same keyword, the later one wins, as in the legacy app.
	var metrics []Metric
	byKeyword := map[string]int{}
	for _, m := range in.Metrics {
		n := NormalizeKeyword(m.Keyword)
		if !seen[n] {
			continue
		}
		m.Keyword = n
		if i, dup := byKeyword[n]; dup {
			metrics[i] = m
			continue
		}
		byKeyword[n] = len(metrics)
		metrics = append(metrics, m)
	}
	in.Keywords, in.Tags, in.Metrics = keywords, tags, metrics
	return s.Store.Save(ctx, in)
}

// List returns saved keywords. A search or term with only spaces is ignored.
func (s *SavedService) List(ctx context.Context, q ListQuery) (ListResult, error) {
	if q.Sort == "" {
		q.Sort = "createdAt"
	}
	if !slices.Contains(SortFields, q.Sort) {
		return ListResult{}, ValidationError("Sort field is not supported.")
	}
	q.Search = strings.TrimSpace(q.Search)
	q.IncludeTerms, q.ExcludeTerms = trimTerms(q.IncludeTerms), trimTerms(q.ExcludeTerms)
	res, err := s.Store.List(ctx, q)
	if err != nil {
		return ListResult{}, err
	}
	if res.Rows == nil {
		res.Rows = []SavedKeyword{}
	}
	if res.Tags == nil {
		res.Tags = []TagSummary{}
	}
	return res, nil
}

// AssignResult reports a tag edit across saved keywords.
type AssignResult struct {
	Success            bool     `json:"success"`
	TaggedCount        int      `json:"taggedCount"`
	AddedTags          []Tag    `json:"addedTags"`
	RemovedTagIDs      []string `json:"removedTagIds"`
	RemovedAssignments int      `json:"removedAssignments"`
}

// UpdateAssignments adds and removes tags on saved keywords.
func (s *SavedService) UpdateAssignments(ctx context.Context, projectID string, savedIDs, add, removeIDs []string) (AssignResult, error) {
	add = NormalizeTags(add)
	if len(add) == 0 && len(removeIDs) == 0 {
		return AssignResult{}, ValidationError("Add or remove at least one tag.")
	}
	out := AssignResult{Success: true, AddedTags: []Tag{}, RemovedTagIDs: []string{}}
	if len(add) > 0 {
		added, err := s.Store.AddTags(ctx, projectID, savedIDs, add)
		if err != nil {
			return AssignResult{}, err
		}
		out.TaggedCount = added.SavedKeywordCount
		out.AddedTags = append(out.AddedTags, added.Tags...)
	}
	if len(removeIDs) > 0 {
		removed, err := s.Store.RemoveTags(ctx, projectID, savedIDs, removeIDs)
		if err != nil {
			return AssignResult{}, err
		}
		out.TaggedCount = max(out.TaggedCount, removed.SavedKeywordCount)
		for _, t := range removed.Tags {
			out.RemovedTagIDs = append(out.RemovedTagIDs, t.ID)
		}
		out.RemovedAssignments = removed.RemovedCount
	}
	return out, nil
}

// UpdateTag renames or recolors a tag. A nil name or unset color is left alone;
// color is a pointer to a pointer so that an explicit null clears it.
func (s *SavedService) UpdateTag(ctx context.Context, projectID, tagID string, name *string, color **string) (*Tag, error) {
	if name == nil && color == nil {
		return nil, ValidationError("Provide a name or color to update.")
	}
	if name != nil {
		if _, _, ok := NormalizeTag(*name); !ok || units(strings.TrimSpace(*name)) > MaxTagNameUnits {
			return nil, ValidationError("Tag names are 1 to 64 characters.")
		}
	}
	if color != nil && *color != nil && !slices.Contains(TagColors, **color) {
		return nil, ValidationError("Tag color is not in the palette.")
	}
	return s.Store.UpdateTag(ctx, projectID, tagID, name, color)
}

// DeleteTag removes an unused tag. A tag still on keywords is refused with a
// *TagInUseError, because the cascade would silently drop its assignments.
func (s *SavedService) DeleteTag(ctx context.Context, projectID, tagID string) error {
	return s.Store.DeleteTag(ctx, projectID, tagID)
}

// Remove deletes saved keywords of the project and returns how many went.
func (s *SavedService) Remove(ctx context.Context, projectID string, ids []string) (int, error) {
	return s.Store.Remove(ctx, projectID, ids)
}

// ResolveMarket picks the request's market or the project's, and refuses one
// the provider would not serve.
func ResolveMarket(locationCode int, languageCode string, project market.Pair) (market.Pair, error) {
	pair := market.Resolve(market.Pair{LocationCode: locationCode, LanguageCode: languageCode}, project)
	if _, ok := market.Lookup(pair.LocationCode); !ok || !market.IsSupportedLanguageCode(pair.LanguageCode) {
		return market.Pair{}, ValidationError("Location or language is not supported.")
	}
	return pair, nil
}

func trimTerms(terms []string) []string {
	var out []string
	for _, t := range terms {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}
