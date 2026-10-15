package sitepublish

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// SchemaOptions chooses which JSON-LD blocks a publish adds to the post.
type SchemaOptions struct {
	// Article sends a BlogPosting block built from the draft.
	Article bool `json:"article"`
	// FAQ adds a FAQPage block, but only when the visible article really has
	// an FAQ section. The flag alone never produces a block.
	FAQ bool `json:"faq"`
}

// AuthorInfo is the author profile a byline and the Article schema are built
// from.
type AuthorInfo struct {
	Name       string
	Role       string
	Bio        string
	PhotoURL   string
	ProfileURL string
}

// SchemaBuild is one schema pass over a draft: the JSON-LD payloads to add,
// notes for the writer about what was skipped, and the visible byline.
type SchemaBuild struct {
	Blocks  []string // JSON-LD payloads, one <script> tag each
	Skipped string   // what could not be built and why, for the writer
	Byline  string   // Markdown appended before rendering; empty when none
	// DateString is the review/modified date shown in the byline.
	DateString string
}

// schemaBlocked is the note prefix every skip shares, so the editor can
// explain in one line why a block is missing.
const schemaBlocked = "Schema skipped: "

// schemaInput gathers everything a draft carries into the JSON-LD builders.
type schemaInput struct {
	Title     string
	Slug      string
	Meta      string
	Content   string
	Image     string // the first image of the published article
	Author    AuthorInfo
	HasAuthor bool
	// Reviewed is the draft's reviewed date; nil until the writer marks one.
	Reviewed *time.Time
	// PostLink is the saved post's URL. A preview has none, and the schema is
	// built without mainEntityOfPage then.
	PostLink string
	// Published is the post's date_gmt from the first save; nil in a preview.
	Published *time.Time
	// NotDate is the date a preview uses when nothing else is known.
	NotDate *time.Time
}

// buildSchemas turns a draft into the payloads WordPress receives. It never
// fails on the content: anything that cannot be built cleanly is skipped
// with a note, because a publish must not die over schema.
func buildSchemas(input schemaInput, options SchemaOptions) SchemaBuild {
	modified := modifiedDate(input)
	var blocks []string
	var notes []string
	if options.Article {
		payload, why := articleSchema(input, modified)
		if payload != "" {
			blocks = append(blocks, payload)
		}
		if why != "" {
			notes = append(notes, why)
		}
	}
	if options.FAQ {
		answers, why := extractFAQ(input.Content)
		if len(answers) > 0 {
			blocks = append(blocks, faqSchema(answers))
		}
		if why != "" {
			notes = append(notes, why)
		}
	}
	return SchemaBuild{
		Blocks: blocks, Skipped: strings.Join(notes, " "),
		Byline: bylineMarkdown(input, modified), DateString: modified.Format("2 Jan 2006"),
	}
}

// modifiedDate is the dateModified: the draft's reviewed date when the writer
// marked one, else the post's publish date, else today. The Article schema
// and the visible byline show the same date, so they never disagree.
func modifiedDate(input schemaInput) time.Time {
	if input.Reviewed != nil {
		return input.Reviewed.UTC()
	}
	if input.Published != nil {
		return input.Published.UTC()
	}
	if input.NotDate != nil {
		return input.NotDate.UTC()
	}
	return time.Now().UTC()
}

// articleSchema returns the BlogPosting payload and why it is missing, if it
// is. A title-less post already cannot publish, so only an unexpected
// encoding failure leaves the Article block off.
func articleSchema(input schemaInput, modified time.Time) (string, string) {
	posting := map[string]any{
		"@context":     "https://schema.org",
		"@type":        "BlogPosting",
		"headline":     strings.TrimSpace(input.Title),
		"dateModified": modified.Format("2006-01-02"),
	}
	switch published := input.Published; {
	case published != nil:
		posting["datePublished"] = published.UTC().Format("2006-01-02")
	default:
		posting["datePublished"] = modified.Format("2006-01-02")
	}
	if image := strings.TrimSpace(input.Image); image != "" {
		posting["image"] = []string{image}
	}
	if meta := strings.TrimSpace(input.Meta); meta != "" {
		posting["description"] = meta
	}
	if input.HasAuthor {
		posting["author"] = personSchema(input.Author)
	}
	if input.PostLink != "" {
		posting["mainEntityOfPage"] = map[string]any{"@type": "WebPage", "@id": input.PostLink}
	}
	payload, err := marshalSchema(posting)
	if err != nil {
		return "", schemaBlocked + "the article JSON-LD could not be built."
	}
	return payload, ""
}

