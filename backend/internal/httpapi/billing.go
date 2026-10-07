package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/billing"
)

// maxWebhookBody bounds a Razorpay webhook; real ones are a few kilobytes.
const maxWebhookBody = 256 << 10

// billingStatus returns the plan of the signed-in user's active organization.
func billingStatus(logger *slog.Logger, svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := billingUser(w, r, logger, svc)
		if !ok {
			return
		}
		status, err := svc.Status(r.Context(), user.OrganizationID)
		if err != nil {
			logger.ErrorContext(r.Context(), "billing status", "err", err)
			writeError(w, logger, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		writeJSON(w, logger, http.StatusOK, status)
	}
}

// billingCheckout starts a pro subscription for the signed-in user's active
// organization. Only its owners and admins may.
func billingCheckout(logger *slog.Logger, svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := billingUser(w, r, logger, svc)
		if !ok {
			return
		}
		if user.Role != "owner" && user.Role != "admin" {
			writeError(w, logger, http.StatusForbidden, "forbidden", "Only workspace owners and admins can change the plan.")
			return
		}
		checkout, err := svc.Checkout(r.Context(), user.OrganizationID)
		if errors.Is(err, billing.ErrAlreadySubscribed) {
			writeError(w, logger, http.StatusConflict, "already_subscribed", "This workspace already has a subscription.")
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "billing checkout", "err", err)
			if errors.Is(err, billing.ErrPaymentProvider) {
				writeError(w, logger, http.StatusBadGateway, "payment_provider_error", "Could not start the payment. Please try again.")
			} else {
				writeError(w, logger, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			}
			return
		}
		writeJSON(w, logger, http.StatusOK, checkout)
	}
}

// billingUser returns the signed-in user when billing is configured and the
// user has an active organization, and otherwise answers the request.
func billingUser(w http.ResponseWriter, r *http.Request, logger *slog.Logger, svc *billing.Service) (auth.User, bool) {
	if svc == nil {
		writeError(w, logger, http.StatusServiceUnavailable, "billing_unavailable", "Billing is not set up on this server.")
		return auth.User{}, false
	}
	user, _ := r.Context().Value(userKey{}).(auth.User)
	if user.OrganizationID == "" {
		writeError(w, logger, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
		return auth.User{}, false
	}
	return user, true
}

// razorpayWebhook applies Razorpay's subscription webhooks. Anything but a
// 2xx makes Razorpay redeliver, so only a delivery that failed for a reason
// that may pass gets a 5xx.
func razorpayWebhook(logger *slog.Logger, svc *billing.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeError(w, logger, http.StatusServiceUnavailable, "billing_unavailable", "Billing is not set up on this server.")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
		if maxErr := (*http.MaxBytesError)(nil); errors.As(err, &maxErr) {
			writeError(w, logger, http.StatusRequestEntityTooLarge, "payload_too_large", "The webhook is too large.")
			return
		}
		if err != nil {
			writeError(w, logger, http.StatusBadRequest, "invalid_event", "The webhook body could not be read.")
			return
		}

		result, err := svc.HandleWebhook(r.Context(), body, r.Header.Get("X-Razorpay-Signature"), r.Header.Get("X-Razorpay-Event-Id"))
		switch {
		case err == nil:
			writeJSON(w, logger, http.StatusOK, map[string]billing.WebhookResult{"status": result})
		case errors.Is(err, billing.ErrInvalidSignature):
			writeError(w, logger, http.StatusUnauthorized, "invalid_signature", "The webhook signature is not valid.")
		case errors.Is(err, billing.ErrInvalidEvent):
			logger.WarnContext(r.Context(), "invalid razorpay webhook", "err", err)
			writeError(w, logger, http.StatusBadRequest, "invalid_event", "The webhook event is not valid.")
		default:
			logger.ErrorContext(r.Context(), "apply razorpay webhook", "err", err)
			writeError(w, logger, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
		}
	}
}
