package projectcontext

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Typed project-context section keys, in the order the digest renders them.
var typedSectionKeys = []string{"business_overview", "current_goal", "positioning", "writing_preferences"}

var sectionLabels = map[string]string{
	"business_overview":   "Business overview",
	"current_goal":        "Current goal",
	"positioning":         "Positioning",
	"writing_preferences": "Writing preferences",
}

const (
	customSectionKeyPrefix = "custom:"
	researchLogLimit       = 20
	researchLogRetention   = 90
)

// projectContext is the digest both project-context tools return. Its JSON
// shape matches the legacy ProjectContextService.getProjectContext result.
type ProjectContext struct {
	Sections        []ContextSection    `json:"sections"`
	MissingSections []string            `json:"missingSections"`
	CustomSections  []CustomSection     `json:"customSections"`
	Competitors     []ContextCompetitor `json:"competitors"`
	KeyPages        []ContextKeyPage    `json:"keyPages"`
	ResearchLog     []ContextResearch   `json:"researchLog"`
	ReportTemplates []ContextTemplate   `json:"reportTemplates"`
}

type ContextSection struct {
	Key       string `json:"key"`
	Content   string `json:"content"`
	UpdatedAt string `json:"updatedAt"`
	UpdatedBy string `json:"updatedBy"`
}

type CustomSection struct {
	Slug      string  `json:"slug"`
	Title     *string `json:"title"`
	Content   string  `json:"content"`
	UpdatedAt string  `json:"updatedAt"`
	UpdatedBy string  `json:"updatedBy"`
}

type ContextCompetitor struct {
	ID        string  `json:"id"`
	ProjectID string  `json:"projectId"`
	Domain    string  `json:"domain"`
	Name      *string `json:"name"`
	Notes     *string `json:"notes"`
	UpdatedAt string  `json:"updatedAt"`
	UpdatedBy string  `json:"updatedBy"`
}

type ContextKeyPage struct {
	ID        string  `json:"id"`
	ProjectID string  `json:"projectId"`
	URL       string  `json:"url"`
	Role      string  `json:"role"`
	Topic     *string `json:"topic"`
	Notes     *string `json:"notes"`
	UpdatedAt string  `json:"updatedAt"`
	UpdatedBy string  `json:"updatedBy"`
}

type ContextResearch struct {
	ID        string `json:"id"`
	EntryDate string `json:"entryDate"`
	Summary   string `json:"summary"`
	CreatedBy string `json:"createdBy"`
}

type ContextTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// loadProjectContext reads every block of a project's memory.
func Load(ctx context.Context, db querier, projectID string) (ProjectContext, error) {
	sections, err := listContextSections(ctx, db, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	competitors, err := listContextCompetitors(ctx, db, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	keyPages, err := listContextKeyPages(ctx, db, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	researchLog, err := listContextResearchLog(ctx, db, projectID)
	if err != nil {
		return ProjectContext{}, err
	}
	templates, err := listContextTemplates(ctx, db, projectID)
	if err != nil {
		return ProjectContext{}, err
	}

	stored := make(map[string]contextSectionRow, len(sections))
	for _, s := range sections {
		stored[s.Key] = s
	}

	ordered := make([]ContextSection, 0, len(typedSectionKeys))
	missing := make([]string, 0, len(typedSectionKeys))
	for _, key := range typedSectionKeys {
		if row, ok := stored[key]; ok {
			ordered = append(ordered, ContextSection{
				Key:       row.Key,
				Content:   row.Content,
				UpdatedAt: row.UpdatedAt,
				UpdatedBy: row.UpdatedBy,
			})
		} else {
			missing = append(missing, key)
		}
	}

	custom := make([]CustomSection, 0)
	for _, s := range sections {
		if !strings.HasPrefix(s.Key, customSectionKeyPrefix) {
			continue
		}
		var title *string
		if s.Title != nil {
			t := *s.Title
			title = &t
		}
		custom = append(custom, CustomSection{
			Slug:      strings.TrimPrefix(s.Key, customSectionKeyPrefix),
			Title:     title,
			Content:   s.Content,
			UpdatedAt: s.UpdatedAt,
			UpdatedBy: s.UpdatedBy,
		})
	}

	return ProjectContext{
		Sections:        ordered,
		MissingSections: missing,
		CustomSections:  custom,
		Competitors:     competitors,
		KeyPages:        keyPages,
		ResearchLog:     researchLog,
		ReportTemplates: templates,
	}, nil
}

type contextSectionRow struct {
	Key       string
	Title     *string
	Content   string
	UpdatedAt string
	UpdatedBy string
}

func listContextSections(ctx context.Context, db querier, projectID string) ([]contextSectionRow, error) {
	rows, err := db.Query(ctx, `
		SELECT key, title, content, updated_at, updated_by
		FROM project_context_sections WHERE project_id = $1 ORDER BY key ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list context sections: %w", err)
	}
	defer rows.Close()
	var out []contextSectionRow
	for rows.Next() {
		var r contextSectionRow
		if err := rows.Scan(&r.Key, &r.Title, &r.Content, &r.UpdatedAt, &r.UpdatedBy); err != nil {
			return nil, fmt.Errorf("scan context section: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func listContextCompetitors(ctx context.Context, db querier, projectID string) ([]ContextCompetitor, error) {
	rows, err := db.Query(ctx, `
		SELECT id, project_id, domain, name, notes, updated_at, updated_by
		FROM project_competitors WHERE project_id = $1 ORDER BY domain ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list competitors: %w", err)
	}
	defer rows.Close()
	out := []ContextCompetitor{}
	for rows.Next() {
		var c ContextCompetitor
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Domain, &c.Name, &c.Notes, &c.UpdatedAt, &c.UpdatedBy); err != nil {
			return nil, fmt.Errorf("scan competitor: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func listContextKeyPages(ctx context.Context, db querier, projectID string) ([]ContextKeyPage, error) {
	rows, err := db.Query(ctx, `
		SELECT id, project_id, url, role, topic, notes, updated_at, updated_by
		FROM project_key_pages WHERE project_id = $1 ORDER BY url ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list key pages: %w", err)
	}
	defer rows.Close()
	out := []ContextKeyPage{}
	for rows.Next() {
		var p ContextKeyPage
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.URL, &p.Role, &p.Topic, &p.Notes, &p.UpdatedAt, &p.UpdatedBy); err != nil {
			return nil, fmt.Errorf("scan key page: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func listContextResearchLog(ctx context.Context, db querier, projectID string) ([]ContextResearch, error) {
	rows, err := db.Query(ctx, `
		SELECT id, entry_date, summary, created_by
		FROM project_research_log WHERE project_id = $1
		ORDER BY created_at DESC, entry_date DESC, id DESC LIMIT $2`, projectID, researchLogLimit)
	if err != nil {
		return nil, fmt.Errorf("list research log: %w", err)
	}
	defer rows.Close()
	out := []ContextResearch{}
	for rows.Next() {
		var r ContextResearch
		if err := rows.Scan(&r.ID, &r.EntryDate, &r.Summary, &r.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan research log: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func listContextTemplates(ctx context.Context, db querier, projectID string) ([]ContextTemplate, error) {
	rows, err := db.Query(ctx, `
		SELECT name, description FROM report_templates
		WHERE project_id = $1 ORDER BY name ASC, id ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list report templates: %w", err)
	}
	defer rows.Close()
	out := []ContextTemplate{}
	for rows.Next() {
		var t ContextTemplate
		if err := rows.Scan(&t.Name, &t.Description); err != nil {
			return nil, fmt.Errorf("scan report template: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// renderProjectContextMarkdown is a byte-for-byte port of the legacy digest so
// the text block a cached client already renders stays identical.
func RenderMarkdown(c ProjectContext) string {
	lines := []string{"# Project context", ""}

	byKey := make(map[string]ContextSection, len(c.Sections))
	for _, s := range c.Sections {
		byKey[s.Key] = s
	}
	for _, key := range typedSectionKeys {
		body := []string{}
		if s, ok := byKey[key]; ok {
			body = []string{s.Content}
		}
		pushSection(&lines, sectionLabels[key], body)
	}
	for _, custom := range c.CustomSections {
		heading := custom.Slug
		if custom.Title != nil {
			heading = *custom.Title
		}
		pushSection(&lines, heading, []string{custom.Content})
	}

	competitorLines := make([]string, 0, len(c.Competitors))
	for _, competitor := range c.Competitors {
		line := "- " + competitor.Domain
		if competitor.Name != nil {
			line += " — " + *competitor.Name
		}
		if competitor.Notes != nil {
			line += " (" + *competitor.Notes + ")"
		}
		competitorLines = append(competitorLines, line)
	}
	pushSection(&lines, "Competitors", competitorLines)

	keyPageLines := make([]string, 0, len(c.KeyPages))
	for _, page := range c.KeyPages {
		line := "- " + page.URL + " — " + page.Role
		if page.Topic != nil {
			line += " · " + *page.Topic
		}
		if page.Notes != nil {
			line += " (" + *page.Notes + ")"
		}
		keyPageLines = append(keyPageLines, line)
	}
	pushSection(&lines, "Key pages", keyPageLines)

	logHeading := "Research log"
	if len(c.ResearchLog) > 0 {
		noun := "entries"
		if len(c.ResearchLog) == 1 {
			noun = "entry"
		}
		logHeading = fmt.Sprintf("Research log (%d %s)", len(c.ResearchLog), noun)
	}
	logLines := make([]string, 0, len(c.ResearchLog)+2)
	for _, entry := range c.ResearchLog {
		logLines = append(logLines, "- "+entry.EntryDate+": "+entry.Summary)
	}
	if len(c.ResearchLog) >= researchLogLimit {
		logLines = append(logLines, "", fmt.Sprintf("_Older entries within the %d-day window are omitted._", researchLogRetention))
	}
	pushSection(&lines, logHeading, logLines)

	if len(c.ReportTemplates) > 0 {
		templateLines := make([]string, 0, len(c.ReportTemplates))
		for _, template := range c.ReportTemplates {
			templateLines = append(templateLines, "- "+template.Name+": "+template.Description)
		}
		pushSection(&lines, "Report templates", templateLines)
	}

	if len(c.MissingSections) > 0 {
		lines = append(lines, "Missing sections: "+strings.Join(c.MissingSections, ", "))
	} else {
		lines = append(lines, "Missing sections: none")
	}

	return strings.Join(lines, "\n")
}

func pushSection(lines *[]string, heading string, body []string) {
	*lines = append(*lines, "## "+heading, "")
	if len(body) > 0 {
		*lines = append(*lines, body...)
	} else {
		*lines = append(*lines, "_Empty_")
	}
	*lines = append(*lines, "")
}
