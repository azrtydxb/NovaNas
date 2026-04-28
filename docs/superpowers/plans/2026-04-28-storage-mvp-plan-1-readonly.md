# Storage MVP — Plan 1: Bootstrap + Read-Only API

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the project skeleton, Postgres schema, host-ops layer, and read-only HTTP endpoints. End state: `nova-api` binary that boots, connects to Postgres, and answers `GET` for `/healthz`, `/disks`, `/pools`, `/pools/:name`, `/datasets`, `/datasets/:fullname`, `/snapshots`. No writes, no jobs, no Redis yet.

**Architecture:** Go binary running on host systemd. Chi router for HTTP. sqlc + goose + pgx for Postgres. `internal/host/exec` shells out to host binaries; `internal/host/{zfs,disks}` parse output into typed Go structs. All ZFS reads are live; no caching.

**Tech Stack:** Go 1.22+, chi, sqlc, goose, pgx, slog, envconfig, testcontainers-go, golangci-lint.

**Reference spec:** `docs/superpowers/specs/2026-04-28-novanas-storage-mvp-design.md`

---

## File Structure (created in this plan)

```
cmd/nova-api/main.go
internal/api/server.go
internal/api/middleware/{requestid,logging,recover,jsonerror}.go
internal/api/handlers/{disks,pools,datasets,snapshots,health}.go
internal/config/config.go
internal/host/exec/{exec,error}.go
internal/host/disks/{disks,parser}.go
internal/host/zfs/pool/{pool,parser}.go
internal/host/zfs/dataset/{dataset,parser}.go
internal/host/zfs/snapshot/{snapshot,parser}.go
internal/store/{db,migrations/0001_init.sql,queries/*.sql,gen/*}
test/fixtures/{lsblk,zpool_list,zpool_status,zpool_get,zfs_list_*,zfs_get,zfs_snap_list}.txt
test/integration/readonly_test.go
sqlc.yaml
Makefile
.golangci.yml
.gitignore
go.mod
README.md
```

---

## Task 1: Initialize Go module and repo skeleton

**Files:**
- Create: `go.mod`, `.gitignore`, `Makefile`, `README.md`, `.golangci.yml`

- [ ] **Step 1: Init the module**

Run from repo root:
```bash
go mod init github.com/novanas/nova-nas
```

- [ ] **Step 2: Write `.gitignore`**

Create `.gitignore`:
```
# Build
/bin/
/dist/
nova-api

# Test artifacts
*.test
*.out
coverage.txt

# IDE
.vscode/
.idea/

# Local env
.env
.env.local
```

- [ ] **Step 3: Write `Makefile`**

Create `Makefile`:
```makefile
.PHONY: build test test-integration test-e2e lint fmt gen run clean

GO ?= go
BIN := bin/nova-api

build:
	$(GO) build -o $(BIN) ./cmd/nova-api

test:
	$(GO) test ./...

test-integration:
	$(GO) test -tags=integration ./test/integration/...

test-e2e:
	$(GO) test -tags=e2e ./test/e2e/...

lint:
	golangci-lint run

fmt:
	$(GO) fmt ./...

gen:
	sqlc generate

run: build
	./$(BIN)

clean:
	rm -rf bin/ dist/
```

- [ ] **Step 4: Write `.golangci.yml`**

Create `.golangci.yml`:
```yaml
run:
  timeout: 5m
linters:
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - goimports
    - revive
    - gosec
    - misspell
linters-settings:
  revive:
    rules:
      - name: exported
issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gosec
```

- [ ] **Step 5: Write `README.md`**

Create `README.md`:
```markdown
# NovaNAS

Storage control plane for a single-node ZFS-based NAS appliance.

## Build
```
make build
```

## Test
```
make test
```

See `docs/superpowers/specs/` for design docs.
```

- [ ] **Step 6: Verify and commit**

Run:
```bash
go mod tidy
make build 2>&1 || true   # will fail until cmd/nova-api exists; that's fine
git add go.mod .gitignore Makefile README.md .golangci.yml
git commit -m "chore: initialize go module and repo skeleton"
```

---

## Task 2: Config loader

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:
```go
package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("LISTEN_ADDR", ":8080")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "postgres://x" {
		t.Errorf("DatabaseURL=%q", cfg.DatabaseURL)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr=%q", cfg.ListenAddr)
	}
	if cfg.ZFSBin != "/sbin/zfs" {
		t.Errorf("ZFSBin default=%q", cfg.ZFSBin)
	}
	if cfg.ZpoolBin != "/sbin/zpool" {
		t.Errorf("ZpoolBin default=%q", cfg.ZpoolBin)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default=%q", cfg.LogLevel)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("LISTEN_ADDR", ":8080")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/...
```
Expected: FAIL — `Load` undefined.

- [ ] **Step 3: Implement `Load`**

Add `kelseyhightower/envconfig`:
```bash
go get github.com/kelseyhightower/envconfig
```

Create `internal/config/config.go`:
```go
// Package config loads application config from environment variables.
package config

import (
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	ListenAddr  string `envconfig:"LISTEN_ADDR" required:"true"`
	ZFSBin      string `envconfig:"ZFS_BIN" default:"/sbin/zfs"`
	ZpoolBin    string `envconfig:"ZPOOL_BIN" default:"/sbin/zpool"`
	LsblkBin    string `envconfig:"LSBLK_BIN" default:"/usr/bin/lsblk"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`
}

func Load() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, err
	}
	return &c, nil
}
```

- [ ] **Step 4: Run test, expect pass**

```bash
go test ./internal/config/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go mod tidy
git add internal/config/ go.mod go.sum
git commit -m "feat(config): add env-based config loader"
```

---

## Task 3: Logger

**Files:**
- Create: `internal/logging/logger.go`, `internal/logging/logger_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/logging/logger_test.go`:
```go
package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew_RespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New("warn", &buf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("info-msg")
	logger.Warn("warn-msg")
	out := buf.String()
	if strings.Contains(out, "info-msg") {
		t.Errorf("info should be suppressed; got: %s", out)
	}
	if !strings.Contains(out, "warn-msg") {
		t.Errorf("warn missing; got: %s", out)
	}
}

func TestNew_RejectsUnknownLevel(t *testing.T) {
	if _, err := New("nope", nil); err == nil {
		t.Fatal("expected error for unknown level")
	}
}
```

- [ ] **Step 2: Run test, expect fail**

```bash
go test ./internal/logging/...
```
Expected: FAIL — `New` undefined.

- [ ] **Step 3: Implement**

Create `internal/logging/logger.go`:
```go
// Package logging wraps slog with level parsing and a default JSON handler.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

func New(level string, w io.Writer) (*slog.Logger, error) {
	if w == nil {
		w = os.Stderr
	}
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("unknown log level %q", level)
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl})
	return slog.New(h), nil
}
```

- [ ] **Step 4: Run test, expect pass**

```bash
go test ./internal/logging/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/logging/
git commit -m "feat(logging): add slog wrapper with level parsing"
```

---

## Task 4: Chi server skeleton + /healthz

**Files:**
- Create: `internal/api/server.go`, `internal/api/server_test.go`
- Create: `internal/api/handlers/health.go`
- Create: `internal/api/middleware/{requestid,logging,recover,jsonerror}.go`
- Create: `cmd/nova-api/main.go`

- [ ] **Step 1: Add chi**

```bash
go get github.com/go-chi/chi/v5
```

- [ ] **Step 2: Write the failing test**

Create `internal/api/server_test.go`:
```go
package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	srv := New(Deps{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.String(); got != `{"status":"ok"}` {
		t.Errorf("body=%q", got)
	}
}
```

- [ ] **Step 3: Run test, expect fail**

```bash
go test ./internal/api/...
```
Expected: FAIL.

- [ ] **Step 4: Write middleware (request id)**

Create `internal/api/middleware/requestid.go`:
```go
// Package middleware contains HTTP middleware shared across handlers.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type ctxKey string

const RequestIDKey ctxKey = "requestID"

const RequestIDHeader = "X-Request-Id"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		ctx := context.WithValue(r.Context(), RequestIDKey, id)
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDOf(ctx context.Context) string {
	v, _ := ctx.Value(RequestIDKey).(string)
	return v
}
```

- [ ] **Step 5: Write recover middleware**

