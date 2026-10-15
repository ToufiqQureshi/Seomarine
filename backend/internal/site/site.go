// Package site renders Uplumo's public marketing pages, the landing page
// and the pricing page, and serves their static assets.
//
// The pages hold no per-request data, so they are rendered once by New and
// served from memory: a template bug fails at startup instead of on a
// visitor's request, and serving a page costs a memory copy.
package site

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	planCatalog "github.com/toufiqqureshi/uplumo/backend/internal/plans"
)

// Prices, limits and features come from the plan catalog, the same list the
// billing API and every plan check read.

// AssetsPrefix is the URL path the embedded static files are served under.
const AssetsPrefix = "/site/"

const (
	name       = "Uplumo"
	signUpPath = "/sign-up"
	// SupportEmail is the address the public pages give for every question.
	SupportEmail = "support@uplumo.com"
	// policyUpdated is the date shown on the legal pages. Change it whenever
	// their text changes.
	policyUpdated = "13 October 2026"
	// operatorName and operatorAddress are the legal name and postal address
	// of whoever runs Uplumo, and supportPhone a support number. The legal
	// and contact pages print each only when it is set. The owner asked for
	// no personal name or number on the site; set a registered business name
	// here if a payment gateway or Google's app review asks for one.
	operatorName    = ""
	operatorAddress = ""
	supportPhone    = ""
	contentType     = "text/html; charset=utf-8"
	// The pages load nothing from other origins and run no script; the
	// JSON-LD block is data, which script-src does not govern.
	contentSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'; img-src 'self'; font-src 'self'; " +
		"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
)

var (
	//go:embed templates/*.html
	templateFS embed.FS
	//go:embed static
	embedded embed.FS
)

// Plan is a price tier as the pages show it.
type Plan struct {
	ID       string
	Name     string
	PriceINR int
	// Period follows the price, e.g. "/month".
	Period        string
	Summary       string
	Highlights    []string
	Limits        []string
	IncludedTools []string
	Credits       int64
	CreditLabel   string
	CTA           string
	// Featured highlights the plan most visitors should pick.
	Featured bool
}

// QA is one FAQ entry.
type QA struct {
	Question, Answer string
}

// page is the data every template receives.
type page struct {
	Path        string
	Title       string
	Description string
	Canonical   string
	OGImage     string
	SignUp      string
	Support     string
	Updated     string
	Operator    string
	Address     string
	Phone       string
	Compare     []CompareRow
	Plans       []Plan
	FAQ         []QA
	JSONLD      any
	// Posts is the blog index; Post is the post on a post page.
	Posts []Post
	Post  *Post
}

// Site serves the pre-rendered pages and the static assets.
type Site struct {
	landing, pricing, blog renderedPage
	// posts are the rendered blog posts, keyed by slug.
	posts map[string]renderedPage
	// info holds the pages that are plain text (about, contact and the legal
	// pages), keyed by URL path.
	info            map[string]renderedPage
	robots, sitemap []byte
	static          fs.FS
	// versions maps each asset name to a hash of its content; asset URLs
	// carry it as ?v=, so a changed file gets a new URL and an unchanged one
	// can be cached forever.
	versions map[string]string
}

type renderedPage struct {
	body []byte
	etag string
}

// New renders the pages with publicURL, the site's absolute origin (e.g.
// https://uplumo.com), as the base of canonical, Open Graph and JSON-LD
// URLs.
func New(publicURL *url.URL) (*Site, error) {
	blogFiles, err := fs.Sub(blogFS, "blog")
	if err != nil {
		return nil, fmt.Errorf("open blog posts: %w", err)
	}
	return build(publicURL, blogFiles)
}

