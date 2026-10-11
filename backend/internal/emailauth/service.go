package emailauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
)

const (
	defaultMinPassword = 8
	defaultMaxPassword = 128

	verificationTTL = time.Hour
	resetTTL        = time.Hour
	// verificationFloor is the least time "send verification email" takes for a
	// signed-out caller, so response time cannot tell a registered address from
	// an unknown one.
	verificationFloor = 500 * time.Millisecond

	resetIdentifierPrefix = "reset-password:"
	genericResetMessage   = "If this email exists in our system, check your email for the reset link"
)

// Config holds the deployment settings of the auth routes.
type Config struct {
	// Secret is BETTER_AUTH_SECRET: it signs session cookies and email tokens.
	Secret string
	// BaseURL is the public origin, BETTER_AUTH_URL. Links in emails and the
	// cookie scheme derive from it.
	BaseURL *url.URL
	// TrustedOrigins are accepted in addition to BaseURL.
	TrustedOrigins []string
	// BypassEmailVerification skips email verification (BYPASS_EMAIL_VERIFICATION).
	// Sign-up then signs the user straight in. Meant for development.
	BypassEmailVerification bool
	// BlockDisposableEmail rejects throwaway-inbox domains at sign-up.
	BlockDisposableEmail bool
	MinPassword          int
	MaxPassword          int
}

// SessionReader resolves the session on a request.
type SessionReader interface {
	CurrentUserID(r *http.Request) (string, bool, error)
}

// Service holds the account rules.
type Service struct {
	DB       *pgxpool.Pool
	Cfg      Config
	Hasher   *Hasher
	Mailer   Mailer
	Captcha  Captcha
	Limiter  Limiter
	Sessions SessionReader
	// Google enables "Sign in with Google"; nil leaves it off.
	Google   *GoogleProvider
	Logger   *slog.Logger
	ClientIP func(*http.Request) string
	Now      func() time.Time

	origins origins
	jar     cookieJar
}

// New wires a Service and derives the origin and cookie policy from cfg.
func New(s Service) *Service {
	if s.Cfg.MinPassword == 0 {
		s.Cfg.MinPassword = defaultMinPassword
	}
	if s.Cfg.MaxPassword == 0 {
		s.Cfg.MaxPassword = defaultMaxPassword
	}
	if s.Hasher == nil {
		s.Hasher = NewHasher(4)
	}
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	s.origins = newOrigins(s.Cfg.BaseURL, s.Cfg.TrustedOrigins...)
	s.jar = cookieJar{secret: []byte(s.Cfg.Secret), secure: s.Cfg.BaseURL.Scheme == "https"}
	return &s
}

// emailPattern is Zod's email regex, which Better Auth validates with, minus its
// two lookaheads (RE2 has none): no leading dot and no "..", checked in
// validEmail.
var emailPattern = regexp.MustCompile(`^[A-Za-z0-9_'+\-\.]*[A-Za-z0-9_+-]@([A-Za-z0-9][A-Za-z0-9\-]*\.)+[A-Za-z]{2,}$`)

func validEmail(address string) bool {
	return !strings.HasPrefix(address, ".") && !strings.Contains(address, "..") && emailPattern.MatchString(address)
}

// passwordLength counts UTF-16 code units, like the JavaScript string length
// the limits were written against.
func passwordLength(p string) int { return len(utf16.Encode([]rune(p))) }

func (s *Service) checkPassword(p string) error {
	switch n := passwordLength(p); {
	case n < s.Cfg.MinPassword:
		return errPasswordTooShort
	case n > s.Cfg.MaxPassword:
		return errPasswordTooLong
	}
	return nil
}

func (s *Service) emailRequired() bool { return !s.Cfg.BypassEmailVerification }

// requestMeta is what a session records about where it was created.
type requestMeta struct {
	IP        string
	UserAgent string
}

// authResult is a successful sign-in or sign-up.
type authResult struct {
	User    User
	Session *Session
	// Remember is false when the user asked not to be remembered.
	Remember bool
}

// SignUpInput is a sign-up request.
type SignUpInput struct {
	Name              string
	Email             string
	Password          string
	Image             *string
	RememberMe        *bool
	AnalyticsOptedOut *bool
	CallbackURL       string
	Meta              requestMeta
}

