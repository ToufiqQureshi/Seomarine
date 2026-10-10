package team

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

// Mount registers the invite endpoint behind the shared session middleware.
func Mount(mux *http.ServeMux, service *Service, withSession func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/organization/invitations", withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "team_unavailable", "Team invitations are not available on this server.")
			return
		}
		user, ok := auth.UserFromContext(r.Context())
		if !ok || user.OrganizationID == "" {
			httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
			return
		}
		var body struct {
			Email string `json:"email"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || strings.TrimSpace(body.Email) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "validation_error", "Enter a valid email address.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		id, err := service.Invite(r.Context(), user, body.Email)
		switch {
		case errors.Is(err, ErrInvalidEmail):
			httpx.WriteError(w, http.StatusBadRequest, "validation_error", "Enter a valid email address.")
		case errors.Is(err, ErrForbidden):
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "Only workspace admins can invite teammates.")
		case errors.Is(err, ErrConflict):
			httpx.WriteError(w, http.StatusConflict, "conflict", "This person is already a member.")
		case errors.Is(err, ErrRateLimited):
			httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Invitation limit reached for today. Try again tomorrow.")
		case errors.Is(err, ErrTooManyPending):
			httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "This workspace already has the maximum number of pending invitations.")
		case errors.Is(err, ErrUpstreamUnavailable):
			httpx.WriteError(w, http.StatusBadGateway, "upstream_unavailable", "The invitation was saved but the email couldn't be sent. Use Resend in a moment to retry.")
		case err != nil:
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not send the invitation.")
		default:
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"invitationId": id})
		}
	})))
}
