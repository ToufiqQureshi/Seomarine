package httpapi

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
)

// spaHandler serves the built React app from dir. A path that names a file
// is served as that file; any other path is a client-side route and gets
// index.html. Missing files that look like assets stay 404 so a stale chunk
// reference fails loudly instead of loading HTML as JavaScript.
func spaHandler(dir string) http.Handler {
	files := os.DirFS(dir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			notFound()(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if info, err := fs.Stat(files, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				// Vite hashes asset names, so they never change under a name.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else if name == "index.html" {
				setIndexHeaders(w)
			}
			http.ServeFileFS(w, r, files, name)
			return
		}
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(files, "index.html"); err != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "frontend_unavailable", "The app is not built on this server.")
			return
		}
		setIndexHeaders(w)
		http.ServeFileFS(w, r, files, "index.html")
	})
}

// setIndexHeaders keeps the entry document fresh and unframeable: the app
// ships no other security headers, so a third-party page could otherwise
// frame it and trick a signed-in member into a one-click action.
func setIndexHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
}
