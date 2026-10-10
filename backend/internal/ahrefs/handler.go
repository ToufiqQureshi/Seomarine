package ahrefs

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxBody = 32 << 10

// Mount registers the project-authorized free domain-rating endpoint.
func Mount(mux *http.ServeMux, service *Service, withSession, withProjectAccess func(http.Handler) http.Handler) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "ahrefs_unavailable", "Domain ratings are not available on this server.")
			return
		}
		var body struct {
			Domains []string `json:"domains"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request is not valid JSON.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		if len(body.Domains) > maxDomains {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "At most 100 domains can be checked at once.")
			return
		}
		result, err := service.Ratings(r.Context(), body.Domains)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "One or more domains are invalid.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, result)
	})
	mux.Handle("POST /api/v1/projects/{projectId}/backlinks/domain-ratings", withSession(withProjectAccess(handler)))
}
