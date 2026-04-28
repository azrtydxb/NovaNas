# Storage MVP — Plan 2: Job System + Write Endpoints

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Redis-backed asynq job system, full set of `POST/PATCH/DELETE` endpoints for pools/datasets/snapshots, audit-log middleware, metadata sub-routes, job streaming via SSE, and concurrency/recovery guarantees.

**Architecture:** Same Go binary; HTTP handler writes audit + job rows in a Postgres txn, enqueues an asynq task on Redis, returns 202 with `Location: /jobs/{id}`. Worker pool runs in the same binary, executes the host-ops call, updates the job row, publishes to Redis pub/sub for live streaming.

**Prerequisites:** Plan 1 complete and merged.

**Reference spec:** `docs/superpowers/specs/2026-04-28-novanas-storage-mvp-design.md` §3 (write endpoints), §5 (jobs/audit_log), §6 (job flow).

---

## File Structure (created in this plan)

```
internal/store/queries/{jobs,audit_log}.sql
internal/host/zfs/pool/write.go               (Create, Destroy, Scrub)
internal/host/zfs/dataset/write.go            (Create, SetProps, Destroy)
internal/host/zfs/snapshot/write.go           (Create, Destroy, Rollback)
internal/host/zfs/names/{names,names_test}.go (validators)
internal/jobs/{kind,dispatcher,worker,recover,sse}.go
internal/jobs/{kind_test,dispatcher_test}.go
internal/api/middleware/audit.go
internal/api/handlers/{pools_write,datasets_write,snapshots_write,jobs,metadata}.go
internal/api/handlers/*_test.go               (per handler)
test/integration/write_test.go
```

---

## Task 1: Add Redis to integration harness

**Files:** Modify `test/integration/main_test.go`

- [ ] **Step 1: Add testcontainers redis module**

```bash
go get github.com/testcontainers/testcontainers-go/modules/redis
```

- [ ] **Step 2: Spin Redis in TestMain**

Edit `test/integration/main_test.go`. Add to imports:
```go
tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
```
Add a package-level var:
```go
var redisAddr string
```
After Postgres startup, before `os.Exit(m.Run())`:
```go
rc, err := tcredis.Run(ctx, "redis:7-alpine")
if err != nil {
    fmt.Fprintln(os.Stderr, "redis start:", err)
    os.Exit(1)
}
defer func() { _ = rc.Terminate(ctx) }()
endpoint, err := rc.Endpoint(ctx, "")
if err != nil {
    fmt.Fprintln(os.Stderr, "redis endpoint:", err)
    os.Exit(1)
}
redisAddr = endpoint
```

- [ ] **Step 3: Verify**

```bash
make test-integration
```
Expected: existing read-only test still passes; new Redis container starts.

- [ ] **Step 4: Commit**

```bash
git add test/integration/main_test.go go.mod go.sum
git commit -m "test(integration): add Redis container to harness"
```

---

## Task 2: Add asynq + redis client

**Files:** `go.mod`, `go.sum`

- [ ] **Step 1: Install**

```bash
go get github.com/hibiken/asynq
go get github.com/redis/go-redis/v9
go mod tidy
```

- [ ] **Step 2: Extend config**

Edit `internal/config/config.go` to add Redis URL:
```go
RedisURL string `envconfig:"REDIS_URL" required:"true"`
```

Edit `internal/config/config_test.go`. Update existing tests to set `t.Setenv("REDIS_URL", "redis://localhost:6379/0")` and add an assertion `if cfg.RedisURL != "redis://localhost:6379/0" {...}`.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/config/...
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat(config): add REDIS_URL"
```

---

## Task 3: Sqlc queries for jobs and audit_log

**Files:** Create `internal/store/queries/jobs.sql`, `internal/store/queries/audit_log.sql`. Re-run sqlc.

- [ ] **Step 1: Write `jobs.sql`**

Create `internal/store/queries/jobs.sql`:
```sql
-- name: InsertJob :one
INSERT INTO jobs (id, kind, target, state, command, request_id)
VALUES ($1, $2, $3, 'queued', $4, $5)
RETURNING *;

-- name: GetJob :one
SELECT * FROM jobs WHERE id = $1;

