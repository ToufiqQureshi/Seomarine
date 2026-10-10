package backlinks

import "context"

// MCPBacklinkFilters contains the filters accepted by get_backlinks_profile.
type MCPBacklinkFilters struct {
	Include          string   `json:"include,omitempty"`
	Exclude          string   `json:"exclude,omitempty"`
	MinDomainRank    *float64 `json:"minDomainRank,omitempty"`
	MaxDomainRank    *float64 `json:"maxDomainRank,omitempty"`
	MinLinkAuthority *float64 `json:"minLinkAuthority,omitempty"`
	MaxLinkAuthority *float64 `json:"maxLinkAuthority,omitempty"`
	MinSpamScore     *float64 `json:"minSpamScore,omitempty"`
	MaxSpamScore     *float64 `json:"maxSpamScore,omitempty"`
	LinkType         string   `json:"linkType,omitempty"`
	HideLost         *bool    `json:"hideLost,omitempty"`
	HideBroken       *bool    `json:"hideBroken,omitempty"`
	DomainFrom       string   `json:"domainFrom,omitempty"`
}

// MCPBacklinkRowsInput is the bounded, validated query shape for the MCP tool.
type MCPBacklinkRowsInput struct {
	Target    string
	Scope     string
	Page      int
	PageSize  int
	SortField string
	SortOrder string
	Mode      string
	Filters   MCPBacklinkFilters
	HideSpam  *bool
}

// MCPBacklinkRows returns one validated page of backlink rows and its
// normalized target. It shares the same validation and DataForSEO service path
// as the authenticated HTTP API.
func (s *Service) MCPBacklinkRows(ctx context.Context, organizationID string, input MCPBacklinkRowsInput) (Target, Page[BacklinkRow], error) {
	in, paging, err := validatePage(pageRequest{
		Target: input.Target, Scope: input.Scope, Page: input.Page,
		PageSize: input.PageSize, SortOrder: input.SortOrder,
	}, input.Target, input.Scope)
	if err != nil {
		return Target{}, Page[BacklinkRow]{}, err
	}
	sortField := input.SortField
	if sortField == "" {
		sortField = "firstSeen"
	}
	sortColumn, ok := map[string]string{
		"rank": "rank", "domainRank": "domain_from_rank",
		"spamScore": "backlink_spam_score", "firstSeen": "first_seen",
	}[sortField]
	if !ok {
		return Target{}, Page[BacklinkRow]{}, inputError("sortField is invalid.")
	}
	mode := input.Mode
	if mode == "" {
		mode = "one_per_domain"
	}
	if mode != "one_per_domain" && mode != "as_is" {
		return Target{}, Page[BacklinkRow]{}, inputError("mode must be one_per_domain or as_is.")
	}
	filter := rowsFilters{
		Include: input.Filters.Include, Exclude: input.Filters.Exclude,
		MinDomainRank: input.Filters.MinDomainRank, MaxDomainRank: input.Filters.MaxDomainRank,
		MinLinkAuthority: input.Filters.MinLinkAuthority, MaxLinkAuthority: input.Filters.MaxLinkAuthority,
		MinSpamScore: input.Filters.MinSpamScore, MaxSpamScore: input.Filters.MaxSpamScore,
		LinkType: input.Filters.LinkType, HideLost: input.Filters.HideLost,
		HideBroken: input.Filters.HideBroken, DomainFrom: input.Filters.DomainFrom,
	}
	if err := validateRowsFilters(filter); err != nil {
		return Target{}, Page[BacklinkRow]{}, err
	}
	hideSpam := true
	if input.HideSpam != nil {
		hideSpam = *input.HideSpam
	}
	target, err := normalizeTarget(input.Target, input.Scope)
	if err != nil {
		return Target{}, Page[BacklinkRow]{}, err
	}
	if countFilters(buildRowsFilters(filter))+countFilters(scopeFilters("url_to", target))+boolInt(hideSpam)*2 > maxFilterConditions {
		return Target{}, Page[BacklinkRow]{}, inputError("Too many filter conditions (maximum 8).")
	}
	result, err := s.Rows(ctx, organizationID, in, pageInput{
		Page: paging.Page, PageSize: paging.PageSize,
		Sort: sortColumn + "," + paging.Sort, Mode: mode,
	}, filter, hideSpam)
	if err != nil {
		return Target{}, Page[BacklinkRow]{}, err
	}
	return target, result, nil
}
