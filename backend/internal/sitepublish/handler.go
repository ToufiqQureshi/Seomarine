package sitepublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/contenteditor"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
)

const (
	maxRequestBody = 8 << 10
	// connectTimeout covers discovery and the sign-in check.
	connectTimeout = 25 * time.Second
	// publishTimeout covers copying up to MaxImagesPerArticle images and
	// saving the post. The handler lifts the server's write deadline to match.
	publishTimeout = 100 * time.Second
	userAgent      = "Uplumo/1.0 (+https://uplumo.com)"
)

// PaidPlans reports whether an organization's plan includes publishing.
type PaidPlans interface {
	HasPaidPlan(ctx context.Context, organizationID string) (bool, error)
}

// Deps wires the site-publishing routes.
type Deps struct {
	Logger *slog.Logger
	// Service is nil when publishing is not configured; routes answer 503.
	Service *Service
	// Plans is nil when billing is not configured: publishing is then open.
	Plans             PaidPlans
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

type handler struct{ Deps }

// Mount registers the site-publishing routes.
func Mount(mux *http.ServeMux, deps Deps) {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	h := handler{deps}
	protect := func(next http.HandlerFunc) http.Handler { return deps.WithSession(deps.WithProjectAccess(next)) }
	mux.Handle("POST /api/v1/projects/{projectId}/site-connection/get", protect(h.getConnection))
	mux.Handle("POST /api/v1/projects/{projectId}/site-connection/save", protect(h.saveConnection))
	mux.Handle("POST /api/v1/projects/{projectId}/site-connection/delete", protect(h.deleteConnection))
	mux.Handle("POST /api/v1/projects/{projectId}/content/drafts/{draftId}/publication/get", protect(h.getPublication))
	mux.Handle("POST /api/v1/projects/{projectId}/content/drafts/{draftId}/publish", protect(h.publish))
}

type connectionView struct {
	Connected   bool       `json:"connected"`
	Platform    string     `json:"platform,omitempty"`
	SiteURL     string     `json:"siteUrl,omitempty"`
	SiteName    string     `json:"siteName,omitempty"`
	Username    string     `json:"username,omitempty"`
	ConnectedAt *time.Time `json:"connectedAt,omitempty"`
}

func viewConnection(c Connection) connectionView {
	connectedAt := c.CreatedAt.UTC()
	return connectionView{Connected: true, Platform: c.Platform, SiteURL: c.SiteURL, SiteName: c.SiteName, Username: c.Username, ConnectedAt: &connectedAt}
}

type publicationView struct {
	Published   bool       `json:"published"`
	SiteURL     string     `json:"siteUrl,omitempty"`
	URL         string     `json:"url,omitempty"`
	Status      string     `json:"status,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

func viewPublication(p Publication) publicationView {
	publishedAt, updatedAt := p.PublishedAt.UTC(), p.UpdatedAt.UTC()
	return publicationView{Published: true, SiteURL: p.SiteURL, URL: p.RemoteURL, Status: p.RemoteStatus, PublishedAt: &publishedAt, UpdatedAt: &updatedAt}
}

func (h handler) getConnection(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) || !decodeEmpty(w, r) {
		return
	}
	connection, err := h.Service.Connection(r.Context(), r.PathValue("projectId"))
	switch {
	case errors.Is(err, ErrNotConnected):
		httpx.WriteJSON(w, http.StatusOK, connectionView{})
	case err != nil:
		h.internal(w, r, "load site connection", err)
	default:
		httpx.WriteJSON(w, http.StatusOK, viewConnection(connection))
	}
}

func (h handler) saveConnection(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	user, ok := h.requireManager(w, r)
	if !ok || !h.requirePlan(w, r) {
		return
	}
	var input ConnectInput
	if !decode(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), connectTimeout)
	defer cancel()
	connection, err := h.Service.Connect(ctx, r.PathValue("projectId"), user.ID, input)
	if err != nil {
		h.writeSiteError(w, r, "connect site", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewConnection(connection))
}

func (h handler) deleteConnection(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	if _, ok := h.requireManager(w, r); !ok || !decodeEmpty(w, r) {
		return
	}
	if err := h.Service.Disconnect(r.Context(), r.PathValue("projectId")); err != nil {
		h.internal(w, r, "disconnect site", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, connectionView{})
}

func (h handler) getPublication(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) || !decodeEmpty(w, r) {
		return
	}
	publication, err := h.Service.Publication(r.Context(), r.PathValue("projectId"), r.PathValue("draftId"))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteJSON(w, http.StatusOK, publicationView{})
	case err != nil:
		h.internal(w, r, "load publication", err)
	default:
		httpx.WriteJSON(w, http.StatusOK, viewPublication(publication))
	}
}

func (h handler) publish(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) || !h.requirePlan(w, r) {
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &input) {
		return
	}
	// Copying images can outlast the server's 30 second write timeout.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(publishTimeout + 10*time.Second)); err != nil {
		h.Logger.WarnContext(r.Context(), "extend publish write deadline", "err", err)
	}
	ctx, cancel := context.WithTimeout(r.Context(), publishTimeout)
	defer cancel()
	publication, err := h.Service.Publish(ctx, r.PathValue("projectId"), r.PathValue("draftId"), input.Status)
	if err != nil {
		h.writeSiteError(w, r, "publish draft", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, viewPublication(publication))
}

// writeSiteError answers with what the customer can do about err.
func (h handler) writeSiteError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	var validation *ValidationError
	var remote *RemoteError
	switch {
	case errors.As(err, &validation):
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", validation.Message)
	case errors.Is(err, ErrNotConnected):
		httpx.WriteError(w, http.StatusConflict, "site_not_connected", "Connect your website in Settings first.")
	case errors.Is(err, contenteditor.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "This draft is no longer available.")
	case errors.Is(err, ErrKeywordStuffing):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "keyword_stuffing", "This article repeats its target topic too often. Rewrite it so it reads naturally, or send it as a draft.")
	case errors.Is(err, ErrNotHTTPS):
		httpx.WriteError(w, http.StatusBadRequest, "site_not_https", "Your website must use https:// before Uplumo can sign in to it.")
	case errors.Is(err, ErrSiteBlocked):
		h.Logger.WarnContext(r.Context(), operation, "err", err)
		httpx.WriteError(w, http.StatusUnprocessableEntity, "site_blocked", "Your website's security blocked Uplumo. On Hostinger, open hPanel, go to Security and turn off bot protection (or ask Hostinger support to allow Uplumo). With Cloudflare or a security plugin, allow the user agent \"Uplumo/1.0\". Then try again.")
	case errors.Is(err, ErrNotWordPress):
		h.Logger.WarnContext(r.Context(), operation, "err", err)
		httpx.WriteError(w, http.StatusUnprocessableEntity, "site_not_wordpress", "We could not find WordPress at this address. Check the address, or ask your developer whether the WordPress REST API is turned off.")
	case errors.Is(err, ErrAppPasswordsOff):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "app_passwords_off", "Application passwords are turned off on this site, often by a security plugin. Turn them on, then try again.")
	case errors.Is(err, ErrAuthHeaderDropped):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "auth_header_dropped", "Your hosting removed the login details before they reached WordPress, so the password was never checked. Add these two lines at the top of the .htaccess file in your site's main folder, then try again: RewriteEngine On and RewriteRule .* - [E=HTTP_AUTHORIZATION:%{HTTP:Authorization}]. Your host's support can do it for you.")
	case errors.Is(err, ErrBadCredentials):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "bad_credentials", "WordPress did not accept this username and application password. Check both. If they are right, your hosting may be removing the login header; ask them to allow the Authorization header.")
	case errors.Is(err, ErrMissingPermission):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "missing_permission", "This WordPress user cannot publish posts and upload images. Use an Editor or Administrator account.")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrSiteUnreachable):
		h.Logger.WarnContext(r.Context(), operation, "err", err)
		httpx.WriteError(w, http.StatusBadGateway, "site_unreachable", "Your website did not answer in time. Check it is online, then try again.")
	case errors.As(err, &remote):
		h.Logger.WarnContext(r.Context(), operation, "status", remote.Status, "code", remote.Code)
		message := "Your website refused the request."
		if remote.Message != "" {
			message += " WordPress said: " + remote.Message
		}
		httpx.WriteError(w, http.StatusBadGateway, "site_error", message)
	default:
		h.internal(w, r, operation, err)
	}
}

func (h handler) ready(w http.ResponseWriter) bool {
	if h.Service == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "publishing_unavailable", "Publishing to your website is not available on this server.")
		return false
	}
	return true
}

// requireManager allows owners and admins of the project's workspace: the
// connection holds a password to the customer's website.
func (h handler) requireManager(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, _ := auth.UserFromContext(r.Context())
	organizationID, _ := auth.ProjectOrganizationFromContext(r.Context())
	if user.OrganizationID != organizationID || (user.Role != "owner" && user.Role != "admin") {
		httpx.WriteError(w, http.StatusForbidden, "forbidden", "Only a workspace owner or admin can connect or disconnect the website.")
		return auth.User{}, false
	}
	return user, true
}

func (h handler) requirePlan(w http.ResponseWriter, r *http.Request) bool {
	if h.Plans == nil {
		return true
	}
	organizationID, _ := auth.ProjectOrganizationFromContext(r.Context())
	allowed, err := h.Plans.HasPaidPlan(r.Context(), organizationID)
	if err != nil {
		h.internal(w, r, "check publishing plan", err)
		return false
	}
	if !allowed {
		httpx.WriteError(w, http.StatusPaymentRequired, "plan_required", "Publishing to your website is available on Basic, Pro and Agency.")
		return false
	}
	return true
}

func (h handler) internal(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if r.Context().Err() != nil {
		return
	}
	h.Logger.ErrorContext(r.Context(), operation, "err", err)
	httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
}

func decodeEmpty(w http.ResponseWriter, r *http.Request) bool {
	var empty map[string]json.RawMessage
	if !decode(w, r, &empty) {
		return false
	}
	if len(empty) != 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "This request takes no fields.")
		return false
	}
	return true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	trimmed := bytes.TrimSpace(body)
	if err != nil || len(trimmed) == 0 || trimmed[0] != '{' {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Send a JSON object.")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Send a JSON object with the expected fields.")
		return false
	}
	return true
}
