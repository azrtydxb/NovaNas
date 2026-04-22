package reconciler

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// FinalizerPrefix is the common namespace for NovaNas-owned finalizers.
const FinalizerPrefix = "novanas.io/"

// Finalizer names for each CRD kind. Kept as constants so controllers do not
// re-derive the string and risk typos breaking deletion.
const (
	FinalizerStoragePool        = FinalizerPrefix + "storagepool"
	FinalizerDisk               = FinalizerPrefix + "disk"
	FinalizerSnapshot           = FinalizerPrefix + "snapshot"
	FinalizerSnapshotSchedule   = FinalizerPrefix + "snapshotschedule"
	FinalizerReplicationTarget  = FinalizerPrefix + "replicationtarget"
	FinalizerReplicationJob     = FinalizerPrefix + "replicationjob"
	FinalizerCloudBackupTarget  = FinalizerPrefix + "cloudbackuptarget"
	FinalizerCloudBackupJob     = FinalizerPrefix + "cloudbackupjob"
	FinalizerScrubSchedule      = FinalizerPrefix + "scrubschedule"
	FinalizerEncryptionPolicy   = FinalizerPrefix + "encryptionpolicy"
	FinalizerKmsKey             = FinalizerPrefix + "kmskey"
	FinalizerCertificate        = FinalizerPrefix + "certificate"
	FinalizerShare              = FinalizerPrefix + "share"
	FinalizerSmbServer          = FinalizerPrefix + "smbserver"
	FinalizerNfsServer          = FinalizerPrefix + "nfsserver"
	FinalizerIscsiTarget        = FinalizerPrefix + "iscsitarget"
	FinalizerNvmeofTarget       = FinalizerPrefix + "nvmeoftarget"
	FinalizerObjectStore        = FinalizerPrefix + "objectstore"
	FinalizerBucketUser         = FinalizerPrefix + "bucketuser"
	FinalizerUser               = FinalizerPrefix + "user"
	FinalizerGroup              = FinalizerPrefix + "group"
	FinalizerApiToken           = FinalizerPrefix + "apitoken"
	FinalizerSshKey             = FinalizerPrefix + "sshkey"
	FinalizerKeycloakRealm      = FinalizerPrefix + "keycloakrealm"
)

// EnsureFinalizer adds name to obj's finalizers if missing and persists via
// Update. Returns (added=true) when an update happened so the caller can
// requeue.
func EnsureFinalizer(ctx context.Context, c client.Client, obj client.Object, name string) (bool, error) {
	if controllerutil.ContainsFinalizer(obj, name) {
		return false, nil
	}
	controllerutil.AddFinalizer(obj, name)
	if err := c.Update(ctx, obj); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveFinalizer strips name from obj's finalizers and persists via Update.
// No-op if the finalizer is absent.
func RemoveFinalizer(ctx context.Context, c client.Client, obj client.Object, name string) error {
	if !controllerutil.ContainsFinalizer(obj, name) {
		return nil
	}
	controllerutil.RemoveFinalizer(obj, name)
	return c.Update(ctx, obj)
}
