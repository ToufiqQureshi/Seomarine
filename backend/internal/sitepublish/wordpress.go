package sitepublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

const (
	maxJSONResponse = 2 << 20
	// MaxImageBytes is the largest image copied into a site's media library.
	MaxImageBytes = 5 << 20
	maxRedirects  = 3
	plainRESTBase = "/?rest_route="
)

// WordPress talks to a WordPress site through its REST API, signed in with an
// application password (built into WordPress since 5.6).
type WordPress struct {
	// HTTP must refuse private addresses and must not follow redirects:
	// audit.Guard's client in production. Redirects are followed here, only
	// for requests that carry no password.
	HTTP *http.Client
}

// Credentials signs requests to one site.
type Credentials struct {
	// APIBase is either "https://site/wp-json" or "https://site/?rest_route=".
	APIBase  string
	Username string
	Password string
}

// SiteInfo is what discovery learns about a site.
type SiteInfo struct {
	// SiteURL is the address the site answered on, after redirects.
	SiteURL string
	APIBase string
	Name    string
}

// RemoteError is an error answer from the site.
type RemoteError struct {
	Status  int
	Code    string
	Message string
}

func (e *RemoteError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("site answered %d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("site answered %d", e.Status)
}

// blockedStatus reports whether status is what firewalls and bot checks answer.
func blockedStatus(status int) bool {
	return status == http.StatusForbidden || status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

// Discover finds the REST API of the WordPress site at siteURL.
func (w WordPress) Discover(ctx context.Context, siteURL string) (SiteInfo, error) {
	base := strings.TrimRight(siteURL, "/")
	var lastErr error
	for _, candidate := range []string{base + "/wp-json/", base + plainRESTBase + "/"} {
		var index struct {
			Name           string          `json:"name"`
			URL            string          `json:"url"`
			Namespaces     []string        `json:"namespaces"`
			Authentication json.RawMessage `json:"authentication"`
		}
		final, err := w.getJSON(ctx, candidate, &index)
		if err != nil {
			lastErr = err
			continue
		}
		if !slices.Contains(index.Namespaces, "wp/v2") {
			lastErr = ErrNotWordPress
			continue
		}
		if !bytes.Contains(index.Authentication, []byte(`"application-passwords"`)) {
			return SiteInfo{}, ErrAppPasswordsOff
		}
		// ".../wp-json/" or ".../?rest_route=/", without the last slash.
		apiBase := strings.TrimSuffix(final, "/")
		parsed, err := url.Parse(final)
		if err != nil {
			return SiteInfo{}, fmt.Errorf("parse discovered api url: %w", err)
		}
		return SiteInfo{SiteURL: parsed.Scheme + "://" + parsed.Host, APIBase: apiBase, Name: strings.TrimSpace(index.Name)}, nil
	}
	// A firewall or bot check in front of the site (Hostinger hCDN,
	// Cloudflare, Wordfence) answers 403/429/503 to servers. The customer can
	// fix that, so it must not read as "not WordPress".
	if remote, ok := errors.AsType[*RemoteError](lastErr); ok && blockedStatus(remote.Status) {
		return SiteInfo{}, fmt.Errorf("%w: %w", ErrSiteBlocked, lastErr)
	}
	// The site answered, just not as WordPress (a 404, an HTML page...).
	if _, ok := errors.AsType[*RemoteError](lastErr); ok || errors.Is(lastErr, ErrNotWordPress) {
		return SiteInfo{}, fmt.Errorf("%w: %w", ErrNotWordPress, lastErr)
	}
	return SiteInfo{}, fmt.Errorf("%w: %w", ErrSiteUnreachable, lastErr)
}

// Verify signs in and checks the user may publish posts and upload images.
// It returns the user's display name.
func (w WordPress) Verify(ctx context.Context, creds Credentials) (string, error) {
	name, _, err := w.verify(ctx, creds)
	return name, err
}

// verify signs in and reads the full edit-context profile.
func (w WordPress) verify(ctx context.Context, creds Credentials) (string, map[string]bool, error) {
	var me struct {
		Name         string          `json:"name"`
		Capabilities map[string]bool `json:"capabilities"`
	}
	err := w.call(ctx, creds, http.MethodGet, "/wp/v2/users/me", url.Values{"context": {"edit"}}, nil, "", nil, &me)
	if remote, ok := errors.AsType[*RemoteError](err); ok {
		switch {
		// WordPress itself answers in JSON with a code; a firewall does not.
		case remote.Code == "" && blockedStatus(remote.Status):
			return "", nil, fmt.Errorf("%w: %w", ErrSiteBlocked, err)
		// WordPress answers rest_not_logged_in only when no credentials
		// reached it; a wrong password gets incorrect_password or
		// invalid_username. So the hosting dropped the Authorization header.
		case remote.Code == "rest_not_logged_in":
			return "", nil, ErrAuthHeaderDropped
		case remote.Status == http.StatusUnauthorized || remote.Status == http.StatusForbidden:
			return "", nil, ErrBadCredentials
		}
	}
	if err != nil {
		return "", nil, err
	}
	if !me.Capabilities["publish_posts"] || !me.Capabilities["upload_files"] {
		return "", nil, ErrMissingPermission
	}
	return me.Name, me.Capabilities, nil
}

// ViewRawHTML reports whether this WordPress user may keep raw script and
// style tags in post content. WordPress strips <script> from post content
// without the unfiltered_html capability (roles above editor have it;
// multisite keeps it away from administrators), so structured data can only
// ride in the post content when it is present.
func (w WordPress) ViewRawHTML(ctx context.Context, creds Credentials) (bool, error) {
	_, capabilities, err := w.verify(ctx, creds)
	if err != nil {
		return false, err
	}
	return capabilities["unfiltered_html"], nil
}

// Media is an image in the site's media library.
type Media struct {
	ID  int64  `json:"id"`
	URL string `json:"url"`
}

// UploadImage adds an image to the site's media library.
func (w WordPress) UploadImage(ctx context.Context, creds Credentials, filename, contentType string, data []byte) (Media, error) {
	var created struct {
		ID        int64  `json:"id"`
		SourceURL string `json:"source_url"`
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	if err := w.call(ctx, creds, http.MethodPost, "/wp/v2/media", nil, data, contentType, map[string]string{"Content-Disposition": disposition}, &created); err != nil {
		return Media{}, fmt.Errorf("upload %s: %w", filename, err)
	}
	if created.ID == 0 || created.SourceURL == "" {
		return Media{}, fmt.Errorf("upload %s: site returned no media id", filename)
	}
	return Media{ID: created.ID, URL: created.SourceURL}, nil
}

// Post is what is sent as a WordPress post.
type Post struct {
	Title         string `json:"title"`
	Content       string `json:"content"`
	Excerpt       string `json:"excerpt"`
	Slug          string `json:"slug,omitempty"`
	Status        string `json:"status"`
	FeaturedMedia int64  `json:"featured_media,omitempty"`
}

// Saved is a post as the site stored it.
type Saved struct {
	ID     string
	Link   string
	Status string
	// DateGMT is the post's publish time (date_gmt), empty when the site
	// does not return one.
	DateGMT string
}

// SavePost creates a post, or updates the post remoteID when it is set. It
// returns ErrRemoteMissing when that post no longer exists.
func (w WordPress) SavePost(ctx context.Context, creds Credentials, remoteID string, post Post) (Saved, error) {
	route := "/wp/v2/posts"
	if remoteID != "" {
		route += "/" + url.PathEscape(remoteID)
	}
	body, err := json.Marshal(post)
	if err != nil {
		return Saved{}, fmt.Errorf("encode post: %w", err)
	}
	var saved struct {
		ID      int64  `json:"id"`
		Link    string `json:"link"`
		Status  string `json:"status"`
		DateGMT string `json:"date_gmt"`
	}
	err = w.call(ctx, creds, http.MethodPost, route, nil, body, "application/json", nil, &saved)
	var remote *RemoteError
	if remoteID != "" && errors.As(err, &remote) && (remote.Status == http.StatusNotFound || remote.Code == "rest_post_invalid_id") {
		return Saved{}, ErrRemoteMissing
	}
	if err != nil {
		return Saved{}, err
	}
	if saved.ID == 0 {
		return Saved{}, errors.New("site returned no post id")
	}
	return Saved{ID: strconv.FormatInt(saved.ID, 10), Link: saved.Link, Status: saved.Status, DateGMT: saved.DateGMT}, nil
}

// FetchImage downloads a public image so it can be copied to the site.
func (w WordPress) FetchImage(ctx context.Context, rawURL string) (data []byte, contentType string, err error) {
	response, _, err := w.get(ctx, rawURL, "image/*")
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("image answered %d", response.StatusCode)
	}
	contentType, _, _ = mime.ParseMediaType(response.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "image/") || contentType == "image/svg+xml" {
		return nil, "", fmt.Errorf("not a PNG, JPEG, GIF or WebP image (%q)", contentType)
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, MaxImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read image: %w", err)
	}
	if len(data) > MaxImageBytes {
		return nil, "", fmt.Errorf("image is larger than %d MB", MaxImageBytes>>20)
	}
	return data, contentType, nil
}

// getJSON fetches an unauthenticated JSON document, following redirects to
// https addresses. It returns the final URL.
func (w WordPress) getJSON(ctx context.Context, rawURL string, dst any) (string, error) {
	response, final, err := w.get(ctx, rawURL, "application/json")
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return "", &RemoteError{Status: response.StatusCode}
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxJSONResponse)).Decode(dst); err != nil {
		return "", ErrNotWordPress
	}
	return final, nil
}