// SignUp creates an account. With email verification required (the hosted
// default) it never signs the user in and answers an existing address exactly
// like a new one, so the response does not reveal which addresses have accounts.
func (s *Service) SignUp(ctx context.Context, in SignUpInput) (authResult, error) {
	if !validEmail(in.Email) {
		return authResult{}, errInvalidEmail
	}
	if in.Password == "" {
		return authResult{}, errInvalidPassword
	}
	if err := s.checkPassword(in.Password); err != nil {
		return authResult{}, err
	}
	if !s.origins.callbackAllowed(in.CallbackURL) {
		return authResult{}, errInvalidCallback
	}
	email := strings.ToLower(in.Email)
	if s.Cfg.BlockDisposableEmail && isDisposableEmail(email) {
		return authResult{}, errDisposableEmail
	}

	now := s.Now()
	_, exists, err := userByEmail(ctx, s.DB, email)
	if err != nil {
		return authResult{}, err
	}
	if exists {
		return s.duplicateSignUp(ctx, in, email, now)
	}

	hash, err := s.Hasher.Hash(ctx, in.Password)
	if err != nil {
		return authResult{}, err
	}
	userID, err := newID(32)
	if err != nil {
		return authResult{}, err
	}
	accountID, err := newID(32)
	if err != nil {
		return authResult{}, err
	}
	optedOut := false
	if in.AnalyticsOptedOut != nil {
		optedOut = *in.AnalyticsOptedOut
	}
	user := User{ID: userID, Name: in.Name, Email: email, Image: in.Image, CreatedAt: now, UpdatedAt: now, AnalyticsOptedOut: &optedOut}

	var result authResult
	err = pgdb.InTx(ctx, s.DB, func(tx pgx.Tx) error {
		if err := insertUser(ctx, tx, user); err != nil {
			return err
		}
		if err := insertCredentialAccount(ctx, tx, accountID, userID, hash, now); err != nil {
			return err
		}
		// Without email verification the user is signed in right away, in the
		// same transaction, so a half-created account never exists.
		if !s.emailRequired() {
			session, err := s.startSession(ctx, tx, user, in.RememberMe == nil || *in.RememberMe, in.Meta)
			if err != nil {
				return err
			}
			result = authResult{User: user, Session: &session, Remember: in.RememberMe == nil || *in.RememberMe}
		}
		return nil
	})
	if isUniqueViolation(err) {
		// Lost a race to create the same address.
		return s.duplicateSignUp(ctx, in, email, now)
	}
	if err != nil {
		return authResult{}, fmt.Errorf("sign up: %w", err)
	}
	if result.Session == nil {
		result = authResult{User: user}
	}

	s.syncSignupContact(user)
	if s.emailRequired() && s.Mailer != nil {
		if err := s.sendVerification(ctx, user.Email, in.CallbackURL); err != nil {
			return authResult{}, fmt.Errorf("send verification email: %w", err)
		}
	}
	return result, nil
}

// duplicateSignUp answers a sign-up for an existing address.
func (s *Service) duplicateSignUp(ctx context.Context, in SignUpInput, email string, now time.Time) (authResult, error) {
	if !s.emailRequired() {
		return authResult{}, errUserExists
	}
	// Spend the time hashing would take, so the two paths cost the same.
	if _, err := s.Hasher.Hash(ctx, in.Password); err != nil {
		return authResult{}, err
	}
	id, err := newID(32)
	if err != nil {
		return authResult{}, err
	}
	optedOut := false
	return authResult{User: User{ID: id, Name: in.Name, Email: email, Image: in.Image, CreatedAt: now, UpdatedAt: now, AnalyticsOptedOut: &optedOut}}, nil
}

// syncSignupContact adds the user to the marketing audience in the background.
// It must never delay or fail sign-up.
func (s *Service) syncSignupContact(user User) {
	if s.Mailer == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.Mailer.UpsertSignupContact(ctx, user.ID, user.Email, user.Name); err != nil {
			s.Logger.WarnContext(ctx, "sync signup contact failed", "user", user.ID, "err", err)
		}
	}()
}

