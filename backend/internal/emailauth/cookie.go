package emailauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	sessionCookieName      = "better-auth.session_token"
	sessionDataCookieName  = "better-auth.session_data"
	dontRememberCookieName = "better-auth.dont_remember"
	securePrefix           = "__Secure-"

	// sessionLifetime and shortSessionLifetime are Better Auth's defaults: seven
	// days, or one day when the user asked not to be remembered.
	sessionLifetime      = 7 * 24 * time.Hour
	shortSessionLifetime = 24 * time.Hour
)

// cookieJar issues and clears the auth cookies for one deployment. Cookie names
// carry the __Secure- prefix exactly when the public URL is https, because that
// is how Better Auth names them and how the rest of the Go API reads them.
type cookieJar struct {
	secret []byte
	secure bool
}

func (j cookieJar) name(base string) string {
	if j.secure {
		return securePrefix + base
	}
	return base
}

// encodeURIComponent escapes like the JavaScript function of the same name,
// which is how the signed value is encoded on the wire.
func encodeURIComponent(s string) string {
	const unreserved = "-_.!~*'()"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', strings.IndexByte(unreserved, c) >= 0:
			b.WriteByte(c)
		default:
			const hex = "0123456789ABCDEF"
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

// sign returns value.signature, URL-encoded, the format better-call uses.
func (j cookieJar) sign(value string) string {
	mac := hmac.New(sha256.New, j.secret)
	mac.Write([]byte(value))
	return encodeURIComponent(value + "." + base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

// setSession writes the signed session token. A session the user did not want
// remembered gets a browser-session cookie and the dont_remember marker.
func (j cookieJar) setSession(w http.ResponseWriter, token string, remember bool) {
	cookie := &http.Cookie{
		Name: j.name(sessionCookieName), Value: j.sign(token), Path: "/",
		HttpOnly: true, Secure: j.secure, SameSite: http.SameSiteLaxMode,
	}
	if remember {
		cookie.MaxAge = int(sessionLifetime.Seconds())
	}
	http.SetCookie(w, cookie)
	if !remember {
		http.SetCookie(w, &http.Cookie{
			Name: j.name(dontRememberCookieName), Value: j.sign("true"), Path: "/",
			HttpOnly: true, Secure: j.secure, SameSite: http.SameSiteLaxMode,
		})
	}
	j.clearSessionData(w)
}

// clearSessionData expires Better Auth's cached session snapshot. The Go API
// never reads it, but the legacy app does, and a snapshot of the previous user
// must not outlive a new sign-in.
func (j cookieJar) clearSessionData(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: j.name(sessionDataCookieName), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: j.secure, SameSite: http.SameSiteLaxMode,
	})
}

// verify checks a value signed by sign and returns it. The comparison is
// constant time.
func (j cookieJar) verify(raw string) (string, bool) {
	value, err := url.PathUnescape(raw)
	if err != nil {
		return "", false
	}
	dot := strings.LastIndexByte(value, '.')
	if dot < 1 {
		return "", false
	}
	payload, signature := value[:dot], value[dot+1:]
	mac := hmac.New(sha256.New, j.secret)
	mac.Write([]byte(payload))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(want)) {
		return "", false
	}
	return payload, true
}
