package gsc

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxConnectionBody = 16 << 10

// ConnectionDeps supplies dependencies and access middleware for setup routes.
type ConnectionDeps struct {
	Logger            *slog.Logger
	Operations        *ConnectionOperations
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// MountConnections registers GSC grant status, property selection and disconnect routes.
func MountConnections(mux *http.ServeMux, d ConnectionDeps) {
	mux.Handle("POST /api/v1/gsc/grant/status", d.WithSession(http.HandlerFunc(d.grantStatus)))
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	base := "POST /api/v1/projects/{projectId}/gsc/"
	mux.Handle(base+"connection/status", protect(http.HandlerFunc(d.connectionStatus)))
	mux.Handle(base+"sites/list", protect(http.HandlerFunc(d.listSites)))
	mux.Handle(base+"connection/set", protect(http.HandlerFunc(d.setSite)))
	mux.Handle(base+"connection/disconnect", protect(http.HandlerFunc(d.disconnect)))
}

func (d ConnectionDeps) grantStatus(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyOrObject(w, r) {
		writeConnectionInvalid(w)
		return
	}
	if d.Operations == nil || d.Operations.Grants == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "gsc_unavailable", "Google Search Console is not configured.")
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	hasGrant, err := d.Operations.Grants.HasGrant(r.Context(), user.ID)
	if err != nil {
		d.fail(w, r, "check GSC grant status", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"connected": hasGrant})
}

func (d ConnectionDeps) connectionStatus(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyOrObject(w, r) {
		writeConnectionInvalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	orgID, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || d.Operations == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "gsc_unavailable", "Google Search Console is not configured.")
		return
	}
	status, err := d.Operations.GetStatus(r.Context(), user.ID, orgID, r.PathValue("projectId"))
	if err != nil {
		d.fail(w, r, "read GSC connection status", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

func (d ConnectionDeps) listSites(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyOrObject(w, r) {
		writeConnectionInvalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	orgID, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || d.Operations == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "gsc_unavailable", "Google Search Console is not configured.")
		return
	}
	accounts, err := d.Operations.ListSites(r.Context(), user.ID, orgID, r.PathValue("projectId"))
	if err != nil {
		d.fail(w, r, "list GSC properties", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

type setSiteRequest struct {
	AccountID string `json:"accountId"`
	SiteURL   string `json:"siteUrl"`
}

func (d ConnectionDeps) setSite(w http.ResponseWriter, r *http.Request) {
	var input setSiteRequest
	if !decodeConnectionJSON(w, r, &input) || input.AccountID == "" || input.SiteURL == "" || len(input.AccountID) > 256 || len(input.SiteURL) > 2048 {
		writeConnectionInvalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	orgID, authorized := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || !authorized {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	if d.Operations == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "gsc_unavailable", "Google Search Console is not configured.")
		return
	}
	connection, err := d.Operations.SetSite(r.Context(), user.ID, orgID, r.PathValue("projectId"), input.AccountID, input.SiteURL)
	if err != nil {
		d.fail(w, r, "set GSC property", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"connected": true, "siteUrl": connection.SiteURL, "connectedByEmail": connection.ConnectedAccountEmail, "connectedAt": connection.CreatedAt})
}

func (d ConnectionDeps) disconnect(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyOrObject(w, r) {
		writeConnectionInvalid(w)
		return
	}
	user, ok := auth.UserFromContext(r.Context())
	orgID, authorized := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || !authorized {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return
	}
	if d.Operations == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "gsc_unavailable", "Google Search Console is not configured.")
		return
	}
	if err := d.Operations.Disconnect(r.Context(), user.ID, orgID, r.PathValue("projectId")); err != nil {
		d.fail(w, r, "disconnect GSC property", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"connected": false})
}

func decodeEmptyOrObject(w http.ResponseWriter, r *http.Request) bool {
	var input map[string]json.RawMessage
	return decodeConnectionJSON(w, r, &input) && input != nil && len(input) == 0
}

func decodeConnectionJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxConnectionBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return false
	}
	return true
}

func (d ConnectionDeps) fail(w http.ResponseWriter, r *http.Request, operation string, err error) {
	switch {
	case errors.Is(err, ErrManageForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage this Search Console connection.")
		return
	case errors.Is(err, ErrSiteUnverified):
		httpx.WriteError(w, http.StatusForbidden, "gsc_site_unverified", "You don't have verified access to that Search Console property.")
		return
	case errors.Is(err, ErrGrantNotFound):
		httpx.WriteError(w, http.StatusNotFound, "google_account_not_found", "That Google account is not connected.")
		return
	case errors.Is(err, ErrSiteNotFound):
		httpx.WriteError(w, http.StatusNotFound, "gsc_site_not_found", "That Search Console property is not available on the selected Google account.")
		return
	case errors.Is(err, ErrGoogleUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, "gsc_unavailable", "Google Search Console is not configured.")
		return
	}
	var provider ConnectionProviderError
	if errors.As(err, &provider) {
		httpx.WriteError(w, provider.Status, provider.Code, provider.Message)
		return
	}
	if r.Context().Err() != nil {
		d.Logger.InfoContext(r.Context(), "GSC request canceled", "operation", operation, "err", err)
		return
	}
	d.Logger.ErrorContext(r.Context(), "GSC connection operation failed", "operation", operation, "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}

func writeConnectionInvalid(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid Search Console request.")
}
