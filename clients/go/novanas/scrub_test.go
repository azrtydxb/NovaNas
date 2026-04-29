package novanas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListScrubPolicies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		if r.Method != "GET" || r.URL.Path != "/api/v1/scrub-policies" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, 200, []ScrubPolicy{{
			ID: "p-1", Name: "weekly", Pools: "tank", Cron: "0 2 * * 0",
			Priority: "high", Enabled: true,
		}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.ListScrubPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListScrubPolicies: %v", err)
	}
	if len(got) != 1 || got[0].Name != "weekly" {
		t.Errorf("got=%+v", got)
	}
}

func TestGetScrubPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/scrub-policies/abc" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, 200, ScrubPolicy{ID: "abc", Name: "weekly"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.GetScrubPolicy(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetScrubPolicy: %v", err)
	}
	if got.Name != "weekly" {
		t.Errorf("got=%+v", got)
	}
}

func TestCreateScrubPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/scrub-policies" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		var p ScrubPolicy
		_ = json.NewDecoder(r.Body).Decode(&p)
		if p.Name != "weekly" || p.Cron != "0 2 * * 0" {
			t.Errorf("body=%+v", p)
		}
		p.ID = "new"
		writeJSON(t, w, 201, p)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.CreateScrubPolicy(context.Background(), ScrubPolicy{
		Name: "weekly", Cron: "0 2 * * 0", Pools: "tank", Priority: "high", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateScrubPolicy: %v", err)
	}
	if got.ID != "new" {
		t.Errorf("got=%+v", got)
	}
}

func TestCreateScrubPolicy_Validates(t *testing.T) {
	c := &Client{BaseURL: "http://x"}
	if _, err := c.CreateScrubPolicy(context.Background(), ScrubPolicy{}); err == nil {
		t.Errorf("expected error for empty policy")
	}
}

func TestUpdateScrubPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.URL.Path != "/api/v1/scrub-policies/abc" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		writeJSON(t, w, 200, ScrubPolicy{ID: "abc", Name: "weekly", Cron: "0 3 * * 0"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.UpdateScrubPolicy(context.Background(), "abc", ScrubPolicy{Cron: "0 3 * * 0"})
	if err != nil {
		t.Fatalf("UpdateScrubPolicy: %v", err)
	}
	if got.Cron != "0 3 * * 0" {
		t.Errorf("got=%+v", got)
	}
}

func TestDeleteScrubPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/api/v1/scrub-policies/abc" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.DeleteScrubPolicy(context.Background(), "abc"); err != nil {
		t.Fatalf("DeleteScrubPolicy: %v", err)
	}
}

func TestScrubPool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/pools/tank/scrub" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if !strings.Contains(r.URL.RawQuery, "action=start") {
			t.Errorf("missing action=start: %s", r.URL.RawQuery)
		}
		w.Header().Set("Location", "/api/v1/jobs/jid")
		writeJSON(t, w, 202, map[string]string{"jobId": "jid"})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	got, err := c.ScrubPool(context.Background(), "tank", "start")
	if err != nil {
		t.Fatalf("ScrubPool: %v", err)
	}
	if got.ID != "jid" {
		t.Errorf("job=%+v", got)
	}
}
