# Storage MVP — Plan 3: OpenAPI, E2E, and Deploy

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wrap up the v1 storage MVP with a hand-authored OpenAPI 3.1 spec, generated TS client, end-to-end tests against real ZFS using sparse loopback devices, deploy artifacts (Dockerfile for CI, systemd unit, Debian package skeleton), and security/hardening (body size limits, stderr redaction, rate limiting).

**Architecture:** Same Go binary; OpenAPI is the contract source-of-truth. Handlers' shapes are validated against generated types; the future React UI consumes a generated TS client. Real-ZFS e2e tests run on a host with ZFS available, gated by build tag and a label-triggered self-hosted runner.

**Prerequisites:** Plan 2 complete and merged.

**Reference spec:** `docs/superpowers/specs/2026-04-28-novanas-storage-mvp-design.md` §3 (API surface), §8 (testing), §9 (security and isolation).

---

## File Structure (created in this plan)

```
api/openapi.yaml
internal/api/oapi/types.go              (generated)
clients/typescript/src/                 (generated, gitignored or committed)
test/e2e/{harness,pool,dataset,snapshot}_test.go
test/e2e/zfs_helpers.go
Dockerfile
deploy/systemd/nova-api.service
deploy/systemd/tmpfiles.d/nova-api.conf
deploy/postgres/nova-api-init.sql
deploy/packaging/debian/{control,postinst,prerm,rules}
internal/api/middleware/{bodylimit,redact}.go
.github/workflows/e2e.yml
```

---

## Task 1: Author OpenAPI 3.1 spec

**Files:** Create `api/openapi.yaml`.

- [ ] **Step 1: Write the spec**