// personSchema is the author node of the Article schema.
func personSchema(author AuthorInfo) map[string]any {
	person := map[string]any{"@type": "Person", "name": strings.TrimSpace(author.Name)}
	if role := strings.TrimSpace(author.Role); role != "" {
		person["jobTitle"] = role
	}
	if bio := strings.TrimSpace(author.Bio); bio != "" {
		person["description"] = bio
	}
	if photo := strings.TrimSpace(author.PhotoURL); photo != "" {
		person["image"] = photo
	}
	if link := strings.TrimSpace(author.ProfileURL); link != "" {
		person["url"] = link
	}
	return person
}

// faqAnswer is one question with the answer text exactly as rendered.
type faqAnswer struct{ Question, Answer string }

// faqSchema returns the FAQPage payload for the given answers.
func faqSchema(answers []faqAnswer) string {
	entities := make([]map[string]any, 0, len(answers))
	for _, answer := range answers {
		entities = append(entities, map[string]any{
			"@type":          "Question",
			"name":           answer.Question,
			"acceptedAnswer": map[string]any{"@type": "Answer", "text": answer.Answer},
		})
	}
	payload, err := marshalSchema(map[string]any{
		"@context": "https://schema.org", "@type": "FAQPage", "mainEntity": entities,
	})
	if err != nil {
		return ""
	}
	return payload
}

// extractFAQ finds the FAQ section: an H2 whose text is "FAQ" or
// "Frequently asked questions" (case-insensitive), followed by H3 questions
// with plain paragraph answers before the next heading.
//
// The question and answer strings come from the same goldmark pass that
// renders the visible article, so schema text and visible text match byte
// for byte, as Google's structured-data policy requires. Anything the text
// extraction cannot copy exactly (inline HTML, images, lists, tables inside
// an answer) leaves the whole FAQPage off with a note — a wrong FAQPage is
// worse than none.
func extractFAQ(content string) ([]faqAnswer, string) {
	document := markdown.Parser().Parse(text.NewReader([]byte(content)))
	source := []byte(content)
	var inFAQ bool
	var current *faqAnswer
	var answers []faqAnswer
	skip := ""
	setSkip := func(reason string) {
		if skip == "" {
			skip = reason
		}
	}
	flush := func() {
		if current == nil {
			return
		}
		if current.Answer == "" {
			setSkip(schemaBlocked + "a question has no answer under it.")
		}
		answers = append(answers, *current)
		current = nil
	}
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Heading:
			if n.Level == 2 {
				flush()
				inFAQ = isFAQTitle(nodeText(source, n))
				return ast.WalkSkipChildren, nil
			}
			if !inFAQ || n.Level != 3 {
				return ast.WalkSkipChildren, nil
			}
			question := strings.TrimSpace(nodeText(source, n))
			if question == "" {
				setSkip(schemaBlocked + "a question heading has no visible text.")
				return ast.WalkSkipChildren, nil
			}
			flush()
			current = &faqAnswer{Question: question}
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph:
			if current == nil || !inFAQ {
				return ast.WalkContinue, nil
			}
			if !paragraphIsPlain(n) {
				setSkip(schemaBlocked + "an answer holds formatting that cannot be copied into the schema text.")
				return ast.WalkSkipChildren, nil
			}
			if answer := strings.TrimSpace(nodeText(source, n)); answer != "" {
				if current.Answer != "" {
					current.Answer += "\n\n"
				}
				current.Answer += answer
			}
			return ast.WalkSkipChildren, nil
		default:
			if current != nil && inFAQ && isContentBlock(node) {
				setSkip(schemaBlocked + "an answer carries a list, table or code block that cannot be copied into the schema text.")
				return ast.WalkSkipChildren, nil
			}
		}
		return ast.WalkContinue, nil
	})
	flush()
	if len(answers) == 0 && skip == "" {
		// No FAQ section found at all: nothing to say.
		return nil, ""
	}
	if skip != "" {
		return nil, skip
	}
	// Keep only questions that have an answer; a question without one is
	// already covered by the skip above only for the first one, so be strict.
	for _, answer := range answers {
		if answer.Answer == "" {
			return nil, schemaBlocked + "a question has no answer under it."
		}
	}
	return answers, ""
}