// build is New with the blog posts read from blogFiles.
func build(publicURL *url.URL, blogFiles fs.FS) (*Site, error) {
	static, err := fs.Sub(embedded, "static")
	if err != nil {
		return nil, fmt.Errorf("open static assets: %w", err)
	}
	versions, err := hashFiles(static)
	if err != nil {
		return nil, err
	}

	asset := func(file string) (string, error) {
		v, ok := versions[file]
		if !ok {
			return "", fmt.Errorf("unknown asset %q", file)
		}
		return AssetsPrefix + file + "?v=" + v, nil
	}
	tmpl, err := template.New("").Funcs(template.FuncMap{"asset": asset, "inr": formatINR}).
		ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse site templates: %w", err)
	}

	origin := strings.TrimSuffix(publicURL.String(), "/")
	ogImage, err := asset("social-card.png")
	if err != nil {
		return nil, err
	}
	plans := plans()
	base := page{
		OGImage: origin + ogImage, SignUp: signUpPath, Support: SupportEmail, Updated: policyUpdated,
		Operator: operatorName, Address: operatorAddress, Phone: supportPhone,
		Plans: plans, Compare: comparison(),
	}

	landing := base
	landing.Path = "/"
	landing.Title = "Uplumo: SEO and AI search analytics with honest INR pricing"
	landing.Description = "Track Google rankings, audit your site and see where AI assistants mention you and send visitors. Choose Free, Basic, Pro or Agency pricing in rupees."
	landing.Canonical = origin + "/"
	landing.FAQ = landingFAQ()
	landing.JSONLD = jsonLD(origin, landing.Description, plans)

	pricing := base
	pricing.Path = "/pricing"
	pricing.Title = "Uplumo pricing: Free, Basic, Pro and Agency plans"
	pricing.Description = fmt.Sprintf("Start free. Basic is %s, Pro is %s and Agency is %s a month in rupees. See what each plan includes, cancel in one click.",
		formatINR(price(planCatalog.Basic)), formatINR(price(planCatalog.Pro)), formatINR(price(planCatalog.Agency)))
	pricing.Canonical = origin + "/pricing"
	pricing.FAQ = pricingFAQ()
	pricing.JSONLD = jsonLD(origin, pricing.Description, plans)

	posts, err := loadPosts(blogFiles)
	if err != nil {
		return nil, err
	}
	s := &Site{static: static, versions: versions, info: map[string]renderedPage{}, posts: map[string]renderedPage{}}
	blog := base
	blog.Path = "/blog"
	blog.Title = "Uplumo blog: SEO and AI search guides"
	blog.Description = "Plain-language guides to ranking on Google and showing up in AI answers, for business owners and agencies in India."
	blog.Canonical = origin + "/blog"
	blog.Posts = posts
	blog.JSONLD = jsonLD(origin, blog.Description, plans)
	if s.blog, err = render(tmpl, "blog.html", blog); err != nil {
		return nil, err
	}
	for _, post := range posts {
		data := base
		data.Path = "/blog/" + post.Slug
		data.Title = post.Title + " | Uplumo"
		data.Description = post.Description
		data.Canonical = origin + data.Path
		data.Post = &post
		data.JSONLD = blogPostingJSONLD(origin, post, base.OGImage)
		if s.posts[post.Slug], err = render(tmpl, "post.html", data); err != nil {
			return nil, err
		}
	}
	for _, p := range infoPages {
		data := base
		data.Path = p.path
		data.Title = p.title
		data.Description = p.description
		data.Canonical = origin + p.path
		data.JSONLD = jsonLD(origin, p.description, plans)
		if s.info[p.path], err = render(tmpl, p.file, data); err != nil {
			return nil, err
		}
	}
	s.robots = []byte("User-agent: *\nAllow: /\nDisallow: /api/\n\nSitemap: " + origin + "/sitemap.xml\n")
	s.sitemap = sitemap(origin, posts)
	if s.landing, err = render(tmpl, "landing.html", landing); err != nil {
		return nil, err
	}
	if s.pricing, err = render(tmpl, "pricing.html", pricing); err != nil {
		return nil, err
	}
	return s, nil
}