Create `api/openapi.yaml`:
```yaml
openapi: 3.1.0
info:
  title: NovaNAS Storage API
  version: 0.1.0
  description: ZFS-based storage control plane for a single-node NAS appliance.
servers:
  - url: /api/v1

paths:
  /healthz:
    get:
      summary: Liveness probe
      operationId: getHealthz
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: object
                properties: { status: { type: string } }

  /disks:
    get:
      operationId: listDisks
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: array
                items: { $ref: '#/components/schemas/Disk' }

  /pools:
    get:
      operationId: listPools
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: array
                items: { $ref: '#/components/schemas/Pool' }
    post:
      operationId: createPool
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/PoolCreateSpec' }
      responses:
        '202': { $ref: '#/components/responses/Accepted' }
        '400': { $ref: '#/components/responses/BadRequest' }

  /pools/{name}:
    parameters:
      - in: path
        name: name
        required: true
        schema: { type: string }
    get:
      operationId: getPool
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema: { $ref: '#/components/schemas/PoolDetail' }
        '404': { $ref: '#/components/responses/NotFound' }
    delete:
      operationId: destroyPool
      responses:
        '202': { $ref: '#/components/responses/Accepted' }
        '404': { $ref: '#/components/responses/NotFound' }

  /pools/{name}/scrub:
    parameters:
      - in: path
        name: name
        required: true
        schema: { type: string }
      - in: query
        name: action
        schema: { type: string, enum: [start, stop] }
    post:
      operationId: scrubPool
      responses:
        '202': { $ref: '#/components/responses/Accepted' }

  /pools/{name}/metadata:
    parameters:
      - in: path
        name: name
        required: true
        schema: { type: string }
    patch:
      operationId: patchPoolMetadata
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/MetadataPatch' }
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema: { $ref: '#/components/schemas/ResourceMetadata' }

  /datasets:
    get:
      operationId: listDatasets
      parameters:
        - in: query
          name: pool
          schema: { type: string }
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: array
                items: { $ref: '#/components/schemas/Dataset' }
    post:
      operationId: createDataset
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/DatasetCreateSpec' }
      responses:
        '202': { $ref: '#/components/responses/Accepted' }

  /datasets/{fullname}:
    parameters:
      - in: path
        name: fullname
        required: true
        schema: { type: string, description: URL-encoded dataset name }
    get:
      operationId: getDataset
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema: { $ref: '#/components/schemas/DatasetDetail' }
        '404': { $ref: '#/components/responses/NotFound' }
    patch:
      operationId: patchDatasetProps
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [properties]
              properties:
                properties:
                  type: object
                  additionalProperties: { type: string }
      responses:
        '202': { $ref: '#/components/responses/Accepted' }
    delete:
      operationId: destroyDataset
      parameters:
        - in: query
          name: recursive
          schema: { type: boolean }
      responses:
        '202': { $ref: '#/components/responses/Accepted' }

  /datasets/{fullname}/metadata:
    parameters:
      - in: path
        name: fullname
        required: true
        schema: { type: string }
    patch:
      operationId: patchDatasetMetadata
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/MetadataPatch' }
      responses:
        '200':
          description: OK

  /datasets/{fullname}/rollback:
    parameters:
      - in: path
        name: fullname
        required: true
        schema: { type: string }
    post:
      operationId: rollbackDataset
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [snapshot]
              properties:
                snapshot: { type: string }
      responses:
        '202': { $ref: '#/components/responses/Accepted' }

  /snapshots:
    get:
      operationId: listSnapshots
      parameters:
        - in: query
          name: dataset
          schema: { type: string }
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: array
                items: { $ref: '#/components/schemas/Snapshot' }
    post:
      operationId: createSnapshot
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [dataset, name]
              properties:
                dataset: { type: string }
                name:    { type: string }
                recursive: { type: boolean, default: false }
      responses:
        '202': { $ref: '#/components/responses/Accepted' }

  /snapshots/{fullname}:
    parameters:
      - in: path
        name: fullname
        required: true
        schema: { type: string }
    delete:
      operationId: destroySnapshot
      responses:
        '202': { $ref: '#/components/responses/Accepted' }

  /snapshots/{fullname}/metadata:
    parameters:
      - in: path
        name: fullname
        required: true
        schema: { type: string }
    patch:
      operationId: patchSnapshotMetadata
      requestBody:
        required: true
        content:
          application/json:
            schema: { $ref: '#/components/schemas/MetadataPatch' }
      responses:
        '200':
          description: OK

  /jobs:
    get:
      operationId: listJobs
      parameters:
        - in: query
          name: state
          schema: { type: string, enum: [queued, running, succeeded, failed, cancelled, interrupted] }
        - in: query
          name: limit
          schema: { type: integer, minimum: 1, maximum: 500 }
        - in: query
          name: offset
          schema: { type: integer, minimum: 0 }
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: array
                items: { $ref: '#/components/schemas/Job' }

  /jobs/{id}:
    parameters:
      - in: path
        name: id
        required: true
        schema: { type: string, format: uuid }
    get:
      operationId: getJob
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema: { $ref: '#/components/schemas/Job' }
        '404': { $ref: '#/components/responses/NotFound' }
    delete:
      operationId: cancelJob
      responses:
        '204':
          description: Cancelled

  /jobs/{id}/stream:
    parameters:
      - in: path
        name: id
        required: true
        schema: { type: string, format: uuid }
    get:
      operationId: streamJob
      responses:
        '200':
          description: SSE stream of job state events
          content:
            text/event-stream:
              schema: { type: string }

components:
  responses:
    Accepted:
      description: Async operation accepted; check Location header for job URL.
      headers:
        Location:
          schema: { type: string }
      content:
        application/json:
          schema:
            type: object
            properties:
              jobId: { type: string, format: uuid }
    BadRequest:
      description: Bad request
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }
    NotFound:
      description: Not found
      content:
        application/json:
          schema: { $ref: '#/components/schemas/Error' }

  schemas:
    Error:
      type: object
      properties:
        error:   { type: string }
        message: { type: string }
    Disk:
      type: object
      properties:
        name:        { type: string }
        sizeBytes:   { type: integer, format: int64 }
        model:       { type: string }
        serial:      { type: string }
        wwn:         { type: string }
        rotational:  { type: boolean }
        inUseByPool: { type: boolean }
    Pool:
      type: object
      properties:
        name:             { type: string }
        sizeBytes:        { type: integer }
        allocated:        { type: integer }
        free:             { type: integer }
        health:           { type: string }
        readOnly:         { type: boolean }
        fragmentationPct: { type: integer }
        capacityPct:      { type: integer }
        dedupRatio:       { type: string }
    PoolDetail:
      type: object
      properties:
        pool:       { $ref: '#/components/schemas/Pool' }
        properties: { type: object, additionalProperties: { type: string } }
        status:
          type: object
          properties:
            state: { type: string }
            vdevs:
              type: array
              items: { $ref: '#/components/schemas/Vdev' }
    Vdev:
      type: object
      properties:
        type:           { type: string }
        path:           { type: string }
        state:          { type: string }
        readErrors:     { type: integer }
        writeErrors:    { type: integer }
        checksumErrors: { type: integer }
        children:
          type: array
          items: { $ref: '#/components/schemas/Vdev' }
    PoolCreateSpec:
      type: object
      required: [name, vdevs]
      properties:
        name: { type: string }
        vdevs:
          type: array
          items:
            type: object
            required: [type, disks]
            properties:
              type: { type: string, enum: [mirror, raidz1, raidz2, raidz3, stripe] }
              disks: { type: array, items: { type: string } }
        log:   { type: array, items: { type: string } }
        cache: { type: array, items: { type: string } }
        spare: { type: array, items: { type: string } }
    Dataset:
      type: object
      properties:
        name:            { type: string }
        type:            { type: string, enum: [filesystem, volume] }
        usedBytes:       { type: integer }
        availableBytes:  { type: integer }
        referencedBytes: { type: integer }
        mountpoint:      { type: string }
        compression:     { type: string }
        recordSizeBytes: { type: integer }
    DatasetDetail:
      type: object
      properties:
        dataset:    { $ref: '#/components/schemas/Dataset' }
        properties: { type: object, additionalProperties: { type: string } }
    DatasetCreateSpec:
      type: object
      required: [parent, name, type]
      properties:
        parent: { type: string }
        name:   { type: string }
        type:   { type: string, enum: [filesystem, volume] }
        volumeSizeBytes: { type: integer }
        properties:
          type: object
          additionalProperties: { type: string }
    Snapshot:
      type: object
      properties:
        name:            { type: string }
        dataset:         { type: string }
        shortName:       { type: string }
        usedBytes:       { type: integer }
        referencedBytes: { type: integer }
        creationUnix:    { type: integer }
    Job:
      type: object
      properties:
        id:        { type: string, format: uuid }
        kind:      { type: string }
        target:    { type: string }
        state:     { type: string }
        command:   { type: string }
        stdout:    { type: string }
        stderr:    { type: string }
        exitCode:  { type: integer, nullable: true }
        error:     { type: string, nullable: true }
        createdAt:  { type: string, format: date-time }
        startedAt:  { type: string, format: date-time, nullable: true }
        finishedAt: { type: string, format: date-time, nullable: true }
    MetadataPatch:
      type: object
      properties:
        display_name: { type: string }
        description:  { type: string }
        tags:
          type: object
          additionalProperties: { type: string }
    ResourceMetadata:
      type: object
      properties:
        id:           { type: integer }
        kind:         { type: string }
        zfsName:      { type: string }
        displayName:  { type: string, nullable: true }
        description:  { type: string, nullable: true }
        tags:         { type: object, nullable: true, additionalProperties: { type: string } }
```

