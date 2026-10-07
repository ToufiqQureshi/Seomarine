package httpapi

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/analytics"
)

//go:embed tracker.js
var trackerJS []byte

const (
	// maxCollectBody bounds a tracker event; real ones are a few hundred bytes.
	maxCollectBody = 4 << 10
	maxSiteKey     = 64
	maxURL         = 2048
	maxScreenWidth = 100_000
)

// serveTracker serves the tracker script. Browsers cache it for an hour and
// then revalidate with the ETag, so a new release reaches every site within
// the hour without a version in the snippet.
func serveTracker() http.HandlerFunc {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(trackerJS))
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, "t.js", time.Time{}, bytes.NewReader(trackerJS))
	}
}

// collectRequest is the tracker's event body.
type collectRequest struct {
	SiteKey     string `json:"k"`
	URL         string `json:"u"`
	Referrer    string `json:"r"`
	ScreenWidth int    `json:"w"`
}

// collect records a tracker event. Any site may post to it, so CORS is open;
// the tracker sends text/plain, which needs no preflight, and the preflight
// handler covers other clients.
func collect(logger *slog.Logger, svc *analytics.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var req collectRequest
		err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCollectBody)).Decode(&req)
		if maxErr := (*http.MaxBytesError)(nil); errors.As(err, &maxErr) {
			writeError(w, logger, http.StatusRequestEntityTooLarge, "payload_too_large", "The event is too large.")
			return
		}
		if err != nil {
			writeError(w, logger, http.StatusBadRequest, "invalid_event", "The event is not valid JSON.")
			return
		}
		page, msg := validateCollect(req)
		if msg != "" {
			writeError(w, logger, http.StatusBadRequest, "invalid_event", msg)
			return
		}

		err = svc.Collect(r.Context(), analytics.Hit{
			SiteKey:     req.SiteKey,
			Page:        page,
			Referrer:    req.Referrer,
			ScreenWidth: req.ScreenWidth,
			IP:          clientIP(r),
			UserAgent:   r.UserAgent(),
		})
		switch {
		case err == nil:
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, analytics.ErrRateLimited):
			writeError(w, logger, http.StatusTooManyRequests, "rate_limited", "Too many events. Slow down.")
		case errors.Is(err, analytics.ErrUnknownSite):
			writeError(w, logger, http.StatusNotFound, "unknown_site", "No site uses this key.")
		case errors.Is(err, analytics.ErrForeignHost):
			writeError(w, logger, http.StatusForbidden, "foreign_host", "This page is not on the project's domain.")
		default:
			logger.ErrorContext(r.Context(), "collect analytics event", "err", err)
			writeError(w, logger, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
		}
	}
}

// validateCollect checks an event's fields and returns the parsed page URL,
// or a message saying what is wrong. A malformed referrer is not an error:
// browsers send what they send, and it counts as no referrer.
func validateCollect(req collectRequest) (*url.URL, string) {
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

// collectPreflight answers CORS preflights for /collect.
func collectPreflight(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// clientIP returns the address of the client. The server runs behind one
// trusted proxy (Railway's edge), which appends the address it saw to
// X-Forwarded-For, so the last entry is the only one a client cannot forge.
// Without a valid header it falls back to the TCP peer.
func clientIP(r *http.Request) string {
	if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
		entries := strings.Split(xff[len(xff)-1], ",")
		if ip, err := netip.ParseAddr(strings.TrimSpace(entries[len(entries)-1])); err == nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type siteResponse struct {
	SiteKey string `json:"siteKey"`
	Snippet string `json:"snippet"`
}

// ensureSite returns the project's site key and tracker snippet, creating
// the site on first call.
func ensureSite(logger *slog.Logger, svc *analytics.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := svc.EnsureSite(r.Context(), r.PathValue("projectId"))
		if err != nil {
			logger.ErrorContext(r.Context(), "ensure analytics site", "err", err)
			writeError(w, logger, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		snippet := fmt.Sprintf(`<script defer data-site="%s" src="%s/t.js"></script>`,
			html.EscapeString(key), html.EscapeString(publicOrigin(r)))
		writeJSON(w, logger, http.StatusOK, siteResponse{SiteKey: key, Snippet: snippet})
	}
}

// publicOrigin is the origin the browser used to reach this server, which
// is where sites must load the tracker from.
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

// summary returns the project's traffic summary for ?from=YYYY-MM-DD&to=YYYY-MM-DD.
func summary(logger *slog.Logger, svc *analytics.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, fromErr := time.Parse(time.DateOnly, r.URL.Query().Get("from"))
		to, toErr := time.Parse(time.DateOnly, r.URL.Query().Get("to"))
		if fromErr != nil || toErr != nil {
			writeError(w, logger, http.StatusBadRequest, "invalid_range", "from and to must be dates like 2026-01-31.")
			return
		}
		sum, err := svc.Summarize(r.Context(), r.PathValue("projectId"), from, to)
		if errors.Is(err, analytics.ErrInvalidRange) {
			writeError(w, logger, http.StatusBadRequest, "invalid_range", "The date range must end on or after its start and span at most 366 days.")
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "summarize analytics", "err", err)
			writeError(w, logger, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		writeJSON(w, logger, http.StatusOK, sum)
	}
}
