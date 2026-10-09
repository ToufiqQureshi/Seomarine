package audit

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// skippedLinkProtocols are href schemes the analyzer ignores.
var skippedLinkProtocols = []string{"javascript:", "mailto:", "tel:", "#"}

// nonContentTags are subtrees whose text is not visible content.
var nonContentTags = map[string]struct{}{
	"script": {}, "style": {}, "noscript": {}, "svg": {},
}

// headingLevels maps a heading tag name to its level.
var headingLevels = map[string]int{"h1": 1, "h2": 2, "h3": 3, "h4": 4, "h5": 5, "h6": 6}

const (
	// maxAnchorChars bounds the stored anchor text.
	maxAnchorChars = 200
	// maxExtractedLinks and maxExtractedImages cap the extracted collections.
	// Crawler-trap and mega-menu pages can carry thousands of links/images per
	// page, and crawled pages sit in memory in persist batches, so uncapped
	// collections were part of the old engine's memory profile.
	maxExtractedLinks  = 1_000
	maxExtractedImages = 1_000
)

// appRootIDs are common single-page-app entry-point element ids.
var appRootIDs = map[string]struct{}{"root": {}, "app": {}, "__next": {}, "__nuxt": {}}

// openAnchor accumulates an in-progress anchor element.
type openAnchor struct {
	href string
	rel  string
	text []string
}

// AnalyzeHTML extracts SEO-relevant data from a page's HTML: title, meta
// description, headings, images, links, canonical, Open Graph tags, structured
// data, robots meta, word count and hreflang. It streams tokens rather than
// building a DOM so memory stays proportional to the accumulated text.
// Mirrors analyzeHtml.
func AnalyzeHTML(document, pageURL string, statusCode int, responseTimeMs int64, redirectURL string) PageAnalysis {
	state := &analyzerState{pageURL: pageURL, linksByTarget: map[string]PageLink{}}
	tokenizer := html.NewTokenizer(strings.NewReader(document))
	tokenizer.AllowCDATA(true)
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break
		}
		switch tokenType {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tokenizer.TagName()
			tagName := string(name)
			attributes := map[string]string{}
			altPresent := false
			if hasAttr {
				for {
					key, value, more := tokenizer.TagAttr()
					attributes[string(key)] = string(value)
					if string(key) == "alt" {
						altPresent = true
					}
					if !more {
						break
					}
				}
			}
			state.onOpenTag(tagName, attributes, altPresent)
		case html.TextToken:
			state.onText(string(tokenizer.Text()))
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			state.onCloseTag(string(name))
		}
	}
	return state.result(statusCode, responseTimeMs, redirectURL)
}

// analyzerState holds the accumulator used while streaming HTML tokens.
type analyzerState struct {
	pageURL string

	title      *string
	titleDone  bool
	titleDepth int
	// noscriptDepth mirrors the DOM difference: element extraction is skipped
	// inside <noscript>.
	noscriptDepth int
	suppressDepth int
	bodyDepth     int
	headDepth     int
	sawBody       bool

	metaDescription     string
	metaDescriptionSet  bool
	canonical           *string
	robotsMeta          *string
	ogTitle             *string
	ogDescription       *string
	ogImage             *string
	hasStructuredData   bool
	hasAppRoot          bool
	hasExecutableScript bool
	hreflangTags        []string

	h1s          []string
	headingOrder []int
	openH1       []string

	images        []ImageRef
	linksByTarget map[string]PageLink
	openAnchor    *openAnchor

	bodyParts     []string
	fallbackParts []string
}

func (s *analyzerState) onMetaTag(attributes map[string]string) {
	content := attributes["content"]
	switch {
	case attributes["name"] == "description":
		if !s.metaDescriptionSet {
			s.metaDescription = strings.TrimSpace(content)
			s.metaDescriptionSet = true
		}
	case attributes["name"] == "robots":
		if s.robotsMeta == nil {
			s.robotsMeta = &content
		}
	case attributes["property"] == "og:title":
		if s.ogTitle == nil {
			s.ogTitle = &content
		}
	case attributes["property"] == "og:description":
		if s.ogDescription == nil {
			s.ogDescription = &content
		}
	case attributes["property"] == "og:image":
		if s.ogImage == nil {
			s.ogImage = &content
		}
	}
}