- [ ] **Step 2: Validate (optional, requires `redocly`)**

```bash
npx --yes @redocly/cli@latest lint api/openapi.yaml
```
Skip if no Node available.

- [ ] **Step 3: Commit**

```bash
git add api/
git commit -m "docs(api): add OpenAPI 3.1 specification"
```

---

## Task 2: Generate Go types via oapi-codegen

**Files:** Create `internal/api/oapi/types.go` (generated), `scripts/gen-openapi.sh`, `Makefile` target.

- [ ] **Step 1: Install tool**

```bash
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
```

- [ ] **Step 2: Write generation script**

Create `scripts/gen-openapi.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p internal/api/oapi
oapi-codegen -generate types -package oapi -o internal/api/oapi/types.go api/openapi.yaml
echo "Generated internal/api/oapi/types.go"
```
Make it executable:
```bash
chmod +x scripts/gen-openapi.sh
```

- [ ] **Step 3: Add Make target**

Append to `Makefile`:
```makefile
gen-openapi:
	./scripts/gen-openapi.sh
```

Update the existing `gen` target:
```makefile
gen: gen-sqlc gen-openapi
gen-sqlc:
	sqlc generate
```

- [ ] **Step 4: Generate**

```bash
make gen-openapi
go build ./...
```

- [ ] **Step 5: Add a smoke test that imports the package**

Create `internal/api/oapi/types_test.go`:
```go
package oapi

import "testing"

func TestTypesCompile(t *testing.T) {
	var _ Disk
	var _ Pool
	var _ Dataset
	var _ Snapshot
	var _ Job
}
```

- [ ] **Step 6: Run test, expect pass**

```bash
go test ./internal/api/oapi/...
```

- [ ] **Step 7: Commit**

```bash
git add scripts/ Makefile internal/api/oapi/
git commit -m "feat(api): generate Go types from openapi.yaml"
```

---

## Task 3: Use generated types in handler responses

**Files:** Modify response paths in `internal/api/handlers/*.go` to encode the generated types instead of internal structs where they differ.

In practice the internal `disks.Disk`, `pool.Pool`, `dataset.Dataset`, `snapshot.Snapshot`, and `storedb.Job` shapes already match the OpenAPI schema (they have the same JSON field names). The OpenAPI types are useful for the TS client; on the Go side, we just need to make sure the JSON output matches.

- [ ] **Step 1: Add a contract test**

