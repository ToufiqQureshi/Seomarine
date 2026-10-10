package projectcontext

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/backlinks"
	"golang.org/x/net/idna"
)

const (
	maxUpdates               = 50
	maxProseChars            = 4000
	maxCustomSections        = 20
	maxCompetitors           = 100
	maxKeyPages              = 100
	researchLogRetentionDays = 90
)

var customSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var typedKeys = map[string]bool{"business_overview": true, "current_goal": true, "positioning": true, "writing_preferences": true}

// Error is a stable input/not-found error shared by the HTTP and MCP surfaces.
type Error struct{ Code, Message string }

func (e *Error) Error() string     { return e.Message }
func invalid(message string) error { return &Error{Code: "VALIDATION_ERROR", Message: message} }

type Service struct {
	Repo Repository
	Now  func() time.Time
}

func (s *Service) Get(ctx context.Context, projectID string) (ProjectContext, error) {
	if s == nil || s.Repo.DB == nil {
		return ProjectContext{}, fmt.Errorf("project context database unavailable")
	}
	return s.Repo.Get(ctx, projectID)
}

// Apply resolves the whole patch while holding the project row lock, then
// commits every write in one transaction. Rejected batches make no changes.
func (s *Service) Apply(ctx context.Context, projectID string, updates []json.RawMessage, author string) (ProjectContext, error) {
	if s == nil || s.Repo.DB == nil {
		return ProjectContext{}, fmt.Errorf("project context database unavailable")
	}
	if projectID == "" || (author != "user" && author != "sam" && author != "mcp") {
		return ProjectContext{}, invalid("projectId and a valid update author are required.")
	}
	if len(updates) < 1 || len(updates) > maxUpdates {
		return ProjectContext{}, invalid("updates must contain 1 to 50 operations.")
	}
	now := s.now().UTC()
	return s.Repo.Apply(ctx, projectID, author, now, func(current ProjectContext) ([]resolvedOp, error) {
		custom := map[string]bool{}
		domains := map[string]bool{}
		urls := map[string]bool{}
		for _, v := range current.CustomSections {
			custom["custom:"+v.Slug] = true
		}
		for _, v := range current.Competitors {
			domains[v.Domain] = true
		}
		for _, v := range current.KeyPages {
			urls[v.URL] = true
		}
		ops := make([]resolvedOp, 0, len(updates))
		for i, raw := range updates {
			op, err := resolve(raw, custom, domains, urls)
			if err != nil {
				return nil, fmt.Errorf("updates[%d] was rejected (nothing in this batch was applied): %w", i, err)
			}
			ops = append(ops, op)
		}
		return ops, nil
	})
}
func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type resolvedOp struct {
	kind, key, title, content string
	competitors               []competitorWrite
	domains                   []string
	pages                     []pageWrite
	urls                      []string
	ids                       []string
	summary                   string
}
type competitorWrite struct {
	domain      string
	name, notes *string
}
type pageWrite struct {
	url, role    string
	topic, notes *string
}