func (s *analyzerState) onLinkTag(attributes map[string]string) {
	switch {
	case attributes["rel"] == "canonical":
		if s.canonical == nil {
			href, ok := attributes["href"]
			if !ok {
				href = ""
			}
			s.canonical = &href
		}
	case attributes["rel"] == "alternate" && attributes["hreflang"] != "":
		s.hreflangTags = append(s.hreflangTags, attributes["hreflang"])
	}
}

func (s *analyzerState) closeAnchor() {
	if s.openAnchor == nil {
		return
	}
	current := s.openAnchor
	s.openAnchor = nil
	if len(s.linksByTarget) >= maxExtractedLinks {
		return
	}
	resolved, ok := normalizeURL(current.href, s.pageURL)
	if !ok {
		return
	}
	if _, exists := s.linksByTarget[resolved]; exists {
		return
	}
	anchor := strings.TrimSpace(collapseWhitespace(strings.Join(current.text, "")))
	anchor = truncateChars(anchor, maxAnchorChars)
	s.linksByTarget[resolved] = PageLink{
		TargetURL:  resolved,
		Anchor:     anchor,
		IsInternal: isSameOrigin(resolved, s.pageURL),
		IsNofollow: hasSpaceSeparatedToken(current.rel, "nofollow"),
	}
}

func (s *analyzerState) onOpenTag(name string, attributes map[string]string, altPresent bool) {
	if _, ok := nonContentTags[name]; ok {
		s.suppressDepth++
	}
	if name == "noscript" {
		s.noscriptDepth++
	}
	if s.noscriptDepth > 0 {
		return
	}
	if s.headDepth == 0 && s.suppressDepth == 0 {
		if _, ok := appRootIDs[attributes["id"]]; ok || name == "app-root" {
			s.hasAppRoot = true
		}
	}
	switch name {
	case "title":
		// Ignore <title> inside <svg>; only the document title counts.
		if !s.titleDone && s.suppressDepth == 0 {
			s.titleDepth++
			if s.title == nil {
				empty := ""
				s.title = &empty
			}
		}
	case "head":
		s.headDepth++
	case "body":
		s.bodyDepth++
		s.sawBody = true
	case "meta":
		s.onMetaTag(attributes)
	case "link":
		s.onLinkTag(attributes)
	case "img":
		if len(s.images) < maxExtractedImages {
			alt := ""
			if altPresent {
				alt = attributes["alt"]
			}
			s.images = append(s.images, ImageRef{Src: attributes["src"], Alt: alt})
		}
	case "script":
		scriptType := strings.ToLower(attributes["type"])
		switch scriptType {
		case "", "module", "text/javascript", "application/javascript":
			s.hasExecutableScript = true
		}
		if attributes["type"] == "application/ld+json" {
			s.hasStructuredData = true
		}
	case "a":
		// HTML forbids nested <a>; browsers implicitly close the open one and
		// the tokenizer has no tree correction, so mirror that.
		s.closeAnchor()
		href := attributes["href"]
		if href != "" && !hasSkippedProtocol(href) {
			s.openAnchor = &openAnchor{href: href, rel: strings.ToLower(attributes["rel"])}
		}
	}
	if level, ok := headingLevels[name]; ok {
		s.headingOrder = append(s.headingOrder, level)
		if level == 1 && s.openH1 == nil {
			s.openH1 = []string{}
		}
	}
}

func (s *analyzerState) onText(text string) {
	if s.suppressDepth > 0 {
		return
	}
	if s.titleDepth > 0 {
		if s.title != nil {
			*s.title += text
		}
		return
	}
	if s.openH1 != nil {
		s.openH1 = append(s.openH1, text)
	}
	if s.openAnchor != nil {
		s.openAnchor.text = append(s.openAnchor.text, text)
	}
	if s.bodyDepth > 0 {
		s.bodyParts = append(s.bodyParts, text)
	} else if s.headDepth == 0 {
		s.fallbackParts = append(s.fallbackParts, text)
	}
}

