package httpapi

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"net/http"
	"time"
)

//go:embed tracker.js
var trackerJS []byte

func serveTracker() http.HandlerFunc {
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(trackerJS))
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, "t.js", time.Time{}, bytes.NewReader(trackerJS))
	}
}
