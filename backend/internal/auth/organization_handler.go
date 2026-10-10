package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

// MountOrganization registers active-organization context and switching APIs.
func MountOrganization(mux *http.ServeMux, service *Service, withSession func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/organization/context", withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
			return
		}
		memberships, err := service.OrganizationMemberships(r.Context(), user.ID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not load your workspace list.")
			return
		}
		name := "Organization"
		for _, membership := range memberships {
			if membership.OrganizationID == user.OrganizationID {
				name = membership.OrganizationName
				break
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"organizationId":   user.OrganizationID,
			"organizationName": name,
			"role":             user.Role,
			"organizations":    memberships,
		})
	})))
	mux.Handle("GET /api/v1/organization/team", withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok || user.OrganizationID == "" {
			httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
			return
		}
		team, err := service.OrganizationTeam(r.Context(), user.OrganizationID, user.Role)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not load your team.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, team)
	})))
	mux.Handle("POST /api/v1/organization/switch", withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
			return
		}
		var body struct {
			OrganizationID string `json:"organizationId"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.OrganizationID == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid workspace.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		if err := service.SwitchOrganization(r, user.ID, body.OrganizationID); err != nil {
			if errors.Is(err, ErrProjectNotFound) {
				httpx.WriteError(w, http.StatusNotFound, "not_found", "That workspace is no longer available.")
				return
			}
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not switch workspaces.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"organizationId": body.OrganizationID})
	})))
	mux.Handle("POST /api/v1/organization/ownership/transfer", withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok || user.OrganizationID == "" {
			httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
			return
		}
		if !hasRole(user.Role, "owner") {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "Only the owner can transfer workspace ownership.")
			return
		}
		var body struct {
			MemberID string `json:"memberId"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.MemberID == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid team member.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		err := service.TransferOrganizationOwnership(r.Context(), user.OrganizationID, user.ID, body.MemberID)
		switch {
		case errors.Is(err, ErrProjectNotFound):
			httpx.WriteError(w, http.StatusNotFound, "not_found", "That team member is no longer available.")
		case errors.Is(err, ErrOrganizationConflict):
			httpx.WriteError(w, http.StatusConflict, "conflict", "You already own this workspace.")
		case errors.Is(err, ErrOrganizationForbidden):
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "Only the owner can transfer workspace ownership.")
		case err != nil:
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not transfer workspace ownership.")
		default:
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"memberId": body.MemberID})
		}
	})))
}
