package novanas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryLogsRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		if r.URL.Path != "/api/v1/logs/query" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != `{job="x"}` {
			t.Errorf("query=%q", r.URL.Query().Get("query"))
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	got, err := c.QueryLogsRange(context.Background(), `{job="x"}`, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "success" {
		t.Errorf("got=%+v", got)
	}
}

func TestQueryLogsRangeRequiresQuery(t *testing.T) {
	c := &Client{BaseURL: "http://x"}
	if _, err := c.QueryLogsRange(context.Background(), "", "", "", 0); err == nil {
		t.Error("expected error")
	}
}

func TestListLogLabelValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/logs/labels/job/values" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":["a","b"]}`))
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).ListLogLabelValues(context.Background(), "job")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Data) != 2 {
		t.Errorf("got=%+v", got)
	}
}