-- name: ListJobs :many
SELECT * FROM jobs
WHERE (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state'))
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: MarkJobRunning :exec
UPDATE jobs
   SET state = 'running', started_at = now()
 WHERE id = $1 AND state IN ('queued','interrupted');

-- name: MarkJobFinished :exec
UPDATE jobs
   SET state = $2,
       stdout = $3,
       stderr = $4,
       exit_code = $5,
       error = $6,
       finished_at = now()
 WHERE id = $1;

-- name: MarkRunningInterrupted :exec
UPDATE jobs
   SET state = 'interrupted',
       error = 'process restarted'
 WHERE state IN ('queued','running');

-- name: CancelJob :exec
UPDATE jobs
   SET state = 'cancelled', finished_at = now()
 WHERE id = $1 AND state IN ('queued','running');
```

- [ ] **Step 2: Write `audit_log.sql`**

Create `internal/store/queries/audit_log.sql`:
```sql
-- name: InsertAudit :exec
INSERT INTO audit_log (actor, action, target, request_id, payload, result)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListAudit :many
SELECT * FROM audit_log
ORDER BY ts DESC
LIMIT $1 OFFSET $2;
```

- [ ] **Step 3: Generate**

```bash
sqlc generate
go build ./...
```
Expected: new files appear in `internal/store/gen/`.

- [ ] **Step 4: Commit**

```bash
git add internal/store/
git commit -m "feat(store): add sqlc queries for jobs and audit_log"
```

---

## Task 4: ZFS naming validators

**Files:** Create `internal/host/zfs/names/names.go`, `internal/host/zfs/names/names_test.go`.

- [ ] **Step 1: Write failing tests**

Create `internal/host/zfs/names/names_test.go`:
```go
package names

import "testing"

func TestValidatePoolName(t *testing.T) {
	good := []string{"tank", "ssd", "p1", "Pool-A", "tank_01"}
	bad := []string{"", "tank/x", "tank@x", "1tank", "log", "mirror", strings("a", 256)}
	for _, n := range good {
		if err := ValidatePoolName(n); err != nil {
			t.Errorf("good %q: %v", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidatePoolName(n); err == nil {
			t.Errorf("bad %q: expected error", n)
		}
	}
}

func TestValidateDatasetName(t *testing.T) {
	good := []string{"tank/home", "tank/home/alice", "tank"}
	bad := []string{"", "/tank", "tank/", "tank//x", "tank@snap", "tank/-leadingdash"}
	for _, n := range good {
		if err := ValidateDatasetName(n); err != nil {
			t.Errorf("good %q: %v", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateDatasetName(n); err == nil {
			t.Errorf("bad %q: expected error", n)
		}
	}
}

func TestValidateSnapshotName(t *testing.T) {
	good := []string{"tank@a", "tank/home@daily-2026-04-27", "p/v@x_1"}
	bad := []string{"", "tank", "tank@", "@x", "tank/@x", "tank@x@y"}
	for _, n := range good {
		if err := ValidateSnapshotName(n); err != nil {
			t.Errorf("good %q: %v", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateSnapshotName(n); err == nil {
			t.Errorf("bad %q: expected error", n)
		}
	}
}

func strings(s string, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = s[0]
	}
	return string(out)
}
```

- [ ] **Step 2: Run, expect fail**

```bash
go test ./internal/host/zfs/names/...
```

- [ ] **Step 3: Implement validators**

Create `internal/host/zfs/names/names.go`:
```go
// Package names validates ZFS pool, dataset, and snapshot names.
package names

import (
	"fmt"
	"strings"
)

const maxNameLen = 255

var reservedPoolNames = map[string]struct{}{
	"mirror": {}, "raidz": {}, "raidz1": {}, "raidz2": {}, "raidz3": {},
	"draid": {}, "spare": {}, "log": {}, "cache": {}, "special": {},
}

func ValidatePoolName(s string) error {
	if s == "" {
		return fmt.Errorf("pool name empty")
	}
	if len(s) > maxNameLen {
		return fmt.Errorf("pool name too long")
	}
	if _, bad := reservedPoolNames[s]; bad {
		return fmt.Errorf("pool name %q is reserved", s)
	}
	if !isAlpha(rune(s[0])) {
		return fmt.Errorf("pool name must start with a letter")
	}
	for _, r := range s {
		if !isAllowedComponent(r) {
			return fmt.Errorf("pool name has illegal character %q", r)
		}
	}
	return nil
}

func ValidateDatasetName(s string) error {
	if s == "" {
		return fmt.Errorf("dataset name empty")
	}
	if len(s) > maxNameLen {
		return fmt.Errorf("dataset name too long")
	}
	if strings.Contains(s, "@") {
		return fmt.Errorf("dataset name cannot contain '@'")
	}
	parts := strings.Split(s, "/")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("dataset name must start with pool")
	}
	if err := ValidatePoolName(parts[0]); err != nil {
		return fmt.Errorf("invalid pool component: %w", err)
	}
	for _, p := range parts[1:] {
		if p == "" {
			return fmt.Errorf("dataset name has empty component")
		}
		if strings.HasPrefix(p, "-") {
			return fmt.Errorf("dataset component cannot start with '-'")
		}
		for _, r := range p {
			if !isAllowedComponent(r) {
				return fmt.Errorf("dataset component has illegal character %q", r)
			}
		}
	}
	return nil
}

func ValidateSnapshotName(s string) error {
	at := strings.IndexByte(s, '@')
	if at <= 0 {
		return fmt.Errorf("snapshot name must contain '<dataset>@<short>'")
	}
	if strings.Count(s, "@") != 1 {
		return fmt.Errorf("snapshot name must contain exactly one '@'")
	}
	if err := ValidateDatasetName(s[:at]); err != nil {
		return err
	}
	short := s[at+1:]
	if short == "" {
		return fmt.Errorf("snapshot short name empty")
	}
	for _, r := range short {
		if !isAllowedComponent(r) {
			return fmt.Errorf("snapshot short name has illegal character %q", r)
		}
	}
	return nil
}

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isAllowedComponent(r rune) bool {
	return isAlpha(r) || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == ':'
}
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./internal/host/zfs/names/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/host/zfs/names/
git commit -m "feat(host/zfs/names): add pool/dataset/snapshot name validators"
```

---

## Task 5: Pool write ops (Create, Destroy, Scrub)

**Files:** Create `internal/host/zfs/pool/write.go`, `internal/host/zfs/pool/write_test.go`.

- [ ] **Step 1: Write failing test**

Create `internal/host/zfs/pool/write_test.go`:
```go
package pool

import "testing"

func TestBuildCreateArgs_Mirror(t *testing.T) {
	spec := CreateSpec{
		Name: "tank",
		Vdevs: []VdevSpec{{
			Type: "mirror",
			Disks: []string{"/dev/disk/by-id/wwn-0xA", "/dev/disk/by-id/wwn-0xB"},
		}},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "-f", "tank", "mirror",
		"/dev/disk/by-id/wwn-0xA", "/dev/disk/by-id/wwn-0xB"}
	if !equal(args, want) {
		t.Errorf("args=%v want=%v", args, want)
	}
}

func TestBuildCreateArgs_RaidzPlusLog(t *testing.T) {
	spec := CreateSpec{
		Name: "tank",
		Vdevs: []VdevSpec{{
			Type: "raidz1",
			Disks: []string{"/dev/A", "/dev/B", "/dev/C"},
		}},
		Log:   []string{"/dev/log1"},
		Cache: []string{"/dev/cache1"},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create", "-f", "tank",
		"raidz1", "/dev/A", "/dev/B", "/dev/C",
		"log", "/dev/log1",
		"cache", "/dev/cache1"}
	if !equal(args, want) {
		t.Errorf("args=%v want=%v", args, want)
	}
}

func TestBuildCreateArgs_BadVdevType(t *testing.T) {
	spec := CreateSpec{Name: "tank", Vdevs: []VdevSpec{{Type: "bogus", Disks: []string{"/dev/a"}}}}
	if _, err := buildCreateArgs(spec); err == nil {
		t.Error("expected error")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run, expect fail**

```bash
go test ./internal/host/zfs/pool/...
```

- [ ] **Step 3: Implement write ops**

Create `internal/host/zfs/pool/write.go`:
```go
package pool

import (
	"context"
	"fmt"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

type CreateSpec struct {
	Name  string     `json:"name"`
	Vdevs []VdevSpec `json:"vdevs"`
	Log   []string   `json:"log,omitempty"`
	Cache []string   `json:"cache,omitempty"`
	Spare []string   `json:"spare,omitempty"`
}

type VdevSpec struct {
	Type  string   `json:"type"` // mirror|raidz1|raidz2|raidz3|stripe
	Disks []string `json:"disks"`
}

var validVdevTypes = map[string]struct{}{
	"mirror": {}, "raidz1": {}, "raidz2": {}, "raidz3": {}, "stripe": {},
}

func buildCreateArgs(spec CreateSpec) ([]string, error) {
	if err := names.ValidatePoolName(spec.Name); err != nil {
		return nil, err
	}
	if len(spec.Vdevs) == 0 {
		return nil, fmt.Errorf("pool create: at least one vdev required")
	}
	args := []string{"create", "-f", spec.Name}
	for _, v := range spec.Vdevs {
		if _, ok := validVdevTypes[v.Type]; !ok {
			return nil, fmt.Errorf("invalid vdev type %q", v.Type)
		}
		if len(v.Disks) == 0 {
			return nil, fmt.Errorf("vdev %q has no disks", v.Type)
		}
		if v.Type != "stripe" {
			args = append(args, v.Type)
		}
		args = append(args, v.Disks...)
	}
	if len(spec.Log) > 0 {
		args = append(args, "log")
		args = append(args, spec.Log...)
	}
	if len(spec.Cache) > 0 {
		args = append(args, "cache")
		args = append(args, spec.Cache...)
	}
	if len(spec.Spare) > 0 {
		args = append(args, "spare")
		args = append(args, spec.Spare...)
	}
	return args, nil
}

func (m *Manager) Create(ctx context.Context, spec CreateSpec) error {
	args, err := buildCreateArgs(spec)
	if err != nil {
		return err
	}
	_, err = exec.Run(ctx, m.ZpoolBin, args...)
	return err
}

func (m *Manager) Destroy(ctx context.Context, name string) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	_, err := exec.Run(ctx, m.ZpoolBin, "destroy", "-f", name)
	return err
}

type ScrubAction string

const (
	ScrubStart ScrubAction = "start"
	ScrubStop  ScrubAction = "stop"
)

func (m *Manager) Scrub(ctx context.Context, name string, action ScrubAction) error {
	if err := names.ValidatePoolName(name); err != nil {
		return err
	}
	args := []string{"scrub"}
	if action == ScrubStop {
		args = append(args, "-s")
	}
	args = append(args, name)
	_, err := exec.Run(ctx, m.ZpoolBin, args...)
	return err
}
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./internal/host/zfs/pool/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/host/zfs/pool/
git commit -m "feat(host/zfs/pool): add Create/Destroy/Scrub"
```

---

## Task 6: Dataset write ops

**Files:** Create `internal/host/zfs/dataset/write.go`, `internal/host/zfs/dataset/write_test.go`.

- [ ] **Step 1: Write failing test**

Create `internal/host/zfs/dataset/write_test.go`:
```go
package dataset

import "testing"

func TestBuildCreateArgs_Filesystem(t *testing.T) {
	spec := CreateSpec{
		Parent: "tank",
		Name:   "home",
		Type:   "filesystem",
		Properties: map[string]string{
			"compression": "lz4",
			"recordsize":  "128K",
		},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	// "create -o compression=lz4 -o recordsize=128K tank/home"
	if args[0] != "create" {
		t.Errorf("args[0]=%q", args[0])
	}
	if args[len(args)-1] != "tank/home" {
		t.Errorf("last arg=%q", args[len(args)-1])
	}
}

func TestBuildCreateArgs_Volume(t *testing.T) {
	spec := CreateSpec{
		Parent: "tank",
		Name:   "vol1",
		Type:   "volume",
		VolumeSizeBytes: 1 << 30,
		Properties: map[string]string{"compression": "off"},
	}
	args, err := buildCreateArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	// expect "-V <size>" present
	saw := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-V" && args[i+1] == "1073741824" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("missing -V; args=%v", args)
	}
}

func TestBuildCreateArgs_RejectBadName(t *testing.T) {
	spec := CreateSpec{Parent: "tank", Name: "bad@name", Type: "filesystem"}
	if _, err := buildCreateArgs(spec); err == nil {
		t.Error("expected error")
	}
}
```

- [ ] **Step 2: Run, expect fail**

```bash
go test ./internal/host/zfs/dataset/...
```

- [ ] **Step 3: Implement**

Create `internal/host/zfs/dataset/write.go`:
```go
package dataset

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

type CreateSpec struct {
	Parent          string            `json:"parent"`
	Name            string            `json:"name"`
	Type            string            `json:"type"` // filesystem|volume
	VolumeSizeBytes uint64            `json:"volumeSizeBytes,omitempty"`
	Properties      map[string]string `json:"properties,omitempty"`
}

func buildCreateArgs(spec CreateSpec) ([]string, error) {
	if spec.Type != "filesystem" && spec.Type != "volume" {
		return nil, fmt.Errorf("invalid dataset type %q", spec.Type)
	}
	full := spec.Parent + "/" + spec.Name
	if err := names.ValidateDatasetName(full); err != nil {
		return nil, err
	}
	args := []string{"create"}
	keys := make([]string, 0, len(spec.Properties))
	for k := range spec.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-o", k+"="+spec.Properties[k])
	}
	if spec.Type == "volume" {
		if spec.VolumeSizeBytes == 0 {
			return nil, fmt.Errorf("volume requires volumeSizeBytes")
		}
		args = append(args, "-V", strconv.FormatUint(spec.VolumeSizeBytes, 10))
	}
	args = append(args, full)
	return args, nil
}

func (m *Manager) Create(ctx context.Context, spec CreateSpec) error {
	args, err := buildCreateArgs(spec)
	if err != nil {
		return err
	}
	_, err = exec.Run(ctx, m.ZFSBin, args...)
	return err
}

func (m *Manager) SetProps(ctx context.Context, name string, props map[string]string) error {
	if err := names.ValidateDatasetName(name); err != nil {
		return err
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := exec.Run(ctx, m.ZFSBin, "set", k+"="+props[k], name); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Destroy(ctx context.Context, name string, recursive bool) error {
	if err := names.ValidateDatasetName(name); err != nil {
		return err
	}
	args := []string{"destroy"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, name)
	_, err := exec.Run(ctx, m.ZFSBin, args...)
	return err
}
```

- [ ] **Step 4: Run tests, expect pass**

- [ ] **Step 5: Commit**

```bash
git add internal/host/zfs/dataset/
git commit -m "feat(host/zfs/dataset): add Create/SetProps/Destroy"
```

---

## Task 7: Snapshot write ops

**Files:** Create `internal/host/zfs/snapshot/write.go`, `internal/host/zfs/snapshot/write_test.go`.

- [ ] **Step 1: Write failing test**

Create `internal/host/zfs/snapshot/write_test.go`:
```go
package snapshot

import "testing"

func TestBuildCreateArgs(t *testing.T) {
	args, err := buildCreateArgs("tank/home", "daily-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "snapshot" || args[1] != "tank/home@daily-1" {
		t.Errorf("args=%v", args)
	}
}

func TestBuildCreateArgs_Recursive(t *testing.T) {
	args, _ := buildCreateArgs("tank", "daily-1", true)
	if len(args) != 3 || args[1] != "-r" {
		t.Errorf("args=%v", args)
	}
}

func TestBuildCreateArgs_BadDataset(t *testing.T) {
	if _, err := buildCreateArgs("", "x", false); err == nil {
		t.Error("expected error")
	}
}

func TestBuildCreateArgs_BadShortName(t *testing.T) {
	if _, err := buildCreateArgs("tank", "bad@name", false); err == nil {
		t.Error("expected error")
	}
}
```

- [ ] **Step 2: Run, expect fail**

- [ ] **Step 3: Implement**

Create `internal/host/zfs/snapshot/write.go`:
```go
package snapshot

import (
	"context"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

func buildCreateArgs(dataset, short string, recursive bool) ([]string, error) {
	full := dataset + "@" + short
	if err := names.ValidateSnapshotName(full); err != nil {
		return nil, err
	}
	args := []string{"snapshot"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, full)
	return args, nil
}

func (m *Manager) Create(ctx context.Context, dataset, short string, recursive bool) error {
	args, err := buildCreateArgs(dataset, short, recursive)
	if err != nil {
		return err
	}
	_, err = exec.Run(ctx, m.ZFSBin, args...)
	return err
}

func (m *Manager) Destroy(ctx context.Context, name string) error {
	if err := names.ValidateSnapshotName(name); err != nil {
		return err
	}
	_, err := exec.Run(ctx, m.ZFSBin, "destroy", name)
	return err
}

func (m *Manager) Rollback(ctx context.Context, snapshot string) error {
	if err := names.ValidateSnapshotName(snapshot); err != nil {
		return err
	}
	_, err := exec.Run(ctx, m.ZFSBin, "rollback", "-r", snapshot)
	return err
}
```

- [ ] **Step 4: Run tests, expect pass**

- [ ] **Step 5: Commit**

```bash
git add internal/host/zfs/snapshot/
git commit -m "feat(host/zfs/snapshot): add Create/Destroy/Rollback"
```

---

## Task 8: Job kinds + payload types

**Files:** Create `internal/jobs/kind.go`, `internal/jobs/kind_test.go`.

- [ ] **Step 1: Write failing test**

Create `internal/jobs/kind_test.go`:
```go
package jobs

import (
	"encoding/json"
	"testing"
)

func TestPoolCreatePayload_Roundtrip(t *testing.T) {
	in := PoolCreatePayload{Name: "tank"}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out PoolCreatePayload
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "tank" {
		t.Errorf("got %+v", out)
	}
}

func TestKindString(t *testing.T) {
	if KindPoolCreate != "pool.create" {
		t.Errorf("KindPoolCreate=%q", KindPoolCreate)
	}
}
```

- [ ] **Step 2: Implement**

Create `internal/jobs/kind.go`:
```go
// Package jobs defines async task types and the dispatch/worker plumbing.
package jobs

import (
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

type Kind string

const (
	KindPoolCreate    Kind = "pool.create"
	KindPoolDestroy   Kind = "pool.destroy"
	KindPoolScrub     Kind = "pool.scrub"
	KindDatasetCreate Kind = "dataset.create"
	KindDatasetSet    Kind = "dataset.set"
	KindDatasetDestroy Kind = "dataset.destroy"
	KindSnapshotCreate  Kind = "snapshot.create"
	KindSnapshotDestroy Kind = "snapshot.destroy"
	KindSnapshotRollback Kind = "snapshot.rollback"
)

type PoolCreatePayload struct {
	Name  string          `json:"name"`
	Spec  pool.CreateSpec `json:"spec"`
}

type PoolDestroyPayload struct {
	Name string `json:"name"`
}

type PoolScrubPayload struct {
	Name   string           `json:"name"`
	Action pool.ScrubAction `json:"action"`
}

type DatasetCreatePayload struct {
	Spec dataset.CreateSpec `json:"spec"`
}

type DatasetSetPayload struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

type DatasetDestroyPayload struct {
	Name      string `json:"name"`
	Recursive bool   `json:"recursive"`
}

type SnapshotCreatePayload struct {
	Dataset   string `json:"dataset"`
	ShortName string `json:"shortName"`
	Recursive bool   `json:"recursive"`
}

type SnapshotDestroyPayload struct {
	Name string `json:"name"`
}

type SnapshotRollbackPayload struct {
	Snapshot string `json:"snapshot"`
}
```

- [ ] **Step 3: Run tests, expect pass; commit**

```bash
go test ./internal/jobs/...
git add internal/jobs/
git commit -m "feat(jobs): define task kinds and payload types"
```

---

## Task 9: Job dispatcher

**Files:** Create `internal/jobs/dispatcher.go`, `internal/jobs/dispatcher_test.go`.

- [ ] **Step 1: Write failing test (integration-style, uses Postgres + Redis)**

Create `internal/jobs/dispatcher_test.go`:
```go
//go:build integration

package jobs_test

// This test file is integration-tagged because it needs Redis + Postgres.
// We rely on the test/integration harness for those resources.
```
The actual dispatcher test lives under `test/integration/` (Task 17). For this task, write a non-integration test for the JSON encoding only:

Create `internal/jobs/dispatcher_unit_test.go`:
```go
package jobs

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestEncodeTask(t *testing.T) {
	id := uuid.New()
	payload, err := json.Marshal(PoolDestroyPayload{Name: "tank"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := encodeTaskBody(id, payload)
	if err != nil {
		t.Fatal(err)
	}
	var got TaskBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.JobID != id.String() {
		t.Errorf("jobID=%q", got.JobID)
	}
	if string(got.Payload) != `{"name":"tank"}` {
		t.Errorf("payload=%q", got.Payload)
	}
}
```

- [ ] **Step 2: Add uuid**

```bash
go get github.com/google/uuid
```

- [ ] **Step 3: Implement dispatcher**

Create `internal/jobs/dispatcher.go`:
```go
package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type TaskBody struct {
	JobID   string          `json:"jobId"`
	Payload json.RawMessage `json:"payload"`
}

func encodeTaskBody(jobID uuid.UUID, payload json.RawMessage) ([]byte, error) {
	return json.Marshal(TaskBody{JobID: jobID.String(), Payload: payload})
}

type Dispatcher struct {
	Client  *asynq.Client
	Queries *storedb.Queries
	DB      pgx.Tx // not used for state — Dispatch starts its own txn via Pool
	Pool    PoolBeginner
}

type PoolBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type DispatchInput struct {
	Kind      Kind
	Target    string
	Payload   any
	Command   string // human-readable cmd preview for the jobs row
	RequestID string
	UniqueKey string // optional; if set, asynq enforces uniqueness
}

type DispatchOutput struct {
	JobID uuid.UUID
}

func (d *Dispatcher) Dispatch(ctx context.Context, in DispatchInput) (DispatchOutput, error) {
	jobID := uuid.New()

	payloadBytes, err := json.Marshal(in.Payload)
	if err != nil {
		return DispatchOutput{}, fmt.Errorf("marshal payload: %w", err)
	}

	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return DispatchOutput{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := d.Queries.WithTx(tx)

	if _, err := q.InsertJob(ctx, storedb.InsertJobParams{
		ID:        jobID,
		Kind:      string(in.Kind),
		Target:    in.Target,
		Command:   in.Command,
		RequestID: in.RequestID,
	}); err != nil {
		return DispatchOutput{}, err
	}

	body, err := encodeTaskBody(jobID, payloadBytes)
	if err != nil {
		return DispatchOutput{}, err
	}

	opts := []asynq.Option{asynq.MaxRetry(0)}
	if in.UniqueKey != "" {
		opts = append(opts, asynq.Unique(0))
		opts = append(opts, asynq.TaskID(in.UniqueKey))
	}
	task := asynq.NewTask(string(in.Kind), body, opts...)
	if _, err := d.Client.EnqueueContext(ctx, task); err != nil {
		return DispatchOutput{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DispatchOutput{}, err
	}
	return DispatchOutput{JobID: jobID}, nil
}
```

Note: `d.DB` field is unused; remove from struct. Final struct:
```go
type Dispatcher struct {
	Client  *asynq.Client
	Queries *storedb.Queries
	Pool    PoolBeginner
}
```

- [ ] **Step 4: Run unit test, expect pass**

```bash
go mod tidy
go test ./internal/jobs/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/ go.mod go.sum
git commit -m "feat(jobs): add dispatcher (jobs row + asynq enqueue in one txn)"
```

---

## Task 10: Worker

**Files:** Create `internal/jobs/worker.go`, `internal/jobs/recover.go`.

- [ ] **Step 1: Implement worker**

Create `internal/jobs/worker.go`:
```go
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type WorkerDeps struct {
	Logger    *slog.Logger
	Queries   *storedb.Queries
	Redis     *redis.Client
	Pools     *pool.Manager
	Datasets  *dataset.Manager
	Snapshots *snapshot.Manager
}

func NewServeMux(d WorkerDeps) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(string(KindPoolCreate), d.handlePoolCreate)
	mux.HandleFunc(string(KindPoolDestroy), d.handlePoolDestroy)
	mux.HandleFunc(string(KindPoolScrub), d.handlePoolScrub)
	mux.HandleFunc(string(KindDatasetCreate), d.handleDatasetCreate)
	mux.HandleFunc(string(KindDatasetSet), d.handleDatasetSet)
	mux.HandleFunc(string(KindDatasetDestroy), d.handleDatasetDestroy)
	mux.HandleFunc(string(KindSnapshotCreate), d.handleSnapshotCreate)
	mux.HandleFunc(string(KindSnapshotDestroy), d.handleSnapshotDestroy)
	mux.HandleFunc(string(KindSnapshotRollback), d.handleSnapshotRollback)
	return mux
}

func (d WorkerDeps) decode(t *asynq.Task, payload any) (uuid.UUID, error) {
	var body TaskBody
	if err := json.Unmarshal(t.Payload(), &body); err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.Parse(body.JobID)
	if err != nil {
		return uuid.Nil, err
	}
	if err := json.Unmarshal(body.Payload, payload); err != nil {
		return id, err
	}
	return id, nil
}

func (d WorkerDeps) markRunning(ctx context.Context, id uuid.UUID) error {
	return d.Queries.MarkJobRunning(ctx, id)
}

func (d WorkerDeps) finish(ctx context.Context, id uuid.UUID, runErr error) {
	state := "succeeded"
	stderr := ""
	stdout := ""
	exitCode := pgtype.Int4{}
	errMsg := pgtype.Text{}
	if runErr != nil {
		state = "failed"
		errMsg = pgtype.Text{String: runErr.Error(), Valid: true}
		var he *exec.HostError
		if errors.As(runErr, &he) {
			stderr = he.Stderr
			exitCode = pgtype.Int4{Int32: int32(he.ExitCode), Valid: true}
		}
	}
	_ = d.Queries.MarkJobFinished(ctx, storedb.MarkJobFinishedParams{
		ID:       id,
		State:    state,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Error:    errMsg,
	})
	_ = d.Redis.Publish(ctx, "job:"+id.String()+":update", state).Err()
}

func (d WorkerDeps) handlePoolCreate(ctx context.Context, t *asynq.Task) error {
	var p PoolCreatePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Create(ctx, p.Spec)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolDestroy(ctx context.Context, t *asynq.Task) error {
	var p PoolDestroyPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Destroy(ctx, p.Name)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolScrub(ctx context.Context, t *asynq.Task) error {
	var p PoolScrubPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Scrub(ctx, p.Name, p.Action)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetCreate(ctx context.Context, t *asynq.Task) error {
	var p DatasetCreatePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.Create(ctx, p.Spec)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetSet(ctx context.Context, t *asynq.Task) error {
	var p DatasetSetPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.SetProps(ctx, p.Name, p.Properties)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetDestroy(ctx context.Context, t *asynq.Task) error {
	var p DatasetDestroyPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.Destroy(ctx, p.Name, p.Recursive)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleSnapshotCreate(ctx context.Context, t *asynq.Task) error {
	var p SnapshotCreatePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Snapshots.Create(ctx, p.Dataset, p.ShortName, p.Recursive)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleSnapshotDestroy(ctx context.Context, t *asynq.Task) error {
	var p SnapshotDestroyPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Snapshots.Destroy(ctx, p.Name)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleSnapshotRollback(ctx context.Context, t *asynq.Task) error {
	var p SnapshotRollbackPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Snapshots.Rollback(ctx, p.Snapshot)
	d.finish(ctx, id, err)
	return err
}
```

- [ ] **Step 2: Implement startup recovery**

Create `internal/jobs/recover.go`:
```go
package jobs

import (
	"context"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

func MarkInterruptedAtStartup(ctx context.Context, q *storedb.Queries) error {
	return q.MarkRunningInterrupted(ctx)
}
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/jobs/
git commit -m "feat(jobs): add asynq worker handlers and crash recovery"
```

---

## Task 11: Wire dispatcher + worker into main

**Files:** Modify `cmd/nova-api/main.go`, `internal/api/server.go`.

- [ ] **Step 1: Update server Deps**

Edit `internal/api/server.go` to add:
```go
Dispatcher *jobs.Dispatcher
Redis      *redis.Client
```
Imports:
```go
"github.com/redis/go-redis/v9"
"github.com/novanas/nova-nas/internal/jobs"
```

- [ ] **Step 2: Update main.go**

Edit `cmd/nova-api/main.go` after store is opened, before `srv := api.New`:
```go
import (
    "github.com/hibiken/asynq"
    "github.com/redis/go-redis/v9"
    "github.com/novanas/nova-nas/internal/jobs"
    "github.com/novanas/nova-nas/internal/host/zfs/snapshot"
    storedb "github.com/novanas/nova-nas/internal/store/gen"
)
...
redisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
if err != nil {
    logger.Error("redis parse", "err", err)
    os.Exit(1)
}
asyncClient := asynq.NewClient(redisOpt)
defer asyncClient.Close()

redisOpts, err := redis.ParseURL(cfg.RedisURL)
if err != nil {
    logger.Error("redis url", "err", err)
    os.Exit(1)
}
redisClient := redis.NewClient(redisOpts)
defer redisClient.Close()

dispatcher := &jobs.Dispatcher{
    Client:  asyncClient,
    Queries: st.Queries,
    Pool:    st.Pool,
}

if err := jobs.MarkInterruptedAtStartup(ctx, st.Queries); err != nil {
    logger.Error("recovery", "err", err)
}

asyncSrv := asynq.NewServer(redisOpt, asynq.Config{
    Concurrency: 4,
    Logger:      asynqSlogAdapter{logger},
})
mux := jobs.NewServeMux(jobs.WorkerDeps{
    Logger:    logger,
    Queries:   st.Queries,
    Redis:     redisClient,
    Pools:     poolMgr,
    Datasets:  datasetMgr,
    Snapshots: snapMgr,
})
go func() {
    if err := asyncSrv.Run(mux); err != nil {
        logger.Error("asynq", "err", err)
    }
}()
defer asyncSrv.Stop()

srv := api.New(api.Deps{
    Logger:     logger,
    Store:      st,
    Disks:      disksLister,
    Pools:      poolMgr,
    Datasets:   datasetMgr,
    Snapshots:  snapMgr,
    Dispatcher: dispatcher,
    Redis:      redisClient,
})
```

Define a small slog adapter at the bottom of main.go:
```go
type asynqSlogAdapter struct{ l *slog.Logger }
func (a asynqSlogAdapter) Debug(args ...any) { a.l.Debug("asynq", "args", args) }
func (a asynqSlogAdapter) Info(args ...any)  { a.l.Info("asynq", "args", args) }
func (a asynqSlogAdapter) Warn(args ...any)  { a.l.Warn("asynq", "args", args) }
func (a asynqSlogAdapter) Error(args ...any) { a.l.Error("asynq", "args", args) }
func (a asynqSlogAdapter) Fatal(args ...any) { a.l.Error("asynq", "args", args) }
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add cmd/ internal/api/
git commit -m "feat: wire asynq client + worker into nova-api"
```

---

## Task 12: Audit middleware

**Files:** Create `internal/api/middleware/audit.go`, `internal/api/middleware/audit_test.go`.

- [ ] **Step 1: Write failing test**

Create `internal/api/middleware/audit_test.go`:
```go
package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type fakeAuditQ struct{ called int; got storedb.InsertAuditParams }

func (f *fakeAuditQ) InsertAudit(_ context.Context, p storedb.InsertAuditParams) error {
	f.called++
	f.got = p
	return nil
}

func TestAudit_RecordsAccepted(t *testing.T) {
	fq := &fakeAuditQ{}
	mw := Audit(fq)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(`{"name":"tank"}`))
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, req)

	if !called {
		t.Fatal("next not called")
	}
	if fq.called != 1 {
		t.Fatalf("audit insert calls=%d", fq.called)
	}
	if fq.got.Result != "accepted" {
		t.Errorf("result=%q", fq.got.Result)
	}
	if fq.got.Action != "POST /api/v1/pools" {
		t.Errorf("action=%q", fq.got.Action)
	}
	if pgtype.Text(fq.got.Actor).Valid {
		t.Errorf("actor should be null in v1")
	}
}
```

- [ ] **Step 2: Implement**

Create `internal/api/middleware/audit.go`:
```go
package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type AuditQuerier interface {
	InsertAudit(ctx context.Context, p storedb.InsertAuditParams) error
}

func Audit(q AuditQuerier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			body, _ := io.ReadAll(io.LimitReader(r.Body, 64*1024))
			r.Body = io.NopCloser(bytes.NewReader(body))

			rw := &respWriter{ResponseWriter: w}
			next.ServeHTTP(rw, r)

			result := "accepted"
			if rw.status >= 400 {
				result = "rejected"
			}

			payload := pgtype.NullJSONB(nil)
			if json.Valid(body) {
				payload = pgtype.JSONB{Bytes: body, Status: pgtype.Present}
			}

			_ = q.InsertAudit(r.Context(), storedb.InsertAuditParams{
				Actor:     pgtype.Text{}, // null in v1
				Action:    r.Method + " " + r.URL.Path,
				Target:    r.URL.Path,
				RequestID: RequestIDOf(r.Context()),
				Payload:   payload,
				Result:    result,
			})
		})
	}
}
```

Note: pgx v5 uses `pgtype.JSONB` and `pgtype.Null...` differently than the snippet above. Adjust to whatever sqlc generated. Look at `internal/store/gen/audit_log.sql.go` for the parameter type. If it's `[]byte`, just pass `body` (or nil if empty). Replace this section accordingly:
```go
var payloadParam []byte
if json.Valid(body) {
    payloadParam = body
}
_ = q.InsertAudit(r.Context(), storedb.InsertAuditParams{
    Actor:     pgtype.Text{Valid: false},
    Action:    r.Method + " " + r.URL.Path,
    Target:    r.URL.Path,
    RequestID: RequestIDOf(r.Context()),
    Payload:   payloadParam,
    Result:    result,
})
```

- [ ] **Step 3: Run tests, expect pass**

```bash
go test ./internal/api/middleware/...
```
If sqlc generated different types, the test will need adjustment to match. The test checks `Result == "accepted"`, which is a string regardless.

- [ ] **Step 4: Wire into router**

Edit `internal/api/server.go`. After the existing middleware:
```go
r.Use(mw.Audit(d.Store.Queries))
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): add audit middleware on state-changing requests"
```

---

## Task 13: Pool write handlers (POST /pools, DELETE /pools/:name, POST /pools/:name/scrub)

**Files:** Create `internal/api/handlers/pools_write.go`, `internal/api/handlers/pools_write_test.go`.

- [ ] **Step 1: Write failing test**

Create `internal/api/handlers/pools_write_test.go`:
```go
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
)

type fakeDispatcher struct {
	calls []jobs.DispatchInput
	out   uuid.UUID
}

func (f *fakeDispatcher) Dispatch(_ context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error) {
	f.calls = append(f.calls, in)
	return jobs.DispatchOutput{JobID: f.out}, nil
}

func TestPoolsCreate_Returns202(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	id := uuid.New()
	disp := &fakeDispatcher{out: id}
	h := &PoolsWriteHandler{Logger: logger, Dispatcher: disp}

	body := `{"name":"tank","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/api/v1/jobs/"+id.String() {
		t.Errorf("Location=%q", loc)
	}
	if len(disp.calls) != 1 || disp.calls[0].Kind != jobs.KindPoolCreate {
		t.Errorf("dispatch=%+v", disp.calls)
	}
}

func TestPoolsCreate_RejectsBadName(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &PoolsWriteHandler{Logger: logger, Dispatcher: &fakeDispatcher{}}
	body := `{"name":"bad/name","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestPoolsCreate_RejectsBadJSON(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &PoolsWriteHandler{Logger: logger, Dispatcher: &fakeDispatcher{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pools", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}
	_ = json.NewDecoder(rr.Body).Decode(&struct{}{})
}
```

- [ ] **Step 2: Run, expect fail**

- [ ] **Step 3: Implement**

Create `internal/api/handlers/pools_write.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/jobs"
)

type Dispatcher interface {
	Dispatch(ctx context.Context, in jobs.DispatchInput) (jobs.DispatchOutput, error)
}

type PoolsWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

func (h *PoolsWriteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var spec pool.CreateSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if err := names.ValidatePoolName(spec.Name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolCreate,
		Target:    spec.Name,
		Payload:   jobs.PoolCreatePayload{Name: spec.Name, Spec: spec},
		Command:   "zpool create " + spec.Name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + spec.Name,
	})
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", err.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"jobId": out.JobID.String()})
}

func (h *PoolsWriteHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolDestroy,
		Target:    name,
		Payload:   jobs.PoolDestroyPayload{Name: name},
		Command:   "zpool destroy " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", err.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.WriteHeader(http.StatusAccepted)
}

func (h *PoolsWriteHandler) Scrub(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return
	}
	action := pool.ScrubStart
	if r.URL.Query().Get("action") == "stop" {
		action = pool.ScrubStop
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindPoolScrub,
		Target:    name,
		Payload:   jobs.PoolScrubPayload{Name: name, Action: action},
		Command:   "zpool scrub " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "pool:" + name,
	})
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", err.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.WriteHeader(http.StatusAccepted)
}
```

- [ ] **Step 4: Wire into server**

Edit `internal/api/server.go`. Add to `Deps`:
```go
PoolsWrite *handlers.PoolsWriteHandler
```
Compose in `New`:
```go
poolsWriteH := &handlers.PoolsWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
// inside Route("/api/v1"):
r.Post("/pools", poolsWriteH.Create)
r.Delete("/pools/{name}", poolsWriteH.Destroy)
r.Post("/pools/{name}/scrub", poolsWriteH.Scrub)
```

Update `cmd/nova-api/main.go` to pass `Dispatcher` (already done in Task 11).

- [ ] **Step 5: Run tests, expect pass**

```bash
go test ./...
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat(api): add POST /pools, DELETE /pools/:name, POST /pools/:name/scrub"
```

---

## Task 14: Dataset write handlers

**Files:** Create `internal/api/handlers/datasets_write.go`, `internal/api/handlers/datasets_write_test.go`.

- [ ] **Step 1: Write failing test**

Create `internal/api/handlers/datasets_write_test.go`:
```go
package handlers

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
)

func TestDatasetsCreate_202(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	disp := &fakeDispatcher{out: uuid.New()}
	h := &DatasetsWriteHandler{Logger: logger, Dispatcher: disp}
	body := `{"parent":"tank","name":"home","type":"filesystem","properties":{"compression":"lz4"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/datasets", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if disp.calls[0].Kind != jobs.KindDatasetCreate {
		t.Errorf("kind=%s", disp.calls[0].Kind)
	}
}

func TestDatasetsDestroy_URLEncoded(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	disp := &fakeDispatcher{out: uuid.New()}
	h := &DatasetsWriteHandler{Logger: logger, Dispatcher: disp}
	r := chi.NewRouter()
	r.Delete("/api/v1/datasets/{fullname}", h.Destroy)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "?recursive=true"
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("status=%d", rr.Code)
	}
	if !disp.calls[0].Payload.(jobs.DatasetDestroyPayload).Recursive {
		t.Errorf("recursive=false")
	}
}

func TestDatasetsSet_PATCH(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	disp := &fakeDispatcher{out: uuid.New()}
	h := &DatasetsWriteHandler{Logger: logger, Dispatcher: disp}
	r := chi.NewRouter()
	r.Patch("/api/v1/datasets/{fullname}", h.SetProps)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home")
	body := `{"properties":{"compression":"zstd"}}`
	req := httptest.NewRequest(http.MethodPatch, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 2: Implement**

Create `internal/api/handlers/datasets_write.go`:
```go
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/jobs"
)

type DatasetsWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

func (h *DatasetsWriteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var spec dataset.CreateSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	full := spec.Parent + "/" + spec.Name
	if err := names.ValidateDatasetName(full); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetCreate,
		Target:    full,
		Payload:   jobs.DatasetCreatePayload{Spec: spec},
		Command:   "zfs create " + full,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + full,
	})
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", err.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.WriteHeader(http.StatusAccepted)
}

func (h *DatasetsWriteHandler) SetProps(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeFullname(w, r)
	if !ok {
		return
	}
	var body struct {
		Properties map[string]string `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if len(body.Properties) == 0 {
		middleware.WriteError(w, http.StatusBadRequest, "no_props", "properties required")
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetSet,
		Target:    name,
		Payload:   jobs.DatasetSetPayload{Name: name, Properties: body.Properties},
		Command:   "zfs set " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", err.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.WriteHeader(http.StatusAccepted)
}

func (h *DatasetsWriteHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	name, ok := decodeFullname(w, r)
	if !ok {
		return
	}
	recursive := r.URL.Query().Get("recursive") == "true"
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind:      jobs.KindDatasetDestroy,
		Target:    name,
		Payload:   jobs.DatasetDestroyPayload{Name: name, Recursive: recursive},
		Command:   "zfs destroy " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
		UniqueKey: "dataset:" + name,
	})
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", err.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.WriteHeader(http.StatusAccepted)
}

func decodeFullname(w http.ResponseWriter, r *http.Request) (string, bool) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid url-encoded name")
		return "", false
	}
	if err := names.ValidateDatasetName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return "", false
	}
	return name, true
}
```

- [ ] **Step 3: Wire into server**

Edit `internal/api/server.go`. Add inside `Route("/api/v1")`:
```go
dsW := &handlers.DatasetsWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
r.Post("/datasets", dsW.Create)
r.Patch("/datasets/{fullname}", dsW.SetProps)
r.Delete("/datasets/{fullname}", dsW.Destroy)
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./...
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): add POST/PATCH/DELETE for datasets"
```

---

## Task 15: Snapshot write handlers + rollback

**Files:** Create `internal/api/handlers/snapshots_write.go`, `internal/api/handlers/snapshots_write_test.go`.

- [ ] **Step 1: Write failing test**

Create `internal/api/handlers/snapshots_write_test.go`:
```go
package handlers

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/jobs"
)

func TestSnapshotsCreate_202(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SnapshotsWriteHandler{Logger: logger, Dispatcher: disp}
	body := `{"dataset":"tank/home","name":"daily-1","recursive":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/snapshots", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	p := disp.calls[0].Payload.(jobs.SnapshotCreatePayload)
	if !p.Recursive || p.Dataset != "tank/home" || p.ShortName != "daily-1" {
		t.Errorf("payload=%+v", p)
	}
}

func TestSnapshotsRollback(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	disp := &fakeDispatcher{out: uuid.New()}
	h := &SnapshotsWriteHandler{Logger: logger, Dispatcher: disp}
	r := chi.NewRouter()
	r.Post("/api/v1/datasets/{fullname}/rollback", h.Rollback)
	target := "/api/v1/datasets/" + url.PathEscape("tank/home") + "/rollback"
	body := `{"snapshot":"daily-1"}`
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 2: Implement**

Create `internal/api/handlers/snapshots_write.go`:
```go
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	"github.com/novanas/nova-nas/internal/jobs"
)

type SnapshotsWriteHandler struct {
	Logger     *slog.Logger
	Dispatcher Dispatcher
}

func (h *SnapshotsWriteHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dataset   string `json:"dataset"`
		Name      string `json:"name"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	full := body.Dataset + "@" + body.Name
	if err := names.ValidateSnapshotName(full); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return
	}
	out, err := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind: jobs.KindSnapshotCreate,
		Target: full,
		Payload: jobs.SnapshotCreatePayload{
			Dataset: body.Dataset, ShortName: body.Name, Recursive: body.Recursive,
		},
		Command: "zfs snapshot " + full,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", err.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.WriteHeader(http.StatusAccepted)
}

func (h *SnapshotsWriteHandler) Destroy(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid url-encoded name")
		return
	}
	if err := names.ValidateSnapshotName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return
	}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind: jobs.KindSnapshotDestroy,
		Target: name,
		Payload: jobs.SnapshotDestroyPayload{Name: name},
		Command: "zfs destroy " + name,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	if derr != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", derr.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.WriteHeader(http.StatusAccepted)
}

func (h *SnapshotsWriteHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	dsName, err := url.PathUnescape(encoded)
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid url-encoded name")
		return
	}
	if err := names.ValidateDatasetName(dsName); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return
	}
	var body struct {
		Snapshot string `json:"snapshot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	full := dsName + "@" + body.Snapshot
	if err := names.ValidateSnapshotName(full); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return
	}
	out, derr := h.Dispatcher.Dispatch(r.Context(), jobs.DispatchInput{
		Kind: jobs.KindSnapshotRollback,
		Target: full,
		Payload: jobs.SnapshotRollbackPayload{Snapshot: full},
		Command: "zfs rollback " + full,
		RequestID: middleware.RequestIDOf(r.Context()),
	})
	if derr != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "dispatch_error", derr.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/jobs/"+out.JobID.String())
	w.WriteHeader(http.StatusAccepted)
}
```

- [ ] **Step 3: Wire into server**

Edit `internal/api/server.go`:
```go
snapW := &handlers.SnapshotsWriteHandler{Logger: d.Logger, Dispatcher: d.Dispatcher}
// inside /api/v1 group:
r.Post("/snapshots", snapW.Create)
r.Delete("/snapshots/{fullname}", snapW.Destroy)
r.Post("/datasets/{fullname}/rollback", snapW.Rollback)
```

- [ ] **Step 4: Run tests, expect pass**

- [ ] **Step 5: Commit**

```bash
git add internal/api/
git commit -m "feat(api): add POST/DELETE snapshots and rollback"
```

---

## Task 16: Jobs handler (GET /jobs, GET /jobs/:id, DELETE /jobs/:id)

**Files:** Create `internal/api/handlers/jobs.go`, `internal/api/handlers/jobs_test.go`.

- [ ] **Step 1: Write failing test**

Create `internal/api/handlers/jobs_test.go`:
```go
package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type fakeJobQ struct {
	get    storedb.Job
	getErr error
}

func (f *fakeJobQ) GetJob(_ context.Context, _ uuid.UUID) (storedb.Job, error) {
	return f.get, f.getErr
}
func (f *fakeJobQ) ListJobs(_ context.Context, _ storedb.ListJobsParams) ([]storedb.Job, error) {
	return []storedb.Job{f.get}, nil
}
func (f *fakeJobQ) CancelJob(_ context.Context, _ uuid.UUID) error { return nil }

func TestJobsGet_404(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := &JobsHandler{Logger: logger, Q: &fakeJobQ{getErr: storedb.ErrNoRows}}
	r := chi.NewRouter()
	r.Get("/jobs/{id}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status=%d", rr.Code)
	}
}
```

Note: pgx returns `pgx.ErrNoRows`, sqlc surfaces it via Go's standard error-wrapping. The test uses a sentinel `storedb.ErrNoRows` — define one in a new file:

Create `internal/store/gen/errs.go`:
```go
package storedb

