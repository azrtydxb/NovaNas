package replication

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/secrets"
)

// Secret keys layout (per-job, under nova/replication/<job-id>/...):
//
//   nova/replication/<id>/s3-access-key   - S3 access key id
//   nova/replication/<id>/s3-secret-key   - S3 secret key
//   nova/replication/<id>/ssh-key         - SSH private key (PEM)
//
// The path component grammar enforced by internal/host/secrets disallows
// dots and underscores at the segment level; we therefore use hyphens.

// SecretKeyPrefix returns the OpenBao key prefix used for a given job.
// Operators interact with this prefix when populating credentials by hand.
func SecretKeyPrefix(jobID uuid.UUID) string {
	return "nova/replication/" + jobID.String()
}

// S3SecretKey returns the bao key for the S3 access key id.
func S3AccessKey(jobID uuid.UUID) string { return SecretKeyPrefix(jobID) + "/s3-access-key" }

// S3SecretKeyKey returns the bao key for the S3 secret key.
func S3SecretKey(jobID uuid.UUID) string { return SecretKeyPrefix(jobID) + "/s3-secret-key" }

// SSHKey returns the bao key under which the SSH private key is stored
// for ZFS / rsync backends.
func SSHKey(jobID uuid.UUID) string { return SecretKeyPrefix(jobID) + "/ssh-key" }

// S3Credentials is the resolved pair returned by LoadS3Credentials.
type S3Credentials struct {
	AccessKey string
	SecretKey string
}

// LoadS3Credentials reads the per-job S3 credentials from the secrets
// manager. ErrNotFound is returned when neither key is present.
func LoadS3Credentials(ctx context.Context, sm secrets.Manager, jobID uuid.UUID) (S3Credentials, error) {
	if sm == nil {
		return S3Credentials{}, errors.New("replication: secrets manager required")
	}
	ak, err := sm.Get(ctx, S3AccessKey(jobID))
	if err != nil {
		return S3Credentials{}, fmt.Errorf("read s3 access key: %w", err)
	}
	sk, err := sm.Get(ctx, S3SecretKey(jobID))
	if err != nil {
		return S3Credentials{}, fmt.Errorf("read s3 secret key: %w", err)
	}
	return S3Credentials{AccessKey: string(ak), SecretKey: string(sk)}, nil
}

// LoadSSHKey reads the per-job SSH private key bytes.
func LoadSSHKey(ctx context.Context, sm secrets.Manager, jobID uuid.UUID) ([]byte, error) {
	if sm == nil {
		return nil, errors.New("replication: secrets manager required")
	}
	return sm.Get(ctx, SSHKey(jobID))
}

// DeleteJobSecrets removes any secrets stored under the per-job prefix.
// Called by the HTTP DELETE handler after the job row is deleted. Errors
// are best-effort and surfaced to the caller.
func DeleteJobSecrets(ctx context.Context, sm secrets.Manager, jobID uuid.UUID) error {
	if sm == nil {
		return nil
	}
	prefix := SecretKeyPrefix(jobID)
	keys, err := sm.List(ctx, prefix)
	if err != nil {
		return fmt.Errorf("list job secrets: %w", err)
	}
	var firstErr error
	for _, k := range keys {
		if err := sm.Delete(ctx, k); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
