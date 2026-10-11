package sam

import (
	"embed"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
)

// The skill files are copies of the public skills under .agents/skills (the
// ones without metadata.internal and not meant for contributors). A test fails
// when a copy drifts from its source.
//
//go:embed skills/*.md
var skillFiles embed.FS

// surfaceNote reframes skills written for external MCP clients so SAM skips
// the steps that do not apply in the app.
const surfaceNote = `> Surface note: you are SAM, running inside the Seomarine app. You are already
> authenticated and scoped to the user's current project — skip any "verify the
> MCP connection", "choose a project", or skill-install steps. You have no
> local filesystem: skip local-folder and file steps.
>
> Your project context is already in your system prompt — read it there; there
> is no get_project_context tool here. Write durable facts about the business
> back with update_project_context. You have no report tools: skip any step
> that says to deliver through the seo-report skill or to save a report, and
> give the findings in chat, short and scannable.
> If a skill step needs a tool you don't have (e.g. project creation), say so
> and point the user at the app page rather than improvising.`

type skill struct {
	Name        string
	Description string
	Body        string
}

// loadSkills parses the embedded skills, sorted by name.
func loadSkills() ([]skill, error) {
	entries, err := skillFiles.ReadDir("skills")
	if err != nil {
		return nil, fmt.Errorf("read embedded skills: %w", err)
	}
	skills := make([]skill, 0, len(entries))
	for _, entry := range entries {
		raw, err := skillFiles.ReadFile(path.Join("skills", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read skill %s: %w", entry.Name(), err)
		}
		s, err := parseSkill(entry.Name(), string(raw))
		if err != nil {
			return nil, err
		}
		skills = append(skills, s)
	}
	slices.SortFunc(skills, func(a, b skill) int { return strings.Compare(a.Name, b.Name) })
	return skills, nil
}

func parseSkill(file, raw string) (skill, error) {
	rest, ok := strings.CutPrefix(strings.ReplaceAll(raw, "\r\n", "\n"), "---\n")
	if !ok {
		return skill{}, fmt.Errorf("skill %s has no frontmatter", file)
	}
	front, body, ok := strings.Cut(rest, "\n---")
	if !ok {
		return skill{}, fmt.Errorf("skill %s has unterminated frontmatter", file)
	}
	var s skill
	for _, line := range strings.Split(front, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		switch strings.TrimSpace(key) {
		case "name":
			s.Name = value
		case "description":
			s.Description = value
		}
	}
	if s.Name == "" || s.Description == "" {
		return skill{}, fmt.Errorf("skill %s needs name and description", file)
	}
	s.Body = surfaceNote + "\n\n" + strings.TrimSpace(body)
	return s, nil
}
