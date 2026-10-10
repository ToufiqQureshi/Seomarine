package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/audit"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/billing"
	"github.com/toufiqqureshi/seomarine/backend/internal/keywords"
)

const (
	// Route is the public MCP endpoint served by the Go dispatcher.
	Route                 = "/mcp"
	requestTimeout        = 30 * time.Second
	maxBodyBytes          = 4 << 20
	apiKeyPrefix          = "oseo_"
	requestLimitPerMinute = 5000
)

var corsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Content-Type, Accept, Authorization, mcp-session-id, MCP-Protocol-Version, Mcp-Method, Mcp-Name",
	"Access-Control-Allow-Methods":  "GET, POST, DELETE, OPTIONS",
	"Access-Control-Allow-Origin":   "*",
	"Access-Control-Expose-Headers": "mcp-session-id",
	"Access-Control-Max-Age":        "86400",
}

// Deps contains the services needed by the MCP dispatcher.
type Deps struct {
	Logger        *slog.Logger
	DB            *pgxpool.Pool
	Redis         *redis.Client
	Auth          *auth.Service
	Billing       *billing.Service
	Upstream      *url.URL
	PublicURL     *url.URL
	Audit         *audit.Service
	Locations     *keywords.LocationService
	SavedKeywords *keywords.SavedService
}

// Mount registers the MCP dispatcher on mux.
func Mount(mux *http.ServeMux, d Deps) {
	h := newHandler(d)
	mux.Handle(Route, h)
}

func newHandler(d Deps) *handler {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return &handler{deps: d}
}

type handler struct {
	deps Deps
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for name, value := range corsHeaders {
		w.Header().Set(name, value)
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	key := apiKeyFromRequest(r)
	if key == "" {
		h.proxy(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	session, errResp := h.authenticateAPIKey(ctx, r, key)
	if errResp != nil {
		h.writeJSON(w, errResp.status, errResp.body)
		return
	}
	h.serveRPC(ctx, w, r, session)
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.deps.Logger.Error("write json response", "err", err)
	}
}
