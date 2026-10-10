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
	Setup             *ConnectionOperations
	SearchConsole     *gsc.Service
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers the authenticated GA4 report endpoint.
func Mount(mux *http.ServeMux, d Deps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	base := "POST /api/v1/projects/{projectId}/ga4/"
	mux.Handle(base+"connection/status", protect(http.HandlerFunc(d.connectionStatus)))
	mux.Handle(base+"properties/list", protect(http.HandlerFunc(d.listProperties)))
	mux.Handle(base+"connection/set", protect(http.HandlerFunc(d.setProperty)))
	mux.Handle(base+"connection/disconnect", protect(http.HandlerFunc(d.disconnect)))
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

func (d Deps) setupScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	user, ok := auth.UserFromContext(r.Context())
	organizationID, hasOrganization := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || !hasOrganization {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return "", "", false
	}
	return user.ID, organizationID, true
}

func (d Deps) connectionStatus(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if err := decodeRequest(w, r, &input); err != nil {
		writeConnectionInvalid(w)
		return
	}
	userID, organizationID, ok := d.setupScope(w, r)
	if !ok {
		return
	}
	if d.Setup == nil {
		writeSetupError(w, ErrConnectionsUnavailable)
		return
	}
	result, err := d.Setup.Status(r.Context(), userID, organizationID, r.PathValue("projectId"))
	if err != nil {
		d.setupFailed(w, r, "read GA4 connection status", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (d Deps) listProperties(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if err := decodeRequest(w, r, &input); err != nil {
		writeConnectionInvalid(w)
		return
	}
	userID, organizationID, ok := d.setupScope(w, r)
	if !ok {
		return
	}
	if d.Setup == nil {
		writeSetupError(w, ErrConnectionsUnavailable)
		return
	}
	accounts, err := d.Setup.ListProperties(r.Context(), userID, organizationID, r.PathValue("projectId"))
	if err != nil {
		d.setupFailed(w, r, "list GA4 properties", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

type setPropertyRequest struct {
	AccountID  string `json:"accountId"`
	PropertyID string `json:"propertyId"`
}

func (d Deps) setProperty(w http.ResponseWriter, r *http.Request) {
	var input setPropertyRequest
	if err := decodeRequest(w, r, &input); err != nil || input.AccountID == "" || len(input.AccountID) > 256 || !validPropertyID(input.PropertyID) {
		writeConnectionInvalid(w)
		return
	}
	userID, organizationID, ok := d.setupScope(w, r)
	if !ok {
		return
	}
	if d.Setup == nil {
		writeSetupError(w, ErrConnectionsUnavailable)
		return
	}
	connection, err := d.Setup.SetProperty(r.Context(), userID, organizationID, r.PathValue("projectId"), input.AccountID, input.PropertyID)
	if err != nil {
		d.setupFailed(w, r, "set GA4 property", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"connected": true, "propertyId": connection.PropertyID, "propertyDisplayName": connection.PropertyDisplayName,
		"propertyTimeZone": connection.PropertyTimeZone, "propertyCurrencyCode": connection.PropertyCurrencyCode, "connectedByEmail": connection.ConnectedAccountEmail, "connectedAt": connection.CreatedAt})
}

func (d Deps) disconnect(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if err := decodeRequest(w, r, &input); err != nil {
		writeConnectionInvalid(w)
		return
	}
	userID, organizationID, ok := d.setupScope(w, r)
	if !ok {
		return
	}
	if d.Setup == nil {
		writeSetupError(w, ErrConnectionsUnavailable)
		return
	}
	if err := d.Setup.Disconnect(r.Context(), userID, organizationID, r.PathValue("projectId")); err != nil {
		d.setupFailed(w, r, "disconnect GA4 property", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"connected": false})
}

func (d Deps) setupFailed(w http.ResponseWriter, r *http.Request, operation string, err error) {
	switch {
	case errors.Is(err, ErrManageForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage this Google Analytics connection.")
		return
	case errors.Is(err, ErrGrantNotFound):
		httpx.WriteError(w, http.StatusNotFound, "google_account_not_found", "That Google account is not connected.")
		return
	case errors.Is(err, ErrPropertyNotFound):
		httpx.WriteError(w, http.StatusNotFound, "ga4_property_not_found", "That Google Analytics property is not available on the selected account.")
		return
	case errors.Is(err, ErrConnectionsUnavailable):
		writeSetupError(w, err)
		return
	}
	var provider connectionProviderError
	if errors.As(err, &provider) {
		httpx.WriteError(w, provider.Status, provider.Code, provider.Message)
		return
	}
	var failure *reportError
	if errors.As(err, &failure) {
		writeError(d.Logger, w, r, err)
		return
	}
	if r.Context().Err() != nil {
		d.Logger.InfoContext(r.Context(), "GA4 setup request canceled", "operation", operation, "err", err)
		return
	}
	d.Logger.ErrorContext(r.Context(), "GA4 setup failed", "operation", operation, "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}

func writeSetupError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrConnectionsUnavailable) {
		httpx.WriteError(w, http.StatusServiceUnavailable, "ga4_unavailable", "Google Analytics setup is not available on this server.")
		return
	}
	httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid Google Analytics request.")
}

func writeConnectionInvalid(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid Google Analytics request.")
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
