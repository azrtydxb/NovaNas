package reconciler

import (
	"context"
	"encoding/base64"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
)

// TransitUnwrapper is the minimal interface kms_resolver needs against an
// OpenBao Transit backend. It matches the openbao.TransitClient contract
// (UnwrapDK) but lives here so the operators module does not import
// storage/internal/openbao directly (that package is internal to the
// storage module).
type TransitUnwrapper interface {
	UnwrapDK(ctx context.Context, masterKeyName string, wrapped []byte) ([]byte, error)
}

// KmsKeyLookup is the function signature expected by the S3 gateway's
// RouteSSE: given a KMS key id, return (rawDK, true) if resolvable,
// otherwise (nil, false). See storage/internal/s3/sse.go.
type KmsKeyLookup func(keyID string) (rawDK []byte, ok bool)

// KmsResolver resolves KMS key ids to raw Dataset Keys by:
//  1. Looking up the KmsKey CRD by spec.keyId (cluster-scoped).
//  2. Extracting status.wrappedDK (OpenBao Transit ciphertext).
//  3. Unwrapping via the supplied TransitUnwrapper.
//
// Unwrap errors are returned as "not found" from the lookup function so
// the S3 layer returns a clean 501/InvalidKey error instead of leaking
// internal details.
type KmsResolver struct {
	// Client is a sigs.k8s.io/controller-runtime client capable of
	// listing KmsKey CRs.
	Client client.Client
	// Transit unwraps ciphertext blobs.
	Transit TransitUnwrapper
	// MasterKeyName is the OpenBao Transit key used to unwrap each
	// KmsKey's wrappedDK. All NovaNas KMS keys share one master key by
	// default ("novanas/chunk-master"); override per-install if needed.
	MasterKeyName string
}

// Lookup returns a KmsKeyLookup suitable for passing into
// storage/internal/s3.RouteSSE. A background context is used because the
// S3 gateway's call site is not context-aware.
func (r *KmsResolver) Lookup(ctx context.Context) KmsKeyLookup {
	return func(keyID string) ([]byte, bool) {
		dk, err := r.resolve(ctx, keyID)
		if err != nil || len(dk) == 0 {
			return nil, false
		}
		return dk, true
	}
}

func (r *KmsResolver) resolve(ctx context.Context, keyID string) ([]byte, error) {
	if r == nil || r.Client == nil || r.Transit == nil {
		return nil, fmt.Errorf("kms resolver not configured")
	}
	if keyID == "" {
		return nil, fmt.Errorf("empty kms key id")
	}

	var list novanasv1alpha1.KmsKeyList
	if err := r.Client.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list KmsKey: %w", err)
	}
	for i := range list.Items {
		k := &list.Items[i]
		if k.Spec.KeyID != keyID {
			continue
		}
		if k.Status.WrappedDK == "" {
			return nil, fmt.Errorf("KmsKey %q has empty status.wrappedDK", keyID)
		}
		wrapped, err := base64.StdEncoding.DecodeString(k.Status.WrappedDK)
		if err != nil {
			// Some operators store the raw "vault:v1:..." string. Fall
			// back to treating it as already-decoded bytes.
			wrapped = []byte(k.Status.WrappedDK)
		}
		master := r.MasterKeyName
		if master == "" {
			master = "novanas/chunk-master"
		}
		raw, err := r.Transit.UnwrapDK(ctx, master, wrapped)
		if err != nil {
			return nil, fmt.Errorf("unwrap KmsKey %q: %w", keyID, err)
		}
		return raw, nil
	}
	return nil, fmt.Errorf("KmsKey %q not found", keyID)
}
