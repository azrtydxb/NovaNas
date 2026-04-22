package s3

import (
	"encoding/base64"
	"fmt"
	"net/http"
)

// SSEMode is the server-side-encryption flavour requested by an S3 client.
type SSEMode int

const (
	// SSEModeNone: no server-side encryption requested. Use the bucket's
	// default scheme (which may itself be SSE-S3 or none).
	SSEModeNone SSEMode = iota
	// SSEModeS3 (AES256): encrypt with the bucket's Dataset Key via the
	// default convergent namespace. Chunks dedup within the bucket's DK.
	SSEModeS3
	// SSEModeKMS: encrypt with a KMS-managed key referenced by a KmsKey
	// CRD. Full wiring requires looking up the KmsKey object and using
	// its wrapped DK; 501 if the referenced key is not found.
	SSEModeKMS
	// SSEModeC: SSE-C -- customer supplies the raw key in headers. Chunks
	// go to the segregated NamespaceSSEC and never dedup.
	SSEModeC
)

// SSERequest is the parsed form of the x-amz-server-side-encryption* headers.
type SSERequest struct {
	Mode SSEMode
	// KMSKeyID is populated for SSEModeKMS.
	KMSKeyID string
	// CustomerAlgorithm is the raw algorithm string for SSE-C
	// (only "AES256" is valid per the spec).
	CustomerAlgorithm string
	// CustomerKey is the raw 32-byte key for SSE-C, decoded from
	// base64 in x-amz-server-side-encryption-customer-key.
	CustomerKey []byte
	// CustomerKeyMD5 is the caller-supplied MD5 of the raw key, for
	// verification (not enforced here -- caller's responsibility).
	CustomerKeyMD5 string
}

// ParseSSEHeaders inspects request headers and returns the SSE mode the
// client is requesting. Unknown / empty combinations return SSEModeNone.
//
// Priority (matches AWS behaviour):
//  1. x-amz-server-side-encryption-customer-algorithm (and friends) -> SSE-C
//  2. x-amz-server-side-encryption: "aws:kms" -> SSE-KMS (with key id from
//     x-amz-server-side-encryption-aws-kms-key-id)
//  3. x-amz-server-side-encryption: "AES256" -> SSE-S3
//  4. otherwise -> None
//
// This is the "header routing decision tree" required by the A6 wiring
// spec. The actual encryption primitives live in storage/internal/crypto.
func ParseSSEHeaders(h http.Header) (*SSERequest, error) {
	out := &SSERequest{Mode: SSEModeNone}

	if alg := h.Get("x-amz-server-side-encryption-customer-algorithm"); alg != "" {
		if alg != "AES256" {
			return nil, fmt.Errorf("s3: SSE-C algorithm %q unsupported (only AES256)", alg)
		}
		rawB64 := h.Get("x-amz-server-side-encryption-customer-key")
		if rawB64 == "" {
			return nil, fmt.Errorf("s3: SSE-C missing customer-key header")
		}
		key, err := base64.StdEncoding.DecodeString(rawB64)
		if err != nil {
			return nil, fmt.Errorf("s3: SSE-C key base64: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("s3: SSE-C key must be 32 bytes, got %d", len(key))
		}
		out.Mode = SSEModeC
		out.CustomerAlgorithm = alg
		out.CustomerKey = key
		out.CustomerKeyMD5 = h.Get("x-amz-server-side-encryption-customer-key-md5")
		return out, nil
	}

	switch h.Get("x-amz-server-side-encryption") {
	case "aws:kms":
		out.Mode = SSEModeKMS
		out.KMSKeyID = h.Get("x-amz-server-side-encryption-aws-kms-key-id")
	case "AES256":
		out.Mode = SSEModeS3
	case "":
		// no-op
	default:
		return nil, fmt.Errorf("s3: unsupported x-amz-server-side-encryption: %q", h.Get("x-amz-server-side-encryption"))
	}
	return out, nil
}

// RouteSSE decides which internal path an S3 request takes based on the
// parsed SSE headers. It returns a human-readable namespace string for
// metrics / logs and an error if the request is well-formed but cannot
// be satisfied (e.g. SSE-KMS without the KmsKey CRD available).
//
// NOTE: this is minimum-viable header routing. Actual crypto happens in
// crypto.EncryptChunk (SSE-S3, SSE-KMS) or crypto.EncryptChunkSSEC
// (SSE-C). TODO(wave-7): plumb the returned namespace into the chunk
// write path so the segregated SSEC namespace is actually used.
func RouteSSE(req *SSERequest, kmsKeyLookup func(keyID string) ([]byte, bool)) (string, error) {
	if req == nil {
		return "default", nil
	}
	switch req.Mode {
	case SSEModeNone:
		return "default", nil
	case SSEModeS3:
		return "default", nil
	case SSEModeKMS:
		if kmsKeyLookup == nil {
			return "", fmt.Errorf("s3: SSE-KMS requested but no KmsKey resolver configured (501)")
		}
		if _, ok := kmsKeyLookup(req.KMSKeyID); !ok {
			return "", fmt.Errorf("s3: KmsKey %q not found (501)", req.KMSKeyID)
		}
		return "default", nil
	case SSEModeC:
		return "ssec", nil
	default:
		return "", fmt.Errorf("s3: unknown SSE mode %d", req.Mode)
	}
}
