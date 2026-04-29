package api

import (
	"net/http"
	"path/filepath"
	"strings"
)

// spaHandler serves the V2 console SPA out of root. Files on disk win;
// every other path falls through to index.html so the client-side
// router can take over (deep links, refresh on a non-root URL).
//
// /api/* routes never reach here — chi.Router matches them first. This
// handler is wired only as the global NotFound, so by the time we're
// invoked we know no API route claimed the request.
func spaHandler(root string) http.HandlerFunc {
	fs := http.FileServer(http.Dir(root))
	return func(w http.ResponseWriter, r *http.Request) {
		// Normalize so we never escape the web root.
		clean := filepath.Clean(r.URL.Path)
		if strings.HasPrefix(clean, "/api/") || clean == "/healthz" || clean == "/metrics" {
			http.NotFound(w, r)
			return
		}
		// If the requested file exists, serve it. Otherwise fall back
		// to index.html so the SPA can render the route.
		full := filepath.Join(root, clean)
		if info, err := http.Dir(root).Open(clean); err == nil {
			st, _ := info.Stat()
			info.Close()
			if st != nil && !st.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
		}
		_ = full
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	}
}
