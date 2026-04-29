package novanas

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// Marketplace mirrors the server-side MarketplaceResponse.
type Marketplace struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	IndexURL    string    `json:"indexUrl"`
	TrustKeyURL string    `json:"trustKeyUrl"`
	TrustKeyPEM string    `json:"trustKeyPem"`
	Locked      bool      `json:"locked"`
	Enabled     bool      `json:"enabled"`
	AddedBy     string    `json:"addedBy,omitempty"`
	AddedAt     time.Time `json:"addedAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

// MarketplaceCreateRequest is the body of POST /marketplaces.
type MarketplaceCreateRequest struct {
	Name        string `json:"name"`
	IndexURL    string `json:"indexUrl"`
	TrustKeyURL string `json:"trustKeyUrl"`
}

// MarketplacePatchRequest is the body of PATCH /marketplaces/{id}.
type MarketplacePatchRequest struct {
	Enabled         *bool `json:"enabled,omitempty"`
	RefreshTrustKey bool  `json:"refreshTrustKey,omitempty"`
}

// ListMarketplaces returns every registered marketplace, locked first.
func (c *Client) ListMarketplaces(ctx context.Context) ([]Marketplace, error) {
	var out []Marketplace
	if _, err := c.do(ctx, http.MethodGet, "/marketplaces", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetMarketplace returns one registered marketplace.
func (c *Client) GetMarketplace(ctx context.Context, id string) (*Marketplace, error) {
	if id == "" {
		return nil, errors.New("novanas: id is required")
	}
	var out Marketplace
	if _, err := c.do(ctx, http.MethodGet, "/marketplaces/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AddMarketplace registers a new marketplace. The server validates
// the index URL is reachable (HTTP HEAD) and the trust-key URL
// returns a parseable cosign public key BEFORE persisting.
func (c *Client) AddMarketplace(ctx context.Context, req MarketplaceCreateRequest) (*Marketplace, error) {
	if req.Name == "" || req.IndexURL == "" || req.TrustKeyURL == "" {
		return nil, errors.New("novanas: name, indexUrl, and trustKeyUrl are required")
	}
	var out Marketplace
	if _, err := c.do(ctx, http.MethodPost, "/marketplaces", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateMarketplace flips enabled and/or triggers a trust-key
// refresh. The locked novanas-official entry refuses
// enabled=false (409).
func (c *Client) UpdateMarketplace(ctx context.Context, id string, req MarketplacePatchRequest) (*Marketplace, error) {
	if id == "" {
		return nil, errors.New("novanas: id is required")
	}
	var out Marketplace
	if _, err := c.do(ctx, http.MethodPatch, "/marketplaces/"+url.PathEscape(id), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteMarketplace removes a marketplace. The locked
// novanas-official entry returns 409.
func (c *Client) DeleteMarketplace(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("novanas: id is required")
	}
	_, err := c.do(ctx, http.MethodDelete, "/marketplaces/"+url.PathEscape(id), nil, nil, nil)
	return err
}

// RefreshMarketplaceTrustKey re-fetches the .pub from the
// marketplace's pinned trust_key_url and updates the row.
func (c *Client) RefreshMarketplaceTrustKey(ctx context.Context, id string) (*Marketplace, error) {
	if id == "" {
		return nil, errors.New("novanas: id is required")
	}
	var out Marketplace
	if _, err := c.do(ctx, http.MethodPost, "/marketplaces/"+url.PathEscape(id)+"/refresh-trust-key", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
