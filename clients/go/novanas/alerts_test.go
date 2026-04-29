package novanas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListAlerts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		if r.URL.Path != "/api/v1/alerts" {
			t.Errorf("path=%s", r.URL.Path)
		}
		writeJSON(t, w, 200, []Alert{{Fingerprint: "abc", Labels: map[string]string{"alertname": "X"}}})
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	got, err := c.ListAlerts(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Fingerprint != "abc" {
		t.Errorf("got=%+v", got)
	}
}

func TestGetAlertByFingerprint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/alerts/abc" {
			t.Errorf("path=%s", r.URL.Path)
		}
		writeJSON(t, w, 200, Alert{Fingerprint: "abc"})
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetAlert(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.Fingerprint != "abc" {
		t.Errorf("got=%+v", got)
	}
}

func TestCreateAndDeleteSilence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			if r.URL.Path != "/api/v1/alert-silences" {
				t.Errorf("create path=%s", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"silenceID":"s1"}`))
		case "DELETE":
			if r.URL.Path != "/api/v1/alert-silences/s1" {
				t.Errorf("delete path=%s", r.URL.Path)
			}
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	body, err := c.CreateSilence(context.Background(), Silence{
		Matchers:  []SilenceMatcher{{Name: "alertname", Value: "X"}},
		StartsAt:  "2026-04-29T00:00:00Z",
		EndsAt:    "2026-04-29T02:00:00Z",
		CreatedBy: "test",
		Comment:   "t",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if string(body) == "" {
		t.Errorf("empty body")
	}
	if err := c.DeleteSilence(context.Background(), "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
