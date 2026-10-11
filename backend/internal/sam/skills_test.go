package sam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedSkillsParse(t *testing.T) {
	skills, err := loadSkills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 9 {
		t.Fatalf("skills = %d, want the 9 public in-app skills", len(skills))
	}
	seen := map[string]bool{}
	for i, s := range skills {
		if s.Name == "" || s.Description == "" || !strings.Contains(s.Body, "Surface note") {
			t.Errorf("incomplete skill %+v", s)
		}
		if seen[s.Name] {
			t.Errorf("duplicate skill %s", s.Name)
		}
		seen[s.Name] = true
		if i > 0 && skills[i-1].Name > s.Name {
			t.Errorf("skills are not sorted: %s before %s", skills[i-1].Name, s.Name)
		}
	}
	for _, internal := range []string{"seo-report", "simple-issue-description", "merge-ready", "papercuts"} {
		if seen[internal] {
			t.Errorf("%s is not an in-app skill", internal)
		}
	}
}

func TestParseSkillFrontmatter(t *testing.T) {
	good, err := parseSkill("x.md", "---\nname: a\ndescription: \"Quoted: with colon\"\n---\n\nBody text\n")
	if err != nil || good.Name != "a" || good.Description != "Quoted: with colon" || !strings.HasSuffix(good.Body, "Body text") {
		t.Fatalf("skill = %+v err = %v", good, err)
	}
	for _, bad := range []string{"no frontmatter", "---\nname: a\n", "---\nname: a\n---\nbody", "---\ndescription: d\n---\nbody"} {
		if _, err := parseSkill("x.md", bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

// The embedded skills and fact sheet are copies of files that live outside the
// Go module. Editing the original without refreshing the copy would silently
// ship stale instructions to users.
func TestEmbeddedCopiesMatchTheirSources(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	skills, err := loadSkills()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents")); err != nil {
		t.Skip("repository sources not available")
	}
	for _, s := range skills {
		source, err := os.ReadFile(filepath.Join(root, ".agents", "skills", s.Name, "SKILL.md"))
		if err != nil {
			t.Errorf("source of skill %s: %v", s.Name, err)
			continue
		}
		copied, _ := skillFiles.ReadFile("skills/" + s.Name + ".md")
		if string(source) != string(copied) {
			t.Errorf("skills/%s.md differs from .agents/skills/%s/SKILL.md; copy it again", s.Name, s.Name)
		}
	}
	if legacy, err := os.ReadFile(filepath.Join(root, "src", "server", "features", "sam", "product-fact-sheet.md")); err == nil && string(legacy) != productFactSheet {
		t.Error("product_fact_sheet.md differs from the legacy copy; the legacy file is deleted at cutover, until then keep both equal")
	}
}

func TestSystemPromptBlocks(t *testing.T) {
	p := projectInfo{ID: "p1", Name: `Acme "Anvils"`, Domain: "", LocationCode: 2840, LanguageCode: "en"}
	skills := []skill{{Name: "a", Description: "does a"}}
	intake := buildSystemPrompt(p, true, "", skills)
	for _, want := range []string{"no website set yet", "There is no business_overview yet", "- a: does a", `"Acme \"Anvils\""`} {
		if !strings.Contains(intake, want) {
			t.Errorf("intake prompt missing %q", want)
		}
	}
	if strings.Contains(intake, "## project_context") {
		t.Error("an empty memory block must be omitted")
	}
	p.Domain = "acme.test"
	steady := buildSystemPrompt(p, false, "  ## Notes\nx  ", nil)
	if !strings.Contains(steady, "Project website: acme.test") || strings.Contains(steady, "There is no business_overview") || strings.Contains(steady, "## skills") {
		t.Errorf("steady prompt wrong: %s", steady)
	}
	if !strings.Contains(steady, "## project_context") || !strings.Contains(steady, "## Notes\nx") {
		t.Error("memory block missing")
	}
	if !strings.Contains(steady, "United States") {
		t.Error("the default market should be named, not just its code")
	}
}