Create `internal/api/middleware/recover.go`:
```go
package middleware

import (
	"log/slog"
	"net/http"
)

func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic in handler",
						"panic", rec,
						"path", r.URL.Path,
						"requestID", RequestIDOf(r.Context()),
					)
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 6: Write logging middleware**

Create `internal/api/middleware/logging.go`:
```go
package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type respWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *respWriter) WriteHeader(s int) {
	rw.status = s
	rw.ResponseWriter.WriteHeader(s)
}
func (rw *respWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &respWriter{ResponseWriter: w}
			next.ServeHTTP(rw, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"bytes", rw.bytes,
				"durMS", time.Since(start).Milliseconds(),
				"requestID", RequestIDOf(r.Context()),
			)
		})
	}
}
```

- [ ] **Step 7: Write json error helper**

Create `internal/api/middleware/jsonerror.go`:
```go
package middleware

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorBody{Error: code, Message: message})
}
```

- [ ] **Step 8: Write health handler**

Create `internal/api/handlers/health.go`:
```go
// Package handlers contains HTTP handlers for the API.
package handlers

import (
	"net/http"
)

func Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
```

- [ ] **Step 9: Write the server**

Create `internal/api/server.go`:
```go
// Package api wires the HTTP server.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/handlers"
	mw "github.com/novanas/nova-nas/internal/api/middleware"
)

type Deps struct {
	Logger *slog.Logger
}

type Server struct {
	deps   Deps
	router chi.Router
}

func New(d Deps) *Server {
	r := chi.NewRouter()
	r.Use(mw.RequestID)
	r.Use(mw.Recoverer(d.Logger))
	r.Use(mw.Logging(d.Logger))
	r.Get("/healthz", handlers.Healthz)
	return &Server{deps: d, router: r}
}

func (s *Server) Handler() http.Handler { return s.router }
```

- [ ] **Step 10: Write `cmd/nova-api/main.go`**

Create `cmd/nova-api/main.go`:
```go
// Command nova-api is the storage control plane HTTP API.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/novanas/nova-nas/internal/api"
	"github.com/novanas/nova-nas/internal/config"
	"github.com/novanas/nova-nas/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger, err := logging.New(cfg.LogLevel, os.Stderr)
	if err != nil {
		panic(err)
	}

	srv := api.New(api.Deps{Logger: logger})
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("starting", "addr", cfg.ListenAddr)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http listen", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	logger.Info("stopped")
}
```

- [ ] **Step 11: Run tests, expect pass**

```bash
go mod tidy
go test ./internal/api/...
make build
```
Expected: tests PASS, build produces `bin/nova-api`.

- [ ] **Step 12: Commit**

```bash
git add internal/api/ internal/logging/ cmd/ go.mod go.sum
git commit -m "feat(api): add chi server skeleton with /healthz"
```

---

## Task 5: sqlc + goose configuration

**Files:**
- Create: `sqlc.yaml`
- Create: `internal/store/migrations/.keep`
- Create: `internal/store/queries/.keep`

- [ ] **Step 1: Install tools (dev only, document in README)**

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
```

- [ ] **Step 2: Write `sqlc.yaml`**

Create `sqlc.yaml`:
```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/store/queries"
    schema: "internal/store/migrations"
    gen:
      go:
        package: "storedb"
        out: "internal/store/gen"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_pointers_for_null_types: true
```

- [ ] **Step 3: Create empty dirs**

```bash
mkdir -p internal/store/migrations internal/store/queries
touch internal/store/migrations/.keep internal/store/queries/.keep
```

- [ ] **Step 4: Add Make target for migrations**

Append to `Makefile`:
```makefile
DB_URL ?= postgres://novanas:novanas@localhost:5432/novanas?sslmode=disable

migrate-up:
	goose -dir internal/store/migrations postgres "$(DB_URL)" up

migrate-down:
	goose -dir internal/store/migrations postgres "$(DB_URL)" down

migrate-status:
	goose -dir internal/store/migrations postgres "$(DB_URL)" status
```

- [ ] **Step 5: Commit**

```bash
git add sqlc.yaml internal/store/ Makefile
git commit -m "chore(store): add sqlc and goose configuration"
```

---

## Task 6: Initial migration (jobs, audit_log, resource_metadata)

**Files:**
- Create: `internal/store/migrations/0001_init.sql`

- [ ] **Step 1: Write migration**

Create `internal/store/migrations/0001_init.sql`:
```sql
-- +goose Up
CREATE TABLE jobs (
    id           uuid        PRIMARY KEY,
    kind         text        NOT NULL,
    target       text        NOT NULL,
    state        text        NOT NULL,
    command      text        NOT NULL DEFAULT '',
    stdout       text        NOT NULL DEFAULT '',
    stderr       text        NOT NULL DEFAULT '',
    exit_code    integer,
    error        text,
    request_id   text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    started_at   timestamptz,
    finished_at  timestamptz,
    CHECK (state IN ('queued','running','succeeded','failed','cancelled','interrupted'))
);
CREATE INDEX jobs_state_idx ON jobs(state);
CREATE INDEX jobs_created_idx ON jobs(created_at DESC);

CREATE TABLE audit_log (
    id           bigserial   PRIMARY KEY,
    ts           timestamptz NOT NULL DEFAULT now(),
    actor        text,
    action       text        NOT NULL,
    target       text        NOT NULL,
    request_id   text        NOT NULL DEFAULT '',
    payload      jsonb,
    result       text        NOT NULL,
    CHECK (result IN ('accepted','rejected'))
);
CREATE INDEX audit_log_ts_idx ON audit_log(ts DESC);

CREATE TABLE resource_metadata (
    id            bigserial   PRIMARY KEY,
    kind          text        NOT NULL,
    zfs_name      text        NOT NULL,
    display_name  text,
    description   text,
    tags          jsonb,
    UNIQUE(kind, zfs_name),
    CHECK (kind IN ('pool','dataset','snapshot'))
);

-- +goose Down
DROP TABLE resource_metadata;
DROP TABLE audit_log;
DROP TABLE jobs;
```

- [ ] **Step 2: Verify SQL parses (locally if Postgres available)**

If a local Postgres is reachable:
```bash
make migrate-up
make migrate-status
make migrate-down
```
Expected: up applies cleanly, status shows applied, down rolls back. Skip if no local Postgres.

- [ ] **Step 3: Commit**

```bash
git add internal/store/migrations/0001_init.sql
git commit -m "feat(store): initial schema (jobs, audit_log, resource_metadata)"
```

---

## Task 7: sqlc query files for read paths

**Files:**
- Create: `internal/store/queries/resource_metadata.sql`
- Create: `internal/store/gen/*.go` (generated)

For Plan 1 we only need `resource_metadata` reads (to enrich GET responses). Jobs and audit_log queries land in Plan 2.

- [ ] **Step 1: Write queries file**

Create `internal/store/queries/resource_metadata.sql`:
```sql
-- name: GetResourceMetadata :one
SELECT id, kind, zfs_name, display_name, description, tags
FROM resource_metadata
WHERE kind = $1 AND zfs_name = $2;

-- name: ListResourceMetadataByKind :many
SELECT id, kind, zfs_name, display_name, description, tags
FROM resource_metadata
WHERE kind = $1
ORDER BY zfs_name;
```

- [ ] **Step 2: Generate**

```bash
sqlc generate
```
Expected: creates `internal/store/gen/{db.go,models.go,resource_metadata.sql.go}`.

- [ ] **Step 3: Add pgx**

```bash
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
```

- [ ] **Step 4: Write a thin DB connector**

Create `internal/store/db.go`:
```go
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

func (s *Store) Close() { s.Pool.Close() }
```

- [ ] **Step 5: Build and commit**

```bash
go mod tidy
go build ./...
git add internal/store/ go.mod go.sum sqlc.yaml
git commit -m "feat(store): generate sqlc queries and add pgx pool wrapper"
```

---

## Task 8: Wire DB into main.go

