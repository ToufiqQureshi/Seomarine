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
	"github.com/toufiqqureshi/uplumo/backend/internal/audit"
	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/backlinks"
	"github.com/toufiqqureshi/uplumo/backend/internal/billing"
	"github.com/toufiqqureshi/uplumo/backend/internal/domain"
	"github.com/toufiqqureshi/uplumo/backend/internal/ga4"
	"github.com/toufiqqureshi/uplumo/backend/internal/gsc"
	"github.com/toufiqqureshi/uplumo/backend/internal/keywords"
	"github.com/toufiqqureshi/uplumo/backend/internal/plans"
	"github.com/toufiqqureshi/uplumo/backend/internal/projectcontext"
	"github.com/toufiqqureshi/uplumo/backend/internal/ranktracking"
	"github.com/toufiqqureshi/uplumo/backend/internal/reports"
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
	Logger  *slog.Logger
	DB      *pgxpool.Pool
	Redis   *redis.Client
	Auth    *auth.Service
	Billing *billing.Service
	Plans   interface {
		Allows(context.Context, string, plans.Feature) (bool, error)
	}
	Credits         CreditBalances
	PublicURL       *url.URL
	Audit           *audit.Service
	Backlinks       *backlinks.Service
	Domain          *domain.Service
	Locations       *keywords.LocationService
	SavedKeywords   *keywords.SavedService
	KeywordResearch *keywords.ResearchService
	GA4             *ga4.Service
	Reports         *reports.Service
	GSC             *gsc.Service
	RankTracking    *ranktracking.Service
	RankChecks      *ranktracking.Checks
	ProjectContext  *projectcontext.Service
}

// Mount registers the MCP dispatcher on mux.
func Mount(mux *http.ServeMux, d Deps) {
	h := newHandler(d)
	mux.Handle(Route, h)
	h.mountOAuth(mux)
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
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	key := apiKeyFromRequest(r)
	if key == "" {
		token := bearerTokenFromRequest(r)
		if token == "" {
			h.oauthChallenge(w, r, "")
			return
		}
		auth, ok, err := h.authenticateOAuthToken(ctx, r, token)
		if err != nil {
			h.deps.Logger.ErrorContext(r.Context(), "mcp OAuth access token lookup failed", "err", err)
			h.writeJSON(w, http.StatusInternalServerError, errorResponse(json.RawMessage("null"), codeServerError, "The authorization service is temporarily unavailable."))
			return
		}
		if !ok {
			h.oauthChallenge(w, r, "invalid_token")
			return
		}
		if limited, retryAfter := h.rateLimited(ctx, auth.UserID); limited {
			resp := rateLimitedResponse("rate_limited", "Rate limit exceeded: 5000 requests per minute", retryAfter)
			h.writeJSON(w, resp.status, resp.body)
			return
		}
		h.serveRPC(ctx, w, r, auth)
		return
	}

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

// CreditBalances reads an organization's remaining credits.
type CreditBalances interface {
	GetBalance(ctx context.Context, organizationID string) (int64, error)
}
