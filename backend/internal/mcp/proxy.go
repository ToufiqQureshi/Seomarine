package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// proxy forwards every unowned request to the legacy app with the caller's
// credential intact so the client sees one coherent tool list.
func (h *handler) proxy(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	h.proxyRequest(ctx, w, r)
}

// proxyBody proxies a body that has already been read (e.g. a batch with a
// tools/call for a legacy tool). The body is replayed.
func (h *handler) proxyBody(w http.ResponseWriter, r *http.Request, body []byte) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	h.proxyBodyRequest(ctx, w, r, body)
}

func (h *handler) proxyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	upstream := h.deps.Upstream
	if upstream == nil {
		h.writeJSON(w, http.StatusBadGateway, errorResponse(json.RawMessage("null"), codeServerError, "Upstream not configured."))
		return
	}

	host := originalHost(r)
	r = r.Clone(ctx)
	r.URL = &url.URL{
		Scheme:   upstream.Scheme,
		Host:     upstream.Host,
		Path:     singleJoiningSlash(upstream.Path, r.URL.Path),
		RawPath:  singleJoiningSlash(upstream.RawPath, r.URL.RawPath),
		RawQuery: r.URL.RawQuery,
	}
	r.Host = upstream.Host
	// Preserve the caller's Host so the legacy app builds absolute URLs from
	// the same origin it already uses for cookies and CORS.
	if host != "" {
		r.Header.Set("X-Forwarded-Host", host)
	}
	// The legacy app expects the real client IP and scheme.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		r.Header.Set("X-Forwarded-Proto", proto)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL = r.URL
			pr.Out.Host = r.Host
			pr.Out.Header = r.Header.Clone()
			// The MCP protocol is stateless; strip hop-by-hop headers that would
			// confuse the legacy transport's connection handling.
			pr.Out.Header.Del("Connection")
			pr.Out.Header.Del("Keep-Alive")
			pr.Out.Header.Del("Proxy-Authorization")
			pr.Out.Header.Del("Proxy-Connection")
			pr.Out.Header.Del("Te")
			pr.Out.Header.Del("Trailers")
			pr.Out.Header.Del("Transfer-Encoding")
			pr.Out.Header.Del("Upgrade")
		},
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			h.deps.Logger.ErrorContext(r.Context(), "proxy to legacy app", "err", err)
			h.writeJSON(w, http.StatusBadGateway, errorResponse(json.RawMessage("null"), codeServerError, "The app is temporarily unavailable. Please try again."))
		},
	}
	proxy.ServeHTTP(w, r)
}

func (h *handler) proxyBodyRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte) {
	upstream := h.deps.Upstream
	if upstream == nil {
		h.writeJSON(w, http.StatusBadGateway, errorResponse(json.RawMessage("null"), codeServerError, "Upstream not configured."))
		return
	}

	// #nosec G704 -- upstream is validated from server configuration, not request input.
	req, err := http.NewRequestWithContext(ctx, r.Method,
		upstream.Scheme+"://"+upstream.Host+singleJoiningSlash(upstream.Path, r.URL.Path)+"?"+r.URL.RawQuery,
		io.NopCloser(strings.NewReader(string(body))))
	if err != nil {
		h.deps.Logger.ErrorContext(ctx, "proxy body build failed", "err", err)
		h.writeJSON(w, http.StatusBadGateway, errorResponse(json.RawMessage("null"), codeServerError, "The app is temporarily unavailable. Please try again."))
		return
	}
	req.Header = r.Header.Clone()
	req.Host = upstream.Host
	if host := originalHost(r); host != "" {
		req.Header.Set("X-Forwarded-Host", host)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	client := &http.Client{Transport: transport, Timeout: requestTimeout}

	// #nosec G704 -- upstream is validated from server configuration, not request input.
	resp, err := client.Do(req)
	if err != nil {
		h.deps.Logger.ErrorContext(ctx, "proxy to legacy app", "err", err)
		h.writeJSON(w, http.StatusBadGateway, errorResponse(json.RawMessage("null"), codeServerError, "The app is temporarily unavailable. Please try again."))
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			h.deps.Logger.WarnContext(ctx, "close proxied MCP response", "err", err)
		}
	}()

	// Preserve CORS headers on the proxied response too.
	for name, value := range corsHeaders {
		w.Header().Set(name, value)
	}
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		h.deps.Logger.WarnContext(ctx, "copy proxied MCP response", "err", err)
	}
}

// proxyRoundTrip sends a buffered JSON-RPC request to the legacy MCP endpoint
// and returns its response for protocol-level merging by the dispatcher.
func (h *handler) proxyRoundTrip(ctx context.Context, r *http.Request, body []byte) (*http.Response, error) {
	upstream := h.deps.Upstream
	if upstream == nil {
		return nil, errors.New("legacy app upstream is not configured")
	}
	target := *upstream
	target.Path = singleJoiningSlash(upstream.Path, r.URL.Path)
	target.RawPath = singleJoiningSlash(upstream.RawPath, r.URL.RawPath)
	target.RawQuery = r.URL.RawQuery
	req, err := http.NewRequestWithContext(ctx, r.Method, target.String(), strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build legacy MCP request: %w", err)
	}
	req.Header = r.Header.Clone()
	req.Host = upstream.Host
	if host := originalHost(r); host != "" {
		req.Header.Set("X-Forwarded-Host", host)
	}
	client := &http.Client{Timeout: requestTimeout}
	// #nosec G704 -- upstream is validated from server configuration, not request input.
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call legacy MCP endpoint: %w", err)
	}
	return resp, nil
}

func originalHost(r *http.Request) string {
	if r.Host != "" {
		return r.Host
	}
	return r.Header.Get("Host")
}

// singleJoiningSlash concatenates two path segments with exactly one slash.
func singleJoiningSlash(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if strings.HasSuffix(a, "/") && strings.HasPrefix(b, "/") {
		return a + b[1:]
	}
	if !strings.HasSuffix(a, "/") && !strings.HasPrefix(b, "/") {
		return a + "/" + b
	}
	return a + b
}