// get sends a GET without credentials, following up to maxRedirects
// redirects that stay on https.
func (w WordPress) get(ctx context.Context, rawURL, accept string) (*http.Response, string, error) {
	current := rawURL
	for range maxRedirects + 1 {
		if err := requireHTTPS(current); err != nil {
			return nil, "", err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			return nil, "", fmt.Errorf("build request: %w", err)
		}
		request.Header.Set("Accept", accept)
		request.Header.Set("User-Agent", userAgent)
		response, err := w.HTTP.Do(request)
		if err != nil {
			return nil, "", err
		}
		if response.StatusCode < 300 || response.StatusCode > 399 {
			return response, response.Request.URL.String(), nil
		}
		location, err := response.Location()
		_ = response.Body.Close()
		if err != nil {
			return nil, "", fmt.Errorf("redirect without a location: %w", err)
		}
		current = location.String()
	}
	return nil, "", errors.New("too many redirects")
}

// call sends an authenticated request to route. It never follows a redirect,
// so the password only goes to the address that was verified.
func (w WordPress) call(ctx context.Context, creds Credentials, method, route string, query url.Values, body []byte, contentType string, headers map[string]string, dst any) error {
	target := endpoint(creds.APIBase, route, query)
	if err := requireHTTPS(target); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	request.SetBasicAuth(creds.Username, creds.Password)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := w.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSiteUnreachable, err)
	}
	defer func() { _ = response.Body.Close() }()
	reader := io.LimitReader(response.Body, maxJSONResponse)
	if response.StatusCode >= 300 {
		remote := &RemoteError{Status: response.StatusCode}
		var payload struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.NewDecoder(reader).Decode(&payload) == nil {
			remote.Code, remote.Message = payload.Code, payload.Message
		}
		return remote
	}
	if err := json.NewDecoder(reader).Decode(dst); err != nil {
		return fmt.Errorf("read site answer: %w", err)
	}
	return nil
}

// endpoint joins a REST route to an API base of either form.
func endpoint(apiBase, route string, query url.Values) string {
	target := apiBase + route
	if len(query) == 0 {
		return target
	}
	if strings.Contains(apiBase, "?") {
		return target + "&" + query.Encode()
	}
	return target + "?" + query.Encode()
}

func requireHTTPS(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return ErrNotHTTPS
	}
	return nil
}