**Files:**
- Modify: `cmd/nova-api/main.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: Update server `Deps` to accept the store**

Edit `internal/api/server.go`. Replace the `Deps` struct and `New` function:
```go
type Deps struct {
	Logger *slog.Logger
	Store  *store.Store
}
```
Add the import:
```go
"github.com/novanas/nova-nas/internal/store"
```

- [ ] **Step 2: Update main to open the store**

Edit `cmd/nova-api/main.go`. After `logger` is created, open the store:
```go
ctx := context.Background()
st, err := store.Open(ctx, cfg.DatabaseURL)
if err != nil {
    logger.Error("db open", "err", err)
    os.Exit(1)
}
defer st.Close()

srv := api.New(api.Deps{Logger: logger, Store: st})
```
Add the import for `store` and remove the unused `context` import if duplicated.

- [ ] **Step 3: Verify build**

```bash
go build ./...
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/nova-api/main.go internal/api/server.go
git commit -m "feat: wire postgres pool into nova-api"
```

---

## Task 9: Host exec primitive

**Files:**
- Create: `internal/host/exec/exec.go`, `internal/host/exec/error.go`, `internal/host/exec/exec_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/host/exec/exec_test.go`:
```go
package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRun_Success(t *testing.T) {
	out, err := Run(context.Background(), "/bin/echo", "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("stdout=%q", out)
	}
}

func TestRun_NonZeroExit(t *testing.T) {
	_, err := Run(context.Background(), "/bin/false")
	if err == nil {
		t.Fatal("expected error")
	}
	var he *HostError
	if !errors.As(err, &he) {
		t.Fatalf("want *HostError, got %T", err)
	}
	if he.ExitCode == 0 {
		t.Errorf("ExitCode=0")
	}
}

