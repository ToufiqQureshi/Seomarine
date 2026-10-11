package reports

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/branding"
)

// The report sandbox lives here so the header `/r/{id}` serves and the
// `sandbox` attribute on the share frame cannot drift. A report is a full HTML
// document written by a model from attacker-influenceable inputs, so it is
// hostile in both places. `allow-same-origin` must never be added.
const (
	iframeSandbox = "allow-popups allow-popups-to-escape-sandbox"
	cspTail       = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'; object-src 'none'; frame-ancestors 'self'"
	// printScript opens every closed <details>, then prints once fonts settle.
	printScript = `addEventListener("load",()=>{for(const d of document.querySelectorAll("details"))d.open=true;setTimeout(()=>print(),150)})`
	// printScriptSHA256 is the base64 SHA-256 of printScript for script-src.
	printScriptSHA256 = "uE0MLbgBgIu3I3pghlY0762kVHKlP7Udz5qzZNT7piw="
)

// ReportCSP is the policy for one response; print mode adds exactly one
// hashed script.
func ReportCSP(print bool) string {
	if !print {
		return "sandbox " + iframeSandbox + "; " + cspTail
	}
	return "sandbox " + iframeSandbox + " allow-scripts allow-modals; script-src 'sha256-" + printScriptSHA256 + "'; " + cspTail
}

// withPrintScript splices the print script before the last </body> without
// parsing a document we did not write.
func withPrintScript(doc string) string {
	tag := "<script>" + printScript + "</script>"
	if i := strings.LastIndex(doc, "</body>"); i != -1 {
		return doc[:i] + tag + doc[i:]
	}
	if i := strings.LastIndex(doc, "</html>"); i != -1 {
		return doc[:i] + tag + doc[i:]
	}
	return doc + tag
}

var bodyOpen = regexp.MustCompile(`(?i)<body\b[^>]*>`)

var brandBarTemplate = template.Must(template.New("bar").Parse(
	`<div data-brand-bar="" style="all:initial;display:flex;align-items:center;gap:12px;padding:12px 24px;border-bottom:3px solid {{.Accent}};background:#ffffff;color:#111111;font-family:ui-sans-serif,system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;font-size:14px;print-color-adjust:exact;-webkit-print-color-adjust:exact">` +
		`{{if .Logo}}<img src="{{.Logo}}" alt="" style="height:32px;width:auto;max-width:160px">{{end}}` +
		`<span style="font-weight:600;font-size:16px">{{.Name}}</span>` +
		`{{if .Site}}<a href="{{.Site}}" target="_blank" rel="noreferrer" style="margin-left:auto;font-size:13px;color:{{.Accent}};text-decoration:none">{{.Host}}</a>{{end}}</div>`))

var accentColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// withBrandBar stamps the white-label bar right after <body>. A document with
// no <body> is served unbranded rather than spliced before its doctype, which
// would drop it into quirks mode.
func withBrandBar(doc string, b *branding.Branding) string {
	if b == nil {
		return doc
	}
	loc := bodyOpen.FindStringIndex(doc)
	if loc == nil {
		return doc
	}
	accent := b.AccentColor
	if !accentColor.MatchString(accent) {
		accent = branding.DefaultAccent
	}
	data := struct {
		Accent template.CSS
		Logo   template.URL
		Name   string
		Site   string
		Host   string
	}{Accent: template.CSS(accent), Name: b.BrandName}
	if b.LogoDataURL != nil && strings.HasPrefix(*b.LogoDataURL, "data:image/") {
		data.Logo = template.URL(*b.LogoDataURL)
	}
	if b.WebsiteURL != nil {
		if u, err := url.Parse(*b.WebsiteURL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			data.Site, data.Host = u.String(), u.Host
		}
	}
	var buf bytes.Buffer
	if err := brandBarTemplate.Execute(&buf, data); err != nil {
		return doc
	}
	return doc[:loc[1]] + buf.String() + doc[loc[1]:]
}

// ViewDeps are the collaborators of the member viewer `/r/{id}`.
type ViewDeps struct {
	Authenticate     func(*http.Request) (auth.User, error)
	AuthorizeProject func(ctx context.Context, userID, projectID string) (string, error)
	Branding         func(ctx context.Context, organizationID string) (branding.Branding, bool, error)
}

func plain(w http.ResponseWriter, body string, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

const notFoundBody = "This report does not exist or you do not have access to it."

// viewReport serves the stored document byte for byte, locked down by the
// report CSP, to a signed-in member of the owning organization. One body for
// "no such report" and "someone else's report" so ids cannot be probed.
func (d Deps) viewReport(w http.ResponseWriter, r *http.Request) {
	if d.Service == nil || d.View.Authenticate == nil {
		http.NotFound(w, r)
		return
	}
	reportID := r.PathValue("reportId")
	user, err := d.View.Authenticate(r)
	if errors.Is(err, auth.ErrUnauthenticated) {
		target := "/r/" + url.PathEscape(reportID)
		w.Header().Set("Cache-Control", "private, no-store")
		http.Redirect(w, r, "/sign-in?redirect="+url.QueryEscape(target), http.StatusFound)
		return
	}
	if err != nil {
		d.logErr(r, "authenticate report viewer", err)
		plain(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	projectID, err := d.Service.Store.ReportProject(r.Context(), reportID)
	if errors.Is(err, ErrNotFound) {
		plain(w, notFoundBody, http.StatusNotFound)
		return
	}
	if err != nil {
		d.logErr(r, "find report project", err)
		plain(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	orgID, err := d.View.AuthorizeProject(r.Context(), user.ID, projectID)
	if errors.Is(err, auth.ErrProjectNotFound) {
		plain(w, notFoundBody, http.StatusNotFound)
		return
	}
	if err != nil {
		d.logErr(r, "authorize report viewer", err)
		plain(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	_, doc, err := d.Service.Store.GetHTML(r.Context(), projectID, reportID)
	if errors.Is(err, ErrNotFound) {
		plain(w, notFoundBody, http.StatusNotFound)
		return
	}
	if err != nil {
		d.logErr(r, "load report", err)
		plain(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	var bar *branding.Branding
	if d.View.Branding != nil {
		b, ok, err := d.View.Branding(r.Context(), orgID)
		if err != nil {
			d.logErr(r, "load branding", err)
		} else if ok {
			bar = &b
		}
	}
	print := r.URL.Query().Get("print") == "1"
	doc = withBrandBar(doc, bar)
	if print {
		doc = withPrintScript(doc)
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", ReportCSP(print))
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	_, _ = io.WriteString(w, doc)
}

func (d Deps) logErr(r *http.Request, msg string, err error) {
	if d.Logger != nil && r.Context().Err() == nil {
		d.Logger.ErrorContext(r.Context(), msg, "err", err)
	}
}

// relativeTime renders "5 minutes ago" style text for the share header.
func relativeTime(then, now time.Time) string {
	d := now.Sub(then)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute")
	case d < 24*time.Hour:
		return plural(int(d/time.Hour), "hour")
	case d < 30*24*time.Hour:
		return plural(int(d/(24*time.Hour)), "day")
	}
	return then.UTC().Format("2 Jan 2006")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit + " ago"
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}
