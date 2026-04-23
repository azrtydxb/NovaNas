// Package hostagent is a thin HTTP client used by the network controllers to
// delegate host-local operations (nmstate apply, nftables rule installation,
// WireGuard/IPsec configuration) to the on-host novanas-agent process.
//
// Rather than extending the storage/agent gRPC proto (which is owned by a
// separate team and has write-contention risk), this client speaks plain
// HTTP/JSON to the agent's dedicated /v1/network/* endpoints. The agent
// returns 200 with a sha256 revision on success; the caller records it in
// the owning CR's Status.AppliedConfigHash to detect drift on subsequent
// reconciles.
package hostagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a minimal HTTP client against the novanas-agent network API.
//
// Zero-value Client uses http.DefaultClient and the empty base URL, which
// makes every call fail with ErrNotConfigured — this mirrors the Noop
// pattern used by the other operator adapters so reconcilers can always
// dereference a non-nil client.
type Client struct {
	// BaseURL is the agent's root, e.g. "http://127.0.0.1:7055".
	BaseURL string
	// HTTP is the transport. Defaults to a 10s-timeout client when nil.
	HTTP *http.Client
}

// ErrNotConfigured is returned by every call on a zero-value Client.
// Reconcilers treat this as a soft failure and reconcile status-only.
var ErrNotConfigured = fmt.Errorf("hostagent: client not configured")

// New constructs a Client with sensible defaults.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Revision hashes a payload into a deterministic revision string. Used both
// by the client and by noop callers that need to populate status.
func Revision(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func (c *Client) do(ctx context.Context, path string, body interface{}) ([]byte, error) {
	if c == nil || c.BaseURL == "" {
		return nil, ErrNotConfigured
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("hostagent: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("hostagent: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	resp, err := h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hostagent: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("hostagent: read: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hostagent: http %d: %s", resp.StatusCode, string(out))
	}
	return out, nil
}

// ApplyNmstate sends a rendered nmstate YAML document to the agent's
// /v1/network/nmstate endpoint. Returns the sha256 revision recorded by
// the agent (or Revision(yaml) as a local fallback when the agent is
// unavailable).
func (c *Client) ApplyNmstate(ctx context.Context, yaml []byte) (string, error) {
	if c == nil || c.BaseURL == "" {
		return Revision(yaml), ErrNotConfigured
	}
	_, err := c.do(ctx, "/v1/network/nmstate", map[string]string{"yaml": string(yaml)})
	if err != nil {
		return "", err
	}
	return Revision(yaml), nil
}

// InstallFirewallRules pushes a rendered nftables ruleset (or equivalent
// eBPF description) to the agent. The agent returns once the kernel has
// committed the change.
func (c *Client) InstallFirewallRules(ctx context.Context, ruleset []byte) (string, error) {
	if c == nil || c.BaseURL == "" {
		return Revision(ruleset), ErrNotConfigured
	}
	_, err := c.do(ctx, "/v1/network/nftables", map[string]string{"ruleset": string(ruleset)})
	if err != nil {
		return "", err
	}
	return Revision(ruleset), nil
}

// InstallTrafficLimits pushes a rendered tc / eBPF limiter config.
func (c *Client) InstallTrafficLimits(ctx context.Context, limits []byte) (string, error) {
	if c == nil || c.BaseURL == "" {
		return Revision(limits), ErrNotConfigured
	}
	_, err := c.do(ctx, "/v1/network/tc", map[string]string{"config": string(limits)})
	if err != nil {
		return "", err
	}
	return Revision(limits), nil
}

// ConfigureTunnel pushes a WireGuard / IPsec / tailscale config to the
// agent, which translates it into a systemd unit drop-in.
func (c *Client) ConfigureTunnel(ctx context.Context, kind string, config []byte) (string, error) {
	if c == nil || c.BaseURL == "" {
		return Revision(config), ErrNotConfigured
	}
	_, err := c.do(ctx, "/v1/network/tunnels/"+kind, map[string]string{"config": string(config)})
	if err != nil {
		return "", err
	}
	return Revision(config), nil
}