Create `test/integration/openapi_contract_test.go`:
```go
//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/novanas/nova-nas/internal/api/oapi"
)

func TestOpenAPI_DisksShape(t *testing.T) {
	ts := startTestServer(t)
	defer ts.Close()
	resp, _ := http.Get(ts.URL + "/api/v1/disks")
	body, _ := io.ReadAll(resp.Body)
	var got []oapi.Disk
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal into oapi.Disk failed: %v\nbody=%s", err, body)
	}
}

func TestOpenAPI_PoolDetailShape(t *testing.T) {
	ts := startTestServer(t)
	defer ts.Close()

	body := `{"name":"validname","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	_, _ = http.Post(ts.URL+"/api/v1/pools", "application/json", strings.NewReader(body))
	// We have no real pool to GET, but we can verify the empty list still parses:
	resp, _ := http.Get(ts.URL + "/api/v1/pools")
	bodyB, _ := io.ReadAll(resp.Body)
	var got []oapi.Pool
	if err := json.Unmarshal(bodyB, &got); err != nil {
		t.Fatalf("unmarshal into oapi.Pool failed: %v\nbody=%s", err, bodyB)
	}
}
```

- [ ] **Step 2: Run, fix mismatches**

```bash
make test-integration
```
If any field name mismatches surface, fix the JSON tags on the internal struct **OR** the OpenAPI schema; prefer aligning the schema to the existing tags. Re-generate with `make gen-openapi`. Repeat until tests pass.

- [ ] **Step 3: Commit**

```bash
git add test/integration/ api/openapi.yaml internal/
git commit -m "test(api): contract-test handler output against generated types"
```

---

## Task 4: Generate TypeScript client

**Files:** Add `clients/typescript/` skeleton; document generator script.

- [ ] **Step 1: Add openapi-typescript-codegen via npm (used in CI/dev only)**

Create `clients/typescript/package.json`:
```json
{
  "name": "@novanas/api-client",
  "version": "0.1.0",
  "private": true,
  "scripts": {
    "gen": "openapi --input ../../api/openapi.yaml --output ./src --client fetch --useUnionTypes"
  },
  "devDependencies": {
    "openapi-typescript-codegen": "^0.29.0"
  }
}
```

- [ ] **Step 2: Add `.gitignore` entry for the generated output**

Append to `.gitignore`:
```
clients/typescript/node_modules/
clients/typescript/src/
```

- [ ] **Step 3: Add a generator script**

Create `scripts/gen-ts-client.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../clients/typescript"
npm install --silent
npm run --silent gen
echo "Generated TypeScript client at clients/typescript/src/"
```
Make it executable:
```bash
chmod +x scripts/gen-ts-client.sh
```

Add Makefile target:
```makefile
gen-ts:
	./scripts/gen-ts-client.sh
```

- [ ] **Step 4: Commit**

```bash
git add clients/ scripts/ Makefile .gitignore
git commit -m "feat(clients): add TS client generator script"
```

---

## Task 5: Body size + redaction middleware

**Files:** Create `internal/api/middleware/bodylimit.go`, `internal/api/middleware/redact.go`.

- [ ] **Step 1: Body limit**

Create `internal/api/middleware/bodylimit.go`:
```go
package middleware

import "net/http"

const DefaultMaxBody = 1 << 20 // 1 MiB

func BodyLimit(max int64) func(http.Handler) http.Handler {
	if max <= 0 {
		max = DefaultMaxBody
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 2: Redact helper**

Create `internal/api/middleware/redact.go`:
```go
package middleware

import "regexp"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)("?password"?\s*[:=]\s*)("[^"]*"|[^,\s}]+)`),
	regexp.MustCompile(`(?i)("?passphrase"?\s*[:=]\s*)("[^"]*"|[^,\s}]+)`),
	regexp.MustCompile(`(?i)("?secret"?\s*[:=]\s*)("[^"]*"|[^,\s}]+)`),
	regexp.MustCompile(`(?i)("?token"?\s*[:=]\s*)("[^"]*"|[^,\s}]+)`),
}

func RedactSecrets(s []byte) []byte {
	out := s
	for _, re := range secretPatterns {
		out = re.ReplaceAll(out, []byte(`$1"<redacted>"`))
	}
	return out
}
```

- [ ] **Step 3: Test redaction**

Create `internal/api/middleware/redact_test.go`:
```go
package middleware

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"password":"hunter2"}`, `{"password":"<redacted>"}`},
		{`{"name":"x","secret":"abc"}`, `{"name":"x","secret":"<redacted>"}`},
		{`{"token":"eyJhbGc"}`, `{"token":"<redacted>"}`},
	}
	for _, c := range cases {
		got := string(RedactSecrets([]byte(c.in)))
		if !strings.Contains(got, "<redacted>") {
			t.Errorf("in=%q got=%q", c.in, got)
		}
	}
}
```

- [ ] **Step 4: Wire into audit + server**

Edit `internal/api/middleware/audit.go`. Replace `payloadParam = body` with:
```go
payloadParam = RedactSecrets(body)
```

Edit `internal/api/server.go`. Add as the second middleware (before audit):
```go
r.Use(mw.BodyLimit(0))
```

- [ ] **Step 5: Run tests, expect pass**

```bash
go test ./internal/api/middleware/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat(api): add body limit and secret redaction"
```

---

## Task 6: E2E test harness — sparse loopback helpers

**Files:** Create `test/e2e/zfs_helpers.go`.

- [ ] **Step 1: Implement helpers**