import "github.com/jackc/pgx/v5"

var ErrNoRows = pgx.ErrNoRows
```

- [ ] **Step 2: Implement**

Create `internal/api/handlers/jobs.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/api/middleware"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type JobsQ interface {
	GetJob(ctx context.Context, id uuid.UUID) (storedb.Job, error)
	ListJobs(ctx context.Context, p storedb.ListJobsParams) ([]storedb.Job, error)
	CancelJob(ctx context.Context, id uuid.UUID) error
}

type JobsHandler struct {
	Logger *slog.Logger
	Q      JobsQ
}

func (h *JobsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", err.Error())
		return
	}
	job, err := h.Q.GetJob(r.Context(), id)
	if err != nil {
		if errors.Is(err, storedb.ErrNoRows) {
			middleware.WriteError(w, http.StatusNotFound, "not_found", "job not found")
			return
		}
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
}

func (h *JobsHandler) List(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	limit := 100
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	params := storedb.ListJobsParams{Limit: int32(limit), Offset: int32(offset)}
	if state != "" {
		params.State = &state
	}
	jobs, err := h.Q.ListJobs(r.Context(), params)
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jobs)
}

func (h *JobsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", err.Error())
		return
	}
	if err := h.Q.CancelJob(r.Context(), id); err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3: Wire into server**