// isFAQTitle matches the two section titles the FAQ check also accepts.
func isFAQTitle(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return normalized == "faq" || normalized == "frequently asked questions"
}

// isContentBlock reports whether a node is a block whose text does not turn
// into a single plain paragraph string: lists, quotes, code, raw HTML and
// GFM tables (the table node comes from the GFM extension, so its kind name
// is matched as a string).
func isContentBlock(node ast.Node) bool {
	switch node.Kind() {
	case ast.KindList, ast.KindListItem, ast.KindBlockquote, ast.KindCodeBlock,
		ast.KindFencedCodeBlock, ast.KindHTMLBlock:
		return true
	case ast.NodeKind(0):
		return false
	}
	return node.Kind().String() == "Table"
}

// nodeText collects the text a node renders visibly. Links, emphasis and
// other wrappers count by their text; raw HTML is excluded because its
// content is markup, not text.
func nodeText(source []byte, node ast.Node) string {
	var out strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case ast.KindText:
			out.Write(child.(*ast.Text).Value(source))
		case ast.KindRawHTML, ast.KindImage, ast.KindAutoLink:
			// Markup, pictures and visible addresses are not phrase text.
		default:
			out.WriteString(nodeText(source, child))
		}
	}
	return out.String()
}

// paragraphIsPlain reports whether a paragraph renders as pure text: no
// images, autolinks or inline HTML (a link's text is fine — its visible text
// is exactly what nodeText collects).
func paragraphIsPlain(node ast.Node) bool {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case ast.KindImage, ast.KindAutoLink, ast.KindRawHTML:
			return false
		default:
			if !paragraphIsPlain(child) {
				return false
			}
		}
	}
	return true
}

// marshalSchema encodes one payload and checks it with json.Valid. HTML
// escaping stays on so a literal "</script>" inside a title or answer cannot
// close the script tag early.
func marshalSchema(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode schema: %w", err)
	}
	if !json.Valid(payload) {
		return "", fmt.Errorf("encode schema: the payload is not valid JSON")
	}
	return string(payload), nil
}

// schemaScriptTag wraps a payload in the script tag WordPress gets inside
// the post content.
func schemaScriptTag(payload string) string {
	return `<script type="application/ld+json">` + payload + `</script>`
}

// bylineMarkdown builds the visible byline paragraph, appended to the
// Markdown before rendering. The author text is escaped so a name or role
// with Markdown control characters cannot inject markup.
func bylineMarkdown(input schemaInput, modified time.Time) string {
	if !input.HasAuthor {
		return ""
	}
	text := "Written by " + markdownEscapeText(strings.TrimSpace(input.Author.Name))
	if role := strings.TrimSpace(input.Author.Role); role != "" {
		text += ", " + markdownEscapeText(role)
	}
	return "\n\n_" + text + " · Updated " + modified.Format("2 Jan 2006") + "_"
}

// markdownEscapeText backslash-escapes the characters that would change
// Markdown rendering, so an author name cannot inject markup into the
// article or its JSON-LD.
func markdownEscapeText(value string) string {
	return strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"#", "\\#",
		"!", "\\!",
		"`", "\\`",
		"|", "\\|",
	).Replace(value)
}