Create `test/e2e/zfs_helpers.go`:
```go
//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// makeLoopback creates a sparse file of the given size and attaches it as a loop device.
// Returns the loop device path (e.g. /dev/loop10). Caller must call detachLoopback on cleanup.
func makeLoopback(t *testing.T, sizeBytes int64) string {
	t.Helper()
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "disk.img")
	if err := os.Truncate(imgPath, 0); err != nil {
		// File doesn't exist yet; create it
	}
	if err := os.WriteFile(imgPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(imgPath, sizeBytes); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("losetup", "-f", "--show", imgPath).CombinedOutput()
	if err != nil {
		t.Fatalf("losetup: %v\n%s", err, out)
	}
	loopDev := strings.TrimSpace(string(out))
	t.Cleanup(func() { _ = exec.Command("losetup", "-d", loopDev).Run() })
	return loopDev
}

// destroyPoolIfExists is a best-effort cleanup helper.
func destroyPoolIfExists(name string) {
	_ = exec.Command("zpool", "destroy", "-f", name).Run()
}

// uniquePoolName returns a name unlikely to collide with anything else on the host.
func uniquePoolName(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("e2e_%d", os.Getpid())
	t.Cleanup(func() { destroyPoolIfExists(name) })
	return name
}

// run is a small shell-out helper used in tests.
func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
```

- [ ] **Step 2: Commit**

```bash
git add test/e2e/
git commit -m "test(e2e): add sparse loopback + cleanup helpers"
```

---

## Task 7: E2E pool lifecycle test

**Files:** Create `test/e2e/pool_test.go`.

- [ ] **Step 1: Write the test**

Create `test/e2e/pool_test.go`:
```go
//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

func TestPool_CreateListDestroy(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root for losetup/zpool")
	}
	loop1 := makeLoopback(t, 256<<20) // 256 MiB
	loop2 := makeLoopback(t, 256<<20)

	mgr := &pool.Manager{ZpoolBin: "/sbin/zpool"}
	name := uniquePoolName(t)

	ctx := context.Background()
	if err := mgr.Create(ctx, pool.CreateSpec{
		Name: name,
		Vdevs: []pool.VdevSpec{{
			Type: "mirror", Disks: []string{loop1, loop2},
		}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	pools, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, p := range pools {
		if p.Name == name {
			found = true
			if p.Health != "ONLINE" {
				t.Errorf("health=%q", p.Health)
			}
		}
	}
	if !found {
		t.Fatalf("pool %s not in list", name)
	}

	d, err := mgr.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Status == nil || d.Status.State != "ONLINE" {
		t.Errorf("status=%+v", d.Status)
	}

	if err := mgr.Destroy(ctx, name); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
}
```
Add `import "os"` at the top.

- [ ] **Step 2: Run on a host with ZFS**

```bash
sudo go test -tags=e2e -run TestPool_CreateListDestroy ./test/e2e/...
```
Expected: PASS on a Linux host with ZFS installed.

- [ ] **Step 3: Commit**

```bash
git add test/e2e/
git commit -m "test(e2e): pool create/list/destroy against real ZFS"
```

---

## Task 8: E2E dataset and snapshot lifecycle

**Files:** Create `test/e2e/dataset_test.go`, `test/e2e/snapshot_test.go`.

- [ ] **Step 1: Dataset lifecycle**

Create `test/e2e/dataset_test.go`:
```go
//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

func TestDataset_CreateGetDestroy(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	loop1 := makeLoopback(t, 256<<20)
	pm := &pool.Manager{ZpoolBin: "/sbin/zpool"}
	name := uniquePoolName(t)
	ctx := context.Background()
	if err := pm.Create(ctx, pool.CreateSpec{
		Name: name,
		Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: []string{loop1}}},
	}); err != nil {
		t.Fatalf("pool create: %v", err)
	}

	dm := &dataset.Manager{ZFSBin: "/sbin/zfs"}
	full := name + "/data"
	if err := dm.Create(ctx, dataset.CreateSpec{
		Parent: name, Name: "data", Type: "filesystem",
		Properties: map[string]string{"compression": "lz4"},
	}); err != nil {
		t.Fatalf("dataset create: %v", err)
	}

	d, err := dm.Get(ctx, full)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Props["compression"] != "lz4" {
		t.Errorf("compression=%q", d.Props["compression"])
	}

	if err := dm.SetProps(ctx, full, map[string]string{"compression": "zstd"}); err != nil {
		t.Fatalf("SetProps: %v", err)
	}
	d, _ = dm.Get(ctx, full)
	if d.Props["compression"] != "zstd" {
		t.Errorf("compression after set=%q", d.Props["compression"])
	}

	if err := dm.Destroy(ctx, full, false); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	_ = pm.Destroy(ctx, name)
}
```

- [ ] **Step 2: Snapshot + rollback**

