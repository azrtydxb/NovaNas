//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx as database/sql driver
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	dbDSN    string
	redisURL string
)

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("novanas"),
		tcpostgres.WithUsername("novanas"),
		tcpostgres.WithPassword("novanas"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "postgres start:", err)
		return 1
	}
	defer func() { _ = pg.Terminate(ctx) }()

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "dsn:", err)
		return 1
	}
	dbDSN = dsn

	if err := applyMigrations(dsn); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		return 1
	}

	rc, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		fmt.Fprintln(os.Stderr, "redis start:", err)
		return 1
	}
	defer func() { _ = rc.Terminate(ctx) }()

	// Use ConnectionString (returns redis://host:port) so callers can
	// feed it to asynq.ParseRedisURI as well as construct
	// asynq.RedisClientOpt directly.
	uri, err := rc.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "redis uri:", err)
		return 1
	}
	redisURL = uri

	return m.Run()
}

// applyMigrations runs all goose migrations in internal/store/migrations
// against the given DSN, using goose itself so any +goose StatementBegin/End
// or future directives behave identically to production migrations.
func applyMigrations(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, "../../internal/store/migrations")
}
