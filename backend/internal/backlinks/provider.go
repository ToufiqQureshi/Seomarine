package backlinks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
)

const (
	pathSummary = "/v3/backlinks/summary/live"
	pathRows    = "/v3/backlinks/backlinks/live"
	pathDomains = "/v3/backlinks/referring_domains/live"
	pathPages   = "/v3/backlinks/domain_pages_summary/live"
	pathHistory = "/v3/backlinks/history/live"
)

type provider struct{ client *dataforseo.Client }

type providerSummary struct {
	Rank                       *float64 `json:"rank"`
	Backlinks                  *float64 `json:"backlinks"`
	ReferringPages             *float64 `json:"referring_pages"`
	ReferringDomains           *float64 `json:"referring_domains"`
	BrokenBacklinks            *float64 `json:"broken_backlinks"`
	BrokenPages                *float64 `json:"broken_pages"`
	BacklinksSpamScore         *float64 `json:"backlinks_spam_score"`
	NewBacklinks               *float64 `json:"new_backlinks"`
	LostBacklinks              *float64 `json:"lost_backlinks"`
	NewReferringDomains        *float64 `json:"new_referring_domains"`
	NewReferringDomainsLegacy  *float64 `json:"new_reffering_domains"`
	LostReferringDomains       *float64 `json:"lost_referring_domains"`
	LostReferringDomainsLegacy *float64 `json:"lost_reffering_domains"`
	Info                       struct {
		TargetSpamScore *float64 `json:"target_spam_score"`
	} `json:"info"`
}
type providerBacklink struct {
	DomainFrom      *string  `json:"domain_from"`
	URLFrom         *string  `json:"url_from"`
	URLTo           *string  `json:"url_to"`
	Anchor          *string  `json:"anchor"`
	ItemType        *string  `json:"item_type"`
	Dofollow        *bool    `json:"dofollow"`
	RelAttributes   []string `json:"rel_attributes"`
	Attributes      []string `json:"attributes"`
	Rank            *float64 `json:"rank"`
	DomainFromRank  *float64 `json:"domain_from_rank"`
	PageFromRank    *float64 `json:"page_from_rank"`
	SpamScore       *float64 `json:"backlink_spam_score"`
	SpamScoreLegacy *float64 `json:"backlinks_spam_score"`
	FirstSeen       *string  `json:"first_seen"`
	LostDate        *string  `json:"lost_date"`
	LastVisited     *string  `json:"last_visited"`
	IsLost          *bool    `json:"is_lost"`
	IsBroken        *bool    `json:"is_broken"`
	LinksCount      *float64 `json:"links_count"`
}
type providerDomain struct {
	Domain          *string  `json:"domain"`
	Backlinks       *float64 `json:"backlinks"`
	ReferringPages  *float64 `json:"referring_pages"`
	Rank            *float64 `json:"rank"`
	SpamScore       *float64 `json:"backlinks_spam_score"`
	FirstSeen       *string  `json:"first_seen"`
	BrokenBacklinks *float64 `json:"broken_backlinks"`
	BrokenPages     *float64 `json:"broken_pages"`
}
type providerPage struct {
	Page             *string  `json:"page"`
	URL              *string  `json:"url"`
	Backlinks        *float64 `json:"backlinks"`
	ReferringDomains *float64 `json:"referring_domains"`
	Rank             *float64 `json:"rank"`
	BrokenBacklinks  *float64 `json:"broken_backlinks"`
}
type providerHistory struct {
	Date                       *string  `json:"date"`
	Backlinks                  *float64 `json:"backlinks"`
	ReferringDomains           *float64 `json:"referring_domains"`
	Rank                       *float64 `json:"rank"`
	NewBacklinks               *float64 `json:"new_backlinks"`
	LostBacklinks              *float64 `json:"lost_backlinks"`
	NewReferringDomains        *float64 `json:"new_referring_domains"`
	NewReferringDomainsLegacy  *float64 `json:"new_reffering_domains"`
	LostReferringDomains       *float64 `json:"lost_referring_domains"`
	LostReferringDomainsLegacy *float64 `json:"lost_reffering_domains"`
}

func (p provider) post(ctx context.Context, org, endpoint string, body any) ([]json.RawMessage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode backlinks request: %w", err)
	}
	response, err := p.client.Do(ctx, org, http.MethodPost, endpoint, raw, false)
	if err != nil {
		return nil, fmt.Errorf("DataForSEO %s: %w", endpoint, err)
	}
	results, err := dataforseo.Results(response)
	if err != nil {
		return nil, fmt.Errorf("DataForSEO %s: %w", endpoint, err)
	}
	return results, nil
}