Create `test/e2e/snapshot_test.go`:
```go
//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
)

func TestSnapshot_CreateRollbackDestroy(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}
	loop1 := makeLoopback(t, 256<<20)
	pm := &pool.Manager{ZpoolBin: "/sbin/zpool"}
	dm := &dataset.Manager{ZFSBin: "/sbin/zfs"}
	sm := &snapshot.Manager{ZFSBin: "/sbin/zfs"}
	name := uniquePoolName(t)
	ctx := context.Background()
	if err := pm.Create(ctx, pool.CreateSpec{
		Name: name,
		Vdevs: []pool.VdevSpec{{Type: "stripe", Disks: []string{loop1}}},
	}); err != nil {
		t.Fatalf("pool create: %v", err)
	}
	defer pm.Destroy(ctx, name)

	full := name + "/data"
	if err := dm.Create(ctx, dataset.CreateSpec{Parent: name, Name: "data", Type: "filesystem"}); err != nil {
		t.Fatalf("dataset create: %v", err)
	}

	if err := sm.Create(ctx, full, "snap1", false); err != nil {
		t.Fatalf("snapshot create: %v", err)
	}
	snaps, err := sm.List(ctx, full)
	if err != nil {
		t.Fatalf("snapshot list: %v", err)
	}
	if len(snaps) != 1 || snaps[0].ShortName != "snap1" {
		t.Errorf("snaps=%+v", snaps)
	}

	if err := sm.Rollback(ctx, full+"@snap1"); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := sm.Destroy(ctx, full+"@snap1"); err != nil {
		t.Fatalf("snapshot destroy: %v", err)
	}
	_ = dm.Destroy(ctx, full, false)
}
```

- [ ] **Step 3: Run**

```bash
sudo go test -tags=e2e ./test/e2e/...
```

- [ ] **Step 4: Commit**

```bash
git add test/e2e/
git commit -m "test(e2e): dataset and snapshot lifecycle"
```

---

## Task 9: Dockerfile (used for CI image and dev runs)

**Files:** Create `Dockerfile`.

- [ ] **Step 1: Write multi-stage Dockerfile**

Create `Dockerfile`:
```dockerfile
# syntax=docker/dockerfile:1.6
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOFLAGS="-trimpath" go build -ldflags="-s -w" -o /out/nova-api ./cmd/nova-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nova-api /usr/local/bin/nova-api
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/nova-api"]
```

This image is for **CI and dev only** — production runs as a host systemd service with the binary installed via `.deb` (Task 11).

- [ ] **Step 2: Build locally**

```bash
docker build -t novanas/nova-api:dev .
```

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "build: add multi-stage Dockerfile (CI/dev image)"
```

---

## Task 10: systemd unit + tmpfiles

**Files:** Create `deploy/systemd/nova-api.service`, `deploy/systemd/tmpfiles.d/nova-api.conf`.

- [ ] **Step 1: Service unit**

Create `deploy/systemd/nova-api.service`:
```ini
[Unit]
Description=NovaNAS API server
After=network-online.target postgresql.service redis.service
Wants=network-online.target
Requires=postgresql.service redis.service

[Service]
Type=simple
EnvironmentFile=/etc/nova-api/env
ExecStart=/usr/bin/nova-api
Restart=on-failure
RestartSec=2
User=root
# Hardening
ProtectSystem=strict
ReadWritePaths=/var/log/nova-api /var/lib/nova-api
ProtectHome=yes
PrivateTmp=yes
NoNewPrivileges=yes
# Capabilities needed for ZFS + future host ops
CapabilityBoundingSet=CAP_SYS_ADMIN CAP_NET_ADMIN CAP_DAC_READ_SEARCH CAP_SYS_MODULE
AmbientCapabilities=CAP_SYS_ADMIN CAP_NET_ADMIN
# ZFS device node
DeviceAllow=/dev/zfs rw

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: tmpfiles**

Create `deploy/systemd/tmpfiles.d/nova-api.conf`:
```
d /var/log/nova-api 0750 root root - -
d /var/lib/nova-api 0750 root root - -
d /etc/nova-api    0755 root root - -
```

- [ ] **Step 3: Commit**

```bash
git add deploy/systemd/
git commit -m "deploy: add systemd unit and tmpfiles"
```

---

## Task 11: Debian package skeleton

**Files:** Create `deploy/packaging/debian/{control,changelog,rules,postinst,prerm,nova-api.install}`.

- [ ] **Step 1: control**

Create `deploy/packaging/debian/control`:
```
Source: nova-api
Section: admin
Priority: optional
Maintainer: NovaNAS <root@localhost>
Build-Depends: debhelper-compat (= 13), golang-go (>= 1.22)
Standards-Version: 4.6.2

Package: nova-api
Architecture: any
Depends: ${shlibs:Depends}, ${misc:Depends},
 postgresql (>= 15),
 redis-server,
 zfsutils-linux
Description: NovaNAS storage control plane
 ZFS-based NAS control plane API server.
```

