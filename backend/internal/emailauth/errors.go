package emailauth

import (
	"fmt"
	"net/http"
)

// apiError is a failure the client sees. Its JSON body is {code, message}, the
// shape Better Auth answers with and the React client reads.
type apiError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// The messages are Better Auth's own, so a client that shows or matches them
// behaves the same against either server.
var (
	errInvalidEmail        = &apiError{http.StatusBadRequest, "INVALID_EMAIL", "Invalid email"}
	errInvalidPassword     = &apiError{http.StatusBadRequest, "INVALID_PASSWORD", "Invalid password"}
	errPasswordTooShort    = &apiError{http.StatusBadRequest, "PASSWORD_TOO_SHORT", "Password too short"}
	errPasswordTooLong     = &apiError{http.StatusBadRequest, "PASSWORD_TOO_LONG", "Password too long"}
	errUserExists          = &apiError{http.StatusUnprocessableEntity, "USER_ALREADY_EXISTS_USE_ANOTHER_EMAIL", "User already exists. Use another email."}
	errFailedCreateUser    = &apiError{http.StatusUnprocessableEntity, "FAILED_TO_CREATE_USER", "Failed to create user"}
	errBadCredentials      = &apiError{http.StatusUnauthorized, "INVALID_EMAIL_OR_PASSWORD", "Invalid email or password"}
	errEmailNotVerified    = &apiError{http.StatusForbidden, "EMAIL_NOT_VERIFIED", "Email not verified"}
	errInvalidToken        = &apiError{http.StatusBadRequest, "INVALID_TOKEN", "Invalid token"}
	errVerifyDisabled      = &apiError{http.StatusBadRequest, "VERIFICATION_EMAIL_NOT_ENABLED", "Verification email isn't enabled"}
	errEmailMismatch       = &apiError{http.StatusBadRequest, "EMAIL_MISMATCH", "Email mismatch"}
	errEmailAlreadyVerfied = &apiError{http.StatusBadRequest, "EMAIL_ALREADY_VERIFIED", "Email is already verified"}
	errResetDisabled       = &apiError{http.StatusBadRequest, "RESET_PASSWORD_DISABLED", "Reset password isn't enabled"}
	errEmailImmutable      = &apiError{http.StatusBadRequest, "EMAIL_CAN_NOT_BE_UPDATED", "Email can not be updated"}
	errNothingToUpdate     = &apiError{http.StatusBadRequest, "BAD_REQUEST", "No fields to update"}
	errUnauthorized        = &apiError{http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized"}
	errDisposableEmail     = &apiError{http.StatusBadRequest, "BAD_REQUEST", "Please sign up with a non-disposable email address."}
	errInvalidCallback     = &apiError{http.StatusForbidden, "INVALID_CALLBACKURL", "Invalid callbackURL"}
	errInvalidOrigin       = &apiError{http.StatusForbidden, "INVALID_ORIGIN", "Invalid origin"}
	errMalformedBody       = &apiError{http.StatusBadRequest, "BAD_REQUEST", "Invalid request body"}
	errTooManyRequests     = &apiError{http.StatusTooManyRequests, "TOO_MANY_REQUESTS", "Too many requests. Please try again later."}
	errInternal            = &apiError{http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Internal server error"}

	errCaptchaMissing = &apiError{http.StatusBadRequest, "MISSING_RESPONSE", "Missing CAPTCHA response"}
	errCaptchaFailed  = &apiError{http.StatusForbidden, "VERIFICATION_FAILED", "Captcha verification failed"}
	errCaptchaUnknown = &apiError{http.StatusInternalServerError, "UNKNOWN_ERROR", "Something went wrong"}
)
