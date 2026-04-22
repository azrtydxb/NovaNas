package openbao

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config configures the HTTP Transit client.
type Config struct {
	// Addr is the OpenBao base URL, e.g. "https://openbao.openbao.svc:8200".
	Addr string
	// Token is the static token to authenticate with. Prefer TokenPath
	// in production (Kubernetes service-account file) so the token can
	// be rotated out-of-process.
	Token string
	// TokenPath is a file to read the token from on each request. Re-read
	// on every call so rotated tokens take effect without restart.
	TokenPath string
	// Namespace is the OpenBao enterprise namespace header
	// ("X-Vault-Namespace"). Usually empty for OSS OpenBao.
	Namespace string
	// MountPath is the Transit engine mount path; defaults to "transit".
	MountPath string
	// InsecureSkipVerify disables TLS verification — DO NOT set true
	// outside dev/test.
	InsecureSkipVerify bool
	// Timeout for a single request.
	Timeout time.Duration
}

// HTTPClient is a minimal OpenBao Transit client. Safe for concurrent use.
type HTTPClient struct {
	cfg    Config
	client *http.Client
}

// NewHTTPClient constructs an HTTPClient. Validates that at least one of
// Token / TokenPath is set and that Addr looks sane.
func NewHTTPClient(cfg Config) (*HTTPClient, error) {
	if cfg.Addr == "" {
		return nil, errors.New("openbao: Addr is required")
	}
	if cfg.Token == "" && cfg.TokenPath == "" {
		return nil, errors.New("openbao: either Token or TokenPath is required")
	}
	if cfg.MountPath == "" {
		cfg.MountPath = "transit"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // operator-controlled
	}
	return &HTTPClient{
		cfg: cfg,
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

func (c *HTTPClient) token() (string, error) {
	if c.cfg.TokenPath != "" {
		data, err := os.ReadFile(c.cfg.TokenPath)
		if err != nil {
			return "", fmt.Errorf("openbao: read token file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return c.cfg.Token, nil
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(buf)
	}
	url := strings.TrimRight(c.cfg.Addr, "/") + "/v1/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	token, err := c.token()
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", token)
	if c.cfg.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.cfg.Namespace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openbao: http: %w", err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		// 204 no content is success for some endpoints; else treat as error.
		if resp.StatusCode == http.StatusNoContent {
			return nil, nil
		}
		return nil, fmt.Errorf("openbao: %s %s: %d: %s", method, path, resp.StatusCode, string(buf))
	}
	if len(buf) == 0 {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("openbao: decode: %w", err)
	}
	return out, nil
}

// WrapDK calls Transit's /encrypt endpoint.
//
// Transit returns a ciphertext string of the form "vault:v<N>:<base64>".
// We return the full string as the opaque wrapped blob — Transit
// consumes it verbatim on Decrypt. We also parse out the version for
// the caller's metadata record.
func (c *HTTPClient) WrapDK(ctx context.Context, masterKeyName string, rawDK []byte) ([]byte, uint64, error) {
	path := fmt.Sprintf("%s/encrypt/%s", c.cfg.MountPath, masterKeyName)
	req := map[string]any{
		"plaintext": base64.StdEncoding.EncodeToString(rawDK),
	}
	out, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, 0, err
	}
	data, _ := out["data"].(map[string]any)
	ct, _ := data["ciphertext"].(string)
	if ct == "" {
		return nil, 0, errors.New("openbao: encrypt returned no ciphertext")
	}
	version, err := parseVersion(ct)
	if err != nil {
		return nil, 0, err
	}
	return []byte(ct), version, nil
}

// UnwrapDK calls Transit's /decrypt endpoint.
func (c *HTTPClient) UnwrapDK(ctx context.Context, masterKeyName string, wrapped []byte) ([]byte, error) {
	path := fmt.Sprintf("%s/decrypt/%s", c.cfg.MountPath, masterKeyName)
	req := map[string]any{
		"ciphertext": string(wrapped),
	}
	out, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	data, _ := out["data"].(map[string]any)
	b64, _ := data["plaintext"].(string)
	if b64 == "" {
		return nil, errors.New("openbao: decrypt returned no plaintext")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("openbao: decode plaintext: %w", err)
	}
	return raw, nil
}

// RotateMasterKey bumps the master key version via Transit's
// /keys/<name>/rotate endpoint.
func (c *HTTPClient) RotateMasterKey(ctx context.Context, masterKeyName string) error {
	path := fmt.Sprintf("%s/keys/%s/rotate", c.cfg.MountPath, masterKeyName)
	_, err := c.do(ctx, http.MethodPost, path, map[string]any{})
	return err
}

// ReadConfig reads the named master key config.
func (c *HTTPClient) ReadConfig(ctx context.Context, masterKeyName string) (TransitKeyConfig, error) {
	path := fmt.Sprintf("%s/keys/%s", c.cfg.MountPath, masterKeyName)
	out, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return TransitKeyConfig{}, err
	}
	data, _ := out["data"].(map[string]any)
	latest, _ := data["latest_version"].(float64)
	minv, _ := data["min_decryption_version"].(float64)
	typ, _ := data["type"].(string)
	exportable, _ := data["exportable"].(bool)
	return TransitKeyConfig{
		Name:          masterKeyName,
		Type:          typ,
		LatestVersion: uint64(latest),
		MinVersion:    uint64(minv),
		Exportable:    exportable,
	}, nil
}

// DeleteKey destroys a Transit key via DELETE /v1/transit/keys/<name>.
// OpenBao requires the key's deletion_allowed=true config flag to be set
// beforehand (see update-key-config endpoint). Returns an error when the
// backend refuses the request so callers can retry or surface the
// compliance failure.
func (c *HTTPClient) DeleteKey(ctx context.Context, masterKeyName string) error {
	path := fmt.Sprintf("%s/keys/%s", c.cfg.MountPath, masterKeyName)
	_, err := c.do(ctx, http.MethodDelete, path, nil)
	return err
}

// parseVersion extracts N from "vault:vN:..." Transit ciphertext.
func parseVersion(ct string) (uint64, error) {
	parts := strings.SplitN(ct, ":", 3)
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "v") {
		return 0, fmt.Errorf("openbao: unrecognised ciphertext format: %q", ct)
	}
	var v uint64
	if _, err := fmt.Sscanf(parts[1], "v%d", &v); err != nil {
		return 0, fmt.Errorf("openbao: parse version: %w", err)
	}
	return v, nil
}

// Compile-time check.
var _ TransitClient = (*HTTPClient)(nil)