Edit `internal/api/server.go`:
```go
jobsH := &handlers.JobsHandler{Logger: d.Logger, Q: d.Store.Queries}
// inside /api/v1 group:
r.Get("/jobs", jobsH.List)
r.Get("/jobs/{id}", jobsH.Get)
r.Delete("/jobs/{id}", jobsH.Cancel)
```

- [ ] **Step 4: Run tests, expect pass**

```bash
go test ./...
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/gen/errs.go internal/api/
git commit -m "feat(api): add job list, get, and cancel endpoints"
```

---

## Task 17: SSE job streaming

**Files:** Create `internal/api/handlers/jobs_sse.go`.

- [ ] **Step 1: Implement SSE handler**

Create `internal/api/handlers/jobs_sse.go`:
```go
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api/middleware"
)

type SSEJobsHandler struct {
	Logger *slog.Logger
	Redis  *redis.Client
	Q      JobsQ
}

func (h *SSEJobsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_id", err.Error())
		return
	}

	job, err := h.Q.GetJob(r.Context(), id)
	if err != nil {
		middleware.WriteError(w, http.StatusNotFound, "not_found", "job not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprintf(w, "event: state\ndata: %s\n\n", job.State)
	flusher.Flush()

	if isTerminal(job.State) {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	sub := h.Redis.Subscribe(ctx, "job:"+id.String()+":update")
	defer sub.Close()
	ch := sub.Channel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: state\ndata: %s\n\n", msg.Payload)
			flusher.Flush()
			if isTerminal(msg.Payload) {
				return
			}
		}
	}
}

func isTerminal(state string) bool {
	switch state {
	case "succeeded", "failed", "cancelled", "interrupted":
		return true
	}
	return false
}
```

