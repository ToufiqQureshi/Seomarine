// Package site renders Seomarine's public marketing pages, the landing page
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
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Prices in rupees per month. They are the only place the public pages take
// prices from; ProPriceINR must match the Razorpay plan RAZORPAY_PLAN_ID_PRO
// bills.
const (
	FreePriceINR = 0
	ProPriceINR  = 1499
)

// AssetsPrefix is the URL path the embedded static files are served under.
const AssetsPrefix = "/site/"

const (
	name        = "Seomarine"
	signUpPath  = "/sign-up"
	contentType = "text/html; charset=utf-8"
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
	Name     string
	PriceINR int
	// Period follows the price, e.g. "/month".
	Period   string
	Summary  string
	Features []string
	CTA      string
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
	ProPriceINR int
	Plans       []Plan
	FAQ         []QA
	JSONLD      any
}

// Site serves the pre-rendered pages and the static assets.
type Site struct {
	landing, pricing renderedPage
	static           fs.FS
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
// https://seomarine.com), as the base of canonical, Open Graph and JSON-LD
// URLs.
func New(publicURL *url.URL) (*Site, error) {
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
	base := page{OGImage: origin + ogImage, SignUp: signUpPath, ProPriceINR: ProPriceINR, Plans: plans}

	landing := base
	landing.Path = "/"
	landing.Title = "Seomarine: SEO and AI search analytics with honest INR pricing"
	landing.Description = "Track rankings, audit your site and see where ChatGPT, Perplexity, Gemini and Claude " +
		"mention you and send visitors. AI visibility included, simple pricing, cancel in one click."
	landing.Canonical = origin + "/"
	landing.FAQ = landingFAQ()
	landing.JSONLD = jsonLD(origin, landing.Description, plans)

	pricing := base
	pricing.Path = "/pricing"
	pricing.Title = "Pricing: Free and Pro plans in INR | Seomarine"
	pricing.Description = fmt.Sprintf("Seomarine is free to start. Pro is %s a month with AI visibility "+
		"included, no row caps and one-click cancel. No add-ons, no credit maths.", formatINR(ProPriceINR))
	pricing.Canonical = origin + "/pricing"
	pricing.FAQ = pricingFAQ()
	pricing.JSONLD = jsonLD(origin, pricing.Description, plans)

	s := &Site{static: static, versions: versions}
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
	servePage(w, r, s.landing, "no-cache")
}

// ServePricing writes the pricing page.
func (s *Site) ServePricing(w http.ResponseWriter, r *http.Request) {
	servePage(w, r, s.pricing, "public, max-age=300")
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
	http.ServeFileFS(w, r, s.static, file)
}

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
	s := strconv.Itoa(rupees)
	if len(s) > 3 {
		head, tail := s[:len(s)-3], s[len(s)-3:]
		for len(head) > 2 {
			head, tail = head[:len(head)-2], head[len(head)-2:]+","+tail
		}
		s = head + "," + tail
	}
	return "₹" + s
}

func plans() []Plan {
	return []Plan{
		{
			Name:     "Free",
			PriceINR: FreePriceINR,
			Period:   "forever",
			Summary:  "For solo owners who want to see what is working.",
			Features: []string{
				"Seomarine Analytics with AI traffic detection",
				"Site audit with every issue explained in plain language",
				"White-label client reports",
				"No card needed to start",
			},
			CTA: "Start free",
		},
		{
			Name:     "Pro",
			PriceINR: ProPriceINR,
			Period:   "/month",
			Summary:  "For businesses and agencies growing in Google and in AI answers.",
			Features: []string{
				"Everything in Free",
				"AI Visibility across ChatGPT, Perplexity, Gemini, AI Overviews and Claude, included",
				"Rank tracking and keyword research",
				"No row caps on Seomarine data",
				"Cancel in one click, any time",
			},
			CTA:      "Start with Pro",
			Featured: true,
		},
	}
}

func landingFAQ() []QA {
	return []QA{
		{"What is Seomarine?", "An SEO and AI search platform. It tracks your Google rankings, audits your site, " +
			"shows where AI assistants mention your brand and measures the visitors they send you."},
		{"How is it different from Semrush or Ahrefs?", "You get the features most sites actually use, " +
			"one clear next step on every screen and AI visibility in the plan instead of a paid add-on. " +
			"Prices are in rupees and there are no row caps on our own data."},
		{"What is AI traffic detection?", "Most visits from ChatGPT, Perplexity or Gemini show up as \"Direct\" " +
			"in ordinary analytics. Seomarine Analytics recognises AI referrers and AI tracking tags, " +
			"so you can see which assistants send you visitors and which pages they land on."},
		{"Does the tracker need a cookie banner?", "The tracker sets no cookies and stores no IP addresses. " +
			"Visitors are counted with a hash that changes every day, so nobody can be followed over time."},
		{"Do I need technical skills?", "No. Every finding is explained in plain language, with what to do next. " +
			"Adding analytics means pasting one script tag into your site."},
	}
}

func pricingFAQ() []QA {
	return []QA{
		{"Is the Free plan really free?", "Yes. It costs " + formatINR(FreePriceINR) +
			" and needs no card. Upgrade only when you want Pro features."},
		{"How do I cancel Pro?", "In one click from Billing. You keep Pro until the end of the month you paid for, " +
			"and you are never charged again."},
		{"Is AI visibility an extra charge?", "No. AI visibility is part of Pro. There is no per-domain add-on."},
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
