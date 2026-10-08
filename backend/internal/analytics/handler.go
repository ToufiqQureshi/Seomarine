package analytics

import (
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const (
	maxCollectBody = 4 << 10
	maxSiteKey     = 64
	maxURL         = 2048
	maxScreenWidth = 100_000
)

// Deps contains analytics services and the root router's authorization middleware.
type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	TrustedProxyCIDRs []netip.Prefix
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers analytics routes on mux.
func Mount(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("POST /collect", collect(d.Logger, d.Service, d.TrustedProxyCIDRs))
	mux.HandleFunc("OPTIONS /collect", collectPreflight)
	mux.Handle("POST /api/v1/projects/{projectId}/analytics/site", d.WithSession(d.WithProjectAccess(ensureSiteHandler(d.Logger, d.Service))))
	mux.Handle("GET /api/v1/projects/{projectId}/analytics/summary", d.WithSession(d.WithProjectAccess(summaryHandler(d.Logger, d.Service))))
	mux.Handle("GET /api/v1/analytics/{siteId}/countries", d.WithSession(countriesHandler(d.Logger, d.Service)))
}

func collectPreflight(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

type siteResponse struct {
	ID      int64  `json:"id"`
	SiteKey string `json:"siteKey"`
	Snippet string `json:"snippet"`
}

// collect records a tracker event. Any site may post to it, so CORS is open.
func collect(logger *slog.Logger, svc *Service, trusted []netip.Prefix) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var req struct {
			SiteKey     string `json:"k"`
			URL         string `json:"u"`
			Referrer    string `json:"r"`
			ScreenWidth int    `json:"w"`
		}
		err := httpx.DecodeJSONLimit(r.Body, maxCollectBody, &req)
		if errors.Is(err, httpx.ErrPayloadTooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "The event is too large.")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_event", "The event is not valid JSON.")
			return
		}
		page, msg := validateCollect(req)
		if msg != "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_event", msg)
			return
		}

		err = svc.Collect(r.Context(), Hit{
			SiteKey:     req.SiteKey,
			Page:        page,
			Referrer:    req.Referrer,
			ScreenWidth: req.ScreenWidth,
			IP:          clientIP(r, trusted),
			UserAgent:   r.UserAgent(),
		})
		switch {
		case err == nil:
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, ErrRateLimited):
			httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Too many events. Slow down.")
		case errors.Is(err, ErrUnknownSite):
			httpx.WriteError(w, http.StatusNotFound, "unknown_site", "No site uses this key.")
		case errors.Is(err, ErrForeignHost):
			httpx.WriteError(w, http.StatusForbidden, "foreign_host", "This page is not on the project's domain.")
		default:
			logger.ErrorContext(r.Context(), "collect analytics event", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
		}
	}
}

// validateCollect checks an event's fields and returns the parsed page URL.
func validateCollect(req struct {
	SiteKey     string `json:"k"`
	URL         string `json:"u"`
	Referrer    string `json:"r"`
	ScreenWidth int    `json:"w"`
}) (*url.URL, string) {
	if req.SiteKey == "" || len(req.SiteKey) > maxSiteKey {
		return nil, "k must be a site key."
	}
	if len(req.URL) > maxURL || len(req.Referrer) > maxURL {
		return nil, fmt.Sprintf("u and r must be at most %d bytes.", maxURL)
	}
	page, err := url.Parse(req.URL)
	if err != nil || (page.Scheme != "http" && page.Scheme != "https") || page.Hostname() == "" {
		return nil, "u must be an absolute http or https URL."
	}
	if req.ScreenWidth < 0 || req.ScreenWidth > maxScreenWidth {
		return nil, fmt.Sprintf("w must be a screen width from 0 to %d.", maxScreenWidth)
	}
	return page, ""
}

// clientIP accepts forwarding headers only from an explicitly trusted peer.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	allowed := false
	for _, prefix := range trusted {
		if prefix.Contains(peer) {
			allowed = true
			break
		}
	}
	if !allowed {
		return peer.String()
	}
	if cf, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); err == nil {
		return cf.String()
	}
	xff := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	entries := strings.Split(xff, ",")
	for i := len(entries) - 1; i >= 0; i-- {
		if ip, err := netip.ParseAddr(strings.TrimSpace(entries[i])); err == nil {
			return ip.String()
		}
	}
	return peer.String()
}

// ensureSiteHandler returns the project's site key and tracker snippet.
func ensureSiteHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := svc.EnsureSite(r.Context(), r.PathValue("projectId"))
		if err != nil {
			logger.ErrorContext(r.Context(), "ensure analytics site", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		id, err := svc.SiteID(r.Context(), r.PathValue("projectId"))
		if err != nil {
			logger.ErrorContext(r.Context(), "load analytics site id", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		snippet := fmt.Sprintf(`<script defer data-site="%s" src="%s/t.js"></script>`,
			html.EscapeString(key), html.EscapeString(publicOrigin(r)))
		httpx.WriteJSON(w, http.StatusOK, siteResponse{ID: id, SiteKey: key, Snippet: snippet})
	}
}

// countriesHandler returns the countries breakdown for a site.
func countriesHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("siteId"), 10, 64)
		if err != nil || id < 1 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_site", "siteId must be a positive number.")
			return
		}
		q := r.URL.Query()
		from, fromErr := time.Parse(time.DateOnly, q.Get("from"))
		to, toErr := time.Parse(time.DateOnly, q.Get("to"))
		if fromErr != nil || toErr != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_range", "from and to must be dates like 2026-01-31.")
			return
		}
		page, limit, pageErr := httpx.ParsePageLimit(q)
		if errors.Is(pageErr, httpx.ErrInvalidPage) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_page", "page must be between 1 and 1000.")
			return
		}
		if errors.Is(pageErr, httpx.ErrInvalidLimit) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100.")
			return
		}
		user, _ := auth.UserFromContext(r.Context())
		result, err := svc.Countries(r.Context(), id, user.ID, from, to, limit, (page-1)*limit)
		switch {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, result)
		case errors.Is(err, ErrUnknownSite):
			httpx.WriteError(w, http.StatusNotFound, "unknown_site", "Site not found.")
		case errors.Is(err, ErrInvalidRange):
			httpx.WriteError(w, http.StatusBadRequest, "invalid_range", "The date range must end on or after its start and span at most 366 days.")
		default:
			logger.ErrorContext(r.Context(), "analytics countries", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
		}
	}
}

// publicOrigin is where the browser reached this server.
func publicOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "http" || proto == "https" {
		scheme = proto
	}
	return scheme + "://" + r.Host
}

// summaryHandler returns the project's traffic summary for a date range.
func summaryHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, fromErr := time.Parse(time.DateOnly, r.URL.Query().Get("from"))
		to, toErr := time.Parse(time.DateOnly, r.URL.Query().Get("to"))
		if fromErr != nil || toErr != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_range", "from and to must be dates like 2026-01-31.")
			return
		}
		sum, err := svc.Summarize(r.Context(), r.PathValue("projectId"), from, to)
		if errors.Is(err, ErrInvalidRange) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_range", "The date range must end on or after its start and span at most 366 days.")
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "summarize analytics", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, sum)
	}
}
