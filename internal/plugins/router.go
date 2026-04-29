package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

// ServiceTokenMinter abstracts the path that mints a fresh OIDC token
// for a plugin's service account. nil disables AuthServiceToken
// routes (they 503 at request time).
type ServiceTokenMinter interface {
	Mint(ctx context.Context, plugin string) (token string, err error)
}

// Router holds the live reverse-proxy handlers for installed plugins.
// nova-api mounts a single chi handler at
// /api/v1/plugins/{name}/api/* that dispatches into this router.
//
// All mutators are safe for concurrent use; reads take a single RLock.
type Router struct {
	Logger      *slog.Logger
	TokenMinter ServiceTokenMinter

	mu     sync.RWMutex
	mounts map[string]*pluginMount // keyed by plugin name
}

type pluginMount struct {
	plugin string
	routes []routeProxy
}

type routeProxy struct {
	prefix   string // mounted prefix, e.g. "/buckets" — match-prefix
	auth     AuthMode
	scopes   []string
	upstream *url.URL
	proxy    *httputil.ReverseProxy
}

// NewRouter constructs an empty Router.
func NewRouter(logger *slog.Logger, m ServiceTokenMinter) *Router {
	return &Router{Logger: logger, TokenMinter: m, mounts: map[string]*pluginMount{}}
}

// Mount installs the routes from a Plugin manifest. Replaces any
// existing mount under the same plugin name (used by Upgrade for an
// atomic swap).
func (rt *Router) Mount(plugin string, routes []APIRoute) error {
	mounts := make([]routeProxy, 0, len(routes))
	for _, r := range routes {
		u, err := url.Parse(r.Upstream)
		if err != nil {
			return fmt.Errorf("plugins: route %q upstream parse: %w", r.Path, err)
		}
		rp := httputil.NewSingleHostReverseProxy(u)
		// Default Director rewrites Host; we additionally prefix the
		// upstream's path so /api/v1/plugins/<name>/api/<path> -> upstream/<route.path>/<remainder>
		mounts = append(mounts, routeProxy{
			prefix:   strings.TrimRight(r.Path, "/"),
			auth:     r.Auth,
			scopes:   r.Scopes,
			upstream: u,
			proxy:    rp,
		})
	}
	rt.mu.Lock()
	rt.mounts[plugin] = &pluginMount{plugin: plugin, routes: mounts}
	rt.mu.Unlock()
	if rt.Logger != nil {
		rt.Logger.Info("plugins router: mounted", "plugin", plugin, "routes", len(mounts))
	}
	return nil
}

// Unmount removes a plugin's routes. No-op if not mounted.
func (rt *Router) Unmount(plugin string) {
	rt.mu.Lock()
	delete(rt.mounts, plugin)
	rt.mu.Unlock()
	if rt.Logger != nil {
		rt.Logger.Info("plugins router: unmounted", "plugin", plugin)
	}
}

// IsMounted reports whether the named plugin currently has routes.
func (rt *Router) IsMounted(plugin string) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	_, ok := rt.mounts[plugin]
	return ok
}

// MountedPlugins returns the names of all plugins with active routes.
func (rt *Router) MountedPlugins() []string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	out := make([]string, 0, len(rt.mounts))
	for k := range rt.mounts {
		out = append(out, k)
	}
	return out
}

// ServeProxy is the handler nova-api wires under
// /api/v1/plugins/{name}/api/*. It looks up the named plugin's
// route table, finds the longest-prefix match, applies the auth
// transform, and forwards.
//
// Request path on entry: r.URL.Path == "/api/v1/plugins/<name>/api/<rest>"
// We pass <rest> down to the route handler unchanged.
func (rt *Router) ServeProxy(w http.ResponseWriter, r *http.Request, plugin, rest string) {
	rt.mu.RLock()
	pm, ok := rt.mounts[plugin]
	rt.mu.RUnlock()
	if !ok {
		http.Error(w, `{"error":"not_mounted","message":"plugin api not registered"}`, http.StatusNotFound)
		return
	}
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	// longest-prefix match
	var match *routeProxy
	for i := range pm.routes {
		rp := &pm.routes[i]
		if rp.prefix == "" || strings.HasPrefix(rest, rp.prefix) {
			if match == nil || len(rp.prefix) > len(match.prefix) {
				match = rp
			}
		}
	}
	if match == nil {
		http.Error(w, `{"error":"no_route","message":"no proxy route matches"}`, http.StatusNotFound)
		return
	}

	// Apply auth transform.
	switch match.auth {
	case AuthBearerPassthrough:
		// Authorization header is forwarded as-is by ReverseProxy.
	case AuthServiceToken:
		if rt.TokenMinter == nil {
			http.Error(w, `{"error":"service_token_unavailable","message":"service-token minting is not configured"}`, http.StatusServiceUnavailable)
			return
		}
		tok, err := rt.TokenMinter.Mint(r.Context(), plugin)
		if err != nil {
			if rt.Logger != nil {
				rt.Logger.Warn("plugins router: mint service token", "plugin", plugin, "err", err)
			}
			http.Error(w, `{"error":"service_token_failed","message":"could not mint service token"}`, http.StatusBadGateway)
			return
		}
		// Strip the caller's auth and substitute the service token.
		r.Header.Del("Authorization")
		r.Header.Set("Authorization", "Bearer "+tok)
	}

	// Rewrite the request path: keep upstream's base path + (rest).
	r2 := r.Clone(r.Context())
	r2.URL.Path = strings.TrimRight(match.upstream.Path, "/") + rest
	r2.URL.RawPath = ""

	// Audit-friendly logging line per proxied call.
	if rt.Logger != nil {
		rt.Logger.Info("plugins router: proxy",
			"plugin", plugin,
			"upstream", match.upstream.String(),
			"path", rest,
			"method", r.Method,
			"auth", string(match.auth),
		)
	}
	match.proxy.ServeHTTP(w, r2)
}
