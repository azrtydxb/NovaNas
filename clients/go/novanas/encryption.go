package novanas

import (
	"context"
	"net/http"
	"net/url"
)

// EncryptionInitRequest is the body for POST /datasets/{full}/encryption.
type EncryptionInitRequest struct {
	Type            string            `json:"type"` // "filesystem" or "volume"
	VolumeSizeBytes uint64            `json:"volumeSizeBytes,omitempty"`
	Algorithm       string            `json:"algorithm,omitempty"` // default "aes-256-gcm"
	Properties      map[string]string `json:"properties,omitempty"`
}

// EncryptionInitResponse is the response from initialize.
type EncryptionInitResponse struct {
	Dataset   string `json:"dataset"`
	Algorithm string `json:"algorithm"`
	Created   string `json:"created"`
}

// EncryptionRecoverResponse is the response from the recover endpoint.
// KeyHex is the 64-character hex of the 32-byte raw ZFS key.
type EncryptionRecoverResponse struct {
	Dataset string `json:"dataset"`
	KeyHex  string `json:"keyHex"`
}

// InitializeDatasetEncryption provisions a fresh encrypted dataset
// (TPM-sealed key escrow). Required permission: nova:encryption:write.
func (c *Client) InitializeDatasetEncryption(ctx context.Context, fullname string, req EncryptionInitRequest) (*EncryptionInitResponse, error) {
	var out EncryptionInitResponse
	path := "/datasets/" + url.PathEscape(fullname) + "/encryption"
	if _, err := c.do(ctx, http.MethodPost, path, nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// LoadDatasetEncryptionKey unwraps the escrowed key and feeds it to
// `zfs load-key`. Idempotent on the server side modulo ZFS's "key
// already loaded" error. Required permission: nova:encryption:write.
func (c *Client) LoadDatasetEncryptionKey(ctx context.Context, fullname string) error {
	path := "/datasets/" + url.PathEscape(fullname) + "/encryption/load-key"
	_, err := c.do(ctx, http.MethodPost, path, nil, nil, nil)
	return err
}

// UnloadDatasetEncryptionKey detaches the in-memory key from ZFS,
// leaving the wrapped blob in escrow. Required permission:
// nova:encryption:write.
func (c *Client) UnloadDatasetEncryptionKey(ctx context.Context, fullname string) error {
	path := "/datasets/" + url.PathEscape(fullname) + "/encryption/unload-key"
	_, err := c.do(ctx, http.MethodPost, path, nil, nil, nil)
	return err
}

// RecoverDatasetEncryptionKey returns the raw 32-byte ZFS key as hex.
// This is the break-glass capability for migrating an encrypted
// dataset to a host with a different TPM, or for offline backup of
// the key. Every call is audit-logged on the server. Required
// permission: nova:encryption:recover (admin-only).
func (c *Client) RecoverDatasetEncryptionKey(ctx context.Context, fullname string) (*EncryptionRecoverResponse, error) {
	var out EncryptionRecoverResponse
	path := "/datasets/" + url.PathEscape(fullname) + "/encryption/recover"
	if _, err := c.do(ctx, http.MethodPost, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
