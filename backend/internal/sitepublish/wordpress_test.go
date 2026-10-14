package sitepublish

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoverFindsTheRESTAPI(t *testing.T) {
	for _, plain := range []bool{false, true} {
		wp := newFakeWordPress(t)
		wp.plain = plain
		site, err := wp.client().Discover(t.Context(), wp.server.URL)
		if err != nil {
			t.Fatalf("plain=%v: Discover: %v", plain, err)
		}
		want := wp.server.URL + "/wp-json"
		if plain {
			want = wp.server.URL + "/?rest_route="
		}
		if site.APIBase != want || site.SiteURL != wp.server.URL || site.Name != "Test Blog" {
			t.Errorf("plain=%v: Discover = %+v, want api base %s", plain, site, want)
		}
		// The discovered base must work for signed-in calls too.
		if _, err := wp.client().Verify(t.Context(), Credentials{APIBase: site.APIBase, Username: testUser, Password: testPassword}); err != nil {
			t.Errorf("plain=%v: Verify through discovered base: %v", plain, err)
		}
	}
}

func TestDiscoverFollowsARedirectToTheRealSite(t *testing.T) {
	wp := newFakeWordPress(t)
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, wp.server.URL+"/wp-json/", http.StatusMovedPermanently)
	}))
	defer redirect.Close()
	site, err := wp.client().Discover(t.Context(), redirect.URL)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if site.SiteURL != wp.server.URL || site.APIBase != wp.server.URL+"/wp-json" {
		t.Errorf("Discover = %+v, want the site the redirect led to", site)
	}
}

func TestDiscoverRefusesWhatIsNotUsableWordPress(t *testing.T) {
	notWordPress := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/wp-json/" {
			_, _ = w.Write([]byte("<html>a shop</html>"))
			return
		}
		http.NotFound(w, r)
	}))
	defer notWordPress.Close()
	if _, err := (WordPress{HTTP: notWordPress.Client()}).Discover(t.Context(), notWordPress.URL); !errors.Is(err, ErrNotWordPress) {
		t.Errorf("HTML site: err = %v, want ErrNotWordPress", err)
	}

	wp := newFakeWordPress(t)
	wp.appPasswords = false
	if _, err := wp.client().Discover(t.Context(), wp.server.URL); !errors.Is(err, ErrAppPasswordsOff) {
		t.Errorf("application passwords off: err = %v, want ErrAppPasswordsOff", err)
	}

	if _, err := wp.client().Discover(t.Context(), strings.Replace(wp.server.URL, "https://", "http://", 1)); !errors.Is(err, ErrSiteUnreachable) || !errors.Is(err, ErrNotHTTPS) {
		t.Errorf("http address: err = %v, want ErrNotHTTPS", err)
	}

	closed := httptest.NewTLSServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	if _, err := wp.client().Discover(t.Context(), closedURL); !errors.Is(err, ErrSiteUnreachable) {
		t.Errorf("site down: err = %v, want ErrSiteUnreachable", err)
	}
}

func TestVerifyChecksPasswordAndPermissions(t *testing.T) {
	wp := newFakeWordPress(t)
	base := wp.server.URL + "/wp-json"
	if _, err := wp.client().Verify(t.Context(), Credentials{APIBase: base, Username: testUser, Password: "wrong"}); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("wrong password: err = %v, want ErrBadCredentials", err)
	}
	for _, missing := range []string{"publish_posts", "upload_files"} {
		wp.capabilities = map[string]bool{"publish_posts": true, "upload_files": true}
		wp.capabilities[missing] = false
		if _, err := wp.client().Verify(t.Context(), Credentials{APIBase: base, Username: testUser, Password: testPassword}); !errors.Is(err, ErrMissingPermission) {
			t.Errorf("without %s: err = %v, want ErrMissingPermission", missing, err)
		}
	}
}

func TestSavePostReportsAPostDeletedOnTheSite(t *testing.T) {
	wp := newFakeWordPress(t)
	creds := Credentials{APIBase: wp.server.URL + "/wp-json", Username: testUser, Password: testPassword}
	if _, err := wp.client().SavePost(t.Context(), creds, "999", Post{Title: "x", Status: StatusDraft}); !errors.Is(err, ErrRemoteMissing) {
		t.Errorf("update of a missing post: err = %v, want ErrRemoteMissing", err)
	}
	if _, err := wp.client().SavePost(t.Context(), Credentials{APIBase: creds.APIBase, Username: testUser, Password: "wrong"}, "", Post{Title: "x", Status: StatusDraft}); err == nil {
		t.Error("create with a wrong password succeeded")
	}
}