- [ ] **Step 2: Wire into server**

Edit `internal/api/server.go`:
```go
sseH := &handlers.SSEJobsHandler{Logger: d.Logger, Redis: d.Redis, Q: d.Store.Queries}
// inside /api/v1 group:
r.Get("/jobs/{id}/stream", sseH.Stream)
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/api/
git commit -m "feat(api): add SSE job streaming via Redis pub/sub"
```

---

## Task 18: Metadata sub-routes (PATCH /pools/:name/metadata, etc.)

**Files:** Create `internal/api/handlers/metadata.go`, `internal/store/queries/metadata.sql` (extension).

- [ ] **Step 1: Add upsert query**

Append to `internal/store/queries/resource_metadata.sql`:
```sql
-- name: UpsertResourceMetadata :one
INSERT INTO resource_metadata (kind, zfs_name, display_name, description, tags)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (kind, zfs_name) DO UPDATE
   SET display_name = COALESCE(EXCLUDED.display_name, resource_metadata.display_name),
       description  = COALESCE(EXCLUDED.description,  resource_metadata.description),
       tags         = COALESCE(EXCLUDED.tags,         resource_metadata.tags)
RETURNING *;

-- name: DeleteResourceMetadata :exec
DELETE FROM resource_metadata WHERE kind = $1 AND zfs_name = $2;
```

