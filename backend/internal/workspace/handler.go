package workspace

import (
	"errors"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

// Mount registers authenticated legacy workspace migration endpoints.
func Mount(mux *http.ServeMux, service *Service, withSession func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/workspaces/legacy/status", withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "workspace_unavailable", "Workspace migration is not available on this server.")
			return
		}
		count, err := service.Status(r.Context())
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not check for legacy workspaces.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]int{"legacyWorkspaceCount": count})
	})))
	mux.Handle("POST /api/v1/workspaces/legacy/merge", withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "workspace_unavailable", "Workspace migration is not available on this server.")
			return
		}
		merged, err := service.Merge(r.Context())
		if errors.Is(err, ErrModeDisabled) {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "Workspace merge is only available in Cloudflare Access mode.")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not migrate the legacy workspaces.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]int{"mergedWorkspaces": merged})
	})))
}
