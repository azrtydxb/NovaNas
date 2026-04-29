package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/tls"
	"github.com/novanas/nova-nas/internal/config"
)

// startTLS launches the HTTPS listener (and optional HTTP redirect)
// in a background goroutine. Returns nil if TLS is disabled (empty
// HTTPSAddr), so callers can fall back to plain HTTP via cfg.ListenAddr.
//
// The returned cancel func tears the listeners down; call it during
// shutdown.
func startTLS(ctx context.Context, cfg config.TLSConfig, log *slog.Logger, handler http.Handler) (cancel func(), err error) {
	if cfg.HTTPSAddr == "" {
		return func() {}, nil
	}
	srv, err := tls.NewReloadableServer(cfg, log)
	if err != nil {
		return nil, err
	}
	tlsCtx, tlsCancel := context.WithCancel(ctx)
	go func() {
		if err := srv.Serve(tlsCtx, handler); err != nil {
			log.Error("tls serve", "err", err)
		}
	}()
	return tlsCancel, nil
}
