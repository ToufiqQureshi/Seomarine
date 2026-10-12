package reports

import (
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/toufiqqureshi/uplumo/backend/internal/branding"
)

const maxDescriptionChars = 200

// shareDescription is the link preview text: the summary's first non-empty
// line, capped. Markdown is not stripped; a stray "##" in a preview is a
// smaller problem than a stripper that eats a leading minus sign.
func shareDescription(summary string) string {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) <= maxDescriptionChars {
			return line
		}
		return strings.TrimRight(string([]rune(line)[:maxDescriptionChars-1]), " \t") + "…"
	}
	return ""
}

const shareStyles = `:root{color-scheme:light dark;--bg:oklch(97.5% 0.006 200);--surface:oklch(100% 0 0);--border:oklch(91% 0.012 210);--text:oklch(23% 0.035 235);--muted:oklch(23% 0.035 235 / .68);--primary:oklch(50% 0.095 195);--primary-text:oklch(100% 0 0);--ghost-hover:oklch(23% 0.035 235 / .08)}
@media (prefers-color-scheme:dark){:root{--bg:oklch(17% 0.022 235);--surface:oklch(21% 0.025 232);--border:oklch(30% 0.025 230);--text:oklch(93% 0.01 200);--muted:oklch(93% 0.01 200 / .65);--primary:oklch(74% 0.12 185);--primary-text:oklch(19% 0.03 230);--ghost-hover:oklch(93% 0.01 200 / .1)}}
*{box-sizing:border-box}
html,body{height:100%;margin:0}
body{display:flex;flex-direction:column;background:var(--bg);color:var(--text);font:14px/1.5 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
header{display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:8px 16px;padding:8px 16px;background:var(--surface);border-bottom:1px solid var(--border)}
.title{width:100%;min-width:0}
@media (min-width:640px){.title{width:auto;flex:1}}
h1{margin:0;font-size:14px;font-weight:500;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.meta{margin:0;font-size:12px;color:var(--muted)}
.actions{display:flex;flex-shrink:0;align-items:center;gap:8px}
.btn{display:inline-flex;align-items:center;gap:6px;height:32px;padding:0 12px;border:1px solid transparent;border-radius:6px;font:inherit;font-weight:600;color:inherit;background:transparent;cursor:pointer;text-decoration:none}
.btn:hover{background:var(--ghost-hover)}
.btn.primary{background:var(--primary);border-color:var(--primary);color:var(--primary-text)}
.btn svg{width:16px;height:16px}
iframe{flex:1;min-height:0;width:100%;border:0;background:var(--surface)}
main{flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:12px;padding:24px;text-align:center}
main h1{white-space:normal;font-size:18px}
main p{max-width:28rem;margin:0;color:var(--muted)}`

// shareScript is the Share button's clipboard handler, the only script on the page.
const shareScript = `document.getElementById("share").addEventListener("click",async function(){
  var url=location.href,label=this.querySelector("span");
  if(matchMedia("(pointer: coarse)").matches&&navigator.share){
    try{await navigator.share({title:document.title,url:url})}catch(e){}
    return;
  }
  try{await navigator.clipboard.writeText(url);label.textContent="Link copied"}
  catch(e){label.textContent="Copy failed"}
  setTimeout(function(){label.textContent="Share"},2000);
});`