// startSession creates a session for user in tx, in the workspace they should
// land in.
func (s *Service) startSession(ctx context.Context, tx querier, user User, remember bool, meta requestMeta) (Session, error) {
	now := s.Now()
	org, err := resolveSignInOrganization(ctx, tx, user, now)
	if err != nil {
		return Session{}, err
	}
	id, err := newID(32)
	if err != nil {
		return Session{}, err
	}
	token, err := newID(32)
	if err != nil {
		return Session{}, err
	}
	lifetime := sessionLifetime
	if !remember {
		lifetime = shortSessionLifetime
	}
	session := Session{ID: id, Token: token, UserID: user.ID, ExpiresAt: now.Add(lifetime), CreatedAt: now, UpdatedAt: now}
	if meta.IP != "" {
		session.IPAddress = &meta.IP
	}
	if meta.UserAgent != "" {
		ua := meta.UserAgent
		if len(ua) > 255 {
			ua = ua[:255]
		}
		session.UserAgent = &ua
	}
	if org != nil {
		session.ActiveOrganizationID = &org.ID
	}
	if err := insertSession(ctx, tx, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

// SignInInput is a sign-in request.
type SignInInput struct {
	Email       string
	Password    string
	CallbackURL string
	RememberMe  *bool
	Meta        requestMeta
}

// SignIn checks the credentials and starts a session. Every failure before the
// password check costs one hash, so an unknown address and a wrong password
// take the same time.
func (s *Service) SignIn(ctx context.Context, in SignInInput) (authResult, error) {
	if !validEmail(in.Email) {
		return authResult{}, errInvalidEmail
	}
	if !s.origins.callbackAllowed(in.CallbackURL) {
		return authResult{}, errInvalidCallback
	}
	user, ok, err := userByEmail(ctx, s.DB, strings.ToLower(in.Email))
	if err != nil {
		return authResult{}, err
	}
	var hash string
	if ok {
		hash, ok, err = credentialHash(ctx, s.DB, user.ID)
		if err != nil {
			return authResult{}, err
		}
	}
	if !ok {
		if _, err := s.Hasher.Hash(ctx, in.Password); err != nil {
			return authResult{}, err
		}
		return authResult{}, errBadCredentials
	}
	match, err := s.Hasher.Verify(ctx, hash, in.Password)
	if errors.Is(err, ErrInvalidHash) {
		return authResult{}, errBadCredentials
	}
	if err != nil {
		return authResult{}, err
	}
	if !match {
		return authResult{}, errBadCredentials
	}
	if s.emailRequired() && !user.EmailVerified {
		return authResult{}, errEmailNotVerified
	}

	remember := in.RememberMe == nil || *in.RememberMe
	var session Session
	err = pgdb.InTx(ctx, s.DB, func(tx pgx.Tx) error {
		var err error
		session, err = s.startSession(ctx, tx, user, remember, in.Meta)
		return err
	})
	if err != nil {
		return authResult{}, fmt.Errorf("start session: %w", err)
	}
	return authResult{User: user, Session: &session, Remember: remember}, nil
}

func (s *Service) verificationURL(address, callbackURL string) (string, error) {
	token, err := signJWT([]byte(s.Cfg.Secret), map[string]any{"email": strings.ToLower(address)}, verificationTTL, s.Now())
	if err != nil {
		return "", err
	}
	callback := "/"
	if callbackURL != "" {
		callback = callbackURL
	}
	return s.apiURL("/verify-email") + "?token=" + token + "&callbackURL=" + encodeURIComponent(callback), nil
}

func (s *Service) apiURL(path string) string {
	return strings.TrimRight(s.Cfg.BaseURL.String(), "/") + "/api/auth" + path
}

func (s *Service) sendVerification(ctx context.Context, address, callbackURL string) error {
	link, err := s.verificationURL(address, callbackURL)
	if err != nil {
		return err
	}
	return s.Mailer.SendVerification(ctx, address, link)
}

// SendVerificationEmail emails a fresh verification link. A signed-out caller
// always gets the same answer and takes at least verificationFloor, so the
// endpoint cannot be used to find out which addresses are registered.
func (s *Service) SendVerificationEmail(ctx context.Context, address, callbackURL string, sessionUserID string) error {
	if !s.emailRequired() || s.Mailer == nil {
		return errVerifyDisabled
	}
	if !validEmail(address) {
		return errInvalidEmail
	}
	if !s.origins.callbackAllowed(callbackURL) {
		return errInvalidCallback
	}
	lower := strings.ToLower(address)

	if sessionUserID != "" {
		user, ok, err := userByID(ctx, s.DB, sessionUserID)
		if err != nil {
			return err
		}
		if !ok || strings.ToLower(user.Email) != lower {
			return errEmailMismatch
		}
		if user.EmailVerified {
			return errEmailAlreadyVerfied
		}
		return s.sendVerification(ctx, user.Email, callbackURL)
	}

	started := time.Now()
	user, ok, err := userByEmail(ctx, s.DB, lower)
	if err != nil {
		return err
	}
	var sendErr error
	if ok && !user.EmailVerified {
		sendErr = s.sendVerification(ctx, user.Email, callbackURL)
	}
	if wait := verificationFloor - time.Since(started); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return sendErr
}

// verifyOutcome says where a verify-email request ends up.
type verifyOutcome struct {
	// ErrorCode is set when the link was bad; the handler redirects with it.
	ErrorCode string
	// Session is set when the user was signed in by the link.
	Session  *Session
	User     User
	Remember bool
}

// VerifyEmail checks a verification link, marks the address verified, and signs
// the user in unless the request already carries their session.
func (s *Service) VerifyEmail(ctx context.Context, token, callbackURL string, sessionUserID string, meta requestMeta) (verifyOutcome, error) {
	if !s.origins.callbackAllowed(callbackURL) {
		return verifyOutcome{}, errInvalidCallback
	}
	claims, err := verifyJWT([]byte(s.Cfg.Secret), token, s.Now())
	switch {
	case errors.Is(err, ErrTokenExpired):
		return verifyOutcome{ErrorCode: "TOKEN_EXPIRED"}, nil
	case err != nil:
		return verifyOutcome{ErrorCode: "INVALID_TOKEN"}, nil
	}
	address, _ := claims["email"].(string)
	// This server never issues change-email tokens; one that carries a target
	// address was not made here.
	if _, isChange := claims["updateTo"]; isChange || !validEmail(address) {
		return verifyOutcome{ErrorCode: "INVALID_TOKEN"}, nil
	}
	user, ok, err := userByEmail(ctx, s.DB, strings.ToLower(address))
	if err != nil {
		return verifyOutcome{}, err
	}
	if !ok {
		return verifyOutcome{ErrorCode: "USER_NOT_FOUND"}, nil
	}
	if user.EmailVerified {
		return verifyOutcome{User: user}, nil
	}

	user.EmailVerified = true
	outcome := verifyOutcome{User: user, Remember: true}
	err = pgdb.InTx(ctx, s.DB, func(tx pgx.Tx) error {
		if err := markEmailVerified(ctx, tx, user.Email, s.Now()); err != nil {
			return err
		}
		if sessionUserID == user.ID {
			return nil
		}
		session, err := s.startSession(ctx, tx, user, true, meta)
		if err != nil {
			return err
		}
		outcome.Session = &session
		return nil
	})
	if err != nil {
		return verifyOutcome{}, fmt.Errorf("verify email: %w", err)
	}
	return outcome, nil
}

// RequestPasswordReset emails a reset link. The answer is the same whether or
// not the address has an account.
func (s *Service) RequestPasswordReset(ctx context.Context, address, redirectTo string) error {
	if s.Mailer == nil {
		return errResetDisabled
	}
	if !validEmail(address) {
		return errInvalidEmail
	}
	if !s.origins.callbackAllowed(redirectTo) {
		return errInvalidCallback
	}
	user, ok, err := userByEmail(ctx, s.DB, strings.ToLower(address))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	token, err := newID(24)
	if err != nil {
		return err
	}
	id, err := newID(32)
	if err != nil {
		return err
	}
	now := s.Now()
	if err := insertVerification(ctx, s.DB, id, resetIdentifierPrefix+token, user.ID, now.Add(resetTTL), now); err != nil {
		return err
	}
	link := s.apiURL("/reset-password/"+token) + "?callbackURL=" + encodeURIComponent(redirectTo)
	if err := s.Mailer.SendPasswordReset(ctx, user.Email, link); err != nil {
		// Reporting this would tell the caller the address exists.
		s.Logger.ErrorContext(ctx, "send password reset email failed", "user", user.ID, "err", err)
	}
	return nil
}

// ResetLinkValid reports whether a reset token can still be used, without using it.
func (s *Service) ResetLinkValid(ctx context.Context, token string) (bool, error) {
	_, ok, err := peekVerification(ctx, s.DB, resetIdentifierPrefix+token, s.Now())
	return ok, err
}

// ResetPassword sets a new password with a reset token, which it uses up, and
// signs the user out everywhere.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if token == "" {
		return errInvalidToken
	}
	if err := s.checkPassword(newPassword); err != nil {
		return err
	}
	userID, ok, err := consumeVerification(ctx, s.DB, resetIdentifierPrefix+token, s.Now())
	if err != nil {
		return err
	}
	if !ok {
		return errInvalidToken
	}
	hash, err := s.Hasher.Hash(ctx, newPassword)
	if err != nil {
		return err
	}
	accountID, err := newID(32)
	if err != nil {
		return err
	}
	return pgdb.InTx(ctx, s.DB, func(tx pgx.Tx) error {
		if err := setPassword(ctx, tx, accountID, userID, hash, s.Now()); err != nil {
			return err
		}
		return deleteUserSessions(ctx, tx, userID)
	})
}

// UpdateProfile changes the signed-in user's own profile fields.
func (s *Service) UpdateProfile(ctx context.Context, userID string, name, image *string, analyticsOptedOut *bool) error {
	if name == nil && image == nil && analyticsOptedOut == nil {
		return errNothingToUpdate
	}
	return updateProfile(ctx, s.DB, userID, name, image, analyticsOptedOut, s.Now())
}
