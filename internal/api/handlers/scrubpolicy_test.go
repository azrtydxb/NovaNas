package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// fakeScrubQueries is the in-memory backing store for ScrubPolicyQueries.
type fakeScrubQueries struct {
	rows map[string]storedb.ScrubPolicy
}

func newFakeScrubQueries() *fakeScrubQueries {
	return &fakeScrubQueries{rows: map[string]storedb.ScrubPolicy{}}
}

func sKey(id pgtype.UUID) string { return uuid.UUID(id.Bytes).String() }

func (f *fakeScrubQueries) ListScrubPolicies(_ context.Context) ([]storedb.ScrubPolicy, error) {
	out := make([]storedb.ScrubPolicy, 0, len(f.rows))
	for _, v := range f.rows {
		out = append(out, v)
	}
	return out, nil
}
func (f *fakeScrubQueries) GetScrubPolicy(_ context.Context, id pgtype.UUID) (storedb.ScrubPolicy, error) {
	if v, ok := f.rows[sKey(id)]; ok {
		return v, nil
	}
	return storedb.ScrubPolicy{}, pgx.ErrNoRows
}
func (f *fakeScrubQueries) CreateScrubPolicy(_ context.Context, arg storedb.CreateScrubPolicyParams) (storedb.ScrubPolicy, error) {
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	row := storedb.ScrubPolicy{
		ID: id, Name: arg.Name, Pools: arg.Pools, Cron: arg.Cron,
		Priority: arg.Priority, Enabled: arg.Enabled, Builtin: arg.Builtin,
	}
	f.rows[sKey(id)] = row
	return row, nil
}
func (f *fakeScrubQueries) UpdateScrubPolicy(_ context.Context, arg storedb.UpdateScrubPolicyParams) (storedb.ScrubPolicy, error) {
	row, ok := f.rows[sKey(arg.ID)]
	if !ok {
		return storedb.ScrubPolicy{}, pgx.ErrNoRows
	}
	row.Pools = arg.Pools
	row.Cron = arg.Cron
	row.Priority = arg.Priority
	row.Enabled = arg.Enabled
	f.rows[sKey(arg.ID)] = row
	return row, nil
}
func (f *fakeScrubQueries) DeleteScrubPolicy(_ context.Context, id pgtype.UUID) error {
	delete(f.rows, sKey(id))
	return nil
}

func newScrubRouter(h *ScrubPolicyHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/scrub-policies", h.List)
	r.Post("/scrub-policies", h.Create)
	r.Get("/scrub-policies/{id}", h.Get)
	r.Patch("/scrub-policies/{id}", h.Update)
	r.Delete("/scrub-policies/{id}", h.Delete)
	r.Post("/pools/{name}/scrub", h.TriggerPoolScrub)
	return r
}

func TestScrubPolicyCreate_OK(t *testing.T) {
	q := newFakeScrubQueries()
	h := &ScrubPolicyHandler{Logger: newDiscardLogger(), Q: q, Dispatcher: &fakeDispatcher{}}
	r := newScrubRouter(h)

	body := `{"name":"weekly","pools":"tank","cron":"0 2 * * 0","priority":"medium","enabled":true}`
	req := httptest.NewRequest("POST", "/scrub-policies", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["name"] != "weekly" {
		t.Errorf("got=%v", got)
	}
}

func TestScrubPolicyCreate_BadCron(t *testing.T) {
	q := newFakeScrubQueries()
	h := &ScrubPolicyHandler{Logger: newDiscardLogger(), Q: q, Dispatcher: &fakeDispatcher{}}
	r := newScrubRouter(h)

	body := `{"name":"weekly","cron":"not a cron","enabled":true}`
	req := httptest.NewRequest("POST", "/scrub-policies", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestScrubPolicyCreate_BadPriority(t *testing.T) {
	q := newFakeScrubQueries()
	h := &ScrubPolicyHandler{Logger: newDiscardLogger(), Q: q, Dispatcher: &fakeDispatcher{}}
	r := newScrubRouter(h)

	body := `{"name":"weekly","cron":"0 2 * * 0","priority":"urgent","enabled":true}`
	req := httptest.NewRequest("POST", "/scrub-policies", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestScrubPolicyListGetUpdateDelete(t *testing.T) {
	q := newFakeScrubQueries()
	h := &ScrubPolicyHandler{Logger: newDiscardLogger(), Q: q, Dispatcher: &fakeDispatcher{}}
	rt := newScrubRouter(h)

	// Create one.
	row, _ := q.CreateScrubPolicy(context.Background(), storedb.CreateScrubPolicyParams{
		Name: "p1", Pools: "tank", Cron: "0 2 * * 0", Priority: "high", Enabled: true,
	})
	id := uuid.UUID(row.ID.Bytes).String()

	// List.
	w := httptest.NewRecorder()
	rt.ServeHTTP(w, httptest.NewRequest("GET", "/scrub-policies", nil))
	if w.Code != 200 {
		t.Fatalf("list status=%d", w.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["name"] != "p1" {
		t.Errorf("list=%v", list)
	}

	// Get.
	w = httptest.NewRecorder()
	rt.ServeHTTP(w, httptest.NewRequest("GET", "/scrub-policies/"+id, nil))
	if w.Code != 200 {
		t.Fatalf("get status=%d", w.Code)
	}

	// Update.
	body := `{"cron":"0 3 * * 0","pools":"*","priority":"low","enabled":false}`
	w = httptest.NewRecorder()
	rt.ServeHTTP(w, httptest.NewRequest("PATCH", "/scrub-policies/"+id, strings.NewReader(body)))
	if w.Code != 200 {
		t.Fatalf("update status=%d body=%s", w.Code, w.Body.String())
	}

	// Delete.
	w = httptest.NewRecorder()
	rt.ServeHTTP(w, httptest.NewRequest("DELETE", "/scrub-policies/"+id, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", w.Code)
	}
}

func TestTriggerPoolScrub_Dispatches(t *testing.T) {
	q := newFakeScrubQueries()
	disp := &fakeDispatcher{out: uuid.New()}
	h := &ScrubPolicyHandler{Logger: newDiscardLogger(), Q: q, Dispatcher: disp}
	r := newScrubRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/pools/tank/scrub", bytes.NewReader(nil)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(disp.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(disp.calls))
	}
	if disp.calls[0].Target != "tank" {
		t.Errorf("target=%s", disp.calls[0].Target)
	}
}

func TestTriggerPoolScrub_BadName(t *testing.T) {
	q := newFakeScrubQueries()
	h := &ScrubPolicyHandler{Logger: newDiscardLogger(), Q: q, Dispatcher: &fakeDispatcher{}}
	r := newScrubRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/pools/--bad/scrub", bytes.NewReader(nil)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d", w.Code)
	}
}
