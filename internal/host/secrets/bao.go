package secrets

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// BaoOpts configures a BaoBackend.
type BaoOpts struct {
	Address   string        // e.g. "https://openbao.local:8200"
	Token     string        // VAULT_TOKEN
	KVMount   string        // e.g. "secret"
	Namespace string        // optional
	Timeout   time.Duration // per-request HTTP timeout (default 10s)

	// HTTPClient lets callers/tests inject a custom http.Client (e.g.
	// to point at an httptest server with a custom transport). If nil,
	// a default client with Timeout is used.
	HTTPClient *http.Client
}

// BaoBackend reads secrets from an OpenBao KV v2 mount.
//
// Storage convention: each secret is stored as a single field "value"
// containing the raw bytes base64-encoded, since KV v2 only persists
// string-valued fields. Get decodes; Set encodes; List walks the KV v2
// metadata tree recursively.
type BaoBackend struct {
	Address   string
	Token     string
	KVMount   string
	Namespace string

	httpc *http.Client
	log   *slog.Logger
}

// NewBaoBackend constructs a BaoBackend. Address, Token and KVMount are
// required. The address is normalized (trailing slash trimmed).
func NewBaoBackend(opts BaoOpts, log *slog.Logger) (*BaoBackend, error) {
	if log == nil {
		log = slog.Default()
	}
	if opts.Address == "" {
		return nil, fmt.Errorf("secrets: bao address is empty")
	}
	if opts.Token == "" {
		return nil, fmt.Errorf("secrets: bao token is empty")
	}
	if opts.KVMount == "" {
		return nil, fmt.Errorf("secrets: bao kv mount is empty")
	}
	u, err := url.Parse(opts.Address)
	if err != nil {
		return nil, fmt.Errorf("secrets: invalid bao address %q: %w", opts.Address, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("secrets: invalid bao address %q (need scheme+host)", opts.Address)
	}
	hc := opts.HTTPClient
	if hc == nil {
		t := opts.Timeout
		if t == 0 {
			t = 10 * time.Second
		}
		hc = &http.Client{Timeout: t}
	}
	return &BaoBackend{
		Address:   strings.TrimRight(opts.Address, "/"),
		Token:     opts.Token,
		KVMount:   strings.Trim(opts.KVMount, "/"),
		Namespace: opts.Namespace,
		httpc:     hc,
		log:       log,
	}, nil
}

// Backend returns "bao".
func (b *BaoBackend) Backend() string { return "bao" }

// dataURL builds the KV v2 /v1/<mount>/data/<key> URL.
func (b *BaoBackend) dataURL(key string) string {
	return fmt.Sprintf("%s/v1/%s/data/%s", b.Address, b.KVMount, key)
}

// metadataURL builds the KV v2 /v1/<mount>/metadata/<key> URL. With
// list=true on a directory it lists children.
func (b *BaoBackend) metadataURL(key string) string {
	return fmt.Sprintf("%s/v1/%s/metadata/%s", b.Address, b.KVMount, key)
}

// do performs an HTTP request with the X-Vault-Token (and optional
// namespace) header set, returning the response body bytes and status
// code. The body is fully read so the caller can inspect it cleanly.
func (b *BaoBackend) do(ctx context.Context, method, u string, body []byte) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-Vault-Token", b.Token)
	if b.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", b.Namespace)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.httpc.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("secrets: bao %s %s: %w", method, u, err)
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("secrets: bao read body: %w", err)
	}
	return resp.StatusCode, rb, nil
}

// kvReadResponse mirrors the subset of KV v2 read response we use.
type kvReadResponse struct {
	Data struct {
		Data     map[string]string `json:"data"`
		Metadata map[string]any    `json:"metadata"`
	} `json:"data"`
}

// kvWriteRequest is the payload for KV v2 writes: {"data": {...}}.
type kvWriteRequest struct {
	Data map[string]string `json:"data"`
}

