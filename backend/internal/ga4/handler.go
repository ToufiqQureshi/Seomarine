package ga4

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/gsc"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxBody = 64 << 10

// Deps supplies the route's service and authentication middleware.
type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	SearchConsole     *gsc.Service
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers the authenticated GA4 report endpoint.
func Mount(mux *http.ServeMux, d Deps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "ga4_unavailable", "Google Analytics reporting is not set up on this server.")
			return
		}
		_, ok := auth.UserFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
			return
		}
		var input ReportInput
		if err := decodeRequest(w, r, &input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must be a single JSON object with valid non-null fields.")
			return
		}
		result, err := d.Service.RunReport(r.Context(), r.PathValue("projectId"), input)
		if err != nil {
			writeError(d.Logger, w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})
	mux.Handle("POST /api/v1/projects/{projectId}/ga4/reports/run", protect(h))
	mux.Handle("POST /api/v1/projects/{projectId}/ga4/overview/organic", protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "ga4_unavailable", "Google Analytics reporting is not set up on this server.")
			return
		}
		if _, ok := auth.UserFromContext(r.Context()); !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
			return
		}
		var input OrganicOverviewInput
		if err := decodeRequest(w, r, &input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must be a single JSON object with valid non-null fields.")
			return
		}
		result, err := d.Service.GetOrganicOverview(r.Context(), r.PathValue("projectId"), input)
		if err != nil {
			writeError(d.Logger, w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})))
	mux.Handle("POST /api/v1/projects/{projectId}/ga4/measurement-health", protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "ga4_unavailable", "Google Analytics reporting is not set up on this server.")
			return
		}
		if _, ok := auth.UserFromContext(r.Context()); !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
			return
		}
		var input struct{}
		if err := decodeRequest(w, r, &input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must be an empty JSON object.")
			return
		}
		result, err := d.Service.GetMeasurementHealth(r.Context(), r.PathValue("projectId"))
		if err != nil {
			writeError(d.Logger, w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})))
	mux.Handle("POST /api/v1/projects/{projectId}/ga4/search-opportunities", protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "ga4_unavailable", "Google Analytics reporting is not set up on this server.")
			return
		}
		if _, ok := auth.UserFromContext(r.Context()); !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
			return
		}
		organizationID, ok := auth.ProjectOrganizationFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		var input SearchOpportunityInput
		if err := decodeRequest(w, r, &input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must be a single JSON object with valid non-null fields.")
			return
		}
		result, err := d.Service.GetSearchOpportunities(r.Context(), d.SearchConsole, organizationID, r.PathValue("projectId"), input)
		if err != nil {
			writeError(d.Logger, w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})))
}

func decodeRequest(w http.ResponseWriter, r *http.Request, destination any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("request body must be an object")
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return err
	}
	for _, field := range fields {
		if bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
			return errors.New("request fields cannot be null")
		}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON value")
	}
	strict := json.NewDecoder(bytes.NewReader(trimmed))
	strict.DisallowUnknownFields()
	return strict.Decode(destination)
}

func writeError(logger *slog.Logger, w http.ResponseWriter, r *http.Request, err error) {
	var typed *reportError
	if errors.As(err, &typed) {
		status := http.StatusBadRequest
		switch typed.Code {
		case "ga4_unavailable", "gsc_unavailable":
			status = http.StatusServiceUnavailable
		case "ga4_not_connected", "gsc_not_connected":
			status = http.StatusNotFound
		case "gsc_rate_limited":
			status = http.StatusTooManyRequests
		case "ga4_reconnect_required", "ga4_property_inaccessible", "ga4_report_incompatible", "ga4_malformed_response", "ga4_quota_exhausted", "ga4_upstream_unavailable", "gsc_reconnect_required", "gsc_upstream_unavailable":
			status = http.StatusBadGateway
		}
		body := map[string]any{"error": map[string]string{"code": typed.Code, "message": typed.Message}}
		if typed.RetryAfterSeconds != nil {
			body["retryAfterSeconds"] = *typed.RetryAfterSeconds
		}
		httpx.WriteJSON(w, status, body)
		return
	}
	if r.Context().Err() != nil {
		logger.InfoContext(r.Context(), "GA4 request canceled", "err", err)
		return
	}
	logger.ErrorContext(r.Context(), "GA4 report failed", "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}