// ServeLanding writes the landing page. It is revalidated on every visit
// because the server answers / with the app for signed-in users.
func (s *Site) ServeLanding(w http.ResponseWriter, r *http.Request) {
	servePage(w, r, s.landing, "no-cache, no-transform")
}

// ServePricing writes the pricing page.
func (s *Site) ServePricing(w http.ResponseWriter, r *http.Request) {
	servePage(w, r, s.pricing, "public, max-age=300, no-transform")
}

// InfoPaths lists the URL paths ServeInfo answers, for the router.
func InfoPaths() []string {
	paths := make([]string, 0, len(infoPages))
	for _, p := range infoPages {
		paths = append(paths, p.path)
	}
	return paths
}

// ServeInfo writes the about, contact or legal page at the request's path.
func (s *Site) ServeInfo(w http.ResponseWriter, r *http.Request) {
	p, ok := s.info[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	servePage(w, r, p, "public, max-age=300, no-transform")
}

// ServeBlog writes the blog index.
func (s *Site) ServeBlog(w http.ResponseWriter, r *http.Request) {
	servePage(w, r, s.blog, "public, max-age=300, no-transform")
}

// ServeBlogPost writes the post named by the {slug} path value.
func (s *Site) ServeBlogPost(w http.ResponseWriter, r *http.Request) {
	p, ok := s.posts[r.PathValue("slug")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	servePage(w, r, p, "public, max-age=300, no-transform")
}

// ServeRobots writes robots.txt: everything public may be crawled, the API
// may not.
func (s *Site) ServeRobots(w http.ResponseWriter, _ *http.Request) {
	serveText(w, "text/plain; charset=utf-8", s.robots)
}

// ServeSitemap writes sitemap.xml, listing every public page.
func (s *Site) ServeSitemap(w http.ResponseWriter, _ *http.Request) {
	serveText(w, "application/xml; charset=utf-8", s.sitemap)
}

func serveText(w http.ResponseWriter, contentType string, body []byte) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", "public, max-age=3600")
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

// sitemap lists every public page: the landing page, pricing, the info pages,
// the blog and each post with the date it last changed.
func sitemap(origin string, posts []Post) []byte {
	var b strings.Builder
	b.WriteString(xml.Header + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	write := func(path, lastmod string) {
		b.WriteString("<url><loc>")
		// Paths are constants or validated slugs; escaping keeps the file
		// valid if one ever carries a special character.
		_ = xml.EscapeText(&b, []byte(origin+path))
		b.WriteString("</loc>")
		if lastmod != "" {
			b.WriteString("<lastmod>" + lastmod + "</lastmod>")
		}
		b.WriteString("</url>\n")
	}
	for _, path := range append([]string{"/", "/pricing", "/blog"}, InfoPaths()...) {
		write(path, "")
	}
	for _, post := range posts {
		write("/blog/"+post.Slug, post.Updated.Format(time.DateOnly))
	}
	b.WriteString("</urlset>\n")
	return []byte(b.String())
}

// ServeAsset serves an embedded static file under AssetsPrefix. A request
// for the current version (?v=) may be cached for a year; any other is
// revalidated by ETag, so an outdated URL never pins old content.
func (s *Site) ServeAsset(w http.ResponseWriter, r *http.Request) {
	file := strings.TrimPrefix(r.URL.Path, AssetsPrefix)
	version, ok := s.versions[file]
	if !ok {
		http.NotFound(w, r)
		return
	}
	cacheControl := "no-cache"
	if r.URL.Query().Get("v") == version {
		cacheControl = "public, max-age=31536000, immutable"
	}
	// Go's built-in MIME table lacks woff2, and slim images have no
	// /etc/mime.types to fill the gap.
	if strings.HasSuffix(file, ".woff2") {
		w.Header().Set("Content-Type", "font/woff2")
	}
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", `"`+version+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// #nosec G703 -- file must match a key in the precomputed embedded-assets allowlist.
	http.ServeFileFS(w, r, s.static, file)
}

// servePage writes a rendered page. Every caller passes no-transform: the
// pages allow no script, so a proxy that rewrites them (Cloudflare swaps each
// email address for a script-decoded placeholder) leaves "[email protected]"
// on screen.
func servePage(w http.ResponseWriter, r *http.Request, p renderedPage, cacheControl string) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", p.etag)
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(p.body))
}

func render(tmpl *template.Template, file string, data page) (renderedPage, error) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, file, data); err != nil {
		return renderedPage{}, fmt.Errorf("render %s: %w", file, err)
	}
	return renderedPage{body: buf.Bytes(), etag: `"` + contentHash(buf.Bytes()) + `"`}, nil
}

func hashFiles(fsys fs.FS) (map[string]string, error) {
	versions := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		versions[path] = contentHash(data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("hash static assets: %w", err)
	}
	return versions, nil
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// formatINR formats non-negative whole rupees with Indian digit grouping:
// the last three digits, then groups of two (₹1,00,000).
func formatINR(rupees int) string {
	return "₹" + formatNumber(int64(rupees))
}

func formatNumber(number int64) string {
	s := strconv.FormatInt(number, 10)
	if len(s) > 3 {
		head, tail := s[:len(s)-3], s[len(s)-3:]
		for len(head) > 2 {
			head, tail = head[:len(head)-2], head[len(head)-2:]+","+tail
		}
		s = head + "," + tail
	}
	return s
}

// infoPages are the plain-text public pages. Each is one template file.
var infoPages = []struct {
	path, file, title, description string
}{
	{"/about", "about.html", "About Uplumo",
		"Uplumo is an SEO and AI search tool built in India for business owners and agencies who want one clear next step."},
	{"/contact", "contact.html", "Contact Uplumo",
		"Get help with your Uplumo account, billing or data. Write to us and a person replies."},
	{"/terms", "terms.html", "Terms of Service | Uplumo",
		"The rules for using Uplumo: your account, plans and billing, acceptable use and our responsibilities."},
	{"/privacy", "privacy.html", "Privacy Policy | Uplumo",
		"What data Uplumo collects, how it uses Google account data, who processes it and how to delete it."},
	{"/refunds", "refunds.html", "Cancellation and Refund Policy | Uplumo",
		"How to cancel Uplumo Pro, what happens after you cancel and when we refund a payment."},
	{"/shipping", "shipping.html", "Shipping and Delivery Policy | Uplumo",
		"Uplumo is online software: nothing is shipped, and access starts as soon as you sign up or pay."},
}

func plans() []Plan {
	catalog := planCatalog.Catalog()
	result := make([]Plan, 0, len(catalog))
	for _, item := range catalog {
		period, cta := "/month", "Choose "+item.Name
		if item.ID == planCatalog.Free {
			period, cta = "forever", "Start free"
		}
		result = append(result, Plan{
			ID: item.ID, Name: item.Name, PriceINR: item.PriceINR,
			Period: period, Summary: item.Summary, Highlights: item.Highlights,
			Limits: planLimits(item), IncludedTools: includedTools(item),
			Credits: item.MonthlyCredits, CreditLabel: formatNumber(item.MonthlyCredits),
			CTA: cta, Featured: item.ID == planCatalog.Pro,
		})
	}
	return result
}

func planLimits(plan planCatalog.Definition) []string {
	limits := []string{
		fmt.Sprintf("%d %s", plan.Sites, plural(plan.Sites, "website", "websites")),
		fmt.Sprintf("%d %s", plan.Members, plural(plan.Members, "team member", "team members")),
	}
	if plan.TrackedKeywords > 0 {
		limits = append(limits, formatNumber(int64(plan.TrackedKeywords))+" tracked keywords")
	}
	if plan.MaxSerpDepth > 0 {
		limits = append(limits, fmt.Sprintf("Rank checks cover the top %d results", plan.MaxSerpDepth))
	}
	return limits
}

func plural(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func includedTools(plan planCatalog.Definition) []string {
	tools := []string{"Uplumo Analytics with AI traffic detection"}
	if plan.Allows(planCatalog.FullAudit) {
		tools = append(tools, "Full site audit")
	} else {
		tools = append(tools, "Starter site audit")
	}
	if plan.MonthlyCredits > 0 {
		tools = append(tools, "SAM SEO assistant")
	}
	for _, item := range []struct {
		feature planCatalog.Feature
		label   string
	}{
		{planCatalog.GoogleData, "Google Search Console and Google Analytics"},
		{planCatalog.Keywords, "Keyword research and SERP analysis"},
		{planCatalog.RankTracking, "Rank tracking"},
		{planCatalog.Publishing, "Publish articles to WordPress"},
		{planCatalog.Backlinks, "Backlink research"},
		{planCatalog.DomainResearch, "Competitor and domain research"},
		{planCatalog.AIVisibility, "AI Visibility"},
		{planCatalog.AIWriting, "AI writing assistant"},
		{planCatalog.WhiteLabel, "White-label reports"},
	} {
		if plan.Allows(item.feature) {
			tools = append(tools, item.label)
		}
	}
	return tools
}

// price returns the monthly price of the plan with id.
func price(id string) int {
	plan, _ := planCatalog.Get(id)
	return plan.PriceINR
}

// CompareRow is one line of the plan comparison: a label and one cell per
// plan, in catalog order.
type CompareRow struct {
	Label string
	Cells []string
}

// comparison builds the comparison table from the catalog, so the page can
// only show what a plan check enforces.
func comparison() []CompareRow {
	catalog := planCatalog.Catalog()
	row := func(label string, cell func(planCatalog.Definition) string) CompareRow {
		cells := make([]string, len(catalog))
		for i, plan := range catalog {
			cells[i] = cell(plan)
		}
		return CompareRow{Label: label, Cells: cells}
	}
	included := func(feature planCatalog.Feature) func(planCatalog.Definition) string {
		return func(plan planCatalog.Definition) string {
			if plan.Allows(feature) {
				return "Included"
			}
			return "—"
		}
	}
	return []CompareRow{
		row("Websites", func(plan planCatalog.Definition) string { return strconv.Itoa(plan.Sites) }),
		row("Team members", func(plan planCatalog.Definition) string { return strconv.Itoa(plan.Members) }),
		row("Monthly credits", func(plan planCatalog.Definition) string { return formatNumber(plan.MonthlyCredits) }),
		row("Uplumo Analytics with AI traffic detection", func(planCatalog.Definition) string { return "Included" }),
		row("SAM SEO assistant", func(plan planCatalog.Definition) string {
			if plan.MonthlyCredits > 0 {
				return "Included"
			}
			return "—"
		}),
		row("Site audit", func(plan planCatalog.Definition) string {
			if plan.Allows(planCatalog.FullAudit) {
				return "Full"
			}
			return "Starter"
		}),
		row("Google Search Console and Google Analytics", included(planCatalog.GoogleData)),
		row("Keyword research", included(planCatalog.Keywords)),
		row("Rank tracking", func(plan planCatalog.Definition) string {
			if !plan.Allows(planCatalog.RankTracking) {
				return "—"
			}
			return strings.TrimPrefix(formatINR(plan.TrackedKeywords), "₹") + " keywords"
		}),
		row("Rank check depth", func(plan planCatalog.Definition) string {
			if plan.MaxSerpDepth == 0 {
				return "—"
			}
			return fmt.Sprintf("Top %d", plan.MaxSerpDepth)
		}),
		row("Backlink and competitor research", included(planCatalog.Backlinks)),
		row("AI Visibility", included(planCatalog.AIVisibility)),
		row("Publish articles to your WordPress site", included(planCatalog.Publishing)),
		row("AI writing assistant", included(planCatalog.AIWriting)),
		row("White-label reports", included(planCatalog.WhiteLabel)),
		row("Monthly price", func(plan planCatalog.Definition) string {
			if plan.PriceINR == 0 {
				return "Free"
			}
			return formatINR(plan.PriceINR)
		}),
	}
}

func landingFAQ() []QA {
	return []QA{
		{"What is Uplumo?", "An SEO and AI search platform. It tracks your Google rankings, audits your site, " +
			"shows where AI assistants mention your brand and measures the visitors they send you."},
		{"How is it different from other SEO platforms?", "Uplumo combines site analytics, Google search data, SEO research and AI visibility. Choose a tier by the number of websites and tools you need."},
		{"What is AI traffic detection?", "Most visits from ChatGPT, Perplexity or Gemini show up as \"Direct\" " +
			"in ordinary analytics. Uplumo Analytics recognises AI referrers and AI tracking tags, " +
			"so you can see which assistants send you visitors and which pages they land on."},
		{"Does the tracker need a cookie banner?", "The tracker sets no cookies and stores no IP addresses. " +
			"Visitors are counted with a hash that changes every day, so nobody can be followed over time."},
		{"Do I need technical skills?", "No. Every finding is explained in plain language, with what to do next. " +
			"Adding analytics means pasting one script tag into your site."},
	}
}

func pricingFAQ() []QA {
	basic, _ := planCatalog.Get(planCatalog.Basic)
	pro, _ := planCatalog.Get(planCatalog.Pro)
	agency, _ := planCatalog.Get(planCatalog.Agency)
	return []QA{
		{"Is the Free plan really free?", "Yes. It costs " + formatINR(0) +
			" and needs no card. Paid plans start at " + formatINR(basic.PriceINR) + " a month."},
		{"What is the difference between Basic and Pro?", "Basic is for ranking one site: keyword research, rank tracking and your Google data. " +
			"Pro adds what you need to beat competitors: backlink and competitor research, AI Visibility and the AI writing assistant, for " +
			strconv.Itoa(pro.Sites) + " sites."},
		{"How do site limits work?", fmt.Sprintf("Free and Basic include one active website, Pro includes %d and Agency includes %d. "+
			"Archiving a site frees a slot; your data stays in the workspace.", pro.Sites, agency.Sites)},
		{"How do I cancel a paid plan?", "In one click from Billing. You keep the plan until the end of the month you paid for, " +
			"and you are never charged again."},
		{"How do I change plan?", "Write to us from your account's email address and we move you. Changing plan from Billing is not built yet."},
		{"Will the price change without notice?", "No. If prices ever change, we tell you before your next bill " +
			"and you can cancel in one click."},
		{"How do I pay?", "Through Razorpay with UPI, cards or netbanking. Prices are in Indian rupees."},
	}
}

// jsonLD describes the organization and the product, with each plan as an
// offer, for search engines.
func jsonLD(origin, description string, plans []Plan) map[string]any {
	offers := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		offers = append(offers, map[string]any{
			"@type":         "Offer",
			"name":          p.Name,
			"price":         strconv.Itoa(p.PriceINR),
			"priceCurrency": "INR",
			"url":           origin + "/pricing",
		})
	}
	orgID := origin + "/#organization"
	return map[string]any{
		"@context": "https://schema.org",
		"@graph": []map[string]any{
			{
				"@type": "Organization",
				"@id":   orgID,
				"name":  name,
				"url":   origin + "/",
				"logo":  origin + AssetsPrefix + "logo.svg",
			},
			{
				"@type":               "SoftwareApplication",
				"name":                name,
				"url":                 origin + "/",
				"description":         description,
				"applicationCategory": "BusinessApplication",
				"operatingSystem":     "Web",
				"publisher":           map[string]string{"@id": orgID},
				"offers":              offers,
			},
		},
	}
}
