package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultMarketplaceIndexURL is the canonical NovaNAS marketplace index.
// Operators can override per-deployment via MARKETPLACE_INDEX_URL.
const DefaultMarketplaceIndexURL = "https://raw.githubusercontent.com/azrtydxb/NovaNas-packages/main/index.json"

// IndexCacheTTL is how long an in-memory copy of the index is reused
// before a fresh fetch.
const IndexCacheTTL = 15 * time.Minute

// Index is the on-the-wire shape of marketplace index.json.
type Index struct {
	Version int            `json:"version"`
	Updated time.Time      `json:"updated,omitempty"`
	Plugins []IndexPlugin `json:"plugins"`
}

// IndexPlugin is one plugin entry — name + a list of available versions.
type IndexPlugin struct {
	Name            string         `json:"name"`
	DisplayName     string         `json:"displayName,omitempty"`
	Vendor          string         `json:"vendor"`
	Category        string         `json:"category"`
	DisplayCategory string         `json:"displayCategory,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	Description     string         `json:"description,omitempty"`
	Icon            string         `json:"icon,omitempty"`
	Homepage        string         `json:"homepage,omitempty"`
	Versions        []IndexVersion `json:"versions"`
}

// IndexVersion is one release of a plugin.
type IndexVersion struct {
	Version      string `json:"version"`
	TarballURL   string `json:"tarballUrl"`
	SignatureURL string `json:"signatureUrl"`
	SHA256       string `json:"sha256,omitempty"`
	Size         int64  `json:"size,omitempty"`
	ReleasedAt   time.Time `json:"releasedAt,omitempty"`
}

// MarketplaceClient fetches and caches the marketplace index and the
// release artifacts (tarball + signature).
type MarketplaceClient struct {
	IndexURL   string
	HTTP       *http.Client

	mu       sync.RWMutex
	cache    *Index
	cachedAt time.Time
}

// NewMarketplaceClient constructs a client. indexURL "" falls back to the
// public default. http nil falls back to a 30s-timeout client.
func NewMarketplaceClient(indexURL string, h *http.Client) *MarketplaceClient {
	if indexURL == "" {
		indexURL = DefaultMarketplaceIndexURL
	}
	if h == nil {
		h = &http.Client{Timeout: 30 * time.Second}
	}
	return &MarketplaceClient{IndexURL: indexURL, HTTP: h}
}

// NewMarketplaceClientFor constructs a single-source MarketplaceClient
// for a registered Marketplace row. Used internally by
// MultiMarketplaceClient to wrap each registered source.
func NewMarketplaceClientFor(m Marketplace, h *http.Client) *MarketplaceClient {
	return NewMarketplaceClient(m.IndexURL, h)
}

// FetchIndex returns the cached index when fresh, otherwise refreshes.
// force=true bypasses the TTL.
func (c *MarketplaceClient) FetchIndex(ctx context.Context, force bool) (*Index, error) {
	if !force {
		c.mu.RLock()
		if c.cache != nil && time.Since(c.cachedAt) < IndexCacheTTL {
			defer c.mu.RUnlock()
			return c.cache, nil
		}
		c.mu.RUnlock()
	}
	idx, err := c.fetch(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cache = idx
	c.cachedAt = time.Now()
	c.mu.Unlock()
	return idx, nil
}

// FindVersion returns the IndexPlugin + IndexVersion for (name, version).
// version "" returns the highest-listed version.
func (c *MarketplaceClient) FindVersion(ctx context.Context, name, version string) (*IndexPlugin, *IndexVersion, error) {
	idx, err := c.FetchIndex(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	for i := range idx.Plugins {
		p := &idx.Plugins[i]
		if p.Name != name {
			continue
		}
		if len(p.Versions) == 0 {
			return nil, nil, fmt.Errorf("plugins: %q has no versions in index", name)
		}
		if version == "" {
			return p, &p.Versions[0], nil
		}
		for j := range p.Versions {
			if p.Versions[j].Version == version {
				return p, &p.Versions[j], nil
			}
		}
		return nil, nil, fmt.Errorf("plugins: %q has no version %q", name, version)
	}
	return nil, nil, fmt.Errorf("plugins: %q not in marketplace index", name)
}

// DownloadArtifacts fetches the tarball and signature bytes for v.
func (c *MarketplaceClient) DownloadArtifacts(ctx context.Context, v *IndexVersion) (tarball, signature []byte, err error) {
	tarball, err = c.getBytes(ctx, v.TarballURL)
	if err != nil {
		return nil, nil, fmt.Errorf("plugins: tarball: %w", err)
	}
	signature, err = c.getBytes(ctx, v.SignatureURL)
	if err != nil {
		return nil, nil, fmt.Errorf("plugins: signature: %w", err)
	}
	return tarball, signature, nil
}

func (c *MarketplaceClient) fetch(ctx context.Context) (*Index, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.IndexURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugins: marketplace HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("plugins: index decode: %w", err)
	}
	if idx.Version == 0 {
		return nil, errors.New("plugins: marketplace index has no version field")
	}
	if idx.Plugins == nil {
		idx.Plugins = []IndexPlugin{}
	}
	return &idx, nil
}

func (c *MarketplaceClient) getBytes(ctx context.Context, url string) ([]byte, error) {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return nil, fmt.Errorf("plugins: invalid url %q", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugins: HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20)) // 256 MiB cap
}