var sharePageTemplate = template.Must(template.New("share").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover"><meta name="robots" content="noindex, nofollow"><title>{{.Title}} · {{.SiteName}}</title>
{{if .OK}}<meta property="og:type" content="article"><meta property="og:site_name" content="{{.SiteName}}"><meta property="og:title" content="{{.Title}}"><meta name="twitter:title" content="{{.Title}}">
{{if .Description}}<meta property="og:description" content="{{.Description}}"><meta name="description" content="{{.Description}}">{{end}}
<meta property="og:url" content="{{.Canonical}}"><meta property="og:image" content="{{.Image}}"><meta property="og:image:width" content="1200"><meta property="og:image:height" content="630"><meta property="og:image:alt" content="{{.Title}} · {{.SiteName}}"><meta name="twitter:image" content="{{.Image}}"><meta name="twitter:image:alt" content="{{.Title}} · {{.SiteName}}"><meta name="twitter:card" content="summary_large_image">{{end}}
<link rel="icon" href="/favicon.ico"><style>{{.Styles}}</style></head><body>
{{if .OK}}<header><div class="title"><h1>{{.Title}}</h1><p class="meta">{{.Byline}} · Updated {{.Updated}}</p></div><div class="actions"><button type="button" class="btn" id="share"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" x2="15.42" y1="13.51" y2="17.49"/><line x1="15.41" x2="8.59" y1="6.51" y2="10.49"/></svg><span>Share</span></button>
{{if .VisitURL}}<a class="btn primary" href="{{.VisitURL}}" target="_blank" rel="noreferrer" style="background:{{.Accent}};border-color:{{.Accent}}">Visit {{.BrandName}}</a>{{else if not .Branded}}<a class="btn primary" href="/" target="_blank" rel="noreferrer">Try Uplumo</a>{{end}}</div></header>
<iframe src="{{.RawPath}}" sandbox="` + iframeSandbox + `" referrerpolicy="no-referrer" title="{{.Title}}"></iframe><script>{{.Script}}</script>
{{else}}<main><h1>{{.Heading}}</h1><p>{{.Detail}}</p><a class="btn primary" href="/" target="_blank" rel="noreferrer">Try Uplumo</a></main>{{end}}</body></html>`))

type sharePage struct {
	OK                                      bool
	Title, SiteName, Description, Canonical string
	Image, Byline, Updated, RawPath         string
	BrandName, Heading, Detail              string
	VisitURL                                template.URL
	Accent                                  template.CSS
	Branded                                 bool
	Styles                                  template.CSS
	Script                                  template.JS
}

func writeSharePage(w http.ResponseWriter, p sharePage, status int) {
	p.Styles, p.Script = template.CSS(shareStyles), template.JS(shareScript)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The head carries the title and summary, so a revoked link must not
	// outlive its document in an intermediary cache.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := sharePageTemplate.Execute(w, p); err != nil {
		return
	}
}

func unavailable(w http.ResponseWriter, heading, detail string) {
	// Every dead end answers 404 with a generic title, so the report's own
	// title stays out of a page the link no longer grants access to.
	writeSharePage(w, sharePage{Title: "Report unavailable", SiteName: "Uplumo", Heading: heading, Detail: detail}, http.StatusNotFound)
}

func (d Deps) sharedPage(w http.ResponseWriter, r *http.Request) {
	missing := func() {
		unavailable(w, "This report isn't shared.", "The link may have been turned off, or the report may have been deleted.")
	}
	if d.Service == nil {
		missing()
		return
	}
	token := r.PathValue("token")
	report, err := d.Service.GetShared(r.Context(), token)
	var typed *Error
	if errors.As(err, &typed) && typed.Code == "NOT_FOUND" {
		missing()
		return
	}
	if err != nil {
		d.logErr(r, "load shared report page", err)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	if report.Archived {
		unavailable(w, "This project has been archived.", "Its reports are hidden until the owner restores it.")
		return
	}
	siteName, byline := "Uplumo", "Made with Uplumo"
	page := sharePage{OK: true, Title: report.Title, Description: shareDescription(report.Summary), RawPath: "/s/" + token + "/raw"}
	if d.View.Branding != nil {
		b, ok, err := d.View.Branding(r.Context(), report.OrganizationID)
		if err != nil {
			d.logErr(r, "load share branding", err)
		} else if ok {
			page.Branded = true
			siteName, byline = b.BrandName, "Prepared by "+b.BrandName
			page.BrandName = b.BrandName
			page.Accent = template.CSS(safeAccent(b))
			page.VisitURL = visitURL(b)
		}
	}
	page.SiteName, page.Byline = siteName, byline
	page.Canonical = d.projectURL(r, "/s/"+token)
	page.Image = d.projectURL(r, "/social-card.png")
	if t, err := time.Parse(time.RFC3339Nano, report.UpdatedAt); err == nil {
		page.Updated = relativeTime(t, time.Now())
	} else {
		page.Updated = report.UpdatedAt
	}
	writeSharePage(w, page, http.StatusOK)
}

func safeAccent(b branding.Branding) string {
	if accentColor.MatchString(b.AccentColor) {
		return b.AccentColor
	}
	return branding.DefaultAccent
}

func visitURL(b branding.Branding) template.URL {
	if b.WebsiteURL == nil {
		return ""
	}
	if !strings.HasPrefix(*b.WebsiteURL, "https://") && !strings.HasPrefix(*b.WebsiteURL, "http://") {
		return ""
	}
	return template.URL(*b.WebsiteURL)
}
