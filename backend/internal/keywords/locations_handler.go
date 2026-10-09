package keywords

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxLocationBody = 4 << 10

type locationLimiter interface {
	Allow(context.Context, string) (bool, error)
}

type redisLocationLimiter struct{ Client *redis.Client }

func (l redisLocationLimiter) Allow(ctx context.Context, userID string) (bool, error) {
	key := "serp-locations:rate:" + userID + ":" + time.Now().UTC().Format("200601021504")
	const script = `local n = redis.call('INCR', KEYS[1]); if n == 1 then redis.call('EXPIRE', KEYS[1], 61) end; return n`
	count, err := l.Client.Eval(ctx, script, []string{key}).Int64()
	return count <= 60, err
}

// LocationDeps are the dependencies for the authenticated SERP location picker.
type LocationDeps struct {
	Logger      *slog.Logger
	Service     *LocationService
	WithSession func(http.Handler) http.Handler
}

// MountLocations registers the search and prewarm endpoints.
func MountLocations(mux *http.ServeMux, deps LocationDeps) {
	mux.Handle("POST /api/v1/serp-locations/search", deps.WithSession(locationHandler(deps, true)))
	mux.Handle("POST /api/v1/serp-locations/prewarm", deps.WithSession(locationHandler(deps, false)))
}

type locationRequest struct {
	CountryCode string `json:"countryCode"`
	Query       string `json:"query,omitempty"`
}

func locationHandler(deps LocationDeps, search bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "locations_unavailable", "Location search is not available.")
			return
		}
		user, ok := auth.UserFromContext(r.Context())
		if !ok || user.OrganizationID == "" {
			httpx.WriteError(w, http.StatusForbidden, "organization_required", "Select an organization to search locations.")
			return
		}
		var req locationRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLocationBody))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid location request.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		if !validCountryCode(req.CountryCode) || (search && (len(utf16.Encode([]rune(req.Query))) < 1 || len(utf16.Encode([]rune(req.Query))) > 100)) || (!search && req.Query != "") {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Country code or search query is invalid.")
			return
		}
		allowed, err := deps.Service.limiter.Allow(r.Context(), user.ID)
		if err != nil {
			deps.Logger.ErrorContext(r.Context(), "SERP location rate limit failed", "err", err)
			httpx.WriteError(w, http.StatusServiceUnavailable, "locations_unavailable", "Location search is temporarily unavailable.")
			return
		}
		if !allowed {
			httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Too many location searches. Try again shortly.")
			return
		}
		country := strings.ToLower(req.CountryCode)
		var rows []SerpLocation
		if search {
			rows, err = deps.Service.Search(r.Context(), user.OrganizationID, country, req.Query)
		} else {
			rows, err = deps.Service.Locations(r.Context(), user.OrganizationID, country)
		}
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			deps.Logger.ErrorContext(r.Context(), "SERP location lookup failed", "err", err)
			status := http.StatusBadGateway
			if errors.Is(err, dataforseo.ErrBillingIssue) || errors.Is(err, ErrLocationCache) {
				status = http.StatusServiceUnavailable
			}
			httpx.WriteError(w, status, "locations_unavailable", "Location search is temporarily unavailable.")
			return
		}
		if search {
			httpx.WriteJSON(w, http.StatusOK, rows)
		} else {
			httpx.WriteJSON(w, http.StatusOK, map[string]bool{"warmed": true})
		}
	}
}

func validCountryCode(value string) bool {
	if len(value) != 2 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
			return false
		}
	}
	return true
}
