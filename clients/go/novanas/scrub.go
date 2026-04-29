package novanas

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// ScrubPolicy mirrors the ScrubPolicy schema in api/openapi.yaml.
type ScrubPolicy struct {
	ID          string    `json:"id,omitempty"`
	Name        string    `json:"name"`
	Pools       string    `json:"pools"`
	Cron        string    `json:"cron"`
	Priority    string    `json:"priority,omitempty"`
	Enabled     bool      `json:"enabled"`
	Builtin     bool      `json:"builtin,omitempty"`
	LastFiredAt time.Time `json:"lastFiredAt,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
}

// ListScrubPolicies returns every scrub policy on the server (GET
// /scrub-policies). Empty result is a normal "no policies configured"
// state — operators that bypassed nova-api install may not have the
// default policy.
func (c *Client) ListScrubPolicies(ctx context.Context) ([]ScrubPolicy, error) {
	var out []ScrubPolicy
	if _, err := c.do(ctx, http.MethodGet, "/scrub-policies", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetScrubPolicy returns a single policy by id.
func (c *Client) GetScrubPolicy(ctx context.Context, id string) (*ScrubPolicy, error) {
	if id == "" {
		return nil, errors.New("novanas: scrub policy id is required")
	}
	var out ScrubPolicy
	if _, err := c.do(ctx, http.MethodGet, "/scrub-policies/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateScrubPolicy creates a new policy and returns the server's view.
// The server fills in id, builtin=false, and timestamps.
func (c *Client) CreateScrubPolicy(ctx context.Context, p ScrubPolicy) (*ScrubPolicy, error) {
	if p.Name == "" || p.Cron == "" {
		return nil, errors.New("novanas: ScrubPolicy requires Name and Cron")
	}
	var out ScrubPolicy
	if _, err := c.do(ctx, http.MethodPost, "/scrub-policies", nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateScrubPolicy replaces the mutable fields (pools, cron, priority,
// enabled) on the policy with the given id.
func (c *Client) UpdateScrubPolicy(ctx context.Context, id string, p ScrubPolicy) (*ScrubPolicy, error) {
	if id == "" {
		return nil, errors.New("novanas: scrub policy id is required")
	}
	var out ScrubPolicy
	if _, err := c.do(ctx, http.MethodPatch, "/scrub-policies/"+url.PathEscape(id), nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteScrubPolicy removes the policy with the given id. Deleting the
// builtin default is allowed but not recommended; the server will not
// re-install it automatically (only first-boot bootstrap does).
func (c *Client) DeleteScrubPolicy(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("novanas: scrub policy id is required")
	}
	_, err := c.do(ctx, http.MethodDelete, "/scrub-policies/"+url.PathEscape(id), nil, nil, nil)
	return err
}

// ScrubPool dispatches an ad-hoc scrub on the named pool. action is
// "start" (default) or "stop". The returned Job stub contains the new
// job ID; use WaitJob to block until the scrub-start command itself
// completes (note: that's the zpool invocation, NOT the scrub — scrubs
// run async in the kernel and can take hours).
func (c *Client) ScrubPool(ctx context.Context, name, action string) (*Job, error) {
	if name == "" {
		return nil, errors.New("novanas: pool name is required")
	}
	q := url.Values{}
	if action != "" {
		q.Set("action", action)
	}
	resp, err := c.do(ctx, http.MethodPost, "/pools/"+url.PathEscape(name)+"/scrub", q, nil, nil)
	if err != nil {
		return nil, err
	}
	return finishJobFromAccepted(resp)
}
