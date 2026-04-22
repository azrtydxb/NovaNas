// Package reconciler — OpenBao PKI-backed CertificateIssuer.
//
// Uses the OpenBao HTTP REST API directly (same pattern as
// storage/internal/openbao/http_client.go) so this module does not need
// a dependency on a storage internal package. The PKI endpoints
// follow the Vault/OpenBao standard shape:
//
//	POST /v1/<mount>/issue/<role>   -> issue a new certificate
//	POST /v1/<mount>/revoke         -> revoke a certificate by serial
//
// Configurable via main.go env wiring.
package reconciler

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenBaoPKIConfig configures an OpenBaoPKIIssuer.
type OpenBaoPKIConfig struct {
	// Addr is the OpenBao base URL (e.g. "https://openbao:8200").
	Addr string
	// Token is a static token; if empty TokenPath is read on each call.
	Token string
	// TokenPath is the file holding the service-account token.
	TokenPath string
	// Namespace is the enterprise X-Vault-Namespace header.
	Namespace string
	// MountPath is the PKI mount (default "pki").
	MountPath string
	// Role is the PKI role to issue against (e.g. "novanas-internal").
	Role string
	// InsecureSkipVerify — do not enable outside dev.
	InsecureSkipVerify bool
	// Timeout per request.
	Timeout time.Duration
}

// OpenBaoPKIIssuer is the production CertificateIssuer. Safe for
// concurrent use.
type OpenBaoPKIIssuer struct {
	cfg    OpenBaoPKIConfig
	client *http.Client
}

// NewOpenBaoPKIIssuer validates config and returns an issuer.
func NewOpenBaoPKIIssuer(cfg OpenBaoPKIConfig) (*OpenBaoPKIIssuer, error) {
	if cfg.Addr == "" {
		return nil, errors.New("openbao pki: Addr is required")
	}
	if cfg.Token == "" && cfg.TokenPath == "" {
		return nil, errors.New("openbao pki: Token or TokenPath is required")
	}
	if cfg.MountPath == "" {
		cfg.MountPath = "pki"
	}
	if cfg.Role == "" {
		cfg.Role = "novanas"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // operator-controlled
	}
	return &OpenBaoPKIIssuer{
		cfg: cfg,
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

// Issue calls POST /v1/<mount>/issue/<role>.
func (i *OpenBaoPKIIssuer) Issue(ctx context.Context, req CertificateRequest) (CertificateBundle, error) {
	ttl := req.Duration
	if ttl == 0 {
		ttl = 365 * 24 * time.Hour
	}
	body := map[string]any{
		"common_name": req.CommonName,
		"ttl":         fmt.Sprintf("%ds", int64(ttl.Seconds())),
	}
	if len(req.DNSNames) > 0 {
		body["alt_names"] = strings.Join(req.DNSNames, ",")
	}
	if len(req.IPSANs) > 0 {
		body["ip_sans"] = strings.Join(req.IPSANs, ",")
	}
	path := fmt.Sprintf("%s/issue/%s", i.cfg.MountPath, i.cfg.Role)
	out, err := i.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return CertificateBundle{}, fmt.Errorf("openbao pki: issue: %w", err)
	}
	data, _ := out["data"].(map[string]any)
	cert, _ := data["certificate"].(string)
	key, _ := data["private_key"].(string)
	ca, _ := data["issuing_ca"].(string)
	serial, _ := data["serial_number"].(string)
	if cert == "" || key == "" {
		return CertificateBundle{}, errors.New("openbao pki: issue returned empty cert or key")
	}
	now := time.Now()
	return CertificateBundle{
		CertPEM:   []byte(cert),
		KeyPEM:    []byte(key),
		CAPEM:     []byte(ca),
		NotBefore: now,
		NotAfter:  now.Add(ttl),
		Serial:    serial,
	}, nil
}

// Revoke calls POST /v1/<mount>/revoke.
func (i *OpenBaoPKIIssuer) Revoke(ctx context.Context, serial string) error {
	if serial == "" {
		return errors.New("openbao pki: empty serial")
	}
	path := fmt.Sprintf("%s/revoke", i.cfg.MountPath)
	_, err := i.do(ctx, http.MethodPost, path, map[string]any{"serial_number": serial})
	if err != nil {
		return fmt.Errorf("openbao pki: revoke: %w", err)
	}
	return nil
}

func (i *OpenBaoPKIIssuer) token() (string, error) {
	if i.cfg.TokenPath != "" {
		data, err := os.ReadFile(i.cfg.TokenPath)
		if err != nil {
			return "", fmt.Errorf("read token file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return i.cfg.Token, nil
}

func (i *OpenBaoPKIIssuer) do(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(buf)
	}
	url := strings.TrimRight(i.cfg.Addr, "/") + "/v1/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	tok, err := i.token()
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", tok)
	if i.cfg.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", i.cfg.Namespace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		if resp.StatusCode == http.StatusNoContent {
			return nil, nil
		}
		return nil, fmt.Errorf("%s %s: %d: %s", method, path, resp.StatusCode, string(buf))
	}
	if len(buf) == 0 {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}

var _ CertificateIssuer = (*OpenBaoPKIIssuer)(nil)
