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

// Deps contains billing services and the root router's authentication middleware.
type Deps struct {
	Logger          *slog.Logger
	Service         *Service
	WithSession     func(http.Handler) http.Handler
	AutumnSecretKey string
	Hosted          bool
}

// Mount registers billing routes on mux.
func Mount(mux *http.ServeMux, d Deps) {
	mux.Handle("POST /webhooks/razorpay", webhookHandler(d.Logger, d.Service))
	mux.Handle("GET /api/v1/billing/status", d.WithSession(statusHandler(d.Logger, d.Service)))
	mux.Handle("POST /api/v1/billing/checkout", d.WithSession(checkoutHandler(d.Logger, d.Service)))
	mux.Handle("POST /api/v1/billing/usage-events", d.WithSession(usageEventsHandler(d.Logger, d.AutumnSecretKey, d.Hosted)))
}

// statusHandler returns the plan of the signed-in user's active organization.
func statusHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := billingUser(w, r, svc)
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

// checkoutHandler starts a pro subscription for the signed-in user's active organization.
func checkoutHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := billingUser(w, r, svc)
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
func billingUser(w http.ResponseWriter, r *http.Request, svc *Service) (auth.User, bool) {
	if svc == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "billing_unavailable", "Billing is not set up on this server.")
		return auth.User{}, false
	}
	user, ok := auth.UserFromContext(r.Context())
	if !ok || user.OrganizationID == "" {
		httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
		return auth.User{}, false
	}
	return user, true
}

// webhookHandler applies Razorpay's subscription webhooks.
func webhookHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "billing_unavailable", "Billing is not set up on this server.")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
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
