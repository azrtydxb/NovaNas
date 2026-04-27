package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	novanas "github.com/azrtydxb/novanas/packages/sdk/go-client"
)

// fakeAPI is a tiny in-memory stand-in for the NovaNas API. It serves
// the Pool list / status patch and BackendAssignment CRUD endpoints
// the pool poller exercises.
type fakeAPI struct {
	Pools []novanas.Pool
	BAs   map[string]*novanas.BackendAssignment
	// Captured writes so tests can assert what the poller did.
	StatusPatches []map[string]any
}

func newFakeAPI(pools []novanas.Pool) *fakeAPI {
	return &fakeAPI{Pools: pools, BAs: map[string]*novanas.BackendAssignment{}}
}

func (f *fakeAPI) Server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/pools", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": f.Pools})
	})
	mux.HandleFunc("/api/v1/pools/", func(w http.ResponseWriter, r *http.Request) {
		// PATCH /api/v1/pools/<name>
		if r.Method == http.MethodPatch {
			body, _ := io.ReadAll(r.Body)
			var p map[string]any
			_ = json.Unmarshal(body, &p)
			f.StatusPatches = append(f.StatusPatches, p)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v1/backend-assignments", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items := make([]novanas.BackendAssignment, 0, len(f.BAs))
			for _, ba := range f.BAs {
				items = append(items, *ba)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		case http.MethodPost:
			var ba novanas.BackendAssignment
			_ = json.NewDecoder(r.Body).Decode(&ba)
			if _, exists := f.BAs[ba.Metadata.Name]; exists {
				http.Error(w, `{"error":"already_exists"}`, http.StatusConflict)
				return
			}
			f.BAs[ba.Metadata.Name] = &ba
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(ba)
		}
	})
	mux.HandleFunc("/api/v1/backend-assignments/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/v1/backend-assignments/")
		switch r.Method {
		case http.MethodGet:
			if ba, ok := f.BAs[name]; ok {
				_ = json.NewEncoder(w).Encode(ba)
				return
			}
			http.NotFound(w, r)
		case http.MethodPatch:
			ba, ok := f.BAs[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			body, _ := io.ReadAll(r.Body)
			var patch map[string]json.RawMessage
			_ = json.Unmarshal(body, &patch)
			if specRaw, ok := patch["spec"]; ok {
				_ = json.Unmarshal(specRaw, &ba.Spec)
			}
			if statusRaw, ok := patch["status"]; ok {
				_ = json.Unmarshal(statusRaw, &ba.Status)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(ba)
		case http.MethodDelete:
			delete(f.BAs, name)
			w.WriteHeader(http.StatusNoContent)
		}
	})
	return httptest.NewServer(mux)
}

func TestPoolPoller_CreatesBackendAssignmentsPerNode(t *testing.T) {
	pool := novanas.Pool{
		APIVersion: "novanas.io/v1alpha1",
		Kind:       "StoragePool",
		Metadata:   novanas.ObjectMeta{Name: "fast"},
		Spec: novanas.PoolSpec{
			BackendType: "raw",
			NodeSelector: &novanas.LabelSelector{
				MatchLabels: map[string]string{"storage": "fast"},
			},
			DeviceFilter: &novanas.DeviceFilter{PreferredClass: "nvme"},
		},
	}
	api := newFakeAPI([]novanas.Pool{pool})
	srv := api.Server()
	defer srv.Close()

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	nodes := []runtime.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{"storage": "fast"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2", Labels: map[string]string{"storage": "fast"}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-3", Labels: map[string]string{"storage": "slow"}}},
	}
	k := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(nodes...).Build()

	poller := &PoolPoller{
		K8s: k,
		API: novanas.New(srv.URL, ""),
	}
	poller.tick(context.Background())

	if got := len(api.BAs); got != 2 {
		t.Fatalf("expected 2 BackendAssignments (one per matching node), got %d", got)
	}
	for _, name := range []string{"fast-node-1", "fast-node-2"} {
		ba, ok := api.BAs[name]
		if !ok {
			t.Errorf("missing BackendAssignment %q", name)
			continue
		}
		if ba.Spec.PoolRef != "fast" {
			t.Errorf("%s: poolRef = %q, want %q", name, ba.Spec.PoolRef, "fast")
		}
		if ba.Spec.BackendType != "raw" {
			t.Errorf("%s: backendType = %q, want raw", name, ba.Spec.BackendType)
		}
		if ba.Spec.DeviceFilter == nil || ba.Spec.DeviceFilter.PreferredClass != "nvme" {
			t.Errorf("%s: deviceFilter wrong = %+v", name, ba.Spec.DeviceFilter)
		}
		if ba.Metadata.Labels["novanas.io/pool"] != "fast" {
			t.Errorf("%s: missing pool label", name)
		}
	}
	if _, leaked := api.BAs["fast-node-3"]; leaked {
		t.Errorf("BackendAssignment for non-matching node-3 should not be created")
	}
	// Pool status should have been patched at least once (Ready phase).
	if len(api.StatusPatches) == 0 {
		t.Error("expected pool status patch")
	}
}

func TestPoolPoller_DeletesOrphans(t *testing.T) {
	pool := novanas.Pool{
		APIVersion: "novanas.io/v1alpha1",
		Kind:       "StoragePool",
		Metadata:   novanas.ObjectMeta{Name: "fast"},
		Spec: novanas.PoolSpec{
			BackendType:  "raw",
			NodeSelector: &novanas.LabelSelector{MatchLabels: map[string]string{"storage": "fast"}},
		},
	}
	api := newFakeAPI([]novanas.Pool{pool})
	// Pre-seed an orphan: BA for a node that no longer exists.
	api.BAs["fast-node-gone"] = &novanas.BackendAssignment{
		APIVersion: "novanas.io/v1alpha1",
		Kind:       "BackendAssignment",
		Metadata: novanas.ObjectMeta{
			Name:   "fast-node-gone",
			Labels: map[string]string{"novanas.io/pool": "fast", "novanas.io/node": "node-gone"},
		},
		Spec: novanas.BackendAssignmentSpec{PoolRef: "fast", NodeName: "node-gone", BackendType: "raw"},
	}
	srv := api.Server()
	defer srv.Close()

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	nodes := []runtime.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{"storage": "fast"}}},
	}
	k := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(nodes...).Build()

	poller := &PoolPoller{K8s: k, API: novanas.New(srv.URL, "")}
	poller.tick(context.Background())

	if _, exists := api.BAs["fast-node-gone"]; exists {
		t.Error("expected orphan BackendAssignment to be deleted")
	}
	if _, ok := api.BAs["fast-node-1"]; !ok {
		t.Error("expected BackendAssignment for node-1 to be created")
	}
}
