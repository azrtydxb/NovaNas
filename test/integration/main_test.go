//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jackc/pgx/v5/pgxpool"
)

var dbDSN string

func TestMain(m *testing.M) {
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
		os.Exit(1)
	}
	defer func() { _ = pg.Terminate(ctx) }()

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "dsn:", err)
		os.Exit(1)
	}
	dbDSN = dsn

	if err := applyMigrations(ctx, dsn); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func applyMigrations(ctx context.Context, dsn string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	migrationsDir, err := filepath.Abs("../../internal/store/migrations")
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return err
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		// goose markers split up/down sections; we only run "up"
		sql := extractGooseUp(string(data))
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
	}
	return nil
}

func extractGooseUp(s string) string {
	upMarker := "-- +goose Up"
	downMarker := "-- +goose Down"
	upIdx := indexAfter(s, upMarker)
	if upIdx < 0 {
		return s
	}
	downIdx := indexAfter(s, downMarker)
	if downIdx < 0 {
		return s[upIdx:]
	}
	return s[upIdx:downIdx]
}

func indexAfter(s, needle string) int {
	i := stringIndex(s, needle)
	if i < 0 {
		return -1
	}
	return i + len(needle)
}

func stringIndex(s, needle string) int {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
