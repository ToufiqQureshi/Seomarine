package emailauth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
)

const (
	maxBodyBytes = 16 << 10
	timeLayout   = "2006-01-02T15:04:05.000Z"
)

// AuthSessions adapts the session reader of the auth package.
type AuthSessions struct{ Auth *auth.Service }

// CurrentUserID returns the signed-in user's id, or false when there is none.
func (a AuthSessions) CurrentUserID(r *http.Request) (string, bool, error) {
	user, err := a.Auth.Authenticate(r)
	if errors.Is(err, auth.ErrUnauthenticated) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return user.ID, true, nil
}

// Mount registers the email and password routes under /api/auth.
func Mount(mux *http.ServeMux, s *Service) {
	route := func(pattern, name string, limit int, window time.Duration, h func(http.ResponseWriter, *http.Request) error) {
		mux.HandleFunc(pattern, s.wrap(name, limit, window, h))
	}
	route("POST /api/auth/sign-up/email", "sign-up", 5, time.Minute, s.signUp)
	route("POST /api/auth/sign-in/email", "sign-in", 10, time.Minute, s.signIn)
	route("GET /api/auth/verify-email", "verify-email", 20, time.Minute, s.verifyEmail)
	route("POST /api/auth/send-verification-email", "send-verification", 3, time.Minute, s.sendVerificationEmail)
	route("POST /api/auth/request-password-reset", "request-reset", 3, time.Minute, s.requestPasswordReset)
	route("GET /api/auth/reset-password/{token}", "reset-callback", 20, time.Minute, s.resetCallback)
	route("POST /api/auth/reset-password", "reset-password", 10, time.Minute, s.resetPassword)
	route("POST /api/auth/update-user", "update-user", 20, time.Minute, s.updateUser)
	route("POST /api/auth/sign-in/social", "sign-in-social", 20, time.Minute, s.signInSocial)
	route("GET /api/auth/callback/{provider}", "oauth-callback", 30, time.Minute, s.googleCallback)
}

// wrap adds rate limiting, the CSRF origin check and uniform error output.
func (s *Service) wrap(name string, limit int, window time.Duration, h func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && !s.origins.requestAllowed(r) {
			writeAPIError(w, errInvalidOrigin)
			return
		}
		if s.Limiter != nil {
			ip := ""
			if s.ClientIP != nil {
				ip = s.ClientIP(r)
			}
			ok, retry, err := s.Limiter.Allow(r.Context(), name+":"+ip, limit, window)
			switch {
			case err != nil:
				// A Redis outage must not lock everyone out of signing in.
				s.Logger.WarnContext(r.Context(), "auth rate limit check failed", "err", err)
			case !ok:
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
				writeAPIError(w, errTooManyRequests)
				return
			}
		}
		err := h(w, r)
		if err == nil {
			return
		}
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			writeAPIError(w, apiErr)
			return
		}
		if r.Context().Err() != nil {
			return
		}
		s.Logger.ErrorContext(r.Context(), "auth request failed", "route", name, "err", err)
		writeAPIError(w, errInternal)
	}
}

func writeAPIError(w http.ResponseWriter, e *apiError) {
	writeJSON(w, e.Status, e)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// decode reads a JSON object body. Unknown fields are ignored: Better Auth
// does, and the client sends fields (fetch options, plugin extras) this server
// has no use for.
func decode(r *http.Request, dst any) error {
	if r.Body == nil {
		return errMalformedBody
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return errMalformedBody
	}
	if len(body) > maxBodyBytes {
		return errMalformedBody
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return errMalformedBody
	}
	return nil
}

func (s *Service) meta(r *http.Request) requestMeta {
	m := requestMeta{UserAgent: r.UserAgent()}
	if s.ClientIP != nil {
		m.IP = s.ClientIP(r)
	}
	return m
}

type userBody struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Email             string  `json:"email"`
	EmailVerified     bool    `json:"emailVerified"`
	Image             *string `json:"image"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
	AnalyticsOptedOut *bool   `json:"analyticsOptedOut,omitempty"`
}

func userJSON(u User) userBody {
	return userBody{
		ID: u.ID, Name: u.Name, Email: u.Email, EmailVerified: u.EmailVerified, Image: u.Image,
		CreatedAt: u.CreatedAt.UTC().Format(timeLayout), UpdatedAt: u.UpdatedAt.UTC().Format(timeLayout),
		AnalyticsOptedOut: u.AnalyticsOptedOut,
	}
}

func (s *Service) signUp(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name              string  `json:"name"`
		Email             string  `json:"email"`
		Password          string  `json:"password"`
		Image             *string `json:"image"`
		CallbackURL       string  `json:"callbackURL"`
		RememberMe        *bool   `json:"rememberMe"`
		AnalyticsOptedOut *bool   `json:"analyticsOptedOut"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if s.Captcha != nil {
		if err := s.verifyCaptcha(r); err != nil {
			return err
		}
	}
	result, err := s.SignUp(r.Context(), SignUpInput{
		Name: in.Name, Email: in.Email, Password: in.Password, Image: in.Image,
		RememberMe: in.RememberMe, AnalyticsOptedOut: in.AnalyticsOptedOut,
		CallbackURL: in.CallbackURL, Meta: s.meta(r),
	})
	if err != nil {
		return err
	}
	body := map[string]any{"token": nil, "user": userJSON(result.User)}
	if result.Session != nil {
		s.jar.setSession(w, result.Session.Token, result.Remember)
		body["token"] = result.Session.Token
	}
	writeJSON(w, http.StatusOK, body)
	return nil
}

