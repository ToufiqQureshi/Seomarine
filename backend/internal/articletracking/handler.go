package articletracking

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
)

type API struct{ Repository Repository }

// Mount registers the project's published-article tracking contract. The
// standard session and project access middleware must be supplied by the app.
func Mount(mux *http.ServeMux, repository Repository, withSession, withProjectAccess func(http.Handler) http.Handler) {
	api := API{Repository: repository}
	protect := func(h http.Handler) http.Handler { return withSession(withProjectAccess(h)) }
	mux.Handle("POST /api/v1/projects/{projectId}/content/drafts/{draftId}/tracking/opt-in", protect(http.HandlerFunc(api.optIn)))
	mux.Handle("POST /api/v1/projects/{projectId}/content/drafts/{draftId}/tracking/get", protect(http.HandlerFunc(api.get)))
}

func (a API) optIn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetKeyword string `json:"targetKeyword"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid tracking request.")
		return
	}
	org, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || org == "" {
		httpx.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found.")
		return
	}
	row, err := a.Repository.OptIn(r.Context(), OptInInput{DraftID: r.PathValue("draftId"), ProjectID: r.PathValue("projectId"), OrganizationID: org, TargetKeyword: body.TargetKeyword})
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, row)
	case errors.Is(err, ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Enter the target keyword to track.")
	case errors.Is(err, ErrNotPublished), errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_published", "Publish this draft before starting tracking.")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Article tracking could not be enabled.")
	}
}

func (a API) get(w http.ResponseWriter, r *http.Request) {
	var empty map[string]json.RawMessage
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&empty) != nil || len(empty) != 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid tracking request.")
		return
	}
	org, ok := auth.ProjectOrganizationFromContext(r.Context())
	if !ok || org == "" {
		httpx.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found.")
		return
	}
	row, err := a.Repository.Get(r.Context(), org, r.PathValue("projectId"), r.PathValue("draftId"))
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Article tracking is not enabled for this draft.")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Article tracking could not be loaded.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, row)
}