- [ ] **Step 2: changelog**

Create `deploy/packaging/debian/changelog`:
```
nova-api (0.1.0-1) unstable; urgency=low

  * Initial packaging.

 -- NovaNAS <root@localhost>  Tue, 28 Apr 2026 00:00:00 +0000
```

- [ ] **Step 3: rules**

Create `deploy/packaging/debian/rules` (executable):
```makefile
#!/usr/bin/make -f

%:
	dh $@

override_dh_auto_build:
	CGO_ENABLED=0 GOFLAGS=-trimpath go build -ldflags="-s -w" -o nova-api ./cmd/nova-api

override_dh_auto_install:
	install -D -m 0755 nova-api debian/nova-api/usr/bin/nova-api
	install -D -m 0644 deploy/systemd/nova-api.service debian/nova-api/lib/systemd/system/nova-api.service
	install -D -m 0644 deploy/systemd/tmpfiles.d/nova-api.conf debian/nova-api/usr/lib/tmpfiles.d/nova-api.conf

override_dh_auto_test:
	# tests run in CI, not at package time
```
Make executable: `chmod +x deploy/packaging/debian/rules`.

- [ ] **Step 4: postinst**

Create `deploy/packaging/debian/postinst` (executable):
```bash
#!/bin/sh
set -e

case "$1" in
    configure)
        if [ ! -f /etc/nova-api/env ]; then
            mkdir -p /etc/nova-api
            cat > /etc/nova-api/env <<'EOF'
DATABASE_URL=postgres://novanas:novanas@127.0.0.1:5432/novanas?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379/0
LISTEN_ADDR=:8080
LOG_LEVEL=info
EOF
            chmod 0640 /etc/nova-api/env
        fi
        systemd-tmpfiles --create /usr/lib/tmpfiles.d/nova-api.conf || true
        deb-systemd-helper enable nova-api.service || true
        deb-systemd-invoke start nova-api.service || true
    ;;
esac
exit 0
```
Make executable: `chmod +x deploy/packaging/debian/postinst`.

- [ ] **Step 5: prerm**

Create `deploy/packaging/debian/prerm` (executable):
```bash
#!/bin/sh
set -e
case "$1" in
    remove|deconfigure|upgrade)
        deb-systemd-invoke stop nova-api.service || true
    ;;
esac
exit 0
```
Make executable.

- [ ] **Step 6: Document build**

Append to `README.md`:
```
## Packaging

Build a Debian package on a machine with `dpkg-buildpackage`:

```
dpkg-buildpackage -us -uc -b
```
```

- [ ] **Step 7: Commit**

```bash
git add deploy/packaging/ README.md
git commit -m "deploy: add Debian package skeleton"
```

---

## Task 12: Postgres bootstrap SQL

**Files:** Create `deploy/postgres/nova-api-init.sql`.

- [ ] **Step 1: Write init SQL**

Create `deploy/postgres/nova-api-init.sql`:
```sql
-- Run as the postgres superuser on the NAS host:
--   sudo -u postgres psql -f /usr/share/nova-api/nova-api-init.sql

CREATE ROLE novanas WITH LOGIN PASSWORD 'novanas';
CREATE DATABASE novanas OWNER novanas;
\c novanas
GRANT ALL ON SCHEMA public TO novanas;
```

- [ ] **Step 2: Document**

Append to `README.md`:
```
## Bootstrap

1. Install postgres and redis (handled by `.deb` dependencies).
2. Initialize the DB:
   ```
   sudo -u postgres psql -f /usr/share/nova-api/nova-api-init.sql
   ```
3. Apply migrations:
   ```
   make migrate-up DB_URL=postgres://novanas:novanas@127.0.0.1:5432/novanas?sslmode=disable
   ```
4. Start the service:
   ```
   sudo systemctl start nova-api
   curl http://localhost:8080/healthz
   ```
```

- [ ] **Step 3: Commit**

```bash
git add deploy/postgres/ README.md
git commit -m "deploy: add Postgres bootstrap SQL and operational README"
```

---

## Task 13: E2E CI workflow on self-hosted runner

**Files:** Create `.github/workflows/e2e.yml`.

- [ ] **Step 1: Write workflow**

Create `.github/workflows/e2e.yml`:
```yaml
name: e2e
on:
  pull_request:
    types: [labeled, synchronize]
  push:
    branches: [main]

jobs:
  zfs-e2e:
    if: github.event_name == 'push' || contains(github.event.pull_request.labels.*.name, 'needs-e2e')
    runs-on: [self-hosted, zfs]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: ZFS sanity
        run: |
          zfs --version
          zpool --version
          test -e /dev/zfs
      - name: Run e2e tests
        run: sudo -E go test -tags=e2e -timeout=10m ./test/e2e/...
```

- [ ] **Step 2: Document runner setup**