// verifyCaptcha checks the Turnstile response the browser sent in a header.
func (s *Service) verifyCaptcha(r *http.Request) error {
	response := r.Header.Get("x-captcha-response")
	if response == "" {
		return errCaptchaMissing
	}
	ip := ""
	if s.ClientIP != nil {
		ip = s.ClientIP(r)
	}
	switch err := s.Captcha.Verify(r.Context(), response, ip); {
	case err == nil:
		return nil
	case errors.Is(err, errCaptchaRejected):
		return errCaptchaFailed
	default:
		s.Logger.ErrorContext(r.Context(), "captcha verification unavailable", "err", err)
		return errCaptchaUnknown
	}
}

func (s *Service) signIn(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		CallbackURL string `json:"callbackURL"`
		RememberMe  *bool  `json:"rememberMe"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	result, err := s.SignIn(r.Context(), SignInInput{
		Email: in.Email, Password: in.Password, CallbackURL: in.CallbackURL,
		RememberMe: in.RememberMe, Meta: s.meta(r),
	})
	if err != nil {
		return err
	}
	s.jar.setSession(w, result.Session.Token, result.Remember)
	body := map[string]any{"redirect": in.CallbackURL != "", "token": result.Session.Token, "user": userJSON(result.User)}
	if in.CallbackURL != "" {
		w.Header().Set("Location", in.CallbackURL)
		body["url"] = in.CallbackURL
	} else {
		body["url"] = nil
	}
	writeJSON(w, http.StatusOK, body)
	return nil
}

// redirectWith appends a query parameter to a callback URL.
func redirectWith(callbackURL, key, value string) string {
	separator := "?"
	if strings.Contains(callbackURL, "?") {
		separator = "&"
	}
	return callbackURL + separator + key + "=" + url.QueryEscape(value)
}

func (s *Service) currentUserID(r *http.Request) (string, error) {
	if s.Sessions == nil {
		return "", nil
	}
	id, _, err := s.Sessions.CurrentUserID(r)
	return id, err
}

func (s *Service) verifyEmail(w http.ResponseWriter, r *http.Request) error {
	token := r.URL.Query().Get("token")
	callbackURL := r.URL.Query().Get("callbackURL")
	if token == "" {
		return &apiError{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: "[query.token] Invalid input: expected string, received undefined"}
	}
	sessionUser, err := s.currentUserID(r)
	if err != nil {
		return err
	}
	outcome, err := s.VerifyEmail(r.Context(), token, callbackURL, sessionUser, s.meta(r))
	if err != nil {
		return err
	}
	if outcome.ErrorCode != "" {
		if callbackURL != "" {
			http.Redirect(w, r, redirectWith(callbackURL, "error", outcome.ErrorCode), http.StatusFound)
			return nil
		}
		return &apiError{Status: http.StatusUnauthorized, Code: outcome.ErrorCode, Message: verifyErrorMessage(outcome.ErrorCode)}
	}
	if outcome.Session != nil {
		s.jar.setSession(w, outcome.Session.Token, outcome.Remember)
	}
	if callbackURL != "" {
		http.Redirect(w, r, callbackURL, http.StatusFound)
		return nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "user": nil})
	return nil
}

func verifyErrorMessage(code string) string {
	switch code {
	case "TOKEN_EXPIRED":
		return "Token expired"
	case "USER_NOT_FOUND":
		return "User not found"
	default:
		return "Invalid token"
	}
}

func (s *Service) sendVerificationEmail(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email       string `json:"email"`
		CallbackURL string `json:"callbackURL"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	sessionUser, err := s.currentUserID(r)
	if err != nil {
		return err
	}
	if err := s.SendVerificationEmail(r.Context(), in.Email, in.CallbackURL, sessionUser); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]bool{"status": true})
	return nil
}

func (s *Service) requestPasswordReset(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email      string `json:"email"`
		RedirectTo string `json:"redirectTo"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.RequestPasswordReset(r.Context(), in.Email, in.RedirectTo); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": genericResetMessage})
	return nil
}

// resetCallback is the link in the reset email. It checks the token and sends
// the browser on to the app's reset page, with the token or an error.
func (s *Service) resetCallback(w http.ResponseWriter, r *http.Request) error {
	token := r.PathValue("token")
	callbackURL := r.URL.Query().Get("callbackURL")
	if callbackURL == "" {
		return &apiError{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR", Message: "[query.callbackURL] Invalid input: expected string, received undefined"}
	}
	if !s.origins.callbackAllowed(callbackURL) {
		return errInvalidCallback
	}
	valid, err := s.ResetLinkValid(r.Context(), token)
	if err != nil {
		return err
	}
	if !valid {
		http.Redirect(w, r, redirectWith(callbackURL, "error", "INVALID_TOKEN"), http.StatusFound)
		return nil
	}
	http.Redirect(w, r, redirectWith(callbackURL, "token", token), http.StatusFound)
	return nil
}

func (s *Service) resetPassword(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		NewPassword string `json:"newPassword"`
		Token       string `json:"token"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	token := in.Token
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if err := s.ResetPassword(r.Context(), token, in.NewPassword); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]bool{"status": true})
	return nil
}

func (s *Service) updateUser(w http.ResponseWriter, r *http.Request) error {
	userID, err := s.currentUserID(r)
	if err != nil {
		return err
	}
	if userID == "" {
		return errUnauthorized
	}
	var in struct {
		Email             *string `json:"email"`
		Name              *string `json:"name"`
		Image             *string `json:"image"`
		AnalyticsOptedOut *bool   `json:"analyticsOptedOut"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Email != nil && *in.Email != "" {
		return errEmailImmutable
	}
	if err := s.UpdateProfile(r.Context(), userID, in.Name, in.Image, in.AnalyticsOptedOut); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]bool{"status": true})
	return nil
}
