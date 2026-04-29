package novanas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// KDCStatus mirrors the Krb5KDCStatus schema in api/openapi.yaml.
type KDCStatus struct {
	Running        bool   `json:"running"`
	Realm          string `json:"realm"`
	DatabaseExists bool   `json:"databaseExists"`
	StashSealed    bool   `json:"stashSealed"`
	PrincipalCount int    `json:"principalCount"`
}

// Principal mirrors the Krb5Principal schema.
type Principal struct {
	Name       string `json:"name"`
	KVNO       int    `json:"kvno,omitempty"`
	Expiration string `json:"expiration,omitempty"`
	Attributes string `json:"attributes,omitempty"`
}

// CreatePrincipalSpec is the request body for POST /krb5/principals.
// Randkey and Password are mutually exclusive; if both are zero, the
// server defaults to randkey (typical service-principal layout).
type CreatePrincipalSpec struct {
	Name     string `json:"name"`
	Randkey  bool   `json:"randkey,omitempty"`
	Password string `json:"password,omitempty"`
}

// GetKDCStatus reads the embedded MIT KDC's runtime status.
func (c *Client) GetKDCStatus(ctx context.Context) (*KDCStatus, error) {
	var out KDCStatus
	if _, err := c.do(ctx, http.MethodGet, "/krb5/kdc/status", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPrincipals returns the names of all principals known to the KDC.
func (c *Client) ListPrincipals(ctx context.Context) ([]string, error) {
	var out []string
	if _, err := c.do(ctx, http.MethodGet, "/krb5/principals", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPrincipal fetches one principal's projection.
func (c *Client) GetPrincipal(ctx context.Context, name string) (*Principal, error) {
	if name == "" {
		return nil, errors.New("novanas: principal name is required")
	}
	var out Principal
	if _, err := c.do(ctx, http.MethodGet, "/krb5/principals/"+url.PathEscape(name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreatePrincipal creates a new principal and returns its projection.
// The server returns 201 Created with the body — synchronous, not async.
func (c *Client) CreatePrincipal(ctx context.Context, spec CreatePrincipalSpec) (*Principal, error) {
	if spec.Name == "" {
		return nil, errors.New("novanas: CreatePrincipalSpec.Name is required")
	}
	if spec.Randkey && spec.Password != "" {
		return nil, errors.New("novanas: randkey and password are mutually exclusive")
	}
	var out Principal
	if _, err := c.do(ctx, http.MethodPost, "/krb5/principals", nil, spec, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePrincipal removes a principal. Returns nil on 204.
func (c *Client) DeletePrincipal(ctx context.Context, name string) error {
	if name == "" {
		return errors.New("novanas: principal name is required")
	}
	_, err := c.do(ctx, http.MethodDelete, "/krb5/principals/"+url.PathEscape(name), nil, nil, nil)
	return err
}

// GetPrincipalKeytab fetches a freshly-rotated keytab for the principal
// as raw bytes (application/octet-stream). The KVNO is incremented as a
// side effect — distribute the returned keytab atomically.
//
// The bytes are an MIT-format keytab; the first byte is 0x05.
func (c *Client) GetPrincipalKeytab(ctx context.Context, name string) ([]byte, error) {
	if name == "" {
		return nil, errors.New("novanas: principal name is required")
	}
	// We bypass do() because the response is binary, not JSON.
	u := c.BaseURL + apiPrefix + "/krb5/principals/" + url.PathEscape(name) + "/keytab"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if ua := c.UserAgent; ua != "" {
		req.Header.Set("User-Agent", ua)
	} else {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}
	if tok := c.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var env struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &env)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Code:       env.Error,
			Message:    env.Message,
		}
	}
	if len(body) == 0 || body[0] != 0x05 {
		return nil, fmt.Errorf("novanas: keytab response is not a valid MIT keytab")
	}
	// Return a copy decoupled from the response buffer.
	out := bytes.Clone(body)
	return out, nil
}
