//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPoolCreate_FullFlow(t *testing.T) {
	ts := startTestServer(t)

	body := `{"name":"tankint","vdevs":[{"type":"mirror","disks":["/dev/A","/dev/B"]}]}`
	resp, err := http.Post(ts.URL+"/api/v1/pools", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	var got map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	jobID := got["jobId"]
	if jobID == "" {
		t.Fatal("missing jobId")
	}

	// Poll for terminal state.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r, err := http.Get(ts.URL + "/api/v1/jobs/" + jobID)
		if err != nil {
			t.Fatal(err)
		}
		var job struct {
			State string `json:"state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&job)
		_ = r.Body.Close()
		if job.State == "succeeded" {
			return
		}
		if job.State == "failed" || job.State == "cancelled" {
			t.Fatalf("job %s ended in state %s", jobID, job.State)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("job did not reach terminal state in 10s")
}