Run:
```bash
sqlc generate
```

- [ ] **Step 2: Implement handler**

Create `internal/api/handlers/metadata.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/novanas/nova-nas/internal/api/middleware"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type MetadataQ interface {
	UpsertResourceMetadata(ctx context.Context, p storedb.UpsertResourceMetadataParams) (storedb.ResourceMetadatum, error)
}

type MetadataHandler struct {
	Logger *slog.Logger
	Q      MetadataQ
}

type metadataPatch struct {
	DisplayName *string         `json:"display_name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Tags        json.RawMessage `json:"tags,omitempty"`
}

func (h *MetadataHandler) patch(kind string, w http.ResponseWriter, r *http.Request, name string) {
	var body metadataPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	params := storedb.UpsertResourceMetadataParams{Kind: kind, ZfsName: name}
	if body.DisplayName != nil {
		params.DisplayName = body.DisplayName
	}
	if body.Description != nil {
		params.Description = body.Description
	}
	if len(body.Tags) > 0 {
		params.Tags = body.Tags
	}
	rec, err := h.Q.UpsertResourceMetadata(r.Context(), params)
	if err != nil {
		middleware.WriteError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rec)
}

func (h *MetadataHandler) PoolPatch(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := names.ValidatePoolName(name); err != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", err.Error())
		return
	}
	h.patch("pool", w, r, name)
}