func commonTarget(target Target) map[string]any {
	return map[string]any{"target": target.APITarget, "include_subdomains": target.IncludeSubdomains, "exclude_internal_backlinks": true, "backlinks_status_type": "live"}
}
func (p provider) summary(ctx context.Context, org string, target Target) (providerSummary, error) {
	results, err := p.post(ctx, org, pathSummary, []map[string]any{commonTarget(target)})
	if err != nil {
		return providerSummary{}, err
	}
	if len(results) == 0 {
		return providerSummary{}, nil
	}
	var got providerSummary
	if err := json.Unmarshal(results[0], &got); err != nil {
		return providerSummary{}, fmt.Errorf("decode backlinks summary: %w", err)
	}
	return got, nil
}
func (p provider) history(ctx context.Context, org, target, from, to string) ([]providerHistory, error) {
	results, err := p.post(ctx, org, pathHistory, []map[string]any{{"target": target, "date_from": from, "date_to": to, "rank_scale": "one_hundred"}})
	if err != nil {
		return nil, err
	}
	return decodeItems[providerHistory](pathHistory, results)
}
func (p provider) rows(ctx context.Context, org string, target Target, page pageInput, filters []any, hideSpam bool) ([]providerBacklink, *int, error) {
	payload := commonTarget(target)
	payload["limit"] = page.PageSize
	payload["offset"] = (page.Page - 1) * page.PageSize
	payload["order_by"] = []string{page.Sort}
	payload["mode"] = page.Mode
	if hideSpam {
		filters = appendFilter(filters, []any{[]any{"backlink_spam_score", "<", 40}, "or", []any{"backlink_spam_score", "=", nil}})
	}
	if len(filters) > 0 {
		payload["filters"] = filters
	}
	results, err := p.post(ctx, org, pathRows, []map[string]any{payload})
	if err != nil {
		return nil, nil, err
	}
	return decodeList[providerBacklink](pathRows, results)
}
func (p provider) domains(ctx context.Context, org string, target Target, page pageInput, filters []any) ([]providerDomain, *int, error) {
	payload := commonTarget(target)
	payload["limit"] = page.PageSize
	payload["offset"] = (page.Page - 1) * page.PageSize
	payload["order_by"] = []string{page.Sort}
	if len(filters) > 0 {
		payload["filters"] = filters
	}
	results, err := p.post(ctx, org, pathDomains, []map[string]any{payload})
	if err != nil {
		return nil, nil, err
	}
	return decodeList[providerDomain](pathDomains, results)
}
func (p provider) pages(ctx context.Context, org string, target Target, page pageInput, filters []any) ([]providerPage, *int, error) {
	payload := commonTarget(target)
	payload["limit"] = page.PageSize
	payload["offset"] = (page.Page - 1) * page.PageSize
	payload["order_by"] = []string{page.Sort}
	if len(filters) > 0 {
		payload["filters"] = filters
	}
	results, err := p.post(ctx, org, pathPages, []map[string]any{payload})
	if err != nil {
		return nil, nil, err
	}
	return decodeList[providerPage](pathPages, results)
}
func appendFilter(current []any, condition any) []any {
	if len(current) == 0 {
		return []any{condition}
	}
	return append(current, "and", condition)
}
func decodeItems[T any](path string, results []json.RawMessage) ([]T, error) {
	if len(results) == 0 {
		return []T{}, nil
	}
	var envelope struct {
		Items []T `json:"items"`
	}
	if err := json.Unmarshal(results[0], &envelope); err != nil {
		return nil, fmt.Errorf("decode %s items: %w", path, err)
	}
	if envelope.Items == nil {
		return []T{}, nil
	}
	return envelope.Items, nil
}
func decodeList[T any](path string, results []json.RawMessage) ([]T, *int, error) {
	if len(results) == 0 {
		return []T{}, nil, nil
	}
	var envelope struct {
		Items      []T  `json:"items"`
		TotalCount *int `json:"total_count"`
	}
	if err := json.Unmarshal(results[0], &envelope); err != nil {
		return nil, nil, fmt.Errorf("decode %s list: %w", path, err)
	}
	if envelope.Items == nil {
		envelope.Items = []T{}
	}
	return envelope.Items, envelope.TotalCount, nil
}