func TestRun_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, "/bin/sleep", "5")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_BinaryNotFound(t *testing.T) {
	_, err := Run(context.Background(), "/nonexistent/binary")
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Implement `HostError`**

Create `internal/host/exec/error.go`:
```go
// Package exec is the only place the API shells out to host commands.
package exec

import "fmt"

type HostError struct {
	Bin      string
	Args     []string
	ExitCode int
	Stderr   string
	Cause    error
}

func (e *HostError) Error() string {
	if e.Cause != nil && e.ExitCode == 0 {
		return fmt.Sprintf("exec %s: %v", e.Bin, e.Cause)
	}
	return fmt.Sprintf("exec %s exit=%d: %s", e.Bin, e.ExitCode, e.Stderr)
}

func (e *HostError) Unwrap() error { return e.Cause }
```

- [ ] **Step 3: Implement `Run`**

Create `internal/host/exec/exec.go`:
```go
package exec

import (
	"bytes"
	"context"
	"errors"
	osexec "os/exec"
)

func Run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := osexec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		exitCode := 0
		var ee *osexec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		}
		return stdout.Bytes(), &HostError{
			Bin:      bin,
			Args:     args,
			ExitCode: exitCode,
			Stderr:   stderr.String(),
			Cause:    err,
		}
	}
	return stdout.Bytes(), nil
}
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./internal/host/exec/...
```
Expected: PASS (assumes Linux/macOS with `/bin/echo`, `/bin/false`, `/bin/sleep`).

- [ ] **Step 5: Commit**

```bash
git add internal/host/exec/
git commit -m "feat(host/exec): add shared exec primitive with structured errors"
```

---

## Task 10: Disks — parser and List

**Files:**
- Create: `internal/host/disks/parser.go`, `internal/host/disks/disks.go`, `internal/host/disks/parser_test.go`
- Create: `test/fixtures/lsblk.json`

- [ ] **Step 1: Capture a fixture**

Create `test/fixtures/lsblk.json` (representative output of `lsblk -J -o NAME,SIZE,MODEL,SERIAL,TYPE,ROTA,FSTYPE,MOUNTPOINT,WWN`):
```json
{
   "blockdevices": [
      {"name":"sda","size":"1000204886016","model":"ST1000DM010-2EP102","serial":"ZN12ABCD","type":"disk","rota":true,"fstype":null,"mountpoint":null,"wwn":"0x5000c500abcdef00",
       "children":[
         {"name":"sda1","size":"1000204869632","model":null,"serial":null,"type":"part","rota":true,"fstype":"zfs_member","mountpoint":null,"wwn":"0x5000c500abcdef00"}
       ]
      },
      {"name":"sdb","size":"500107862016","model":"INTEL SSDSC2KW","serial":"BTYM12345678","type":"disk","rota":false,"fstype":null,"mountpoint":null,"wwn":"0x55cd2e404abcdef0"}
   ]
}
```

- [ ] **Step 2: Write the failing parser test**

Create `internal/host/disks/parser_test.go`:
```go
package disks

import (
	"os"
	"testing"
)

func TestParseLsblk(t *testing.T) {
	data, err := os.ReadFile("../../../test/fixtures/lsblk.json")
	if err != nil {
		t.Fatal(err)
	}
	disks, err := parseLsblk(data)
	if err != nil {
		t.Fatalf("parseLsblk: %v", err)
	}
	if len(disks) != 2 {
		t.Fatalf("want 2 disks, got %d", len(disks))
	}
	sda := disks[0]
	if sda.Name != "sda" || sda.SizeBytes != 1000204886016 || !sda.Rotational {
		t.Errorf("sda parsed wrong: %+v", sda)
	}
	if !sda.InUseByPool {
		t.Errorf("sda has zfs_member child; should be InUseByPool")
	}
	sdb := disks[1]
	if sdb.Rotational || sdb.InUseByPool {
		t.Errorf("sdb parsed wrong: %+v", sdb)
	}
}
```

- [ ] **Step 3: Run, expect fail**

```bash
go test ./internal/host/disks/...
```
Expected: FAIL — `parseLsblk` undefined.

- [ ] **Step 4: Implement parser**

Create `internal/host/disks/parser.go`:
```go
// Package disks lists physical block devices and their identity.
package disks

import (
	"encoding/json"
	"strconv"
)

type Disk struct {
	Name        string `json:"name"`
	SizeBytes   uint64 `json:"sizeBytes"`
	Model       string `json:"model,omitempty"`
	Serial      string `json:"serial,omitempty"`
	WWN         string `json:"wwn,omitempty"`
	Rotational  bool   `json:"rotational"`
	InUseByPool bool   `json:"inUseByPool"`
}

type lsblkRoot struct {
	BlockDevices []lsblkDev `json:"blockdevices"`
}

type lsblkDev struct {
	Name     string     `json:"name"`
	Size     any        `json:"size"`
	Model    *string    `json:"model"`
	Serial   *string    `json:"serial"`
	Type     string     `json:"type"`
	Rota     bool       `json:"rota"`
	FsType   *string    `json:"fstype"`
	WWN      *string    `json:"wwn"`
	Children []lsblkDev `json:"children"`
}

func parseLsblk(data []byte) ([]Disk, error) {
	var r lsblkRoot
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	out := make([]Disk, 0, len(r.BlockDevices))
	for _, d := range r.BlockDevices {
		if d.Type != "disk" {
			continue
		}
		out = append(out, Disk{
			Name:        d.Name,
			SizeBytes:   parseSize(d.Size),
			Model:       deref(d.Model),
			Serial:      deref(d.Serial),
			WWN:         deref(d.WWN),
			Rotational:  d.Rota,
			InUseByPool: hasZFSMember(d),
		})
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parseSize(v any) uint64 {
	switch x := v.(type) {
	case float64:
		return uint64(x)
	case string:
		n, _ := strconv.ParseUint(x, 10, 64)
		return n
	}
	return 0
}

func hasZFSMember(d lsblkDev) bool {
	if d.FsType != nil && *d.FsType == "zfs_member" {
		return true
	}
	for _, c := range d.Children {
		if hasZFSMember(c) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Implement `List`**

Create `internal/host/disks/disks.go`:
```go
package disks

import (
	"context"

	"github.com/novanas/nova-nas/internal/host/exec"
)

type Lister struct {
	LsblkBin string
}

func (l *Lister) List(ctx context.Context) ([]Disk, error) {
	out, err := exec.Run(ctx, l.LsblkBin, "-J", "-b",
		"-o", "NAME,SIZE,MODEL,SERIAL,TYPE,ROTA,FSTYPE,MOUNTPOINT,WWN")
	if err != nil {
		return nil, err
	}
	return parseLsblk(out)
}
```

- [ ] **Step 6: Run tests, expect pass**

```bash
go test ./internal/host/disks/...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/host/disks/ test/fixtures/lsblk.json
git commit -m "feat(host/disks): list block devices via lsblk"
```

---

## Task 11: Disks handler

**Files:**
- Create: `internal/api/handlers/disks.go`
- Create: `internal/api/handlers/disks_test.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: Write the failing test**

Create `internal/api/handlers/disks_test.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/novanas/nova-nas/internal/host/disks"
)

type fakeDiskLister struct{ result []disks.Disk }

func (f *fakeDiskLister) List(_ context.Context) ([]disks.Disk, error) {
	return f.result, nil
}

func TestDisksList(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &DisksHandler{
		Logger: logger,
		Lister: &fakeDiskLister{result: []disks.Disk{
			{Name: "sda", SizeBytes: 1000, Rotational: true},
		}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got []disks.Disk
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "sda" {
		t.Errorf("body=%+v", got)
	}
}
```

- [ ] **Step 2: Run, expect fail**

```bash
go test ./internal/api/handlers/...
```
Expected: FAIL.

- [ ] **Step 3: Implement handler**

Create `internal/api/handlers/disks.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/disks"
)

type DiskLister interface {
	List(ctx context.Context) ([]disks.Disk, error)
}

type DisksHandler struct {
	Logger *slog.Logger
	Lister DiskLister
}

func (h *DisksHandler) List(w http.ResponseWriter, r *http.Request) {
	ds, err := h.Lister.List(r.Context())
	if err != nil {
		h.Logger.Error("disks list", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ds)
}
```

- [ ] **Step 4: Wire into server**

Edit `internal/api/server.go`. Update `Deps`:
```go
type Deps struct {
	Logger *slog.Logger
	Store  *store.Store
	Disks  handlers.DiskLister
}
```
Update `New` to register:
```go
disksH := &handlers.DisksHandler{Logger: d.Logger, Lister: d.Disks}
r.Route("/api/v1", func(r chi.Router) {
    r.Get("/disks", disksH.List)
})
```

Update `cmd/nova-api/main.go`:
```go
import "github.com/novanas/nova-nas/internal/host/disks"
...
disksLister := &disks.Lister{LsblkBin: cfg.LsblkBin}
srv := api.New(api.Deps{
    Logger: logger,
    Store:  st,
    Disks:  disksLister,
})
```

- [ ] **Step 5: Run tests, expect pass**

```bash
go test ./...
go build ./...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api/ cmd/
git commit -m "feat(api): add GET /api/v1/disks"
```

---

## Task 12: ZFS pool — List

**Files:**
- Create: `internal/host/zfs/pool/pool.go`, `internal/host/zfs/pool/parser.go`, `internal/host/zfs/pool/parser_test.go`
- Create: `test/fixtures/zpool_list.txt`

- [ ] **Step 1: Capture fixture**

Create `test/fixtures/zpool_list.txt` (output of `zpool list -H -p -o name,size,allocated,free,health,readonly,fragmentation,capacity,dedupratio`):
```
tank	1000204886016	123456789012	876748097004	ONLINE	off	5	12	1.00x
ssd	500107862016	0	500107862016	ONLINE	off	0	0	1.00x
```

- [ ] **Step 2: Write the failing test**

Create `internal/host/zfs/pool/parser_test.go`:
```go
package pool

import (
	"os"
	"testing"
)

func TestParseList(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zpool_list.txt")
	if err != nil {
		t.Fatal(err)
	}
	pools, err := parseList(data)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("want 2, got %d", len(pools))
	}
	tank := pools[0]
	if tank.Name != "tank" || tank.SizeBytes != 1000204886016 || tank.Health != "ONLINE" {
		t.Errorf("tank=%+v", tank)
	}
	if tank.Allocated != 123456789012 || tank.Free != 876748097004 {
		t.Errorf("tank alloc/free=%+v", tank)
	}
}
```

- [ ] **Step 3: Run, expect fail**

```bash
go test ./internal/host/zfs/pool/...
```
Expected: FAIL.

- [ ] **Step 4: Implement parser and types**

Create `internal/host/zfs/pool/parser.go`:
```go
// Package pool wraps `zpool` for read and write operations on storage pools.
package pool

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type Pool struct {
	Name          string `json:"name"`
	SizeBytes     uint64 `json:"sizeBytes"`
	Allocated     uint64 `json:"allocated"`
	Free          uint64 `json:"free"`
	Health        string `json:"health"`
	ReadOnly      bool   `json:"readOnly"`
	Fragmentation int    `json:"fragmentationPct"`
	Capacity      int    `json:"capacityPct"`
	DedupRatio    string `json:"dedupRatio"`
}

func parseList(data []byte) ([]Pool, error) {
	var out []Pool
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 9 {
			return nil, fmt.Errorf("zpool list: expected 9 fields, got %d in %q", len(f), line)
		}
		p := Pool{
			Name:       f[0],
			Health:     f[4],
			ReadOnly:   f[5] == "on",
			DedupRatio: f[8],
		}
		var err error
		if p.SizeBytes, err = parseUint(f[1]); err != nil {
			return nil, err
		}
		if p.Allocated, err = parseUint(f[2]); err != nil {
			return nil, err
		}
		if p.Free, err = parseUint(f[3]); err != nil {
			return nil, err
		}
		if p.Fragmentation, err = parseInt(f[6]); err != nil {
			return nil, err
		}
		if p.Capacity, err = parseInt(f[7]); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, sc.Err()
}

func parseUint(s string) (uint64, error) {
	if s == "-" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}

func parseInt(s string) (int, error) {
	if s == "-" {
		return 0, nil
	}
	return strconv.Atoi(s)
}
```

- [ ] **Step 5: Implement `List`**

Create `internal/host/zfs/pool/pool.go`:
```go
package pool

import (
	"context"

	"github.com/novanas/nova-nas/internal/host/exec"
)

type Manager struct {
	ZpoolBin string
}

func (m *Manager) List(ctx context.Context) ([]Pool, error) {
	out, err := exec.Run(ctx, m.ZpoolBin, "list", "-H", "-p",
		"-o", "name,size,allocated,free,health,readonly,fragmentation,capacity,dedupratio")
	if err != nil {
		return nil, err
	}
	return parseList(out)
}
```

- [ ] **Step 6: Run tests, expect pass**

```bash
go test ./internal/host/zfs/pool/...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/host/zfs/pool/ test/fixtures/zpool_list.txt
git commit -m "feat(host/zfs/pool): list pools via zpool list"
```

---

## Task 13: ZFS pool — Get (status + properties)

**Files:**
- Modify: `internal/host/zfs/pool/parser.go`, `internal/host/zfs/pool/pool.go`
- Create: `internal/host/zfs/pool/parser_get_test.go`
- Create: `test/fixtures/zpool_get.txt`, `test/fixtures/zpool_status.txt`

- [ ] **Step 1: Capture fixtures**

Create `test/fixtures/zpool_get.txt` (output of `zpool get -H -p all tank`):
```
tank	size	1000204886016	-
tank	capacity	12	-
tank	altroot	-	default
tank	health	ONLINE	-
tank	guid	13871256791234567890	-
tank	version	-	default
tank	bootfs	-	default
tank	delegation	on	default
tank	autoreplace	off	default
tank	cachefile	-	default
tank	failmode	wait	default
tank	listsnapshots	off	default
tank	autoexpand	off	default
tank	dedupratio	1.00x	-
tank	free	876748097004	-
tank	allocated	123456789012	-
tank	readonly	off	-
tank	ashift	12	local
tank	comment	-	default
tank	expandsize	-	-
tank	freeing	0	-
tank	fragmentation	5%	-
tank	leaked	0	-
tank	multihost	off	default
```

Create `test/fixtures/zpool_status.txt` (output of `zpool status -P tank`):
```
  pool: tank
 state: ONLINE
config:

	NAME                                          STATE     READ WRITE CKSUM
	tank                                          ONLINE       0     0     0
	  mirror-0                                    ONLINE       0     0     0
	    /dev/disk/by-id/wwn-0x5000c500abcdef00    ONLINE       0     0     0
	    /dev/disk/by-id/wwn-0x5000c500fedcba98    ONLINE       0     0     0

errors: No known data errors
```

- [ ] **Step 2: Write the failing test**

Create `internal/host/zfs/pool/parser_get_test.go`:
```go
package pool

import (
	"os"
	"testing"
)

func TestParseProps(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zpool_get.txt")
	if err != nil {
		t.Fatal(err)
	}
	props, err := parseProps(data)
	if err != nil {
		t.Fatalf("parseProps: %v", err)
	}
	if props["health"] != "ONLINE" {
		t.Errorf("health=%q", props["health"])
	}
	if props["ashift"] != "12" {
		t.Errorf("ashift=%q", props["ashift"])
	}
}

func TestParseStatus_VdevTree(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zpool_status.txt")
	if err != nil {
		t.Fatal(err)
	}
	st, err := parseStatus(data)
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if st.State != "ONLINE" {
		t.Errorf("state=%q", st.State)
	}
	if len(st.Vdevs) == 0 {
		t.Fatal("no vdevs")
	}
	if st.Vdevs[0].Type != "mirror" {
		t.Errorf("vdev0=%+v", st.Vdevs[0])
	}
	if len(st.Vdevs[0].Children) != 2 {
		t.Errorf("mirror children=%d", len(st.Vdevs[0].Children))
	}
}
```

- [ ] **Step 3: Run, expect fail**

```bash
go test ./internal/host/zfs/pool/...
```
Expected: FAIL.

- [ ] **Step 4: Extend parser**

Append to `internal/host/zfs/pool/parser.go`:
```go
type Status struct {
	State string `json:"state"`
	Vdevs []Vdev `json:"vdevs"`
}

type Vdev struct {
	Type     string `json:"type"`     // mirror, raidz1, raidz2, raidz3, stripe, log, cache, spare
	Path     string `json:"path,omitempty"`
	State    string `json:"state"`
	ReadErr  uint64 `json:"readErrors"`
	WriteErr uint64 `json:"writeErrors"`
	CksumErr uint64 `json:"checksumErrors"`
	Children []Vdev `json:"children,omitempty"`
}

func parseProps(data []byte) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			return nil, fmt.Errorf("zpool get: bad line %q", line)
		}
		// f[0]=pool name, f[1]=property, f[2]=value, f[3]=source
		out[f[1]] = f[2]
	}
	return out, sc.Err()
}

func parseStatus(data []byte) (*Status, error) {
	st := &Status{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	inConfig := false
	var stack []*Vdev
	rootSeen := false

	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "state:") {
			st.State = strings.TrimSpace(strings.TrimPrefix(trim, "state:"))
			continue
		}
		if strings.HasPrefix(trim, "config:") {
			inConfig = true
			continue
		}
		if strings.HasPrefix(trim, "errors:") {
			inConfig = false
			continue
		}
		if !inConfig || trim == "" {
			continue
		}
		// header line
		if strings.HasPrefix(trim, "NAME") {
			continue
		}
		// Determine indent depth in tabs
		depth := 0
		for _, c := range line {
			if c == '\t' {
				depth++
			} else {
				break
			}
		}
		fields := strings.Fields(trim)
		if len(fields) < 2 {
			continue
		}
		name, state := fields[0], fields[1]

		// Skip the root pool line itself
		if !rootSeen {
			rootSeen = true
			continue
		}

		v := Vdev{State: state}
		switch {
		case strings.HasPrefix(name, "mirror"):
			v.Type = "mirror"
		case strings.HasPrefix(name, "raidz3"):
			v.Type = "raidz3"
		case strings.HasPrefix(name, "raidz2"):
			v.Type = "raidz2"
		case strings.HasPrefix(name, "raidz1"), strings.HasPrefix(name, "raidz"):
			v.Type = "raidz1"
		case name == "logs":
			v.Type = "log"
		case name == "cache":
			v.Type = "cache"
		case name == "spares":
			v.Type = "spare"
		default:
			v.Type = "disk"
			v.Path = name
		}

		// Pop stack to current depth
		for len(stack) > 0 && len(stack) >= depth {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			st.Vdevs = append(st.Vdevs, v)
			stack = append(stack, &st.Vdevs[len(st.Vdevs)-1])
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, v)
			stack = append(stack, &parent.Children[len(parent.Children)-1])
		}
	}
	return st, sc.Err()
}
```

- [ ] **Step 5: Add `Get` method**

Append to `internal/host/zfs/pool/pool.go`:
```go
type Detail struct {
	Pool   Pool              `json:"pool"`
	Props  map[string]string `json:"properties"`
	Status *Status           `json:"status"`
}

func (m *Manager) Get(ctx context.Context, name string) (*Detail, error) {
	listOut, err := exec.Run(ctx, m.ZpoolBin, "list", "-H", "-p",
		"-o", "name,size,allocated,free,health,readonly,fragmentation,capacity,dedupratio", name)
	if err != nil {
		var he *exec.HostError
		if errors.As(err, &he) && strings.Contains(he.Stderr, "no such pool") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	pools, err := parseList(listOut)
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return nil, ErrNotFound
	}

	propsOut, err := exec.Run(ctx, m.ZpoolBin, "get", "-H", "-p", "all", name)
	if err != nil {
		return nil, err
	}
	props, err := parseProps(propsOut)
	if err != nil {
		return nil, err
	}

	statusOut, err := exec.Run(ctx, m.ZpoolBin, "status", "-P", name)
	if err != nil {
		return nil, err
	}
	st, err := parseStatus(statusOut)
	if err != nil {
		return nil, err
	}

	return &Detail{Pool: pools[0], Props: props, Status: st}, nil
}
```

Add at top of `pool.go`:
```go
import (
	"errors"
	"strings"
)

var ErrNotFound = errors.New("pool not found")
```

- [ ] **Step 6: Run tests, expect pass**

```bash
go test ./internal/host/zfs/pool/...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/host/zfs/pool/ test/fixtures/zpool_get.txt test/fixtures/zpool_status.txt
git commit -m "feat(host/zfs/pool): add Get with props and vdev status"
```

---

## Task 14: Pools handlers (GET /pools, GET /pools/:name)

**Files:**
- Create: `internal/api/handlers/pools.go`, `internal/api/handlers/pools_test.go`
- Modify: `internal/api/server.go`, `cmd/nova-api/main.go`

- [ ] **Step 1: Write failing test**

Create `internal/api/handlers/pools_test.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

type fakePoolMgr struct {
	list     []pool.Pool
	detail   *pool.Detail
	getErr   error
}

func (f *fakePoolMgr) List(_ context.Context) ([]pool.Pool, error) { return f.list, nil }
func (f *fakePoolMgr) Get(_ context.Context, _ string) (*pool.Detail, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.detail, nil
}

func TestPoolsList(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &PoolsHandler{Logger: logger, Pools: &fakePoolMgr{
		list: []pool.Pool{{Name: "tank", Health: "ONLINE"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []pool.Pool
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].Name != "tank" {
		t.Errorf("body=%+v", got)
	}
}

func TestPoolsGet_NotFound(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &PoolsHandler{Logger: logger, Pools: &fakePoolMgr{getErr: pool.ErrNotFound}}
	r := chi.NewRouter()
	r.Get("/api/v1/pools/{name}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/nope", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPoolsGet_Found(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &PoolsHandler{Logger: logger, Pools: &fakePoolMgr{
		detail: &pool.Detail{Pool: pool.Pool{Name: "tank"}},
	}}
	r := chi.NewRouter()
	r.Get("/api/v1/pools/{name}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pools/tank", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status=%d", rr.Code)
	}
}
```

- [ ] **Step 2: Run, expect fail**

```bash
go test ./internal/api/handlers/...
```
Expected: FAIL.

- [ ] **Step 3: Implement handler**

Create `internal/api/handlers/pools.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

type PoolManager interface {
	List(ctx context.Context) ([]pool.Pool, error)
	Get(ctx context.Context, name string) (*pool.Detail, error)
}

type PoolsHandler struct {
	Logger *slog.Logger
	Pools  PoolManager
}

func (h *PoolsHandler) List(w http.ResponseWriter, r *http.Request) {
	pools, err := h.Pools.List(r.Context())
	if err != nil {
		h.Logger.Error("pools list", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pools)
}

func (h *PoolsHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	d, err := h.Pools.Get(r.Context(), name)
	if err != nil {
		if errors.Is(err, pool.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "pool not found")
			return
		}
		h.Logger.Error("pools get", "err", err, "name", name)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}
```

- [ ] **Step 4: Wire into server**

Edit `internal/api/server.go`. Add to `Deps`:
```go
Pools handlers.PoolManager
```
Register routes in `New`:
```go
poolsH := &handlers.PoolsHandler{Logger: d.Logger, Pools: d.Pools}
r.Route("/api/v1", func(r chi.Router) {
    r.Get("/disks", disksH.List)
    r.Get("/pools", poolsH.List)
    r.Get("/pools/{name}", poolsH.Get)
})
```

Edit `cmd/nova-api/main.go`. Add:
```go
import "github.com/novanas/nova-nas/internal/host/zfs/pool"
...
poolMgr := &pool.Manager{ZpoolBin: cfg.ZpoolBin}
srv := api.New(api.Deps{
    Logger: logger,
    Store:  st,
    Disks:  disksLister,
    Pools:  poolMgr,
})
```

- [ ] **Step 5: Run tests, expect pass**

```bash
go test ./...
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/ cmd/
git commit -m "feat(api): add GET /pools and GET /pools/:name"
```

---

## Task 15: ZFS dataset — List

**Files:**
- Create: `internal/host/zfs/dataset/dataset.go`, `internal/host/zfs/dataset/parser.go`, `internal/host/zfs/dataset/parser_test.go`
- Create: `test/fixtures/zfs_list_datasets.txt`

- [ ] **Step 1: Capture fixture**

Create `test/fixtures/zfs_list_datasets.txt` (output of `zfs list -H -p -t filesystem,volume -o name,type,used,available,referenced,mountpoint,compression,recordsize`):
```
tank	filesystem	123456789	876543210	98765	/tank	lz4	131072
tank/home	filesystem	23456789	876543210	23456789	/tank/home	lz4	131072
tank/vol1	volume	1073741824	876543210	1073741824	-	lz4	-
```

- [ ] **Step 2: Write failing test**

Create `internal/host/zfs/dataset/parser_test.go`:
```go
package dataset

import (
	"os"
	"testing"
)

func TestParseList(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zfs_list_datasets.txt")
	if err != nil {
		t.Fatal(err)
	}
	ds, err := parseList(data)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}
	if len(ds) != 3 {
		t.Fatalf("want 3, got %d", len(ds))
	}
	if ds[0].Name != "tank" || ds[0].Type != "filesystem" || ds[0].UsedBytes != 123456789 {
		t.Errorf("ds[0]=%+v", ds[0])
	}
	if ds[2].Type != "volume" || ds[2].Mountpoint != "" {
		t.Errorf("ds[2]=%+v", ds[2])
	}
}
```

- [ ] **Step 3: Run, expect fail**

```bash
go test ./internal/host/zfs/dataset/...
```

- [ ] **Step 4: Implement parser and types**

Create `internal/host/zfs/dataset/parser.go`:
```go
// Package dataset wraps `zfs` for filesystem and volume datasets.
package dataset

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type Dataset struct {
	Name           string `json:"name"`
	Type           string `json:"type"` // filesystem|volume
	UsedBytes      uint64 `json:"usedBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	ReferencedBytes uint64 `json:"referencedBytes"`
	Mountpoint     string `json:"mountpoint,omitempty"`
	Compression    string `json:"compression,omitempty"`
	RecordSizeBytes uint64 `json:"recordSizeBytes,omitempty"`
}