func (s *analyzerState) onCloseTag(name string) {
	if _, ok := nonContentTags[name]; ok && s.suppressDepth > 0 {
		s.suppressDepth--
	}
	if name == "noscript" && s.noscriptDepth > 0 {
		s.noscriptDepth--
		return
	}
	if s.noscriptDepth > 0 {
		return
	}
	if name == "title" && s.titleDepth > 0 {
		s.titleDepth--
		if s.titleDepth == 0 {
			s.titleDone = true
		}
	}
	if name == "head" && s.headDepth > 0 {
		s.headDepth--
	}
	if name == "body" && s.bodyDepth > 0 {
		s.bodyDepth--
	}
	if name == "a" {
		s.closeAnchor()
	}
	if name == "h1" && s.openH1 != nil {
		s.h1s = append(s.h1s, strings.TrimSpace(strings.Join(s.openH1, "")))
		s.openH1 = nil
	}
}

func (s *analyzerState) result(statusCode int, responseTimeMs int64, redirectURL string) PageAnalysis {
	s.closeAnchor()
	parts := s.fallbackParts
	if s.sawBody {
		parts = s.bodyParts
	}
	bodyText := collapseWhitespace(strings.Join(parts, ""))
	wordCount := 0
	if bodyText != "" {
		wordCount = len(strings.Fields(bodyText))
	}
	title := ""
	if s.title != nil {
		title = strings.TrimSpace(*s.title)
	}
	links := make([]PageLink, 0, len(s.linksByTarget))
	for _, link := range s.linksByTarget {
		links = append(links, link)
	}
	// Iterating a Go map is unordered; sort by target URL so the extracted set
	// is deterministic (tests and content hashing depend on it).
	sortPageLinks(links)
	return PageAnalysis{
		URL:               s.pageURL,
		StatusCode:        statusCode,
		RedirectURL:       redirectURL,
		ResponseTimeMs:    responseTimeMs,
		Title:             title,
		MetaDescription:   s.metaDescription,
		Canonical:         derefString(s.canonical),
		RobotsMeta:        derefString(s.robotsMeta),
		OgTitle:           derefString(s.ogTitle),
		OgDescription:     derefString(s.ogDescription),
		OgImage:           derefString(s.ogImage),
		H1s:               s.h1s,
		HeadingOrder:      s.headingOrder,
		WordCount:         wordCount,
		BodyText:          bodyText,
		JavaScriptShell:   s.hasAppRoot && s.hasExecutableScript && wordCount < 20 && len(s.headingOrder) == 0 && len(s.linksByTarget) == 0 && len(s.images) == 0,
		Images:            s.images,
		Links:             links,
		HasStructuredData: s.hasStructuredData,
		HreflangTags:      s.hreflangTags,
	}
}

// sortPageLinks orders extracted links by target URL so a map-backed extraction
// is deterministic.
func sortPageLinks(links []PageLink) {
	slices.SortFunc(links, func(a, b PageLink) int { return strings.Compare(a.TargetURL, b.TargetURL) })
}

// hasSkippedProtocol reports whether an href starts with a scheme the analyzer
// ignores.
func hasSkippedProtocol(href string) bool {
	for _, prefix := range skippedLinkProtocols {
		if strings.HasPrefix(href, prefix) {
			return true
		}
	}
	return false
}

// hasSpaceSeparatedToken reports whether value contains token as a
// whitespace-separated word.
func hasSpaceSeparatedToken(value, token string) bool {
	for field := range strings.FieldsSeq(value) {
		if field == token {
			return true
		}
	}
	return false
}

// collapseWhitespace replaces every run of whitespace with a single space and
// trims the result.
func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// derefString returns the pointed-to string or "" for nil.
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