Append to `README.md`:
```
## E2E runner

The `zfs-e2e` workflow runs on a self-hosted runner labeled `self-hosted` and `zfs`. Provision a Linux VM with:
- Ubuntu 22.04+ or Debian 12+
- `zfsutils-linux` installed and `/dev/zfs` accessible
- Runner user has passwordless sudo (for `losetup` and ZFS ops in tests)
- GitHub Actions runner registered with labels `self-hosted,zfs`
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ README.md
git commit -m "ci: add e2e workflow on self-hosted ZFS runner"
```

---

## Task 14: Final hardening pass + smoke run

**Files:** Modify `internal/api/server.go` and `cmd/nova-api/main.go` for last polish.

- [ ] **Step 1: Add sane HTTP timeouts**

Edit `cmd/nova-api/main.go`. Replace the `httpSrv` initializer with:
```go
httpSrv := &http.Server{
    Addr:              cfg.ListenAddr,
    Handler:           srv.Handler(),
    ReadHeaderTimeout: 10 * time.Second,
    ReadTimeout:       30 * time.Second,
    WriteTimeout:      0, // SSE needs unlimited write
    IdleTimeout:       60 * time.Second,
}
```

- [ ] **Step 2: Set SSE-specific timeouts on the route**

We can't easily set per-route timeouts with `chi` and stdlib server, so leave the global `WriteTimeout=0`. Document this in a code comment near the server config.

- [ ] **Step 3: Smoke run end-to-end**

On the dev box (or any box with Postgres+Redis+ZFS):
```bash
make build
sudo cp bin/nova-api /usr/bin/nova-api
sudo cp deploy/systemd/nova-api.service /etc/systemd/system/
sudo cp deploy/systemd/tmpfiles.d/nova-api.conf /usr/lib/tmpfiles.d/
sudo systemd-tmpfiles --create
sudo -u postgres psql -f deploy/postgres/nova-api-init.sql
make migrate-up DB_URL=postgres://novanas:novanas@127.0.0.1:5432/novanas?sslmode=disable
sudo mkdir -p /etc/nova-api
echo 'DATABASE_URL=postgres://novanas:novanas@127.0.0.1:5432/novanas?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379/0
LISTEN_ADDR=:8080
LOG_LEVEL=info' | sudo tee /etc/nova-api/env
sudo systemctl daemon-reload
sudo systemctl start nova-api
curl http://localhost:8080/healthz
```
Expected: `{"status":"ok"}`.

Try a real flow:
```bash
curl http://localhost:8080/api/v1/disks
curl http://localhost:8080/api/v1/pools
```

- [ ] **Step 4: Commit**

```bash
git add cmd/
git commit -m "chore: tune HTTP timeouts and document SSE write-timeout=0"
```

---

## Task 15: Final spec/plan reconciliation

**Files:** Update `docs/superpowers/specs/2026-04-28-novanas-storage-mvp-design.md` if anything in implementation diverged.

- [ ] **Step 1: Diff implementation against spec**

Re-read each section of the spec and verify:
- §3 API surface: every listed endpoint exists.
- §5 schema: matches `internal/store/migrations/0001_init.sql`.
- §6 job flow: matches `internal/jobs/dispatcher.go` and `worker.go`.
- §7 layout: matches the actual repo.
- §8 testing: unit + integration + e2e all present.
- §9 isolation: nova-api on host (not k3s), audit middleware redacts secrets, body limit set.

Document any deviations in a short "Implementation notes" section appended to the spec, e.g. version pins or behavior choices not in the spec.

- [ ] **Step 2: Tag the release**

```bash
git tag -a v0.1.0 -m "Storage MVP v0.1.0"
```

- [ ] **Step 3: Commit any reconciliation edits**

```bash
git add docs/
git commit -m "docs(spec): reconcile spec with v0.1.0 implementation"
```

---

## Done

End state for the storage MVP (Plans 1+2+3 combined):
- `nova-api` Go binary on host systemd, talking to Postgres + Redis on the same host.
- Full read + write API for ZFS pools, datasets, snapshots; URL-encoded full names; metadata sub-routes.
- Asynq-backed job system with SSE streaming; durable history in Postgres; concurrency keyed per pool/dataset.
- Audit log with secret redaction; body size limited; HTTP timeouts set.
- OpenAPI 3.1 spec is the SoT; Go types and TS client both generated from it.
- Unit + integration (testcontainers) + e2e (real ZFS on a self-hosted runner) test suites.
- Dockerfile for CI/dev; systemd unit and `.deb` skeleton for production install on the A/B image (when that lands).
- Operational README with bootstrap, migrate, run, and runner setup instructions.

The future React UI consumes `clients/typescript/src/`. Future plans (auth, shares, apps in k3s, network config, A/B root packaging) are independent of this MVP and follow their own brainstorm → spec → plan cycles.
