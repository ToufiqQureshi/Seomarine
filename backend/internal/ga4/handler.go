package ga4

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxBody = 64 << 10

// Deps supplies the route's service and authentication middleware.
type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers the authenticated GA4 report endpoint.
func Mount(mux *http.ServeMux, d Deps) {
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
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request is not valid JSON.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		result, err := d.Service.RunReport(r.Context(), r.PathValue("projectId"), input)
		if err != nil {
			writeError(d.Logger, w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	mux.Handle("POST /api/v1/projects/{projectId}/ga4/reports/run", protect(h))
}

func writeError(logger *slog.Logger, w http.ResponseWriter, r *http.Request, err error) {
	var typed *reportError
	if errors.As(err, &typed) {
		status := http.StatusBadRequest
		switch typed.Code {
		case "ga4_unavailable":
			status = http.StatusServiceUnavailable
		case "ga4_not_connected":
			status = http.StatusNotFound
		case "ga4_reconnect_required", "ga4_property_inaccessible", "ga4_report_incompatible", "ga4_malformed_response", "ga4_quota_exhausted", "ga4_upstream_unavailable":
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
