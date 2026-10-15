package linksuggest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/gsc"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
)

// sourceTimeout bounds every source query, so a slow Search Console or
// audit read cannot hold the request open.
const sourceTimeout = 20 * time.Second

// GSC serves pages from the project's connected Search Console property.
// Google first-party reads are free; they are still quota-limited and
// bounded.
type GSC struct {
	// Service is nil when Google is not configured; the source then returns
	// no pages and the caller falls back to the audit.
	Service *gsc.Service
}

// sourceWindow is how far back GSC data is read.
const sourceWindow = 90 * 24 * time.Hour

// Pages returns the project's pages with impressions in the last 90 days,
// each with its top queries, ranked by clicks. Every read is bounded: at
// most 1,000 page rows and 1,000 query+page pair rows.
func (g GSC) Pages(ctx context.Context, organizationID, projectID string) ([]SourcePage, error) {
	if g.Service == nil {
		return nil, errors.New("search console is not configured on this server")
	}
	sourceCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
	defer cancel()
	end := time.Now().UTC().AddDate(0, 0, -3) // Search Console's data lag.
	start := end.Add(-sourceWindow)
	_, pageRows, err := g.Service.GetPageRows(sourceCtx, organizationID, projectID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	pairs, err := g.Service.GetQueryPageRows(sourceCtx, organizationID, projectID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		// Queries are a nicer reason line, not a requirement.
		pairs = []gsc.SearchRow{}
	}
	queriesByPage := topQueries(pairs)
	pages := make([]SourcePage, 0, len(pageRows))
	seenURLs := map[string]bool{}
	for _, row := range pageRows {
		if len(row.Keys) == 0 || row.Impressions <= 0 {
			continue
		}
		pageURL := normalizePageURL(row.Keys[0])
		if pageURL == "" || seenURLs[pageURL] {
			continue
		}
		seenURLs[pageURL] = true
		pages = append(pages, SourcePage{
			URL: pageURL, Title: pageTitle(row.Keys[0]),
			Clicks: row.Clicks, Impressions: row.Impressions,
			Queries: queriesByPage[pageURL],
		})
		if len(pages) >= MaxSourcePages {
			break
		}
	}
	return pages, nil
}

// topQueries groups the query+page rows per page, keeping each page's three
// best queries by clicks.
func topQueries(rows []gsc.SearchRow) map[string][]string {
	type queryClicks struct {
		query  string
		clicks float64
	}
	byPage := map[string][]queryClicks{}
	for _, row := range rows {
		if len(row.Keys) < 2 {
			continue
		}
		query, page := strings.TrimSpace(row.Keys[0]), normalizePageURL(row.Keys[1])
		if query == "" || page == "" {
			continue
		}
		byPage[page] = append(byPage[page], queryClicks{query: query, clicks: row.Clicks})
	}
	out := make(map[string][]string, len(byPage))
	for page, entries := range byPage {
		entry := entries
		slices.SortStableFunc(entry, func(a, b queryClicks) int {
			switch {
			case a.clicks > b.clicks:
				return -1
			case a.clicks < b.clicks:
				return 1
			default:
				return strings.Compare(a.query, b.query)
			}
		})
		if len(entry) > 3 {
			entry = entry[:3]
		}
		for _, item := range entry {
			out[page] = append(out[page], item.query)
		}
	}
	return out
}

// pageTitle derives a readable fallback title from the URL's last segment;
// audit pages carry their real <title>, GSC rows carry only an address.
func pageTitle(rawURL string) string {
	end := strings.TrimRight(rawURL, "/")
	if last := strings.LastIndexAny(end, "/?"); last >= 0 {
		segment := strings.TrimPrefix(end[last+1:], "p=")
		segment = strings.Split(segment, "#")[0]
		return strings.ReplaceAll(segment, "-", " ")
	}
	return ""
}

// AuditSource returns the crawled pages of the project's latest completed
// audit, scoped by workspace and project, for workspaces without the Search
// Console connection.
type AuditSource struct{ DB *pgxpool.Pool }

// Pages loads at most MaxSourcePages pages of the latest completed audit.
// Another workspace's audit can never appear: the query joins the project
// to the calling organization.
func (r AuditSource) Pages(ctx context.Context, organizationID, projectID string) ([]SourcePage, error) {
	sourceCtx, cancel := context.WithTimeout(ctx, sourceTimeout)
	defer cancel()
	var auditID string
	err := r.DB.QueryRow(sourceCtx, `SELECT a.id FROM go_audits a
		JOIN projects p ON p.id = a.project_id AND p.organization_id = $1
		WHERE a.project_id = $2 AND a.status = 'completed'
		ORDER BY a.started_at DESC LIMIT 1`, organizationID, projectID).Scan(&auditID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find latest completed audit: %w", err)
	}
	rows, err := r.DB.Query(sourceCtx, `SELECT url, COALESCE(title, '') FROM go_audit_pages
		WHERE audit_id = $1 AND url <> '' LIMIT $2`, auditID, MaxSourcePages)
	if err != nil {
		return nil, fmt.Errorf("list audit pages: %w", err)
	}
	defer rows.Close()
	pages := make([]SourcePage, 0, MaxSourcePages)
	for rows.Next() {
		var url, title string
		if err := rows.Scan(&url, &title); err != nil {
			return nil, fmt.Errorf("scan audit page: %w", err)
		}
		if pageURL := normalizePageURL(url); pageURL != "" {
			pages = append(pages, SourcePage{URL: pageURL, Title: strings.TrimSpace(title)})
		}
	}
	return pages, rows.Err()
}

// normalizePageURL keeps one canonical address per page: no whitespace, no
// trailing slash, so the draft's own slug cannot be outwitted by either.
func normalizePageURL(rawURL string) string {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return ""
	}
	return strings.TrimRight(value, "/")
}

// Mount registers the link suggestion route. Like the content editor's
// routes, any member of the workspace may call it: it reads free data.
func Mount(mux *http.ServeMux, deps Deps) {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	h := handler{deps}
	protect := func(next http.HandlerFunc) http.Handler { return deps.WithSession(deps.WithProjectAccess(next)) }
	mux.Handle("POST /api/v1/projects/{projectId}/content/link-suggestions", protect(h.suggest))
}

// Deps wires the route. Service is nil when the database is not configured;
// the route then answers 503.
type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

type handler struct{ Deps }

func (h handler) suggest(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "link_suggestions_unavailable", "Internal link suggestions are not available on this server.")
		return
	}
	var input Input
	if !decode(w, r, &input) {
		return
	}
	organizationID, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || organizationID == "" {
		// requireProjectAccess already refused a foreign project; this is a
		// second guard for the plain handler path.
		httpx.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found.")
		return
	}
	suggestions, err := h.Service.Suggestions(r.Context(), organizationID, r.PathValue("projectId"), input)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, suggestions)
	case errors.As(err, new(*ValidationError)):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		if r.Context().Err() == nil {
			h.Logger.ErrorContext(r.Context(), "suggest internal links", "err", err)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
	}
}

// maxRequestBytes bounds the body: the editor's 150,000-character limit
// grows to at most about 600 KB of UTF-8, plus slack for the other fields.
const maxRequestBytes = 800 << 10

func decode(w http.ResponseWriter, r *http.Request, dst *Input) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	trimmed := bytes.TrimSpace(body)
	if err != nil || len(trimmed) == 0 || trimmed[0] != '{' {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Send a JSON object with the draft text.")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Send a JSON object with the draft text.")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
		return false
	}
	return true
}
