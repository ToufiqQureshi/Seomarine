package sitepublish

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const testUser = "editor"

// testPassword is 24 characters, like a WordPress application password, and
// built at run time so it does not look like a committed secret.
var testPassword = strings.Repeat("fake", 6)

// fakeWordPress is an in-process WordPress REST API.
type fakeWordPress struct {
	server *httptest.Server

	mu           sync.Mutex
	plain        bool // only ?rest_route= works, as with plain permalinks
	appPasswords bool
	capabilities map[string]bool
	posts        map[int64]Post
	media        []string // filenames uploaded, in order
	nextID       int64
	requests     []string // "METHOD route", in order
}

func newFakeWordPress(t *testing.T) *fakeWordPress {
	t.Helper()
	wp := &fakeWordPress{
		appPasswords: true,
		capabilities: map[string]bool{"publish_posts": true, "upload_files": true},
		posts:        map[int64]Post{},
		nextID:       100,
	}
	wp.server = httptest.NewTLSServer(http.HandlerFunc(wp.serve))
	t.Cleanup(wp.server.Close)
	return wp
}

// client mirrors production: audit.Guard's client never follows redirects.
func (wp *fakeWordPress) client() WordPress {
	client := wp.server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return WordPress{HTTP: client}
}

func (wp *fakeWordPress) serve(w http.ResponseWriter, r *http.Request) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	var route string
	switch {
	case strings.HasPrefix(r.URL.Path, "/wp-json") && !wp.plain:
		route = strings.TrimPrefix(r.URL.Path, "/wp-json")
	case r.URL.Path == "/" && r.URL.Query().Has("rest_route"):
		route = r.URL.Query().Get("rest_route")
	default:
		http.NotFound(w, r)
		return
	}
	wp.requests = append(wp.requests, r.Method+" "+route)
	if route == "/" {
		index := map[string]any{"name": "Test Blog", "namespaces": []string{"oembed/1.0", "wp/v2"}}
		if wp.appPasswords {
			index["authentication"] = map[string]any{"application-passwords": map[string]any{}}
		}
		writeJSON(w, http.StatusOK, index)
		return
	}
	user, password, ok := r.BasicAuth()
	if !ok || user != testUser || password != testPassword {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "rest_not_logged_in", "message": "You are not currently logged in."})
		return
	}
	switch {
	case route == "/wp/v2/users/me" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"name": "Editor", "capabilities": wp.capabilities})
	case route == "/wp/v2/media" && r.Method == http.MethodPost:
		_, _ = io.Copy(io.Discard, r.Body)
		name := r.Header.Get("Content-Disposition")
		wp.media = append(wp.media, name)
		wp.nextID++
		writeJSON(w, http.StatusCreated, map[string]any{"id": wp.nextID, "source_url": wp.server.URL + "/uploads/" + strconv.FormatInt(wp.nextID, 10)})
	case route == "/wp/v2/posts" && r.Method == http.MethodPost:
		post, ok := decodePost(w, r)
		if !ok {
			return
		}
		wp.nextID++
		wp.posts[wp.nextID] = post
		writeJSON(w, http.StatusCreated, map[string]any{"id": wp.nextID, "link": wp.server.URL + "/?p=" + strconv.FormatInt(wp.nextID, 10), "status": post.Status})
	case strings.HasPrefix(route, "/wp/v2/posts/") && r.Method == http.MethodPost:
		id, _ := strconv.ParseInt(strings.TrimPrefix(route, "/wp/v2/posts/"), 10, 64)
		if _, exists := wp.posts[id]; !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"code": "rest_post_invalid_id", "message": "Invalid post ID."})
			return
		}
		post, ok := decodePost(w, r)
		if !ok {
			return
		}
		wp.posts[id] = post
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "link": wp.server.URL + "/?p=" + strconv.FormatInt(id, 10), "status": post.Status})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"code": "rest_no_route", "message": "No route was found."})
	}
}

func decodePost(w http.ResponseWriter, r *http.Request) (Post, bool) {
	var post Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "rest_invalid_json"})
		return Post{}, false
	}
	return post, true
}

func (wp *fakeWordPress) snapshot() (posts map[int64]Post, media []string, requests []string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	posts = map[int64]Post{}
	for id, post := range wp.posts {
		posts[id] = post
	}
	return posts, append([]string(nil), wp.media...), append([]string(nil), wp.requests...)
}

func (wp *fakeWordPress) deletePosts() {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.posts = map[int64]Post{}
}

// newImageHost serves images on another origin than the WordPress site.
func newImageHost(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.png", "/b.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG fake"))
		case "/big.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(make([]byte, MaxImageBytes+1))
		case "/logo.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte("<svg/>"))
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
