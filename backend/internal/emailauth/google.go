package emailauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

const (
	googleProviderID  = "google"
	stateCookieName   = "better-auth.state"
	stateIdentifier   = "oauth-state:"
	stateTTL          = 10 * time.Minute
	oauthHTTPTimeout  = 10 * time.Second
	maxOAuthBodyBytes = 1 << 20

	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
)

// GoogleProvider is the Google sign-in client. Its zero endpoints are Google's;
// tests point them at a fake.
type GoogleProvider struct {
	ClientID     string
	ClientSecret string
	HTTP         *http.Client
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
}

func (g *GoogleProvider) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: oauthHTTPTimeout}
}

func pick(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

// googleProfile is what Google says about the person who just signed in.
type googleProfile struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// exchange trades the authorization code for the profile. The access token goes
// straight from Google's token endpoint to Google's userinfo endpoint over TLS
// and is never stored: Seomarine uses Google here only to learn who the user is.
func (g *GoogleProvider) exchange(ctx context.Context, code, verifier, redirectURI string) (googleProfile, error) {
	form := url.Values{
		"client_id": {g.ClientID}, "client_secret": {g.ClientSecret}, "code": {code},
		"code_verifier": {verifier}, "redirect_uri": {redirectURI}, "grant_type": {"authorization_code"},
	}
	ctx, cancel := context.WithTimeout(ctx, oauthHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pick(g.TokenURL, googleTokenURL), strings.NewReader(form.Encode()))
	if err != nil {
		return googleProfile{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := g.client().Do(req)
	if err != nil {
		return googleProfile{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return googleProfile{}, fmt.Errorf("token endpoint returned %s", resp.Status)
	}
	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOAuthBodyBytes)).Decode(&tokens); err != nil || tokens.AccessToken == "" {
		return googleProfile{}, errors.New("token endpoint returned no access token")
	}

	infoReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pick(g.UserInfoURL, googleUserInfoURL), nil)
	if err != nil {
		return googleProfile{}, fmt.Errorf("build userinfo request: %w", err)
	}
	infoReq.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	infoResp, err := g.client().Do(infoReq)
	if err != nil {
		return googleProfile{}, fmt.Errorf("fetch userinfo: %w", err)
	}
	defer infoResp.Body.Close()
	if infoResp.StatusCode != http.StatusOK {
		return googleProfile{}, fmt.Errorf("userinfo endpoint returned %s", infoResp.Status)
	}
	var profile googleProfile
	if err := json.NewDecoder(io.LimitReader(infoResp.Body, maxOAuthBodyBytes)).Decode(&profile); err != nil {
		return googleProfile{}, fmt.Errorf("decode userinfo: %w", err)
	}
	return profile, nil
}

// oauthState is what a sign-in remembers between the redirect to Google and the
// callback.
type oauthState struct {
	CallbackURL string `json:"callbackURL"`
	ErrorURL    string `json:"errorURL,omitempty"`
	NewUserURL  string `json:"newUserURL,omitempty"`
	Verifier    string `json:"codeVerifier"`
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Service) redirectURI() string { return s.apiURL("/callback/" + googleProviderID) }

func (s *Service) defaultErrorURL() string {
	return strings.TrimRight(s.Cfg.BaseURL.String(), "/") + "/auth-error"
}

func (s *Service) signInSocial(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Provider           string `json:"provider"`
		CallbackURL        string `json:"callbackURL"`
		ErrorCallbackURL   string `json:"errorCallbackURL"`
		NewUserCallbackURL string `json:"newUserCallbackURL"`
		DisableRedirect    bool   `json:"disableRedirect"`
		IDToken            any    `json:"idToken"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Provider != googleProviderID || s.Google == nil {
		return &apiError{Status: http.StatusNotFound, Code: "PROVIDER_NOT_FOUND", Message: "Provider not found"}
	}
	if in.IDToken != nil {
		return &apiError{Status: http.StatusBadRequest, Code: "ID_TOKEN_NOT_SUPPORTED", Message: "id_token not supported"}
	}
	for _, target := range []string{in.CallbackURL, in.ErrorCallbackURL, in.NewUserCallbackURL} {
		if !s.origins.callbackAllowed(target) {
			return errInvalidCallback
		}
	}
	callback := in.CallbackURL
	if callback == "" {
		callback = "/"
	}

	state, err := newID(32)
	if err != nil {
		return err
	}
	verifier, err := newID(64)
	if err != nil {
		return err
	}
	rowID, err := newID(32)
	if err != nil {
		return err
	}
	stored, err := json.Marshal(oauthState{CallbackURL: callback, ErrorURL: in.ErrorCallbackURL, NewUserURL: in.NewUserCallbackURL, Verifier: verifier})
	if err != nil {
		return fmt.Errorf("encode oauth state: %w", err)
	}
	now := s.Now()
	if err := insertVerification(r.Context(), s.DB, rowID, stateIdentifier+state, string(stored), now.Add(stateTTL), now); err != nil {
		return err
	}
	// The cookie ties the callback to the browser that started the sign-in, so
	// a link someone else crafted cannot finish a login in this browser.
	http.SetCookie(w, &http.Cookie{
		Name: s.jar.name(stateCookieName), Value: s.jar.sign(state), Path: "/", MaxAge: int(stateTTL.Seconds()),
		HttpOnly: true, Secure: s.jar.secure, SameSite: http.SameSiteLaxMode,
	})

	query := url.Values{
		"client_id": {s.Google.ClientID}, "redirect_uri": {s.redirectURI()}, "response_type": {"code"},
		"scope": {"email profile openid"}, "state": {state},
		"code_challenge": {pkceChallenge(verifier)}, "code_challenge_method": {"S256"},
	}
	target := pick(s.Google.AuthURL, googleAuthURL) + "?" + query.Encode()
	if !in.DisableRedirect {
		w.Header().Set("Location", target)
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": target, "redirect": !in.DisableRedirect})
	return nil
}

// googleCallback finishes a Google sign-in. Every failure redirects the browser
// to the error page with a code, never to a JSON error, because the user is in
// the middle of a page navigation.
func (s *Service) googleCallback(w http.ResponseWriter, r *http.Request) error {
	if s.Google == nil || r.PathValue("provider") != googleProviderID {
		return &apiError{Status: http.StatusNotFound, Code: "PROVIDER_NOT_FOUND", Message: "Provider not found"}
	}
	fail := func(errorURL, code string) error {
		http.Redirect(w, r, redirectWith(errorURL, "error", code), http.StatusFound)
		return nil
	}
	query := r.URL.Query()
	state := query.Get("state")
	if state == "" {
		return fail(s.defaultErrorURL(), "state_not_found")
	}
	cookie, err := r.Cookie(s.jar.name(stateCookieName))
	if err != nil {
		return fail(s.defaultErrorURL(), "state_mismatch")
	}
	signedState, ok := s.jar.verify(cookie.Value)
	if !ok || signedState != state {
		return fail(s.defaultErrorURL(), "state_mismatch")
	}
	// Burn the state first: it works once, whatever happens next.
	raw, found, err := consumeVerification(r.Context(), s.DB, stateIdentifier+state, s.Now())
	if err != nil {
		return err
	}
	if !found {
		return fail(s.defaultErrorURL(), "please_restart_the_process")
	}
	var saved oauthState
	if json.Unmarshal([]byte(raw), &saved) != nil {
		return fail(s.defaultErrorURL(), "please_restart_the_process")
	}
	errorURL := s.defaultErrorURL()
	if saved.ErrorURL != "" && s.origins.callbackAllowed(saved.ErrorURL) {
		errorURL = saved.ErrorURL
	}
	if e := query.Get("error"); e != "" {
		return fail(errorURL, e)
	}
	code := query.Get("code")
	if code == "" {
		return fail(errorURL, "no_code")
	}

	profile, err := s.Google.exchange(r.Context(), code, saved.Verifier, s.redirectURI())
	if err != nil {
		s.Logger.WarnContext(r.Context(), "google sign-in exchange failed", "err", err)
		return fail(errorURL, "invalid_code")
	}
	if profile.Subject == "" {
		return fail(errorURL, "unable_to_get_user_info")
	}
	if profile.Email == "" {
		return fail(errorURL, "email_not_found")
	}

	user, session, registered, code2, err := s.signInGoogleUser(r.Context(), profile, s.meta(r))
	if err != nil {
		s.Logger.ErrorContext(r.Context(), "google sign-in failed", "err", err)
		return fail(errorURL, "internal_server_error")
	}
	if code2 != "" {
		return fail(errorURL, code2)
	}
	s.jar.setSession(w, session.Token, true)
	// The state cookie has done its job.
	http.SetCookie(w, &http.Cookie{Name: s.jar.name(stateCookieName), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.jar.secure, SameSite: http.SameSiteLaxMode})

	target := saved.CallbackURL
	if registered && saved.NewUserURL != "" && s.origins.callbackAllowed(saved.NewUserURL) {
		target = saved.NewUserURL
	}
	if registered {
		s.syncSignupContact(user)
	}
	http.Redirect(w, r, target, http.StatusFound)
	return nil
}

// signInGoogleUser resolves the Google profile to an account and starts a
// session. failure is a callback error code, set when the sign-in is refused
// for a reason the user can be told about.
func (s *Service) signInGoogleUser(ctx context.Context, p googleProfile, meta requestMeta) (user User, session Session, registered bool, failure string, err error) {
	email := strings.ToLower(p.Email)
	err = pgdb.InTx(ctx, s.DB, func(tx pgx.Tx) error {
		now := s.Now()
		existingID, hasAccount, err := googleAccountUser(ctx, tx, p.Subject)
		if err != nil {
			return err
		}
		var found bool
		if hasAccount {
			user, found, err = userByID(ctx, tx, existingID)
		} else {
			user, found, err = userByEmail(ctx, tx, email)
		}
		if err != nil {
			return err
		}

		switch {
		case found && hasAccount:
			if p.EmailVerified && !user.EmailVerified && email == user.Email {
				if err := markEmailVerified(ctx, tx, user.Email, now); err != nil {
					return err
				}
				user.EmailVerified = true
			}
		case found:
			// Same address, new sign-in method: link only when both sides vouch
			// for the address. Otherwise anyone could take over an account by
			// registering its email with a provider that does not check it.
			if !p.EmailVerified || !user.EmailVerified {
				failure = "account_not_linked"
				return nil
			}
			accountID, err := newID(32)
			if err != nil {
				return err
			}
			if err := insertOAuthAccount(ctx, tx, accountID, user.ID, googleProviderID, p.Subject, now); err != nil {
				return err
			}
		default:
			if s.Cfg.BlockDisposableEmail && isDisposableEmail(email) {
				failure = "disposable_email"
				return nil
			}
			userID, err := newID(32)
			if err != nil {
				return err
			}
			accountID, err := newID(32)
			if err != nil {
				return err
			}
			optedOut := false
			var image *string
			if p.Picture != "" {
				image = &p.Picture
			}
			user = User{ID: userID, Name: p.Name, Email: email, EmailVerified: p.EmailVerified, Image: image, CreatedAt: now, UpdatedAt: now, AnalyticsOptedOut: &optedOut}
			if err := insertUser(ctx, tx, user); err != nil {
				return err
			}
			if err := insertOAuthAccount(ctx, tx, accountID, userID, googleProviderID, p.Subject, now); err != nil {
				return err
			}
			registered = true
		}

		session, err = s.startSession(ctx, tx, user, true, meta)
		return err
	})
	return user, session, registered, failure, err
}