func resolve(raw json.RawMessage, custom, domains, urls map[string]bool) (resolvedOp, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return resolvedOp{}, invalid("each update must be a JSON object.")
	}
	decode := func(dst any) error {
		d := json.NewDecoder(strings.NewReader(string(raw)))
		d.DisallowUnknownFields()
		if err := d.Decode(dst); err != nil {
			return invalid("update fields are invalid.")
		}
		return nil
	}
	var key string
	for _, candidate := range []string{"section", "customSection", "deleteCustomSection", "addCompetitors", "removeCompetitors", "addKeyPages", "removeKeyPages", "removeResearchLog", "appendResearchLog"} {
		if _, ok := fields[candidate]; ok {
			if key != "" {
				return resolvedOp{}, invalid("each update must contain exactly one operation.")
			}
			key = candidate
		}
	}
	if key == "" {
		return resolvedOp{}, invalid("unsupported project context update.")
	}
	requiredFields := map[string][]string{
		"section": {"content"}, "customSection": {"content"},
	}
	for _, field := range requiredFields[key] {
		if _, ok := fields[field]; !ok {
			return resolvedOp{}, invalid("required update fields are missing.")
		}
	}
	switch key {
	case "section":
		var in struct {
			Section string `json:"section"`
			Content string `json:"content"`
		}
		if err := decode(&in); err != nil {
			return resolvedOp{}, err
		}
		if !typedKeys[in.Section] {
			return resolvedOp{}, invalid("section is not a supported project context section.")
		}
		content := strings.TrimSpace(in.Content)
		if !fits(content, maxProseChars) {
			return resolvedOp{}, invalid("Sections are capped at 4000 characters.")
		}
		if content == "" {
			return resolvedOp{kind: "delete_section", key: in.Section}, nil
		}
		return resolvedOp{kind: "upsert_section", key: in.Section, content: content}, nil
	case "customSection":
		var in struct {
			CustomSection string  `json:"customSection"`
			Title         *string `json:"title"`
			Content       string  `json:"content"`
		}
		if err := decode(&in); err != nil {
			return resolvedOp{}, err
		}
		slug := strings.TrimSpace(in.CustomSection)
		if len(utf16.Encode([]rune(slug))) > 60 || !customSlugPattern.MatchString(slug) {
			return resolvedOp{}, invalid("Use a lowercase custom section slug like launch-plan.")
		}
		content := strings.TrimSpace(in.Content)
		if !fits(content, maxProseChars) {
			return resolvedOp{}, invalid("Sections are capped at 4000 characters.")
		}
		k := "custom:" + slug
		if content == "" {
			delete(custom, k)
			return resolvedOp{kind: "delete_section", key: k}, nil
		}
		if in.Title != nil {
			title := strings.TrimSpace(*in.Title)
			if title == "" || !fits(title, 120) {
				return resolvedOp{}, invalid("Custom section titles must be 1 to 120 characters.")
			}
			in.Title = &title
		}
		if !custom[k] && len(custom) >= maxCustomSections {
			return resolvedOp{}, invalid("A project can hold 20 custom sections. Delete one first.")
		}
		custom[k] = true
		return resolvedOp{kind: "upsert_section", key: k, title: titleValue(in.Title), content: content}, nil
	case "deleteCustomSection":
		var in struct {
			Slug string `json:"deleteCustomSection"`
		}
		if err := decode(&in); err != nil {
			return resolvedOp{}, err
		}
		slug := strings.TrimSpace(in.Slug)
		if !customSlugPattern.MatchString(slug) {
			return resolvedOp{}, invalid("Use a lowercase custom section slug like launch-plan.")
		}
		key := "custom:" + slug
		delete(custom, key)
		return resolvedOp{kind: "delete_section", key: key}, nil
	case "addCompetitors":
		var in struct {
			Rows []struct {
				Domain string  `json:"domain"`
				Name   *string `json:"name"`
				Notes  *string `json:"notes"`
			} `json:"addCompetitors"`
		}
		if err := decode(&in); err != nil {
			return resolvedOp{}, err
		}
		if len(in.Rows) < 1 || len(in.Rows) > 100 {
			return resolvedOp{}, invalid("addCompetitors must contain 1 to 100 rows.")
		}
		rows := make([]competitorWrite, 0, len(in.Rows))
		idx := map[string]int{}
		for _, v := range in.Rows {
			if !fits(v.Domain, 255) {
				return resolvedOp{}, invalid("Competitor domains are limited to 255 characters.")
			}
			d, err := backlinks.NormalizeProjectDomain(v.Domain)
			if err != nil {
				return resolvedOp{}, invalid("Enter a valid competitor domain.")
			}
			n, err := optionalText(v.Name, 120)
			if err != nil {
				return resolvedOp{}, err
			}
			notes, err := optionalText(v.Notes, 500)
			if err != nil {
				return resolvedOp{}, err
			}
			row := competitorWrite{d, n, notes}
			if i, ok := idx[d]; ok {
				rows[i] = row
			} else {
				idx[d] = len(rows)
				rows = append(rows, row)
			}
		}
		add := 0
		for _, v := range rows {
			if !domains[v.domain] {
				add++
			}
		}
		if len(domains)+add > maxCompetitors {
			return resolvedOp{}, invalid("A project can track 100 competitors. Remove some first.")
		}
		for _, v := range rows {
			domains[v.domain] = true
		}
		return resolvedOp{kind: "upsert_competitors", competitors: rows}, nil
	case "removeCompetitors":
		var in struct {
			Domains []string `json:"removeCompetitors"`
		}
		if err := decode(&in); err != nil {
			return resolvedOp{}, err
		}
		if len(in.Domains) < 1 || len(in.Domains) > 100 {
			return resolvedOp{}, invalid("removeCompetitors must contain 1 to 100 domains.")
		}
		out := []string{}
		seen := map[string]bool{}
		for _, v := range in.Domains {
			d, err := backlinks.NormalizeProjectDomain(v)
			if err != nil {
				return resolvedOp{}, invalid("Enter a valid competitor domain.")
			}
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
			delete(domains, d)
		}
		return resolvedOp{kind: "delete_competitors", domains: out}, nil
	case "addKeyPages":
		var in struct {
			Rows []struct {
				URL   string  `json:"url"`
				Role  *string `json:"role"`
				Topic *string `json:"topic"`
				Notes *string `json:"notes"`
			} `json:"addKeyPages"`
		}
		if err := decode(&in); err != nil {
			return resolvedOp{}, err
		}
		if len(in.Rows) < 1 || len(in.Rows) > 100 {
			return resolvedOp{}, invalid("addKeyPages must contain 1 to 100 rows.")
		}
		rows := []pageWrite{}
		idx := map[string]int{}
		for _, v := range in.Rows {
			u, err := normalizePageURL(v.URL)
			if err != nil {
				return resolvedOp{}, err
			}
			role := ""
			if v.Role != nil {
				role = *v.Role
				if role != "hub" && role != "spoke" && role != "money" && role != "other" {
					return resolvedOp{}, invalid("Key page role must be hub, spoke, money, or other.")
				}
			}
			topic, err := optionalText(v.Topic, 200)
			if err != nil {
				return resolvedOp{}, err
			}
			notes, err := optionalText(v.Notes, 500)
			if err != nil {
				return resolvedOp{}, err
			}
			row := pageWrite{u, role, topic, notes}
			if i, ok := idx[u]; ok {
				rows[i] = row
			} else {
				idx[u] = len(rows)
				rows = append(rows, row)
			}
		}
		add := 0
		for _, v := range rows {
			if !urls[v.url] {
				add++
			}
		}
		if len(urls)+add > maxKeyPages {
			return resolvedOp{}, invalid("A project can hold 100 key pages. This is a shortlist, not a page inventory.")
		}
		for _, v := range rows {
			urls[v.url] = true
		}
		return resolvedOp{kind: "upsert_pages", pages: rows}, nil
	case "removeKeyPages":
		var in struct {
			URLs []string `json:"removeKeyPages"`
		}
		if err := decode(&in); err != nil {
			return resolvedOp{}, err
		}
		if len(in.URLs) < 1 || len(in.URLs) > 100 {
			return resolvedOp{}, invalid("removeKeyPages must contain 1 to 100 URLs.")
		}
		out := []string{}
		seen := map[string]bool{}
		for _, v := range in.URLs {
			u, err := normalizePageURL(v)
			if err != nil {
				return resolvedOp{}, err
			}
			if !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
			delete(urls, u)
		}
		return resolvedOp{kind: "delete_pages", urls: out}, nil
	case "removeResearchLog":
		var in struct {
			IDs []string `json:"removeResearchLog"`
		}
		if err := decode(&in); err != nil {
			return resolvedOp{}, err
		}
		if len(in.IDs) < 1 || len(in.IDs) > 100 {
			return resolvedOp{}, invalid("removeResearchLog must contain 1 to 100 ids.")
		}
		return resolvedOp{kind: "delete_research", ids: in.IDs}, nil
	case "appendResearchLog":
		var in struct {
			Append struct {
				Summary string `json:"summary"`
			} `json:"appendResearchLog"`
		}
		if err := decode(&in); err != nil {
			return resolvedOp{}, err
		}
		summary := strings.TrimSpace(in.Append.Summary)
		if summary == "" || !fits(summary, 1000) {
			return resolvedOp{}, invalid("Research log summaries must be 1 to 1000 characters.")
		}
		return resolvedOp{kind: "append_research", summary: summary}, nil
	default:
		return resolvedOp{}, invalid("Unsupported project context update.")
	}
}
func titleValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func optionalText(v *string, max int) (*string, error) {
	if v == nil {
		return nil, nil
	}
	x := strings.TrimSpace(*v)
	if !fits(x, max) {
		return nil, invalid(fmt.Sprintf("Text must be no more than %d characters.", max))
	}
	return &x, nil
}
func fits(s string, max int) bool { return len(utf16.Encode([]rune(s))) <= max }
func normalizePageURL(raw string) (string, error) {
	if !fits(raw, 2048) || strings.TrimSpace(raw) == "" {
		return "", invalid("Key page URL must be 1 to 2048 characters.")
	}
	in := strings.TrimSpace(raw)
	if !strings.Contains(in, "://") {
		in = "https://" + in
	}
	u, err := url.Parse(in)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", invalid("Not a valid page URL: " + raw)
	}
	host, err := idna.Lookup.ToASCII(strings.ToLower(u.Hostname()))
	if err != nil {
		return "", invalid("Not a valid page URL: " + raw)
	}
	host = strings.TrimPrefix(host, "www.")
	port := u.Port()
	u.Scheme = "https"
	u.Host = host
	if port != "" {
		u.Host = host + ":" + port
	}
	u.Fragment = ""
	out := u.String()
	if (u.Path == "" || u.Path == "/") && u.RawQuery == "" {
		out = strings.TrimSuffix(out, "/")
	}
	return out, nil
}