func parseList(data []byte) ([]Dataset, error) {
	var out []Dataset
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 8 {
			return nil, fmt.Errorf("zfs list: 8 fields expected, got %d in %q", len(f), line)
		}
		d := Dataset{
			Name:        f[0],
			Type:        f[1],
			Compression: f[6],
		}
		var err error
		if d.UsedBytes, err = parseUint(f[2]); err != nil {
			return nil, err
		}
		if d.AvailableBytes, err = parseUint(f[3]); err != nil {
			return nil, err
		}
		if d.ReferencedBytes, err = parseUint(f[4]); err != nil {
			return nil, err
		}
		if f[5] != "-" {
			d.Mountpoint = f[5]
		}
		if d.RecordSizeBytes, err = parseUint(f[7]); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, sc.Err()
}

func parseUint(s string) (uint64, error) {
	if s == "-" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}
```

- [ ] **Step 5: Implement `List`**

Create `internal/host/zfs/dataset/dataset.go`:
```go
package dataset

import (
	"context"
	"errors"

	"github.com/novanas/nova-nas/internal/host/exec"
)

var ErrNotFound = errors.New("dataset not found")

type Manager struct {
	ZFSBin string
}

func (m *Manager) List(ctx context.Context, pool string) ([]Dataset, error) {
	args := []string{"list", "-H", "-p", "-t", "filesystem,volume",
		"-o", "name,type,used,available,referenced,mountpoint,compression,recordsize"}
	if pool != "" {
		args = append(args, "-r", pool)
	}
	out, err := exec.Run(ctx, m.ZFSBin, args...)
	if err != nil {
		return nil, err
	}
	return parseList(out)
}
```

- [ ] **Step 6: Run tests, expect pass**

```bash
go test ./internal/host/zfs/dataset/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/host/zfs/dataset/ test/fixtures/zfs_list_datasets.txt
git commit -m "feat(host/zfs/dataset): list filesystems and volumes"
```

---

## Task 16: ZFS dataset — Get (properties)

**Files:**
- Modify: `internal/host/zfs/dataset/parser.go`, `internal/host/zfs/dataset/dataset.go`
- Create: `internal/host/zfs/dataset/parser_get_test.go`
- Create: `test/fixtures/zfs_get.txt`

- [ ] **Step 1: Capture fixture**

Create `test/fixtures/zfs_get.txt` (output of `zfs get -H -p all tank/home`):
```
tank/home	type	filesystem	-
tank/home	creation	1714000000	-
tank/home	used	23456789	-
tank/home	available	876543210	-
tank/home	referenced	23456789	-
tank/home	compressratio	1.00x	-
tank/home	mounted	yes	-
tank/home	quota	0	default
tank/home	reservation	0	default
tank/home	recordsize	131072	default
tank/home	mountpoint	/tank/home	default
tank/home	sharenfs	off	default
tank/home	checksum	on	default
tank/home	compression	lz4	inherited from tank
tank/home	atime	on	default
tank/home	devices	on	default
tank/home	exec	on	default
tank/home	setuid	on	default
tank/home	readonly	off	default
tank/home	zoned	off	default
tank/home	snapdir	hidden	default
tank/home	aclmode	discard	default
tank/home	aclinherit	restricted	default
tank/home	createtxg	123	-
tank/home	canmount	on	default
```

- [ ] **Step 2: Write failing test**

Create `internal/host/zfs/dataset/parser_get_test.go`:
```go
package dataset

