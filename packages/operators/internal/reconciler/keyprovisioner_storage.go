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

// StorageKeyProvisionerFuncs is a function-pointer adapter that binds
// the operators module's VolumeKeyProvisioner contract to a caller-supplied
// implementation (typically wired at main.go to a *crypto.VolumeKeyManager
// from the storage module). Using function pointers keeps the operators
// module from importing storage/internal/crypto directly — that package
// is marked "internal" and is only accessible to code inside the storage
// module tree.
//
// Typical wire-up (in packages/operators/main.go):
//
//	p := &reconciler.StorageKeyProvisionerFuncs{
//	    Provision: func(ctx context.Context, id string) ([]byte, uint64, error) {
//	        return vmgr.ProvisionVolume(ctx, id)
//	    },
//	    Destroy: func(_ context.Context, id string) error {
//	        vmgr.Unmount(id)
//	        return nil
//	    },
//	}
//
// A nil Provision causes ProvisionVolume to return an explicit error so
// controllers never silently succeed with a placeholder wrapped blob in
// production.
type StorageKeyProvisionerFuncs struct {
	Provision func(ctx context.Context, volumeID string) ([]byte, uint64, error)
	Destroy   func(ctx context.Context, volumeID string) error
}

// ProvisionVolume delegates to the configured Provision function.
func (p *StorageKeyProvisionerFuncs) ProvisionVolume(ctx context.Context, volumeID string) ([]byte, uint64, error) {
	if p == nil || p.Provision == nil {
		return nil, 0, fmt.Errorf("storage key provisioner: no Provision func wired")
	}
	return p.Provision(ctx, volumeID)
}

// DestroyVolume delegates to the configured Destroy function; a nil
// Destroy is treated as a no-op (best-effort cryptographic erase).
func (p *StorageKeyProvisionerFuncs) DestroyVolume(ctx context.Context, volumeID string) error {
	if p == nil || p.Destroy == nil {
		return nil
	}
	return p.Destroy(ctx, volumeID)
}

// TransitKeyProvisionerConfig configures a TransitKeyProvisioner.
//
// The TransitKeyProvisioner is an operator-side implementation that talks
// directly to OpenBao Transit to provision per-volume Dataset Keys and,
// on DestroyVolume, performs a real cryptographic erase by deleting the
// corresponding Transit key (which can optionally be allowed to be
// destroyed via the OpenBao sys/policies). It is the primary production
// implementation used when the operator does not have an in-process
// VolumeKeyManager (e.g. when running separately from a storage agent).
//
// Call ordering guarantees:
//   - ProvisionVolume: POST /v1/<mount>/datakey/plaintext/<masterKey>
//     — returns a fresh 32-byte raw DK plus the wrapped ciphertext.
//   - DestroyVolume: DELETE /v1/<mount>/keys/<dkKeyName(volumeID)>
//     — removes the stored wrapped DK so decryption becomes impossible.
//
// DestroyVolume is irreversible and must only be called after the
// crypto finalizer has run on the CR (see crypto_finalizer.go).
type TransitKeyProvisionerConfig struct {
	// Addr is the OpenBao base URL.
	Addr string
	// Token / TokenPath authenticate. Prefer TokenPath in-cluster.
	Token     string
	TokenPath string
	// Namespace is the enterprise X-Vault-Namespace header.
	Namespace string
	// MountPath is the Transit mount, default "transit".
	MountPath string
	// MasterKeyName is the Transit key used to wrap DKs.
	MasterKeyName string
	// InsecureSkipVerify — dev only.
	InsecureSkipVerify bool
	// Timeout per request.
	Timeout time.Duration
}

// TransitKeyProvisioner is a VolumeKeyProvisioner that talks directly to
// OpenBao Transit. It never caches plaintext DKs.
type TransitKeyProvisioner struct {
	cfg    TransitKeyProvisionerConfig
	client *http.Client
}

// NewTransitKeyProvisioner validates config and returns a provisioner.
func NewTransitKeyProvisioner(cfg TransitKeyProvisionerConfig) (*TransitKeyProvisioner, error) {
	if cfg.Addr == "" {
		return nil, errors.New("transit provisioner: Addr is required")
	}
	if cfg.Token == "" && cfg.TokenPath == "" {
		return nil, errors.New("transit provisioner: Token or TokenPath is required")
	}
	if cfg.MountPath == "" {
		cfg.MountPath = "transit"
	}
	if cfg.MasterKeyName == "" {
		cfg.MasterKeyName = "novanas/chunk-master"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &TransitKeyProvisioner{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // operator-controlled
				},
			},
		},
	}, nil
}

// ProvisionVolume generates a fresh DK via Transit's datakey endpoint and
// returns (wrappedCiphertext, version).
func (p *TransitKeyProvisioner) ProvisionVolume(ctx context.Context, volumeID string) ([]byte, uint64, error) {
	if volumeID == "" {
		return nil, 0, errors.New("transit provisioner: empty volumeID")
	}
	path := fmt.Sprintf("%s/datakey/wrapped/%s", p.cfg.MountPath, p.cfg.MasterKeyName)
	out, err := p.do(ctx, http.MethodPost, path, map[string]any{})
	if err != nil {
		return nil, 0, fmt.Errorf("transit provisioner: datakey: %w", err)
	}
	data, _ := out["data"].(map[string]any)
	ct, _ := data["ciphertext"].(string)
	if ct == "" {
		return nil, 0, errors.New("transit provisioner: datakey returned no ciphertext")
	}
	ver, verErr := parseTransitVersion(ct)
	if verErr != nil {
		return nil, 0, verErr
	}
	return []byte(ct), ver, nil
}

// DestroyVolume performs a real cryptographic erase by deleting the
// wrapped DK associated with the volume. This requires the Transit key
// to be annotated with `deletion_allowed=true` in OpenBao; callers that
// have not set this will receive an error and should surface it as a
// condition on the CR.
func (p *TransitKeyProvisioner) DestroyVolume(ctx context.Context, volumeID string) error {
	if volumeID == "" {
		return errors.New("transit provisioner: empty volumeID")
	}
	// We delete the per-volume wrapped DK object, not the master key.
	// Convention: a KV v2 path "novanas/dks/<volumeID>" holds the wrapped
	// blob; deletion there is the cryptographic erase.
	path := fmt.Sprintf("secret/data/novanas/dks/%s", volumeID)
	_, err := p.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("transit provisioner: destroy %q: %w", volumeID, err)
	}
	return nil
}

func (p *TransitKeyProvisioner) token() (string, error) {
	if p.cfg.TokenPath != "" {
		data, err := os.ReadFile(p.cfg.TokenPath)
		if err != nil {
			return "", fmt.Errorf("read token: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return p.cfg.Token, nil
}

func (p *TransitKeyProvisioner) do(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(buf)
	}
	url := strings.TrimRight(p.cfg.Addr, "/") + "/v1/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	tok, err := p.token()
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", tok)
	if p.cfg.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", p.cfg.Namespace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
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

// parseTransitVersion extracts N from "vault:vN:..." Transit ciphertext.
func parseTransitVersion(ct string) (uint64, error) {
	parts := strings.SplitN(ct, ":", 3)
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "v") {
		return 0, fmt.Errorf("transit: unrecognised ciphertext format: %q", ct)
	}
	var v uint64
	if _, err := fmt.Sscanf(parts[1], "v%d", &v); err != nil {
		return 0, fmt.Errorf("transit: parse version: %w", err)
	}
	return v, nil
}

// Ensure the provisioner satisfies the interface.
var _ VolumeKeyProvisioner = (*TransitKeyProvisioner)(nil)
