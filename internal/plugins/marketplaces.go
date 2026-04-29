// Package plugins — multi-marketplace support.
//
// MultiMarketplaceClient holds the registry of registered marketplaces
// (DB-backed in production, in-memory for tests via the
// MarketplacesStore interface) and dispatches FetchAll / FindVersion /
// DownloadAndVerify across all enabled marketplaces.
//
// Each marketplace has its OWN pinned trust key (`trust_key_pem`).
// Trust pinning is at the source-of-truth at install time; the
// `trust_key_url` is stored only as a hint for explicit
// refresh-trust-key calls. We never auto-refresh — a malicious
// marketplace could otherwise rotate its key after gaining trust.
package plugins

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// OfficialMarketplaceName is the reserved, immutable name of the
// locked novanas-official marketplace entry. The bootstrap path in
// nova-api seeds this row from MARKETPLACE_INDEX_URL +
// MARKETPLACE_TRUST_KEY_PATH on first start.
const OfficialMarketplaceName = "novanas-official"

// Marketplace is one row of the marketplaces registry, exposed at the
// plugins-package level so handlers/tests don't need to import the
// generated storedb package directly.
type Marketplace struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	IndexURL    string    `json:"indexUrl"`
	TrustKeyURL string    `json:"trustKeyUrl"`
	TrustKeyPEM string    `json:"trustKeyPem"`
	Locked      bool      `json:"locked"`
	Enabled     bool      `json:"enabled"`
	AddedBy     string    `json:"addedBy,omitempty"`
	AddedAt     time.Time `json:"addedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// MarketplacesStore is the registry persistence interface. The pgx
// implementation lives in marketplaces_pgxstore.go. Tests use an
// in-memory implementation.
type MarketplacesStore interface {
	List(ctx context.Context) ([]Marketplace, error)
	ListEnabled(ctx context.Context) ([]Marketplace, error)
	Get(ctx context.Context, id uuid.UUID) (Marketplace, error)
	GetByName(ctx context.Context, name string) (Marketplace, error)
	Create(ctx context.Context, m Marketplace) (Marketplace, error)
	UpdateEnabled(ctx context.Context, id uuid.UUID, enabled bool) (Marketplace, error)
	UpdateTrustKey(ctx context.Context, id uuid.UUID, pem string) (Marketplace, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// MergedIndex is the union of every enabled marketplace's index. Each
// plugin entry is tagged with the marketplace ID and name it came
// from; collisions (same plugin name across multiple marketplaces)
// produce multiple entries.
type MergedIndex struct {
	Version int                  `json:"version"`
	Updated time.Time            `json:"updated,omitempty"`
	Plugins []MergedIndexPlugin  `json:"plugins"`
	Sources []MergedIndexSource  `json:"sources"`
}

// MergedIndexPlugin is one tagged plugin entry in the merged index.
type MergedIndexPlugin struct {
	IndexPlugin
	MarketplaceID   uuid.UUID `json:"marketplaceId"`
	MarketplaceName string    `json:"marketplaceName"`
}

// MergedIndexSource describes one marketplace contribution to the
// merged index — useful for the GET /marketplaces/{id} detail view
// and the audit-which-source-published-this UX.
type MergedIndexSource struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	IndexURL    string    `json:"indexUrl"`
	PluginCount int       `json:"pluginCount"`
	Status      string    `json:"status"`           // "ok" | "error"
	Error       string    `json:"error,omitempty"`
	FetchedAt   time.Time `json:"fetchedAt"`
}

// MultiMarketplaceClient is the registry-aware orchestration layer
// that the plugin Manager talks to instead of a single
// MarketplaceClient.
type MultiMarketplaceClient struct {
	Store MarketplacesStore
	HTTP  *http.Client

	// PluginsRoot is where per-marketplace pinned trust-key PEM files
	// are written for the Verifier to read. When empty, falls back to
	// DefaultPluginsRoot.
	PluginsRoot string

	mu      sync.RWMutex
	clients map[uuid.UUID]*MarketplaceClient // by marketplace ID
}

// NewMultiMarketplaceClient constructs the multi-source client. The
// HTTP client falls back to a 30s-timeout default.
func NewMultiMarketplaceClient(store MarketplacesStore, h *http.Client) *MultiMarketplaceClient {
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	return &MultiMarketplaceClient{
		Store:   store,
		HTTP:    h,
		clients: map[uuid.UUID]*MarketplaceClient{},
	}
}

// clientFor returns a per-marketplace MarketplaceClient, constructing
// it on first use. The cache is keyed by ID; callers refreshing trust
// keys or removing a marketplace must call invalidate(id).
func (mc *MultiMarketplaceClient) clientFor(m Marketplace) *MarketplaceClient {
	mc.mu.RLock()
	c, ok := mc.clients[m.ID]
	mc.mu.RUnlock()
	if ok {
		return c
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if c, ok := mc.clients[m.ID]; ok {
		return c
	}
	c = NewMarketplaceClientFor(m, mc.HTTP)
	mc.clients[m.ID] = c
	return c
}

func (mc *MultiMarketplaceClient) invalidate(id uuid.UUID) {
	mc.mu.Lock()
	delete(mc.clients, id)
	mc.mu.Unlock()
}

// FetchAll fetches every enabled marketplace's index in parallel and
// merges them into a tagged MergedIndex.
func (mc *MultiMarketplaceClient) FetchAll(ctx context.Context) (*MergedIndex, error) {
	if mc == nil || mc.Store == nil {
		return nil, errors.New("plugins: multi-marketplace store not configured")
	}
	rows, err := mc.Store.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugins: list marketplaces: %w", err)
	}
	out := &MergedIndex{Version: 1, Updated: time.Now().UTC()}
	type result struct {
		m       Marketplace
		idx     *Index
		err     error
		fetched time.Time
	}
	results := make([]result, len(rows))
	var wg sync.WaitGroup
	for i, m := range rows {
		i, m := i, m
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := mc.clientFor(m)
			idx, err := c.FetchIndex(ctx, false)
			results[i] = result{m: m, idx: idx, err: err, fetched: time.Now().UTC()}
		}()
	}
	wg.Wait()
	for _, r := range results {
		src := MergedIndexSource{
			ID:        r.m.ID,
			Name:      r.m.Name,
			IndexURL:  r.m.IndexURL,
			Status:    "ok",
			FetchedAt: r.fetched,
		}
		if r.err != nil {
			src.Status = "error"
			src.Error = r.err.Error()
			out.Sources = append(out.Sources, src)
			continue
		}
		src.PluginCount = len(r.idx.Plugins)
		out.Sources = append(out.Sources, src)
		for _, p := range r.idx.Plugins {
			out.Plugins = append(out.Plugins, MergedIndexPlugin{
				IndexPlugin:     p,
				MarketplaceID:   r.m.ID,
				MarketplaceName: r.m.Name,
			})
		}
	}
	// Stable order for callers: locked first, then by marketplace name,
	// then by plugin name.
	sort.SliceStable(out.Plugins, func(i, j int) bool {
		a, b := out.Plugins[i], out.Plugins[j]
		// Locked first: we lookup via Sources pos.
		ai := indexOfSource(out.Sources, a.MarketplaceID)
		bi := indexOfSource(out.Sources, b.MarketplaceID)
		if ai != bi {
			return ai < bi
		}
		return a.Name < b.Name
	})
	return out, nil
}

func indexOfSource(srcs []MergedIndexSource, id uuid.UUID) int {
	for i, s := range srcs {
		if s.ID == id {
			return i
		}
	}
	return len(srcs)
}

// FindVersion finds (plugin, version, marketplace) tuple. An empty
// marketplaceID searches all enabled marketplaces; collisions prefer
// locked first then registration order.
func (mc *MultiMarketplaceClient) FindVersion(ctx context.Context, name, version, marketplaceID string) (*IndexPlugin, *IndexVersion, *Marketplace, error) {
	if mc == nil || mc.Store == nil {
		return nil, nil, nil, errors.New("plugins: multi-marketplace store not configured")
	}
	if marketplaceID != "" {
		id, err := uuid.Parse(marketplaceID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("plugins: invalid marketplace id: %w", err)
		}
		m, err := mc.Store.Get(ctx, id)
		if err != nil {
			return nil, nil, nil, err
		}
		if !m.Enabled {
			return nil, nil, nil, fmt.Errorf("plugins: marketplace %q is disabled", m.Name)
		}
		c := mc.clientFor(m)
		p, v, err := c.FindVersion(ctx, name, version)
		if err != nil {
			return nil, nil, nil, err
		}
		return p, v, &m, nil
	}
	rows, err := mc.Store.ListEnabled(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	var firstErr error
	for _, m := range rows {
		c := mc.clientFor(m)
		p, v, err := c.FindVersion(ctx, name, version)
		if err == nil {
			mm := m
			return p, v, &mm, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, nil, nil, firstErr
	}
	return nil, nil, nil, fmt.Errorf("plugins: %q not found in any enabled marketplace", name)
}

// DownloadAndVerify fetches the tarball + signature from the named
// marketplace's IndexVersion and verifies against THAT marketplace's
// trust key. The resulting tarball is returned only on a successful
// verify.
func (mc *MultiMarketplaceClient) DownloadAndVerify(ctx context.Context, marketplaceID uuid.UUID, v *IndexVersion) ([]byte, error) {
	if mc == nil || mc.Store == nil {
		return nil, errors.New("plugins: multi-marketplace store not configured")
	}
	m, err := mc.Store.Get(ctx, marketplaceID)
	if err != nil {
		return nil, err
	}
	c := mc.clientFor(m)
	tarball, sig, err := c.DownloadArtifacts(ctx, v)
	if err != nil {
		return nil, err
	}
	keyPath, err := mc.writePinnedKey(m)
	if err != nil {
		return nil, fmt.Errorf("plugins: pin trust key: %w", err)
	}
	verifier := &Verifier{PublicKeyPath: keyPath}
	if err := verifier.Verify(ctx, tarball, sig); err != nil {
		return nil, fmt.Errorf("plugins: signature: %w", err)
	}
	return tarball, nil
}

// writePinnedKey writes the marketplace's pinned PEM to a file under
// PluginsRoot/.trust/<id>.pub and returns the absolute path. The file
// is rewritten every time so trust-key refreshes propagate.
func (mc *MultiMarketplaceClient) writePinnedKey(m Marketplace) (string, error) {
	root := mc.PluginsRoot
	if root == "" {
		root = DefaultPluginsRoot
	}
	dir := filepath.Join(root, ".trust")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, m.ID.String()+".pub")
	if err := os.WriteFile(path, []byte(m.TrustKeyPEM), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// MemMarketplacesStore is an in-memory MarketplacesStore for tests.
// It enforces the unique-name constraint and the locked-row
// invariants the production store does.
type MemMarketplacesStore struct {
	mu   sync.Mutex
	rows map[uuid.UUID]Marketplace
}

// NewMemMarketplacesStore constructs an empty in-memory store.
func NewMemMarketplacesStore() *MemMarketplacesStore {
	return &MemMarketplacesStore{rows: map[uuid.UUID]Marketplace{}}
}

func (s *MemMarketplacesStore) List(_ context.Context) ([]Marketplace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Marketplace, 0, len(s.rows))
	for _, m := range s.rows {
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Locked != out[j].Locked {
			return out[i].Locked
		}
		return out[i].AddedAt.Before(out[j].AddedAt)
	})
	return out, nil
}

func (s *MemMarketplacesStore) ListEnabled(ctx context.Context) ([]Marketplace, error) {
	all, _ := s.List(ctx)
	out := make([]Marketplace, 0, len(all))
	for _, m := range all {
		if m.Enabled {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *MemMarketplacesStore) Get(_ context.Context, id uuid.UUID) (Marketplace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.rows[id]
	if !ok {
		return Marketplace{}, fmt.Errorf("plugins: marketplace %s: not found", id)
	}
	return m, nil
}

func (s *MemMarketplacesStore) GetByName(_ context.Context, name string) (Marketplace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.rows {
		if m.Name == name {
			return m, nil
		}
	}
	return Marketplace{}, fmt.Errorf("plugins: marketplace %q: not found", name)
}

func (s *MemMarketplacesStore) Create(_ context.Context, m Marketplace) (Marketplace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.rows {
		if ex.Name == m.Name {
			return Marketplace{}, fmt.Errorf("plugins: marketplace %q already exists", m.Name)
		}
	}
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	now := time.Now().UTC()
	if m.AddedAt.IsZero() {
		m.AddedAt = now
	}
	m.UpdatedAt = now
	s.rows[m.ID] = m
	return m, nil
}

func (s *MemMarketplacesStore) UpdateEnabled(_ context.Context, id uuid.UUID, enabled bool) (Marketplace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.rows[id]
	if !ok {
		return Marketplace{}, fmt.Errorf("plugins: marketplace %s: not found", id)
	}
	m.Enabled = enabled
	m.UpdatedAt = time.Now().UTC()
	s.rows[id] = m
	return m, nil
}

func (s *MemMarketplacesStore) UpdateTrustKey(_ context.Context, id uuid.UUID, pem string) (Marketplace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.rows[id]
	if !ok {
		return Marketplace{}, fmt.Errorf("plugins: marketplace %s: not found", id)
	}
	m.TrustKeyPEM = pem
	m.UpdatedAt = time.Now().UTC()
	s.rows[id] = m
	return m, nil
}

func (s *MemMarketplacesStore) Delete(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rows[id]; !ok {
		return fmt.Errorf("plugins: marketplace %s: not found", id)
	}
	delete(s.rows, id)
	return nil
}

// SeedOfficialIfMissing inserts the locked novanas-official marketplace
// row if it does not already exist. indexURL and trustKeyPath come
// from the existing MARKETPLACE_INDEX_URL / MARKETPLACE_TRUST_KEY_PATH
// env vars (backward-compat). The trust key file is read at boot;
// failures here are returned so the caller can decide whether to log
// or fail.
func SeedOfficialIfMissing(ctx context.Context, store MarketplacesStore, indexURL, trustKeyPath string) (Marketplace, bool, error) {
	if store == nil {
		return Marketplace{}, false, errors.New("plugins: store not configured")
	}
	if existing, err := store.GetByName(ctx, OfficialMarketplaceName); err == nil {
		return existing, false, nil
	}
	if indexURL == "" {
		indexURL = DefaultMarketplaceIndexURL
	}
	pem, err := readTrustKey(trustKeyPath)
	if err != nil {
		return Marketplace{}, false, fmt.Errorf("plugins: read trust key %q: %w", trustKeyPath, err)
	}
	m := Marketplace{
		ID:          uuid.New(),
		Name:        OfficialMarketplaceName,
		IndexURL:    indexURL,
		TrustKeyURL: "",
		TrustKeyPEM: pem,
		Locked:      true,
		Enabled:     true,
		AddedBy:     "system",
		AddedAt:     time.Now().UTC(),
	}
	created, err := store.Create(ctx, m)
	if err != nil {
		return Marketplace{}, false, err
	}
	return created, true, nil
}

func readTrustKey(path string) (string, error) {
	if path == "" {
		return "", errors.New("plugins: trust key path empty")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