func (h *MetadataHandler) DatasetPatch(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil || names.ValidateDatasetName(name) != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid dataset name")
		return
	}
	h.patch("dataset", w, r, name)
}

func (h *MetadataHandler) SnapshotPatch(w http.ResponseWriter, r *http.Request) {
	encoded := chi.URLParam(r, "fullname")
	name, err := url.PathUnescape(encoded)
	if err != nil || names.ValidateSnapshotName(name) != nil {
		middleware.WriteError(w, http.StatusBadRequest, "bad_name", "invalid snapshot name")
		return
	}
	h.patch("snapshot", w, r, name)
}
```

- [ ] **Step 3: Wire into server**

Edit `internal/api/server.go`:
```go
metaH := &handlers.MetadataHandler{Logger: d.Logger, Q: d.Store.Queries}
// inside /api/v1 group:
r.Patch("/pools/{name}/metadata", metaH.PoolPatch)
r.Patch("/datasets/{fullname}/metadata", metaH.DatasetPatch)
r.Patch("/snapshots/{fullname}/metadata", metaH.SnapshotPatch)
```

- [ ] **Step 4: Build, commit**

```bash
go build ./...
git add internal/store/ internal/api/
git commit -m "feat(api): add metadata PATCH for pools/datasets/snapshots"
```

---

## Task 19: Integration test — full write flow

**Files:** Create `test/integration/write_test.go`.

- [ ] **Step 1: Replace stub managers with fake host-exec**

We can't run real ZFS in integration tests; we use a fake `Manager` that records calls. Create a thin shim or use the existing `pool.Manager`/`dataset.Manager`/`snapshot.Manager` with a stub `ZFSBin`/`ZpoolBin` pointing to a script.

Simplest: add an exported `Runner` field to the managers (a function with `Run` signature) that defaults to `exec.Run`, and let tests inject a fake. Refactor:

Edit `internal/host/exec/exec.go` to expose a func type:
```go
type Runner func(ctx context.Context, bin string, args ...string) ([]byte, error)

