package backlinks

import "time"

type Scope string

const (
	ScopeExactURL   Scope = "exact_url"
	ScopeSubfolder  Scope = "subfolder"
	ScopeDomain     Scope = "domain"
	ScopeSubdomains Scope = "subdomains"
)

type Target struct {
	APITarget         string
	Display           string
	Scope             Scope
	Path              string
	IncludeSubdomains bool
}

type Page[T any] struct {
	Rows       []T    `json:"rows"`
	TotalCount *int   `json:"totalCount"`
	HasMore    bool   `json:"hasMore"`
	Page       int    `json:"page"`
	PageSize   int    `json:"pageSize"`
	FetchedAt  string `json:"fetchedAt"`
}

type Summary struct {
	Rank                 *float64 `json:"rank"`
	Backlinks            *float64 `json:"backlinks"`
	ReferringPages       *float64 `json:"referringPages"`
	ReferringDomains     *float64 `json:"referringDomains"`
	BrokenBacklinks      *float64 `json:"brokenBacklinks"`
	BrokenPages          *float64 `json:"brokenPages"`
	BacklinksSpamScore   *float64 `json:"backlinksSpamScore"`
	TargetSpamScore      *float64 `json:"targetSpamScore"`
	NewBacklinks         *float64 `json:"newBacklinks"`
	LostBacklinks        *float64 `json:"lostBacklinks"`
	NewReferringDomains  *float64 `json:"newReferringDomains"`
	LostReferringDomains *float64 `json:"lostReferringDomains"`
}

type Trend struct {
	Date             string   `json:"date"`
	Backlinks        *float64 `json:"backlinks"`
	ReferringDomains *float64 `json:"referringDomains"`
	Rank             *float64 `json:"rank"`
}

type NewLostTrend struct {
	Date                 string   `json:"date"`
	NewBacklinks         *float64 `json:"newBacklinks"`
	LostBacklinks        *float64 `json:"lostBacklinks"`
	NewReferringDomains  *float64 `json:"newReferringDomains"`
	LostReferringDomains *float64 `json:"lostReferringDomains"`
}

type Overview struct {
	Target        string         `json:"target"`
	DisplayTarget string         `json:"displayTarget"`
	Scope         Scope          `json:"scope"`
	Summary       Summary        `json:"summary"`
	Trends        []Trend        `json:"trends"`
	NewLostTrends []NewLostTrend `json:"newLostTrends"`
	FetchedAt     string         `json:"fetchedAt"`
}

type BacklinkRow struct {
	DomainFrom     *string  `json:"domainFrom"`
	URLFrom        *string  `json:"urlFrom"`
	URLTo          *string  `json:"urlTo"`
	Anchor         *string  `json:"anchor"`
	ItemType       *string  `json:"itemType"`
	IsDofollow     *bool    `json:"isDofollow"`
	RelAttributes  []string `json:"relAttributes"`
	Rank           *float64 `json:"rank"`
	DomainFromRank *float64 `json:"domainFromRank"`
	PageFromRank   *float64 `json:"pageFromRank"`
	SpamScore      *float64 `json:"spamScore"`
	FirstSeen      *string  `json:"firstSeen"`
	LastSeen       *string  `json:"lastSeen"`
	IsLost         bool     `json:"isLost"`
	IsBroken       bool     `json:"isBroken"`
	LinksCount     *float64 `json:"linksCount"`
}

type ReferringDomainRow struct {
	Domain          *string  `json:"domain"`
	Backlinks       *float64 `json:"backlinks"`
	ReferringPages  *float64 `json:"referringPages"`
	Rank            *float64 `json:"rank"`
	SpamScore       *float64 `json:"spamScore"`
	FirstSeen       *string  `json:"firstSeen"`
	BrokenBacklinks *float64 `json:"brokenBacklinks"`
	BrokenPages     *float64 `json:"brokenPages"`
}

type TopPageRow struct {
	Page             *string  `json:"page"`
	Backlinks        *float64 `json:"backlinks"`
	ReferringDomains *float64 `json:"referringDomains"`
	Rank             *float64 `json:"rank"`
	BrokenBacklinks  *float64 `json:"brokenBacklinks"`
}

func timestamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }
