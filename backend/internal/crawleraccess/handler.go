package crawleraccess

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

// Mount registers crawler credential endpoints. Secrets are never returned.
func Mount(mux *http.ServeMux, service *Service, withSession func(http.Handler) http.Handler, withProjectAccess func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/crawler-access/credentials", withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "crawler_access_unavailable", "Crawler access is not available on this server.")
			return
		}
		user, ok := auth.UserFromContext(r.Context())
		if !ok || user.OrganizationID == "" {
			httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
			return
		}
		rows, err := service.List(r.Context(), user.OrganizationID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not load crawler access.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, rows)
	})))
	mux.Handle("POST /api/v1/projects/{projectId}/crawler-access/save", withSession(withProjectAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "crawler_access_unavailable", "Crawler access is not available on this server.")
			return
		}
		user, ok := auth.UserFromContext(r.Context())
		org, projectOK := auth.ProjectOrganizationFromContext(r.Context())
		if !ok || !projectOK {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "You cannot manage crawler access for this project.")
			return
		}
		if !canManageIntegrations(user.Role) {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "Only workspace admins can manage crawler access.")
			return
		}
		var body struct {
			Host           string `json:"host"`
			SignatureInput string `json:"signatureInput"`
			Signature      string `json:"signature"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "validation_error", "Enter a valid Shopify signature.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		result, err := service.Save(r.Context(), org, r.PathValue("projectId"), user.ID, body.Host, body.SignatureInput, body.Signature)
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "This project is no longer available.")
			return
		}
		if err != nil {
			if strings.Contains(err.Error(), "invalid host") {
				httpx.WriteError(w, http.StatusBadRequest, "validation_error", "Enter a valid domain.")
			} else {
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not save crawler access.")
			}
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	}))))
	mux.Handle("POST /api/v1/crawler-access/delete", withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "crawler_access_unavailable", "Crawler access is not available on this server.")
			return
		}
		user, ok := auth.UserFromContext(r.Context())
		if !ok || user.OrganizationID == "" {
			httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
			return
		}
		if !canManageIntegrations(user.Role) {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "Only workspace admins can manage crawler access.")
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.ID == "" {
			httpx.WriteError(w, http.StatusBadRequest, "validation_error", "Choose a valid crawler credential.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		if err := service.Delete(r.Context(), user.OrganizationID, body.ID); errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "That crawler credential is no longer available.")
			return
		} else if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not remove crawler access.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
	})))
}

func canManageIntegrations(role string) bool { return roleCanManage(role) }
func roleCanManage(role string) bool {
	for _, part := range strings.Split(role, ",") {
		if strings.TrimSpace(part) == "owner" || strings.TrimSpace(part) == "admin" {
			return true
		}
	}
	return false
}
