// Package store wires a pgx pool to sqlc-generated queries.
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type Store struct {
	Pool    *pgxpool.Pool
	Queries *storedb.Queries
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConns = 8
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool, Queries: storedb.New(pool)}, nil
}

func (s *Store) Close() {
	if s == nil || s.Pool == nil {
		return
	}
	s.Pool.Close()
}
