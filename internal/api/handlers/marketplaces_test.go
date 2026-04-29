package handlers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/plugins"
)

var (
	pemOnce sync.Once
	pemStr  string
)

// handlerTestPEM lazily generates a real ECDSA P-256 public key in
// PEM form so the validatePublicKeyPEM check accepts it.
func handlerTestPEM() string {
	pemOnce.Do(func() {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			panic(err)
		}
		der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			panic(err)
		}
		pemStr = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	})
	return pemStr
}

func newMarketplacesRouter(h *MarketplacesHandler) chi.Router {
	r := chi.NewRouter()
	r.Get("/marketplaces", h.List)
	r.Get("/marketplaces/{id}", h.Get)
	r.Post("/marketplaces", h.Create)
	r.Patch("/marketplaces/{id}", h.Patch)
	r.Delete("/marketplaces/{id}", h.Delete)
	r.Post("/marketplaces/{id}/refresh-trust-key", h.RefreshTrustKey)
	return r
}

func seedLocked(t *testing.T, store plugins.MarketplacesStore) plugins.Marketplace {
	t.Helper()
	m, err := store.Create(context.Background(), plugins.Marketplace{
		ID:          uuid.New(),
		Name:        plugins.OfficialMarketplaceName,
		IndexURL:    "https://example/index.json",
		TrustKeyPEM: handlerTestPEM(),
		Locked:      true,
		Enabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMarketplaces_ListAndGet(t *testing.T) {
	store := plugins.NewMemMarketplacesStore()
	locked := seedLocked(t, store)
	h := &MarketplacesHandler{Store: store}
	router := newMarketplacesRouter(h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/marketplaces", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list []MarketplaceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != plugins.OfficialMarketplaceName {
		t.Fatalf("list = %+v", list)
	}
	if !list[0].Locked {
		t.Errorf("expected locked=true")
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/marketplaces/"+locked.ID.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMarketplaces_DeleteLockedReturns409(t *testing.T) {
	store := plugins.NewMemMarketplacesStore()
	locked := seedLocked(t, store)
	h := &MarketplacesHandler{Store: store}
	router := newMarketplacesRouter(h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/marketplaces/"+locked.ID.String(), nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	// Row must still exist.
	if _, err := store.Get(context.Background(), locked.ID); err != nil {
		t.Errorf("row was deleted despite 409: %v", err)
	}
}

func TestMarketplaces_DisableLockedReturns409(t *testing.T) {
	store := plugins.NewMemMarketplacesStore()
	locked := seedLocked(t, store)
	h := &MarketplacesHandler{Store: store}
	router := newMarketplacesRouter(h)

	body, _ := json.Marshal(MarketplacePatchRequest{Enabled: ptrBool(false)})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/marketplaces/"+locked.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	got, _ := store.Get(context.Background(), locked.ID)
	if !got.Enabled {
		t.Errorf("locked row was disabled")
	}
}

func TestMarketplaces_CreateValidates(t *testing.T) {
	// Run a fake index server (HEAD reachable) and a fake trust-key
	// server (returns the PEM). Confirm the row lands enabled, not
	// locked, with the fetched PEM pinned.
	idxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer idxSrv.Close()
	keySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(handlerTestPEM()))
	}))
	defer keySrv.Close()

	store := plugins.NewMemMarketplacesStore()
	h := &MarketplacesHandler{Store: store}
	router := newMarketplacesRouter(h)

	body, _ := json.Marshal(MarketplaceCreateRequest{
		Name: "truecharts", IndexURL: idxSrv.URL, TrustKeyURL: keySrv.URL,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/marketplaces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var got MarketplaceResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Locked {
		t.Errorf("non-official entry should not be locked")
	}
	if got.TrustKeyPEM == "" {
		t.Errorf("PEM not pinned")
	}
	if got.IndexURL != idxSrv.URL {
		t.Errorf("indexURL roundtrip: %q", got.IndexURL)
	}
}

func TestMarketplaces_CreateRejectsReservedName(t *testing.T) {
	store := plugins.NewMemMarketplacesStore()
	h := &MarketplacesHandler{Store: store}
	router := newMarketplacesRouter(h)

	body, _ := json.Marshal(MarketplaceCreateRequest{
		Name: plugins.OfficialMarketplaceName, IndexURL: "https://x", TrustKeyURL: "https://y",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/marketplaces", bytes.NewReader(body))
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMarketplaces_CreateRejectsBadKey(t *testing.T) {
	idxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer idxSrv.Close()
	keySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not a pem key"))
	}))
	defer keySrv.Close()

	store := plugins.NewMemMarketplacesStore()
	h := &MarketplacesHandler{Store: store}
	router := newMarketplacesRouter(h)

	body, _ := json.Marshal(MarketplaceCreateRequest{Name: "garbage", IndexURL: idxSrv.URL, TrustKeyURL: keySrv.URL})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/marketplaces", bytes.NewReader(body))
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad key, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMarketplaces_RefreshTrustKey(t *testing.T) {
	keySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(handlerTestPEM()))
	}))
	defer keySrv.Close()

	store := plugins.NewMemMarketplacesStore()
	ctx := context.Background()
	created, _ := store.Create(ctx, plugins.Marketplace{
		ID: uuid.New(), Name: "x", IndexURL: "https://example",
		TrustKeyURL: keySrv.URL, TrustKeyPEM: "old",
	})
	h := &MarketplacesHandler{Store: store}
	router := newMarketplacesRouter(h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/marketplaces/"+created.ID.String()+"/refresh-trust-key", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	got, _ := store.Get(ctx, created.ID)
	if got.TrustKeyPEM == "old" {
		t.Errorf("PEM was not refreshed")
	}
}

func ptrBool(b bool) *bool { return &b }
