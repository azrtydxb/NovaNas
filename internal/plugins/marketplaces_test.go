package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// testTrustKeyPEM is an arbitrary marker string. We never call the
// real Verifier in these tests; the seed/registry round-trip only
// stores the PEM verbatim.
const testTrustKeyPEM = "PEM-PLACEHOLDER"

func TestSeedOfficialIfMissing_CreatesLockedRow(t *testing.T) {
	store := NewMemMarketplacesStore()
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "trust.pub")
	if err := os.WriteFile(keyPath, []byte(testTrustKeyPEM), 0o644); err != nil {
		t.Fatal(err)
	}
	m, created, err := SeedOfficialIfMissing(context.Background(), store, "https://example.test/index.json", keyPath)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true on first seed")
	}
	if !m.Locked || !m.Enabled {
		t.Errorf("expected locked+enabled, got locked=%v enabled=%v", m.Locked, m.Enabled)
	}
	if m.Name != OfficialMarketplaceName {
		t.Errorf("name = %q, want %q", m.Name, OfficialMarketplaceName)
	}

	// Idempotent: a second call must not create a new row.
	m2, created2, err := SeedOfficialIfMissing(context.Background(), store, "https://other.test/", keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if created2 {
		t.Errorf("expected created=false on re-seed")
	}
	if m2.ID != m.ID {
		t.Errorf("re-seed returned different ID: %s vs %s", m2.ID, m.ID)
	}
	all, _ := store.List(context.Background())
	if len(all) != 1 {
		t.Errorf("expected 1 row, got %d", len(all))
	}
}

func TestMemStore_ListLockedFirst(t *testing.T) {
	s := NewMemMarketplacesStore()
	ctx := context.Background()
	_, _ = s.Create(ctx, Marketplace{Name: "alpha", Enabled: true})
	_, _ = s.Create(ctx, Marketplace{Name: OfficialMarketplaceName, Locked: true, Enabled: true})
	_, _ = s.Create(ctx, Marketplace{Name: "beta", Enabled: true})
	rows, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}
	if !rows[0].Locked {
		t.Errorf("row 0 = %q, expected locked-first", rows[0].Name)
	}
}

func TestMultiMarketplaceClient_FetchAllAndFindVersion(t *testing.T) {
	// Two fake marketplaces; the second has a plugin not present in
	// the first; the first has a plugin present in both. FindVersion
	// without marketplaceID should prefer the locked entry.
	idxA := Index{Version: 1, Plugins: []IndexPlugin{
		{Name: "shared", Versions: []IndexVersion{{Version: "1.0.0", TarballURL: "https://a/p.tgz", SignatureURL: "https://a/p.sig"}}},
		{Name: "only-a", Versions: []IndexVersion{{Version: "0.1.0"}}},
	}}
	idxB := Index{Version: 1, Plugins: []IndexPlugin{
		{Name: "shared", Versions: []IndexVersion{{Version: "2.0.0"}}},
		{Name: "only-b", Versions: []IndexVersion{{Version: "1.0.0"}}},
	}}
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(idxA)
	}))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(idxB)
	}))
	defer srvB.Close()

	store := NewMemMarketplacesStore()
	ctx := context.Background()
	mA, _ := store.Create(ctx, Marketplace{
		ID: uuid.New(), Name: OfficialMarketplaceName, IndexURL: srvA.URL,
		TrustKeyPEM: testTrustKeyPEM, Locked: true, Enabled: true,
	})
	mB, _ := store.Create(ctx, Marketplace{
		ID: uuid.New(), Name: "extra", IndexURL: srvB.URL,
		TrustKeyPEM: testTrustKeyPEM, Locked: false, Enabled: true,
	})
	mc := NewMultiMarketplaceClient(store, nil)

	merged, err := mc.FetchAll(ctx)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(merged.Plugins) != 4 {
		t.Errorf("expected 4 entries (2 sources + collisions), got %d", len(merged.Plugins))
	}
	// "shared" must appear from both sources.
	count := 0
	for _, p := range merged.Plugins {
		if p.Name == "shared" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected shared in 2 sources, got %d", count)
	}
	if len(merged.Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(merged.Sources))
	}

	// FindVersion with empty marketplaceID prefers locked.
	_, ver, mp, err := mc.FindVersion(ctx, "shared", "", "")
	if err != nil {
		t.Fatalf("FindVersion: %v", err)
	}
	if mp.ID != mA.ID {
		t.Errorf("expected locked marketplace %s, got %s", mA.ID, mp.ID)
	}
	if ver.Version != "1.0.0" {
		t.Errorf("version = %q, want 1.0.0", ver.Version)
	}

	// Pinned to extra → version 2.0.0
	_, ver, mp, err = mc.FindVersion(ctx, "shared", "", mB.ID.String())
	if err != nil {
		t.Fatalf("FindVersion(pinned): %v", err)
	}
	if mp.ID != mB.ID {
		t.Errorf("pinned wrong marketplace: %s", mp.ID)
	}
	if ver.Version != "2.0.0" {
		t.Errorf("version = %q, want 2.0.0", ver.Version)
	}
}

func TestMultiMarketplaceClient_DisabledExcluded(t *testing.T) {
	store := NewMemMarketplacesStore()
	ctx := context.Background()
	idx := Index{Version: 1, Plugins: []IndexPlugin{{Name: "x", Versions: []IndexVersion{{Version: "1.0.0"}}}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(idx)
	}))
	defer srv.Close()
	_, _ = store.Create(ctx, Marketplace{
		ID: uuid.New(), Name: "off", IndexURL: srv.URL,
		TrustKeyPEM: testTrustKeyPEM, Enabled: false,
	})
	mc := NewMultiMarketplaceClient(store, nil)
	merged, err := mc.FetchAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Sources) != 0 {
		t.Errorf("disabled marketplace leaked into FetchAll: %+v", merged.Sources)
	}
}