// kvListResponse is the payload returned by LIST on metadata.
type kvListResponse struct {
	Data struct {
		Keys []string `json:"keys"`
	} `json:"data"`
}

// Get reads <mount>/data/<key>, base64-decodes the "value" field.
func (b *BaoBackend) Get(ctx context.Context, key string) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	status, body, err := b.do(ctx, http.MethodGet, b.dataURL(key), nil)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
		// fall through
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("secrets: bao get %q: status %d: %s", key, status, snippet(body))
	}
	var r kvReadResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("secrets: bao decode %q: %w", key, err)
	}
	enc, ok := r.Data.Data["value"]
	if !ok {
		return nil, fmt.Errorf("secrets: bao response for %q missing 'value' field", key)
	}
	out, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, fmt.Errorf("secrets: bao decode value for %q: %w", key, err)
	}
	return out, nil
}

// Set writes <mount>/data/<key> with {"data":{"value":<base64>}}.
func (b *BaoBackend) Set(ctx context.Context, key string, value []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	payload := kvWriteRequest{
		Data: map[string]string{
			"value": base64.StdEncoding.EncodeToString(value),
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	status, rb, err := b.do(ctx, http.MethodPost, b.dataURL(key), body)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("secrets: bao set %q: status %d: %s", key, status, snippet(rb))
	}
	return nil
}

// Delete removes the secret and all versions via DELETE on metadata.
func (b *BaoBackend) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	status, rb, err := b.do(ctx, http.MethodDelete, b.metadataURL(key), nil)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return fmt.Errorf("secrets: bao delete %q: status %d: %s", key, status, snippet(rb))
	}
}

// List returns all keys under prefix by recursively LIST'ing the KV v2
// metadata tree. Subdirectories are returned by OpenBao with a trailing
// "/" in the keys array; we recurse into those.
func (b *BaoBackend) List(ctx context.Context, prefix string) ([]string, error) {
	if err := validatePrefix(prefix); err != nil {
		return nil, err
	}
	// Find the directory portion of prefix to start listing from. KV v2
	// LIST takes a directory; we list the directory containing prefix
	// and filter results.
	startDir := ""
	if prefix != "" {
		if strings.HasSuffix(prefix, "/") {
			startDir = strings.TrimRight(prefix, "/")
		} else if i := strings.LastIndex(prefix, "/"); i >= 0 {
			startDir = prefix[:i]
		}
	}
	var out []string
	if err := b.listRecursive(ctx, startDir, prefix, &out); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// listRecursive walks the KV v2 metadata tree under dir, collecting
// full keys that start with prefix.
func (b *BaoBackend) listRecursive(ctx context.Context, dir, prefix string, out *[]string) error {
	u := b.metadataURL(dir)
	if !strings.HasSuffix(u, "/") {
		u += "/"
	}
	// "LIST" is a Vault/Bao convention; we use the documented
	// equivalent ?list=true so we don't need a custom HTTP method.
	u += "?list=true"
	status, body, err := b.do(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
		// fall through
	case http.StatusNotFound:
		// Empty / nonexistent directory -> nothing to add.
		return nil
	default:
		return fmt.Errorf("secrets: bao list %q: status %d: %s", dir, status, snippet(body))
	}
	var r kvListResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("secrets: bao decode list %q: %w", dir, err)
	}
	for _, k := range r.Data.Keys {
		full := k
		if dir != "" {
			full = dir + "/" + k
		}
		if strings.HasSuffix(k, "/") {
			child := strings.TrimRight(full, "/")
			if err := b.listRecursive(ctx, child, prefix, out); err != nil {
				return err
			}
			continue
		}
		if prefix == "" || strings.HasPrefix(full, prefix) {
			*out = append(*out, full)
		}
	}
	return nil
}

// snippet returns the first 200 bytes of body for inclusion in errors.
func snippet(body []byte) string {
	if len(body) == 0 {
		return "(empty body)"
	}
	const max = 200
	if len(body) > max {
		return string(body[:max]) + "..."
	}
	return string(body)
}
