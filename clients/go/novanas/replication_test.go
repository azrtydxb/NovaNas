package novanas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListReplicationJobs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/replication-jobs" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, 200, []ReplicationJob{{
			ID: "j-1", Name: "nightly", Backend: "zfs", Direction: "push",
			Source: ReplicationSource{Dataset: "tank/data"},
		}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.ListReplicationJobs(context.Background())
	if err != nil {
		t.Fatalf("ListReplicationJobs: %v", err)
	}
	if len(got) != 1 || got[0].Name != "nightly" {
		t.Errorf("got=%+v", got)
	}
}

func TestCreateReplicationJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/replication-jobs" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var j ReplicationJob
		_ = json.NewDecoder(r.Body).Decode(&j)
		if j.Backend != "s3" || j.Direction != "push" {
			t.Errorf("body=%+v", j)
		}
		j.ID = "new"
		writeJSON(t, w, 201, j)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.CreateReplicationJob(context.Background(), ReplicationJob{
		Name: "to-s3", Backend: "s3", Direction: "push",
		Source:      ReplicationSource{Path: "/srv/data"},
		Destination: ReplicationDestination{Bucket: "backup"},
	})
	if err != nil {
		t.Fatalf("CreateReplicationJob: %v", err)
	}
	if got.ID != "new" {
		t.Errorf("got=%+v", got)
	}
}

func TestCreateReplicationJob_Validates(t *testing.T) {
	c := &Client{BaseURL: "http://x"}
	if _, err := c.CreateReplicationJob(context.Background(), ReplicationJob{}); err == nil {
		t.Errorf("expected validation error")
	}
}

func TestRunReplicationJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/replication-jobs/abc/run" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, 202, JobDispatchResult{JobID: "task-1"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.RunReplicationJob(context.Background(), "abc")
	if err != nil {
		t.Fatalf("RunReplicationJob: %v", err)
	}
	if got.JobID != "task-1" {
		t.Errorf("got=%+v", got)
	}
}

func TestListReplicationRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/replication-jobs/abc/runs" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("limit=%q", r.URL.Query().Get("limit"))
		}
		writeJSON(t, w, 200, ReplicationRunsPage{
			Runs: []ReplicationRun{{ID: "r-1", JobID: "abc", Outcome: "succeeded"}},
		})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	got, err := c.ListReplicationRuns(context.Background(), "abc", 5, "")
	if err != nil {
		t.Fatalf("ListReplicationRuns: %v", err)
	}
	if len(got.Runs) != 1 || got.Runs[0].Outcome != "succeeded" {
		t.Errorf("got=%+v", got)
	}
}

func TestDeleteReplicationJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/v1/replication-jobs/abc" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	if err := c.DeleteReplicationJob(context.Background(), "abc"); err != nil {
		t.Fatalf("DeleteReplicationJob: %v", err)
	}
}

func TestUpdateReplicationJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.URL.Path != "/api/v1/replication-jobs/abc" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, 200, ReplicationJob{ID: "abc", Name: "renamed", Backend: "zfs", Direction: "push"})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	got, err := c.UpdateReplicationJob(context.Background(), "abc", ReplicationJob{Name: "renamed"})
	if err != nil {
		t.Fatalf("UpdateReplicationJob: %v", err)
	}
	if got.Name != "renamed" {
		t.Errorf("got=%+v", got)
	}
}