import (
	"os"
	"testing"
)

func TestParseProps(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zfs_get.txt")
	if err != nil {
		t.Fatal(err)
	}
	props, err := parseProps(data)
	if err != nil {
		t.Fatalf("parseProps: %v", err)
	}
	if props["compression"] != "lz4" {
		t.Errorf("compression=%q", props["compression"])
	}
	if props["mountpoint"] != "/tank/home" {
		t.Errorf("mountpoint=%q", props["mountpoint"])
	}
}
```

- [ ] **Step 3: Run, expect fail**

```bash
go test ./internal/host/zfs/dataset/...
```

- [ ] **Step 4: Implement `parseProps` and `Get`**

Append to `internal/host/zfs/dataset/parser.go`:
```go
func parseProps(data []byte) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			return nil, fmt.Errorf("zfs get: bad line %q", line)
		}
		out[f[1]] = f[2]
	}
	return out, sc.Err()
}
```

Append to `internal/host/zfs/dataset/dataset.go`:
```go
type Detail struct {
	Dataset Dataset           `json:"dataset"`
	Props   map[string]string `json:"properties"`
}

func (m *Manager) Get(ctx context.Context, name string) (*Detail, error) {
	listOut, err := exec.Run(ctx, m.ZFSBin, "list", "-H", "-p",
		"-t", "filesystem,volume",
		"-o", "name,type,used,available,referenced,mountpoint,compression,recordsize",
		name)
	if err != nil {
		var he *exec.HostError
		if errors.As(err, &he) && strings.Contains(he.Stderr, "does not exist") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ds, err := parseList(listOut)
	if err != nil {
		return nil, err
	}
	if len(ds) == 0 {
		return nil, ErrNotFound
	}
	propsOut, err := exec.Run(ctx, m.ZFSBin, "get", "-H", "-p", "all", name)
	if err != nil {
		return nil, err
	}
	props, err := parseProps(propsOut)
	if err != nil {
		return nil, err
	}
	return &Detail{Dataset: ds[0], Props: props}, nil
}
```
Add at top of `dataset.go`:
```go
import "strings"
```

- [ ] **Step 5: Run tests, expect pass**

```bash
go test ./internal/host/zfs/dataset/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/host/zfs/dataset/ test/fixtures/zfs_get.txt
git commit -m "feat(host/zfs/dataset): add Get with properties"
```

---

## Task 17: Datasets handlers (GET /datasets, GET /datasets/:fullname)

**Files:**
- Create: `internal/api/handlers/datasets.go`, `internal/api/handlers/datasets_test.go`
- Modify: `internal/api/server.go`, `cmd/nova-api/main.go`

- [ ] **Step 1: Write failing test**

Create `internal/api/handlers/datasets_test.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

type fakeDatasetMgr struct {
	list   []dataset.Dataset
	detail *dataset.Detail
	err    error
}

func (f *fakeDatasetMgr) List(_ context.Context, _ string) ([]dataset.Dataset, error) {
	return f.list, f.err
}
func (f *fakeDatasetMgr) Get(_ context.Context, _ string) (*dataset.Detail, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.detail, nil
}

func TestDatasetsList(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &DatasetsHandler{Logger: logger, Datasets: &fakeDatasetMgr{
		list: []dataset.Dataset{{Name: "tank/home", Type: "filesystem"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []dataset.Dataset
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].Name != "tank/home" {
		t.Errorf("body=%+v", got)
	}
}

func TestDatasetsGet_URLEncoded(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &DatasetsHandler{Logger: logger, Datasets: &fakeDatasetMgr{
		detail: &dataset.Detail{Dataset: dataset.Dataset{Name: "tank/home"}},
	}}
	r := chi.NewRouter()
	r.Get("/api/v1/datasets/{fullname}", h.Get)

	target := "/api/v1/datasets/" + url.PathEscape("tank/home")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 2: Run, expect fail**

```bash
go test ./internal/api/handlers/...
```

- [ ] **Step 3: Implement handler**

Create `internal/api/handlers/datasets.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

type DatasetManager interface {
	List(ctx context.Context, pool string) ([]dataset.Dataset, error)
	Get(ctx context.Context, name string) (*dataset.Detail, error)
}

type DatasetsHandler struct {
	Logger   *slog.Logger
	Datasets DatasetManager
}

func (h *DatasetsHandler) List(w http.ResponseWriter, r *http.Request) {
	pool := r.URL.Query().Get("pool")
	ds, err := h.Datasets.List(r.Context(), pool)
	if err != nil {
		h.Logger.Error("datasets list", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ds)
}

func (h *DatasetsHandler) Get(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid url-encoded name")
		return
	}
	d, err := h.Datasets.Get(r.Context(), name)
	if err != nil {
		if errors.Is(err, dataset.ErrNotFound) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "dataset not found")
			return
		}
		h.Logger.Error("datasets get", "err", err, "name", name)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}
```

- [ ] **Step 4: Wire into server**

Edit `internal/api/server.go`. Add to `Deps`:
```go
Datasets handlers.DatasetManager
```
Register in `New`:
```go
dsH := &handlers.DatasetsHandler{Logger: d.Logger, Datasets: d.Datasets}
r.Route("/api/v1", func(r chi.Router) {
    r.Get("/disks", disksH.List)
    r.Get("/pools", poolsH.List)
    r.Get("/pools/{name}", poolsH.Get)
    r.Get("/datasets", dsH.List)
    r.Get("/datasets/{fullname}", dsH.Get)
})
```

Edit `cmd/nova-api/main.go`. Add:
```go
import "github.com/novanas/nova-nas/internal/host/zfs/dataset"
...
datasetMgr := &dataset.Manager{ZFSBin: cfg.ZFSBin}
srv := api.New(api.Deps{
    Logger:   logger,
    Store:    st,
    Disks:    disksLister,
    Pools:    poolMgr,
    Datasets: datasetMgr,
})
```

- [ ] **Step 5: Run tests, expect pass**

```bash
go test ./...
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/ cmd/
git commit -m "feat(api): add GET /datasets and GET /datasets/:fullname"
```

---

## Task 18: ZFS snapshot — List

**Files:**
- Create: `internal/host/zfs/snapshot/snapshot.go`, `internal/host/zfs/snapshot/parser.go`, `internal/host/zfs/snapshot/parser_test.go`
- Create: `test/fixtures/zfs_snap_list.txt`

- [ ] **Step 1: Capture fixture**

Create `test/fixtures/zfs_snap_list.txt` (output of `zfs list -H -p -t snapshot -o name,used,referenced,creation`):
```
tank/home@daily-2026-04-27	12345	23456789	1714003200
tank/home@daily-2026-04-28	0	23456789	1714089600
tank/vol1@pre-upgrade	1024	1073741824	1713000000
```

- [ ] **Step 2: Write failing test**

Create `internal/host/zfs/snapshot/parser_test.go`:
```go
package snapshot

import (
	"os"
	"testing"
)

func TestParseList(t *testing.T) {
	data, err := os.ReadFile("../../../../test/fixtures/zfs_snap_list.txt")
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := parseList(data)
	if err != nil {
		t.Fatalf("parseList: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("want 3, got %d", len(snaps))
	}
	if snaps[0].Name != "tank/home@daily-2026-04-27" {
		t.Errorf("snap0=%+v", snaps[0])
	}
	if snaps[0].Dataset != "tank/home" || snaps[0].ShortName != "daily-2026-04-27" {
		t.Errorf("split wrong: %+v", snaps[0])
	}
	if snaps[2].Dataset != "tank/vol1" || snaps[2].ShortName != "pre-upgrade" {
		t.Errorf("split wrong: %+v", snaps[2])
	}
}
```

- [ ] **Step 3: Run, expect fail**

```bash
go test ./internal/host/zfs/snapshot/...
```

- [ ] **Step 4: Implement parser**

Create `internal/host/zfs/snapshot/parser.go`:
```go
// Package snapshot wraps `zfs` for snapshot operations.
package snapshot

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type Snapshot struct {
	Name           string `json:"name"`
	Dataset        string `json:"dataset"`
	ShortName      string `json:"shortName"`
	UsedBytes      uint64 `json:"usedBytes"`
	ReferencedBytes uint64 `json:"referencedBytes"`
	CreationUnix   int64  `json:"creationUnix"`
}

func parseList(data []byte) ([]Snapshot, error) {
	var out []Snapshot
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			return nil, fmt.Errorf("zfs snapshot list: 4 fields expected, got %d in %q", len(f), line)
		}
		s := Snapshot{Name: f[0]}
		at := strings.IndexByte(f[0], '@')
		if at <= 0 {
			return nil, fmt.Errorf("snapshot name missing '@': %q", f[0])
		}
		s.Dataset = f[0][:at]
		s.ShortName = f[0][at+1:]
		var err error
		if s.UsedBytes, err = parseUint(f[1]); err != nil {
			return nil, err
		}
		if s.ReferencedBytes, err = parseUint(f[2]); err != nil {
			return nil, err
		}
		if s.CreationUnix, err = strconv.ParseInt(f[3], 10, 64); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

func parseUint(s string) (uint64, error) {
	if s == "-" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}
```

- [ ] **Step 5: Implement `List`**

Create `internal/host/zfs/snapshot/snapshot.go`:
```go
package snapshot

import (
	"context"

	"github.com/novanas/nova-nas/internal/host/exec"
)

type Manager struct {
	ZFSBin string
}

func (m *Manager) List(ctx context.Context, dataset string) ([]Snapshot, error) {
	args := []string{"list", "-H", "-p", "-t", "snapshot",
		"-o", "name,used,referenced,creation"}
	if dataset != "" {
		args = append(args, "-r", dataset)
	}
	out, err := exec.Run(ctx, m.ZFSBin, args...)
	if err != nil {
		return nil, err
	}
	return parseList(out)
}
```

- [ ] **Step 6: Run tests, expect pass**

```bash
go test ./internal/host/zfs/snapshot/...
```

- [ ] **Step 7: Commit**

```bash
git add internal/host/zfs/snapshot/ test/fixtures/zfs_snap_list.txt
git commit -m "feat(host/zfs/snapshot): list snapshots"
```

---

## Task 19: Snapshots handler (GET /snapshots)

**Files:**
- Create: `internal/api/handlers/snapshots.go`, `internal/api/handlers/snapshots_test.go`
- Modify: `internal/api/server.go`, `cmd/nova-api/main.go`

- [ ] **Step 1: Write failing test**

Create `internal/api/handlers/snapshots_test.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
)

type fakeSnapMgr struct{ list []snapshot.Snapshot }

func (f *fakeSnapMgr) List(_ context.Context, _ string) ([]snapshot.Snapshot, error) {
	return f.list, nil
}

func TestSnapshotsList(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &SnapshotsHandler{Logger: logger, Snapshots: &fakeSnapMgr{
		list: []snapshot.Snapshot{{Name: "tank/home@a", Dataset: "tank/home", ShortName: "a"}},
	}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/snapshots", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []snapshot.Snapshot
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].Name != "tank/home@a" {
		t.Errorf("body=%+v", got)
	}
}
```

- [ ] **Step 2: Run, expect fail**

- [ ] **Step 3: Implement handler**

Create `internal/api/handlers/snapshots.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
)

type SnapshotManager interface {
	List(ctx context.Context, dataset string) ([]snapshot.Snapshot, error)
}

type SnapshotsHandler struct {
	Logger    *slog.Logger
	Snapshots SnapshotManager
}

func (h *SnapshotsHandler) List(w http.ResponseWriter, r *http.Request) {
	dataset := r.URL.Query().Get("dataset")
	snaps, err := h.Snapshots.List(r.Context(), dataset)
	if err != nil {
		h.Logger.Error("snapshots list", "err", err)
		middleware.WriteError(w, http.StatusInternalServerError, "host_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snaps)
}
```

- [ ] **Step 4: Wire into server**

Edit `internal/api/server.go`. Add to `Deps`:
```go
Snapshots handlers.SnapshotManager
```
Register in `New`:
```go
snapH := &handlers.SnapshotsHandler{Logger: d.Logger, Snapshots: d.Snapshots}
// inside Route("/api/v1"):
r.Get("/snapshots", snapH.List)
```

Edit `cmd/nova-api/main.go`:
```go
import "github.com/novanas/nova-nas/internal/host/zfs/snapshot"
...
snapMgr := &snapshot.Manager{ZFSBin: cfg.ZFSBin}
srv := api.New(api.Deps{
    Logger:    logger,
    Store:     st,
    Disks:     disksLister,
    Pools:     poolMgr,
    Datasets:  datasetMgr,
    Snapshots: snapMgr,
})
```

- [ ] **Step 5: Run tests, expect pass**

```bash
go test ./...
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/ cmd/
git commit -m "feat(api): add GET /snapshots"
```

---

## Task 20: Integration test harness

**Files:**
- Create: `test/integration/main_test.go`, `test/integration/readonly_test.go`

- [ ] **Step 1: Add testcontainers**

```bash
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
```

- [ ] **Step 2: Write the harness**

Create `test/integration/main_test.go`:
```go
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
```

- [ ] **Step 3: Write the read-only test**

Create `test/integration/readonly_test.go`:
```go
//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/novanas/nova-nas/internal/api"
	"github.com/novanas/nova-nas/internal/api/handlers"
	"github.com/novanas/nova-nas/internal/host/disks"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/store"
)

type stubDisks struct{}
func (stubDisks) List(_ context.Context) ([]disks.Disk, error) {
	return []disks.Disk{{Name: "sda", SizeBytes: 1000}}, nil
}

type stubPools struct{}
func (stubPools) List(_ context.Context) ([]pool.Pool, error) {
	return []pool.Pool{{Name: "tank", Health: "ONLINE"}}, nil
}
func (stubPools) Get(_ context.Context, name string) (*pool.Detail, error) {
	if name == "tank" {
		return &pool.Detail{Pool: pool.Pool{Name: "tank"}}, nil
	}
	return nil, pool.ErrNotFound
}

type stubDatasets struct{}
func (stubDatasets) List(_ context.Context, _ string) ([]dataset.Dataset, error) {
	return []dataset.Dataset{{Name: "tank/home", Type: "filesystem"}}, nil
}
func (stubDatasets) Get(_ context.Context, name string) (*dataset.Detail, error) {
	if name == "tank/home" {
		return &dataset.Detail{Dataset: dataset.Dataset{Name: "tank/home"}}, nil
	}
	return nil, dataset.ErrNotFound
}

type stubSnapshots struct{}
func (stubSnapshots) List(_ context.Context, _ string) ([]snapshot.Snapshot, error) {
	return []snapshot.Snapshot{{Name: "tank/home@a", Dataset: "tank/home", ShortName: "a"}}, nil
}

func TestReadOnlyEndpoints(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv := api.New(api.Deps{
		Logger:    logger,
		Store:     st,
		Disks:     handlers.DiskLister(stubDisks{}),
		Pools:     handlers.PoolManager(stubPools{}),
		Datasets:  handlers.DatasetManager(stubDatasets{}),
		Snapshots: handlers.SnapshotManager(stubSnapshots{}),
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		path string
		want int
	}{
		{"/healthz", http.StatusOK},
		{"/api/v1/disks", http.StatusOK},
		{"/api/v1/pools", http.StatusOK},
		{"/api/v1/pools/tank", http.StatusOK},
		{"/api/v1/pools/missing", http.StatusNotFound},
		{"/api/v1/datasets", http.StatusOK},
		{"/api/v1/datasets/" + url.PathEscape("tank/home"), http.StatusOK},
		{"/api/v1/datasets/" + url.PathEscape("missing"), http.StatusNotFound},
		{"/api/v1/snapshots", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + c.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != c.want {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("status=%d want=%d body=%s", resp.StatusCode, c.want, body)
			}
		})
	}

	// Sanity check JSON shape on /pools
	resp, _ := http.Get(ts.URL + "/api/v1/pools")
	var pools []pool.Pool
	_ = json.NewDecoder(resp.Body).Decode(&pools)
	if len(pools) != 1 || pools[0].Name != "tank" {
		t.Errorf("pools=%+v", pools)
	}
}
```

- [ ] **Step 4: Run integration tests**

```bash
go mod tidy
make test-integration
```
Expected: PASS. Requires Docker.

- [ ] **Step 5: Commit**

```bash
git add test/integration/ go.mod go.sum
git commit -m "test(integration): add read-only HTTP harness with testcontainers"
```

---

## Task 21: CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the workflow**

Create `.github/workflows/ci.yml`:
```yaml
name: ci
on:
  pull_request:
  push:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true
      - name: Build
        run: go build ./...
      - name: Unit tests
        run: go test -race ./...
      - name: Integration tests
        run: go test -race -tags=integration ./test/integration/...

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true
      - uses: golangci/golangci-lint-action@v6
        with:
          version: v1.59
```

- [ ] **Step 2: Commit**

```bash
git add .github/
git commit -m "ci: add GitHub Actions workflow for build/test/lint"
```

---

## Done

End state:
- `nova-api` boots, connects to Postgres, listens on `LISTEN_ADDR`.
- `GET /healthz` returns `{"status":"ok"}`.
- `GET /api/v1/disks` returns block devices.
- `GET /api/v1/pools` and `GET /api/v1/pools/{name}` return ZFS pool data.
- `GET /api/v1/datasets`, `GET /api/v1/datasets/{fullname}` return dataset data.
- `GET /api/v1/snapshots` returns snapshot data.
- Postgres schema for `jobs`, `audit_log`, `resource_metadata` is in place but only `resource_metadata` reads are wired.
- Unit + integration tests pass; CI runs both.

**Next:** Plan 2 covers the job system (Redis + asynq), all `POST/PATCH/DELETE` endpoints, audit-log middleware, metadata sub-routes, and SSE job streaming. Plan 3 covers OpenAPI codegen, e2e tests with real ZFS, deploy artifacts (systemd unit, .deb), and security hardening.
