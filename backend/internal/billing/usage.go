package billing

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
)

type usageRange struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// usageEventsHandler lists the active organization's settled credit spends
// for the billing usage chart. Start and end are Unix milliseconds.
func usageEventsHandler(logger *slog.Logger, credits *CreditLedgerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.UserFromContext(r.Context())
		if !ok || user.OrganizationID == "" {
			httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
			return
		}
		var in usageRange
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil || in.Start < 0 || in.End < in.Start {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid billing usage date range.")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "The request must contain exactly one JSON value.")
			return
		}
		if credits == nil {
			httpx.WriteJSON(w, http.StatusOK, []UsageEvent{})
			return
		}
		events, err := credits.UsageEvents(r.Context(), user.OrganizationID, in.Start, in.End)
		if err != nil {
			logger.ErrorContext(r.Context(), "load billing usage events", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Could not load billing usage history. Please try again.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, events)
	}
}

// creditsHandler returns the plan and remaining credits in one call, which is
// everything the plan gate and the credit banners read.
func creditsHandler(logger *slog.Logger, svc *Service, credits *CreditLedgerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := billingUser(w, r, svc)
		if !ok {
			return
		}
		status, err := svc.Status(r.Context(), user.OrganizationID)
		if err != nil {
			logger.ErrorContext(r.Context(), "billing credits status", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		var balance int64
		if credits != nil {
			if balance, err = credits.GetBalance(r.Context(), user.OrganizationID); err != nil {
				logger.ErrorContext(r.Context(), "billing credit balance", "err", err)
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
				return
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"plan":                status.Plan,
			"status":              status.Status,
			"currentPeriodEnd":    status.CurrentPeriodEnd,
			"balanceCredits":      balance,
			"balanceUsd":          float64(balance) / CreditsPerUSD,
			"monthlyGrantCredits": svc.MonthlyGrantCredits,
			"paymentsConfigured":    svc.PaymentsConfigured(),
		})
	}
}
