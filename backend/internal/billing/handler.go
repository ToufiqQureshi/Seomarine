package billing

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxWebhookBody = 256 << 10

// Mount registers billing routes on the root mux.
func Mount(mux *http.ServeMux, logger *slog.Logger, svc *Service, withSession func(http.Handler) http.Handler) {
	mux.Handle("POST /webhooks/razorpay", WebhookHandler(logger, svc))
	mux.Handle("GET /api/v1/billing/status", withSession(StatusHandler(logger, svc)))
	mux.Handle("POST /api/v1/billing/checkout", withSession(CheckoutHandler(logger, svc)))
}

// StatusHandler returns the plan of the signed-in user's active organization.
func StatusHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := billingUser(w, r, logger, svc)
		if !ok {
			return
		}
		status, err := svc.Status(r.Context(), user.OrganizationID)
		if err != nil {
			logger.ErrorContext(r.Context(), "billing status", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, status)
	}
}

// CheckoutHandler starts a pro subscription for the signed-in user's active organization.
func CheckoutHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := billingUser(w, r, logger, svc)
		if !ok {
			return
		}
		if user.Role != "owner" && user.Role != "admin" {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "Only workspace owners and admins can change the plan.")
			return
		}
		checkout, err := svc.Checkout(r.Context(), user.OrganizationID)
		if errors.Is(err, ErrAlreadySubscribed) {
			httpx.WriteError(w, http.StatusConflict, "already_subscribed", "This workspace already has a subscription.")
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "billing checkout", "err", err)
			if errors.Is(err, ErrPaymentProvider) {
				httpx.WriteError(w, http.StatusBadGateway, "payment_provider_error", "Could not start the payment. Please try again.")
			} else {
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			}
			return
		}
		httpx.WriteJSON(w, http.StatusOK, checkout)
	}
}

// billingUser returns the signed-in user when billing is configured and the user has an active organization.
func billingUser(w http.ResponseWriter, r *http.Request, logger *slog.Logger, svc *Service) (auth.User, bool) {
	if svc == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "billing_unavailable", "Billing is not set up on this server.")
		return auth.User{}, false
	}
	user, ok := httpx.UserFromContext(r.Context())
	if !ok || user.OrganizationID == "" {
		httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
		return auth.User{}, false
	}
	return user, true
}

// WebhookHandler applies Razorpay's subscription webhooks.
func WebhookHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "billing_unavailable", "Billing is not set up on this server.")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
		if maxErr := (*http.MaxBytesError)(nil); errors.As(err, &maxErr) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "The webhook is too large.")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_event", "The webhook body could not be read.")
			return
		}

		result, err := svc.HandleWebhook(r.Context(), body, r.Header.Get("X-Razorpay-Signature"), r.Header.Get("X-Razorpay-Event-Id"))
		switch {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, map[string]WebhookResult{"status": result})
		case errors.Is(err, ErrInvalidSignature):
			httpx.WriteError(w, http.StatusUnauthorized, "invalid_signature", "The webhook signature is not valid.")
		case errors.Is(err, ErrInvalidEvent):
			logger.WarnContext(r.Context(), "invalid razorpay webhook", "err", err)
			httpx.WriteError(w, http.StatusBadRequest, "invalid_event", "The webhook event is not valid.")
		default:
			logger.ErrorContext(r.Context(), "apply razorpay webhook", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
		}
	}
}
