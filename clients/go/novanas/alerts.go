package novanas

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

// Alert mirrors the Alertmanager v2 alert object surfaced by
// /api/v1/alerts. The shape follows AM upstream — labels, annotations,
// status, and a stable fingerprint.
type Alert struct {
	Fingerprint  string            `json:"fingerprint"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	StartsAt     string            `json:"startsAt,omitempty"`
	EndsAt       string            `json:"endsAt,omitempty"`
	UpdatedAt    string            `json:"updatedAt,omitempty"`
	GeneratorURL string            `json:"generatorURL,omitempty"`
	Status       *AlertStatus      `json:"status,omitempty"`
}

// AlertStatus is the inner Alertmanager status object.
type AlertStatus struct {
	State       string   `json:"state,omitempty"`
	SilencedBy  []string `json:"silencedBy,omitempty"`
	InhibitedBy []string `json:"inhibitedBy,omitempty"`
}

// SilenceMatcher is a label-matcher used to scope a Silence.
type SilenceMatcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual *bool  `json:"isEqual,omitempty"`
}

// Silence mirrors the Alertmanager silence schema.
type Silence struct {
	ID        string           `json:"id,omitempty"`
	Matchers  []SilenceMatcher `json:"matchers"`
	StartsAt  string           `json:"startsAt"`
	EndsAt    string           `json:"endsAt"`
	CreatedBy string           `json:"createdBy"`
	Comment   string           `json:"comment"`
	Status    *SilenceStatus   `json:"status,omitempty"`
}

// SilenceStatus is the inner status (active/expired/pending).
type SilenceStatus struct {
	State string `json:"state"`
}

// AlertReceiver is one entry in /alert-receivers (read-only).
type AlertReceiver struct {
	Name string `json:"name"`
}

// ListAlerts fetches all firing/active/resolved alerts from
// Alertmanager. Optional filters are forwarded as query parameters.
func (c *Client) ListAlerts(ctx context.Context, filters url.Values) ([]Alert, error) {
	var out []Alert
	if _, err := c.do(ctx, http.MethodGet, "/alerts", filters, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetAlert fetches a single alert by fingerprint. Returns *APIError with
// StatusCode=404 when the fingerprint is unknown.
func (c *Client) GetAlert(ctx context.Context, fingerprint string) (*Alert, error) {
	if fingerprint == "" {
		return nil, errors.New("novanas: fingerprint is required")
	}
	var out Alert
	if _, err := c.do(ctx, http.MethodGet, "/alerts/"+url.PathEscape(fingerprint), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListSilences returns all silences known to Alertmanager.
func (c *Client) ListSilences(ctx context.Context) ([]Silence, error) {
	var out []Silence
	if _, err := c.do(ctx, http.MethodGet, "/alert-silences", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSilence creates a new silence; AM returns the assigned ID in
// the response body. The returned Silence preserves whatever shape AM
// emits (some deployments echo the full object, some only the ID).
func (c *Client) CreateSilence(ctx context.Context, s Silence) (json.RawMessage, error) {
	var out json.RawMessage
	if _, err := c.do(ctx, http.MethodPost, "/alert-silences", nil, s, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteSilence expires the silence with the given ID.
func (c *Client) DeleteSilence(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("novanas: id is required")
	}
	_, err := c.do(ctx, http.MethodDelete, "/alert-silences/"+url.PathEscape(id), nil, nil, nil)
	return err
}

// ListAlertReceivers returns the list of configured Alertmanager
// receivers. Read-only — receiver editing happens out-of-band by editing
// AM's config file.
func (c *Client) ListAlertReceivers(ctx context.Context) ([]AlertReceiver, error) {
	var out []AlertReceiver
	if _, err := c.do(ctx, http.MethodGet, "/alert-receivers", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
