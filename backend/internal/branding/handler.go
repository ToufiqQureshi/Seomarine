package branding

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

// maxBrandingBody leaves room for the largest legal payload: a MaxLogoChars
// data URL, a name, a color and a website.
const maxBrandingBody = 350 << 10

// Mount registers branding routes on the root mux. withSession attaches the
// signed-in user, whose active organization is the row that is read or written.
func Mount(mux *http.ServeMux, logger *slog.Logger, svc *Service, withSession func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/branding", withSession(GetHandler(logger, svc)))
	mux.Handle("POST /api/v1/branding", withSession(SaveHandler(logger, svc)))
	mux.Handle("POST /api/v1/branding/reset", withSession(ResetHandler(logger, svc)))
}

// brandingPayload is the request body. The nullable fields are raw JSON so a
// missing key can be told apart from an explicit null, which the legacy schema
// rejected: every key is required, though two of them may be null.
type brandingPayload struct {
	BrandName   *string         `json:"brandName"`
	AccentColor *string         `json:"accentColor"`
	LogoDataURL json.RawMessage `json:"logoDataUrl"`
	WebsiteURL  json.RawMessage `json:"websiteUrl"`
}

var (
	accentPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	// SVG is safe in this position: the logo renders through <img>, which
	// never runs its scripts.
	logoPattern = regexp.MustCompile(`^data:image/(png|jpeg|webp|svg\+xml);base64,[A-Za-z0-9+/]+={0,2}$`)
)

// validate mirrors brandingInputSchema: it trims the name, enforces the length
// caps and patterns, and returns the value that may be stored.
func (p brandingPayload) validate() (Input, error) {
	if p.BrandName == nil {
		return Input{}, errors.New("brandName is required")
	}
	in := Input{BrandName: strings.TrimSpace(*p.BrandName)}
	if n := utf16Len(in.BrandName); n < 1 || n > MaxNameChars {
		return Input{}, fmt.Errorf("brandName must be 1 to %d characters", MaxNameChars)
	}
	if p.AccentColor == nil || !accentPattern.MatchString(*p.AccentColor) {
		return Input{}, errors.New("accentColor must be a hex color like #2563eb")
	}
	in.AccentColor = *p.AccentColor

	logo, err := optionalString(p.LogoDataURL, "logoDataUrl")
	if err != nil {
		return Input{}, err
	}
	if logo != nil {
		if utf16Len(*logo) > MaxLogoChars {
			return Input{}, fmt.Errorf("logoDataUrl must be at most %d characters", MaxLogoChars)
		}
		if !logoPattern.MatchString(*logo) {
			return Input{}, errors.New("logoDataUrl must be a base64 png, jpeg, webp or svg data URL")
		}
	}
	in.LogoDataURL = logo

	website, err := optionalString(p.WebsiteURL, "websiteUrl")
	if err != nil {
		return Input{}, err
	}
	if website != nil {
		if utf16Len(*website) > MaxWebsiteChars {
			return Input{}, fmt.Errorf("websiteUrl must be at most %d characters", MaxWebsiteChars)
		}
		parsed, err := url.Parse(*website)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return Input{}, errors.New("websiteUrl must be a full http or https URL")
		}
	}
	in.WebsiteURL = website

	return in, nil
}

// optionalString reads a nullable string field, treating an explicit null as
// absent and a missing key or another JSON type as invalid.
func optionalString(raw json.RawMessage, field string) (*string, error) {
	if raw == nil {
		return nil, fmt.Errorf("%s is required", field)
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s must be a string or null", field)
	}
	return &value, nil
}

// utf16Len counts UTF-16 code units, matching JavaScript's String#length that
// the legacy schema's max() measured.
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// canUpdateOrganization reports whether role may change an organization
// setting, mirroring the legacy gate on the better-auth organization
// `update` permission. better-auth stores multiple roles as one
// comma-separated string; unknown names fail closed.
func canUpdateOrganization(role string) bool {
	for _, name := range strings.Split(role, ",") {
		switch strings.TrimSpace(name) {
		case "owner", "admin":
			return true
		}
	}
	return false
}

// brandingOrg returns the request's active organization.
func brandingOrg(w http.ResponseWriter, r *http.Request) (string, bool) {
	user, ok := httpx.UserFromContext(r.Context())
	if !ok || user.OrganizationID == "" {
		httpx.WriteError(w, http.StatusForbidden, "no_organization", "Open a workspace first, then try again.")
		return "", false
	}
	return user.OrganizationID, true
}

// GetHandler returns the active organization's branding, or null when it uses
// the default product branding.
func GetHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		organizationID, ok := brandingOrg(w, r)
		if !ok {
			return
		}
		b, found, err := svc.Get(r.Context(), organizationID)
		if err != nil {
			logger.ErrorContext(r.Context(), "load branding", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		if !found {
			httpx.WriteJSON(w, http.StatusOK, nil)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, b)
	}
}

// SaveHandler writes the active organization's branding.
func SaveHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		organizationID, ok := brandingOrg(w, r)
		if !ok {
			return
		}
		user, _ := httpx.UserFromContext(r.Context())
		if !canUpdateOrganization(user.Role) {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "Your organization role does not allow this action.")
			return
		}

		var payload brandingPayload
		err := httpx.DecodeJSONLimit(r.Body, maxBrandingBody, &payload)
		if errors.Is(err, httpx.ErrPayloadTooLarge) {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "The branding is too large.")
			return
		}
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_branding", "The branding is not valid JSON.")
			return
		}
		input, err := payload.validate()
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_branding", err.Error())
			return
		}
		if err := svc.Save(r.Context(), organizationID, input); err != nil {
			logger.ErrorContext(r.Context(), "save branding", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ResetHandler returns the active organization to the default product branding.
func ResetHandler(logger *slog.Logger, svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		organizationID, ok := brandingOrg(w, r)
		if !ok {
			return
		}
		user, _ := httpx.UserFromContext(r.Context())
		if !canUpdateOrganization(user.Role) {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "Your organization role does not allow this action.")
			return
		}
		if err := svc.Reset(r.Context(), organizationID); err != nil {
			logger.ErrorContext(r.Context(), "reset branding", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