var DefaultRunner Runner = Run
```

Edit each Manager (`pool.Manager`, `dataset.Manager`, `snapshot.Manager`) to add an optional `Runner exec.Runner` field that replaces direct `exec.Run` calls when non-nil. Default to `exec.Run` when nil. Update all internal call sites to:
```go
runner := m.Runner
if runner == nil {
    runner = exec.Run
}
runner(ctx, m.ZpoolBin, args...)
```

- [ ] **Step 2: Write integration test**

Create `test/integration/write_test.go`:
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
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api"
	"github.com/novanas/nova-nas/internal/api/handlers"
	"github.com/novanas/nova-nas/internal/host/disks"
	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/store"
)

type stubDisks struct{}
func (stubDisks) List(_ context.Context) ([]disks.Disk, error) { return nil, nil }

func okRunner(_ context.Context, _ string, _ ...string) ([]byte, error) { return nil, nil }

func TestPoolCreate_FullFlow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	asyncOpt, _ := asynq.ParseRedisURI("redis://" + redisAddr)
	asyncClient := asynq.NewClient(asyncOpt)
	defer asyncClient.Close()
	rc := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rc.Close()

	pm := &pool.Manager{ZpoolBin: "/bin/true", Runner: okRunner}
	dm := &dataset.Manager{ZFSBin: "/bin/true", Runner: okRunner}
	sm := &snapshot.Manager{ZFSBin: "/bin/true", Runner: okRunner}

	disp := &jobs.Dispatcher{Client: asyncClient, Queries: st.Queries, Pool: st.Pool}

	asyncSrv := asynq.NewServer(asyncOpt, asynq.Config{Concurrency: 2})
	mux := jobs.NewServeMux(jobs.WorkerDeps{
		Logger: logger, Queries: st.Queries, Redis: rc,
		Pools: pm, Datasets: dm, Snapshots: sm,
	})
	go asyncSrv.Run(mux)
	defer asyncSrv.Stop()

	srv := api.New(api.Deps{
		Logger:     logger,
		Store:      st,
		Disks:      handlers.DiskLister(stubDisks{}),
		Pools:      pm,
		Datasets:   dm,
		Snapshots:  sm,
		Dispatcher: disp,
		Redis:      rc,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"name":"tank","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	resp, err := http.Post(ts.URL+"/api/v1/pools", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	var got map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&got)
	jobID := got["jobId"]

	// Poll for terminal state
	for i := 0; i < 50; i++ {
		r, _ := http.Get(ts.URL + "/api/v1/jobs/" + jobID)
		var job map[string]any
		_ = json.NewDecoder(r.Body).Decode(&job)
		_ = r.Body.Close()
		state, _ := job["state"].(string)
		if state == "succeeded" {
			return
		}
		if state == "failed" || state == "cancelled" {
			t.Fatalf("job %s ended in state %s", jobID, state)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("job did not reach terminal state in time")
	_ = exec.HostError{} // keep import
}
```

- [ ] **Step 3: Run integration tests**

```bash
make test-integration
```

- [ ] **Step 4: Commit**

```bash
git add test/integration/ internal/host/
git commit -m "test(integration): cover full POST /pools → worker → DB flow"
```

---

## Task 20: Apply audit middleware properly + smoke test

**Files:** Modify `internal/api/server.go`, add `test/integration/audit_test.go`.

- [ ] **Step 1: Smoke test the audit_log writes**

Create `test/integration/audit_test.go`:
```go
//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAudit_RecordedOnPOST(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Use the same harness already running in TestPoolCreate_FullFlow.
	// Call any POST endpoint and verify a row lands in audit_log.
	ts := startTestServer(t)
	defer ts.Close()

	body := `{"name":"validname","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	resp, _ := http.Post(ts.URL+"/api/v1/pools", "application/json", strings.NewReader(body))
	resp.Body.Close()

	var n int
	row := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'POST /api/v1/pools'`)
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Errorf("expected audit row, got none")
	}
}
```

The `startTestServer` helper should be extracted from `TestPoolCreate_FullFlow` into a shared helper file. Create `test/integration/helpers.go`:
```go
//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/api"
	"github.com/novanas/nova-nas/internal/api/handlers"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	"github.com/novanas/nova-nas/internal/jobs"
	"github.com/novanas/nova-nas/internal/store"
)

func startTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	asyncOpt, _ := asynq.ParseRedisURI("redis://" + redisAddr)
	asyncClient := asynq.NewClient(asyncOpt)
	t.Cleanup(func() { _ = asyncClient.Close() })
	rc := redis.NewClient(&redis.Options{Addr: redisAddr})
	t.Cleanup(func() { _ = rc.Close() })

	pm := &pool.Manager{ZpoolBin: "/bin/true", Runner: okRunner}
	dm := &dataset.Manager{ZFSBin: "/bin/true", Runner: okRunner}
	sm := &snapshot.Manager{ZFSBin: "/bin/true", Runner: okRunner}

	disp := &jobs.Dispatcher{Client: asyncClient, Queries: st.Queries, Pool: st.Pool}

	asyncSrv := asynq.NewServer(asyncOpt, asynq.Config{Concurrency: 2})
	mux := jobs.NewServeMux(jobs.WorkerDeps{
		Logger: logger, Queries: st.Queries, Redis: rc,
		Pools: pm, Datasets: dm, Snapshots: sm,
	})
	go asyncSrv.Run(mux)
	t.Cleanup(asyncSrv.Stop)

	srv := api.New(api.Deps{
		Logger:     logger,
		Store:      st,
		Disks:      handlers.DiskLister(stubDisks{}),
		Pools:      pm,
		Datasets:   dm,
		Snapshots:  sm,
		Dispatcher: disp,
		Redis:      rc,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}
```

Refactor `TestPoolCreate_FullFlow` to use `startTestServer(t)`.

- [ ] **Step 2: Run integration tests**

```bash
make test-integration
```

- [ ] **Step 3: Commit**

```bash
git add test/integration/
git commit -m "test(integration): assert audit_log writes on POST"
```

---

## Done

End state for Plan 2:
- `POST /pools`, `DELETE /pools/:name`, `POST /pools/:name/scrub` return 202 with job IDs.
- `POST /datasets`, `PATCH /datasets/:fullname`, `DELETE /datasets/:fullname` work.
- `POST /snapshots`, `DELETE /snapshots/:fullname`, `POST /datasets/:fullname/rollback` work.
- `GET /jobs`, `GET /jobs/:id`, `DELETE /jobs/:id`, `GET /jobs/:id/stream` (SSE) work.
- `PATCH` metadata sub-routes for pools/datasets/snapshots persist to `resource_metadata`.
- Audit middleware records every state-changing call.
- Worker handles all task kinds; crash recovery marks running jobs interrupted on startup.
- Asynq `unique` keyed by `pool:<name>` and `dataset:<name>` prevents overlapping ops.
- Integration tests cover the full POST → worker → DB flow with stubbed host-exec.

**Next:** Plan 3 covers OpenAPI spec + codegen, real-ZFS e2e tests, deploy artifacts (systemd unit, .deb), security/hardening (body size limits, secret redaction), and operational docs.
