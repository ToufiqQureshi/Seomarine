package google

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

// AccountDeps contains the dependencies for Google account removal endpoints.
type AccountDeps struct {
	Repository  AccountRepository
	Logger      *slog.Logger
	WithSession func(http.Handler) http.Handler
}

type accountRequest struct {
	Provider  string `json:"provider"`
	AccountID string `json:"accountId"`
	Confirmed *bool  `json:"confirmed,omitempty"`
}

func decodeAccountRequest(w http.ResponseWriter, r *http.Request, requireConfirmation bool) (accountRequest, error) {
	var input accountRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return input, errors.New("multiple json values")
	}
	if _, _, _, err := accountScope(input.Provider); err != nil || input.AccountID == "" || len(utf16.Encode([]rune(input.AccountID))) > 256 {
		return input, errors.New("invalid account")
	}
	if requireConfirmation && (input.Confirmed == nil || !*input.Confirmed) {
		return input, errors.New("confirmation required")
	}
	if !requireConfirmation && input.Confirmed != nil {
		return input, errors.New("unexpected confirmation")
	}
	return input, nil
}

// MountAccounts registers authenticated account impact and removal routes.
func MountAccounts(mux *http.ServeMux, deps AccountDeps) {
	mux.Handle("POST /api/v1/google/accounts/impact", deps.WithSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input, err := decodeAccountRequest(w, r, false)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid Google account.")
			return
		}
		user, _ := auth.UserFromContext(r.Context())
		count, err := deps.Repository.RemovalImpact(r.Context(), user.ID, input.Provider, input.AccountID)
		if err != nil {
			deps.Logger.ErrorContext(r.Context(), "count google account mappings", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not check connected projects.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]int{"projectCount": count})
	})))
	mux.Handle("POST /api/v1/google/accounts/remove", deps.WithSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input, err := decodeAccountRequest(w, r, true)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Confirm a valid Google account to remove it.")
			return
		}
		user, _ := auth.UserFromContext(r.Context())
		if err := deps.Repository.Remove(r.Context(), user.ID, input.Provider, input.AccountID); err != nil {
			deps.Logger.ErrorContext(r.Context(), "remove google account", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not remove the Google account.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"removed": true})
	})))
}
