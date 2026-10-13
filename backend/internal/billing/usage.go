package billing

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

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
			"monthlyGrantCredits": svc.MonthlyGrantFor(status.Plan),
			"paymentsConfigured":  svc.PaymentsConfigured(),
			"availablePlans":      svc.AvailablePlans(),
		})
	}
}

// usage is the Billing screen's credit summary.
type usage struct {
	Plan      string `json:"plan"`
	Unlimited bool   `json:"unlimited"`
	// ChargesCredits is false on a server without payments: nothing is
	// charged, so the screen hides credits.
	ChargesCredits bool  `json:"chargesCredits"`
	BalanceCredits int64 `json:"balanceCredits"`
	MonthlyCredits int64 `json:"monthlyCredits"`
	// PeriodStart is when the current billing month began; Spent covers it.
	PeriodStart time.Time      `json:"periodStart"`
	PeriodEnd   *time.Time     `json:"periodEnd"`
	Spent       []FeatureSpend `json:"spent"`
	Costs       []ActionCost   `json:"costs"`
}

// periodStart is the start of the billing month that ends at end, or the
// start of the calendar month (UTC) without a subscription.
func periodStart(now time.Time, end *time.Time) time.Time {
	if end != nil && end.After(now) {
		return end.AddDate(0, -1, 0)
	}
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// usageHandler returns the active workspace's credits: what is left, what
// the plan grants, where this month's went, and what each action costs.
func usageHandler(logger *slog.Logger, svc *Service, credits *CreditLedgerService, now func() time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := billingUser(w, r, svc)
		if !ok {
			return
		}
		status, err := svc.Status(r.Context(), user.OrganizationID)
		if err != nil {
			logger.ErrorContext(r.Context(), "billing usage status", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		out := usage{
			Plan: status.Plan, Unlimited: status.Unlimited, ChargesCredits: svc.PaymentsConfigured(),
			MonthlyCredits: svc.MonthlyGrantFor(status.Plan), PeriodEnd: status.CurrentPeriodEnd,
			PeriodStart: periodStart(now(), status.CurrentPeriodEnd), Spent: []FeatureSpend{}, Costs: ActionCosts(),
		}
		if credits != nil {
			if out.BalanceCredits, err = credits.GetBalance(r.Context(), user.OrganizationID); err == nil {
				out.Spent, err = credits.SpendByFeature(r.Context(), user.OrganizationID, out.PeriodStart)
			}
			if err != nil {
				logger.ErrorContext(r.Context(), "billing usage", "err", err)
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
				return
			}
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}
