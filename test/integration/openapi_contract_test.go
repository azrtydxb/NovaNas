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
	resp, err := http.Get(ts.URL + "/api/v1/disks")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var got []oapi.Disk
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal into []oapi.Disk failed: %v\nbody=%s", err, body)
	}
}

func TestOpenAPI_PoolsListShape(t *testing.T) {
	ts := startTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/pools")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var got []oapi.Pool
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal into []oapi.Pool failed: %v\nbody=%s", err, body)
	}
}

func TestOpenAPI_DatasetsListShape(t *testing.T) {
	ts := startTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/datasets")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var got []oapi.Dataset
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal into []oapi.Dataset failed: %v\nbody=%s", err, body)
	}
}

func TestOpenAPI_SnapshotsListShape(t *testing.T) {
	ts := startTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/snapshots")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var got []oapi.Snapshot
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal into []oapi.Snapshot failed: %v\nbody=%s", err, body)
	}
}

func TestOpenAPI_AcceptedEnvelope(t *testing.T) {
	ts := startTestServer(t)
	body := `{"name":"contract","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	resp, err := http.Post(ts.URL+"/api/v1/pools", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	// Body shape: { "jobId": "..." } — assert jobId is present and parseable.
	var got map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["jobId"] == "" {
		t.Errorf("missing jobId in 202 body: %+v", got)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/api/v1/jobs/") {
		t.Errorf("Location header missing or wrong: %q", loc)
	}
}