func TestFetchImageAcceptsOnlySmallRasterImages(t *testing.T) {
	images := newImageHost(t)
	client := WordPress{HTTP: images.Client()}
	if data, contentType, err := client.FetchImage(t.Context(), images.URL+"/a.png"); err != nil || contentType != "image/png" || len(data) == 0 {
		t.Errorf("png: got %d bytes %q, err %v", len(data), contentType, err)
	}
	for _, path := range []string{"/big.png", "/logo.svg", "/page", "/missing.png"} {
		if _, _, err := client.FetchImage(t.Context(), images.URL+path); err == nil {
			t.Errorf("%s: FetchImage succeeded, want an error", path)
		}
	}
	if _, _, err := client.FetchImage(t.Context(), "http://example.com/a.png"); !errors.Is(err, ErrNotHTTPS) {
		t.Errorf("http image: err = %v, want ErrNotHTTPS", err)
	}
}

func TestSignedInCallsNeverFollowARedirect(t *testing.T) {
	var leaked bool
	elsewhere := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); ok {
			leaked = true
		}
	}))
	defer elsewhere.Close()
	redirecting := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer redirecting.Close()
	// The production client refuses redirects; mirror that here.
	client := redirecting.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	_, err := WordPress{HTTP: client}.Verify(t.Context(), Credentials{APIBase: redirecting.URL + "/wp-json", Username: testUser, Password: testPassword})
	if err == nil || leaked {
		t.Errorf("Verify through a redirect: err = %v, password sent elsewhere = %v", err, leaked)
	}
}

// A firewall in front of the site (Hostinger hCDN, Cloudflare) answers
// servers with an HTML 403/429/503. That is something the customer can fix,
// so it must not be reported as "not WordPress" or as a wrong password.
func TestFirewallBlocksAreToldApartFromNotWordPress(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		firewall := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("<html>Checking your browser</html>"))
		}))
		wp := WordPress{HTTP: firewall.Client()}
		_, err := wp.Discover(t.Context(), firewall.URL)
		if !errors.Is(err, ErrSiteBlocked) || errors.Is(err, ErrNotWordPress) {
			t.Errorf("discover behind %d: err = %v, want ErrSiteBlocked", status, err)
		}
		_, err = wp.Verify(t.Context(), Credentials{APIBase: firewall.URL + "/wp-json", Username: testUser, Password: testPassword})
		if !errors.Is(err, ErrSiteBlocked) || errors.Is(err, ErrBadCredentials) {
			t.Errorf("verify behind %d: err = %v, want ErrSiteBlocked", status, err)
		}
		firewall.Close()
	}

	missing := httptest.NewTLSServer(http.NotFoundHandler())
	defer missing.Close()
	if _, err := (WordPress{HTTP: missing.Client()}).Discover(t.Context(), missing.URL); !errors.Is(err, ErrNotWordPress) || errors.Is(err, ErrSiteBlocked) {
		t.Errorf("404 everywhere: err = %v, want ErrNotWordPress", err)
	}

	// WordPress's own 403 carries a JSON code: that is a permission answer.
	forbidden := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusForbidden, map[string]any{"code": "rest_forbidden", "message": "Sorry, you are not allowed to do that."})
	}))
	defer forbidden.Close()
	if _, err := (WordPress{HTTP: forbidden.Client()}).Verify(t.Context(), Credentials{APIBase: forbidden.URL + "/wp-json", Username: testUser, Password: testPassword}); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("WordPress 403: err = %v, want ErrBadCredentials", err)
	}
}

// Hostinger and other LiteSpeed/CGI hosts drop the Authorization header, and
// WordPress then answers as if no login was sent. The customer must be told
// to fix the host, not to retype a password that was never checked.
func TestVerifyTellsADroppedLoginHeaderFromAWrongPassword(t *testing.T) {
	wp := newFakeWordPress(t)
	base := wp.server.URL + "/wp-json"
	if _, err := wp.client().Verify(t.Context(), Credentials{APIBase: base, Username: testUser, Password: "wrong"}); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("wrong password: err = %v, want ErrBadCredentials", err)
	}
	wp.stripsAuth = true
	_, err := wp.client().Verify(t.Context(), Credentials{APIBase: base, Username: testUser, Password: testPassword})
	if !errors.Is(err, ErrAuthHeaderDropped) || errors.Is(err, ErrBadCredentials) {
		t.Errorf("header dropped by the host: err = %v, want ErrAuthHeaderDropped", err)
	}
}
