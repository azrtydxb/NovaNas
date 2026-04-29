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
		// Vite emits <script type="module" crossorigin>; the crossorigin
		// attribute makes Chrome do a CORS fetch even for same-origin
		// scripts. Without an ACAO header the script silently fails to
		// execute (Chrome doesn't surface this as a console error in
		// production builds). Set ACAO to the request's Origin so the
		// browser is happy.
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		// If the requested file exists, serve it. Otherwise fall back
		// to index.html so the SPA can render the route.
		if info, err := http.Dir(root).Open(clean); err == nil {
			st, _ := info.Stat()
			info.Close()
			if st != nil && !st.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	}
}
