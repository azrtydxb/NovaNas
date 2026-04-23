package controllers

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// The tests in this file exercise the happy path of each A7-Operators-Part1
// controller: create CR, reconcile twice (finalizer-add then work), then
// assert the Ready/Active/Scheduled/Observed phase is present. Fake
// controller-runtime client + Noop* interface defaults keep these
// self-contained and fast.

func TestStoragePoolReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.StoragePool{ObjectMeta: newClusterMeta("fast")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &StoragePoolReconciler{BaseReconciler: newPart2Base(c, s, "StoragePool"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("fast"))
	var got novanasv1alpha1.StoragePool
	if err := c.Get(context.Background(), client.ObjectKey{Name: "fast"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase == "" {
		t.Fatalf("phase empty")
	}
	if len(got.Status.Conditions) == 0 {
		t.Fatalf("expected conditions to be set")
	}
}

func TestDiskReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Disk{ObjectMeta: newClusterMeta("disk-a")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &DiskReconciler{BaseReconciler: newPart2Base(c, s, "Disk"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("disk-a"))
	var got novanasv1alpha1.Disk
	_ = c.Get(context.Background(), client.ObjectKey{Name: "disk-a"}, &got)
	if got.Status.State == "" {
		t.Fatalf("expected disk state to be set")
	}
}

func TestSnapshotReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Snapshot{ObjectMeta: newClusterMeta("snap1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &SnapshotReconciler{BaseReconciler: newPart2Base(c, s, "Snapshot"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("snap1"))
	var got novanasv1alpha1.Snapshot
	_ = c.Get(context.Background(), client.ObjectKey{Name: "snap1"}, &got)
	if got.Status.Phase != "Completed" {
		t.Fatalf("phase = %q, want Completed", got.Status.Phase)
	}
}

func TestSnapshotScheduleReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.SnapshotSchedule{ObjectMeta: newClusterMeta("sched1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &SnapshotScheduleReconciler{BaseReconciler: newPart2Base(c, s, "SnapshotSchedule"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("sched1"))
	var got novanasv1alpha1.SnapshotSchedule
	_ = c.Get(context.Background(), client.ObjectKey{Name: "sched1"}, &got)
	if got.Status.Phase != "Scheduled" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestReplicationTargetReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ReplicationTarget{ObjectMeta: newClusterMeta("rt1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ReplicationTargetReconciler{BaseReconciler: newPart2Base(c, s, "ReplicationTarget"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("rt1"))
	var got novanasv1alpha1.ReplicationTarget
	_ = c.Get(context.Background(), client.ObjectKey{Name: "rt1"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestReplicationJobReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ReplicationJob{ObjectMeta: newClusterMeta("rj1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ReplicationJobReconciler{BaseReconciler: newPart2Base(c, s, "ReplicationJob"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("rj1"))
	var got novanasv1alpha1.ReplicationJob
	_ = c.Get(context.Background(), client.ObjectKey{Name: "rj1"}, &got)
	if got.Status.Phase != "Completed" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestCloudBackupTargetReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s3-creds", Namespace: "novanas-system"},
		Data:       map[string][]byte{"AWS_ACCESS_KEY_ID": []byte("x"), "AWS_SECRET_ACCESS_KEY": []byte("y")},
	}
	cr := &novanasv1alpha1.CloudBackupTarget{
		ObjectMeta: newClusterMeta("cbt1"),
		Spec: novanasv1alpha1.CloudBackupTargetSpec{
			Provider: "s3",
			Bucket:   "novanas-backups",
			Region:   "us-east-1",
			CredentialsSecret: novanasv1alpha1.SecretKeyRef{
				Name: "s3-creds", Namespace: "novanas-system", Key: "AWS_ACCESS_KEY_ID",
			},
		},
	}
	c := newPart2Client(s, []client.Object{cr, sec}, []client.Object{cr})
	r := &CloudBackupTargetReconciler{
		BaseReconciler: newPart2Base(c, s, "CloudBackupTarget"),
		Recorder:       newPart2Recorder(),
		Prober:         stubProber{},
	}
	mustReconcileOK(t, context.Background(), r, part2Request("cbt1"))
	var got novanasv1alpha1.CloudBackupTarget
	_ = c.Get(context.Background(), client.ObjectKey{Name: "cbt1"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if !got.Status.Reachable {
		t.Fatalf("expected Reachable=true")
	}
}

func TestCloudBackupJobReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.CloudBackupJob{
		ObjectMeta: newClusterMeta("cbj1"),
		Spec: novanasv1alpha1.CloudBackupJobSpec{
			Target: "cbt1",
			Source: novanasv1alpha1.VolumeSourceRef{Kind: "BlockVolume", Name: "vol1"},
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &CloudBackupJobReconciler{BaseReconciler: newPart2Base(c, s, "CloudBackupJob"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("cbj1"))
	var got novanasv1alpha1.CloudBackupJob
	_ = c.Get(context.Background(), client.ObjectKey{Name: "cbj1"}, &got)
	if got.Status.Phase != "Succeeded" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestScrubScheduleReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ScrubSchedule{ObjectMeta: newClusterMeta("scrub1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ScrubScheduleReconciler{BaseReconciler: newPart2Base(c, s, "ScrubSchedule"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("scrub1"))
	var got novanasv1alpha1.ScrubSchedule
	_ = c.Get(context.Background(), client.ObjectKey{Name: "scrub1"}, &got)
	if got.Status.Phase != "Scheduled" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestEncryptionPolicyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.EncryptionPolicy{ObjectMeta: newClusterMeta("ep1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &EncryptionPolicyReconciler{BaseReconciler: newPart2Base(c, s, "EncryptionPolicy"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("ep1"))
	var got novanasv1alpha1.EncryptionPolicy
	_ = c.Get(context.Background(), client.ObjectKey{Name: "ep1"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestKmsKeyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.KmsKey{ObjectMeta: newClusterMeta("k1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &KmsKeyReconciler{BaseReconciler: newPart2Base(c, s, "KmsKey"), Recorder: newPart2Recorder(), KeyProvisioner: reconciler.NoopKeyProvisioner{}}
	mustReconcileOK(t, context.Background(), r, part2Request("k1"))
	var got novanasv1alpha1.KmsKey
	_ = c.Get(context.Background(), client.ObjectKey{Name: "k1"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestCertificateReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Certificate{ObjectMeta: newNsMeta("ns", "c1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &CertificateReconciler{BaseReconciler: newPart2Base(c, s, "Certificate"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2NsRequest("ns", "c1"))
	var got novanasv1alpha1.Certificate
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "c1"}, &got)
	if got.Status.Phase != "Issued" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if got.Status.NotAfter == nil {
		t.Fatalf("expected NotAfter populated, got nil")
	}
	if got.Status.SerialNumber == "" {
		t.Fatalf("expected SerialNumber populated, got empty")
	}
}

func TestShareReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Share{ObjectMeta: newClusterMeta("s1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ShareReconciler{BaseReconciler: newPart2Base(c, s, "Share"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("s1"))
	var got novanasv1alpha1.Share
	_ = c.Get(context.Background(), client.ObjectKey{Name: "s1"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestSmbServerReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.SmbServer{ObjectMeta: newClusterMeta("smb1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &SmbServerReconciler{BaseReconciler: newPart2Base(c, s, "SmbServer"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("smb1"))
	var got novanasv1alpha1.SmbServer
	_ = c.Get(context.Background(), client.ObjectKey{Name: "smb1"}, &got)
	if got.Status.Phase == "" {
		t.Fatalf("expected phase set")
	}
}

func TestNfsServerReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.NfsServer{ObjectMeta: newClusterMeta("nfs1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &NfsServerReconciler{BaseReconciler: newPart2Base(c, s, "NfsServer"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("nfs1"))
	var got novanasv1alpha1.NfsServer
	_ = c.Get(context.Background(), client.ObjectKey{Name: "nfs1"}, &got)
	if got.Status.Phase == "" {
		t.Fatalf("expected phase set")
	}
}

func TestIscsiTargetReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.IscsiTarget{ObjectMeta: newClusterMeta("iqn1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &IscsiTargetReconciler{BaseReconciler: newPart2Base(c, s, "IscsiTarget"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("iqn1"))
	var got novanasv1alpha1.IscsiTarget
	_ = c.Get(context.Background(), client.ObjectKey{Name: "iqn1"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestNvmeofTargetReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.NvmeofTarget{ObjectMeta: newClusterMeta("nqn1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &NvmeofTargetReconciler{BaseReconciler: newPart2Base(c, s, "NvmeofTarget"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("nqn1"))
	var got novanasv1alpha1.NvmeofTarget
	_ = c.Get(context.Background(), client.ObjectKey{Name: "nqn1"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestObjectStoreReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ObjectStore{ObjectMeta: newClusterMeta("os1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ObjectStoreReconciler{BaseReconciler: newPart2Base(c, s, "ObjectStore"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("os1"))
	var got novanasv1alpha1.ObjectStore
	_ = c.Get(context.Background(), client.ObjectKey{Name: "os1"}, &got)
	if got.Status.Phase == "" {
		t.Fatalf("expected phase set")
	}
}

func TestBucketUserReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.BucketUser{ObjectMeta: newClusterMeta("bu1")}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &BucketUserReconciler{BaseReconciler: newPart2Base(c, s, "BucketUser"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("bu1"))
	var got novanasv1alpha1.BucketUser
	_ = c.Get(context.Background(), client.ObjectKey{Name: "bu1"}, &got)
	if got.Status.Phase != "Ready" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

func TestUserReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.User{
		ObjectMeta: newClusterMeta("alice"),
		Spec:       novanasv1alpha1.UserSpec{Username: "alice", Email: "alice@example.com"},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &UserReconciler{BaseReconciler: newPart2Base(c, s, "User"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("alice"))
	var got novanasv1alpha1.User
	_ = c.Get(context.Background(), client.ObjectKey{Name: "alice"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if got.Status.KeycloakID == "" {
		t.Fatalf("expected KeycloakID populated")
	}
}

func TestGroupReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Group{
		ObjectMeta: newClusterMeta("admins"),
		Spec:       novanasv1alpha1.GroupSpec{Name: "admins", Members: []string{"alice", "bob"}},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &GroupReconciler{BaseReconciler: newPart2Base(c, s, "Group"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("admins"))
	var got novanasv1alpha1.Group
	_ = c.Get(context.Background(), client.ObjectKey{Name: "admins"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if got.Status.MemberCount != 2 {
		t.Fatalf("MemberCount = %d; want 2", got.Status.MemberCount)
	}
	if got.Status.KeycloakID == "" {
		t.Fatalf("expected KeycloakID populated")
	}
}

func TestApiTokenReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ApiToken{
		ObjectMeta: newClusterMeta("tok1"),
		Spec:       novanasv1alpha1.ApiTokenSpec{Owner: "alice", Scopes: []string{"read"}},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ApiTokenReconciler{BaseReconciler: newPart2Base(c, s, "ApiToken"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("tok1"))
	var got novanasv1alpha1.ApiToken
	_ = c.Get(context.Background(), client.ObjectKey{Name: "tok1"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if got.Status.TokenID == "" {
		t.Fatalf("expected TokenID populated")
	}
	if got.Status.RawTokenSecret == "" {
		t.Fatalf("expected RawTokenSecret delivered on first reconcile")
	}
	if got.Status.LastRotatedAt == nil {
		t.Fatalf("expected LastRotatedAt populated")
	}
}

func TestSshKeyReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	// A realistic ed25519 public key blob so fingerprint parsing succeeds.
	const pub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGxxTqhF9N1l9YkXxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx alice@example.com"
	cr := &novanasv1alpha1.SshKey{
		ObjectMeta: newClusterMeta("ssh1"),
		Spec:       novanasv1alpha1.SshKeySpec{Owner: "alice", PublicKey: pub},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &SshKeyReconciler{BaseReconciler: newPart2Base(c, s, "SshKey"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("ssh1"))
	var got novanasv1alpha1.SshKey
	_ = c.Get(context.Background(), client.ObjectKey{Name: "ssh1"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if got.Status.KeyType != "ssh-ed25519" {
		t.Fatalf("KeyType = %q; want ssh-ed25519", got.Status.KeyType)
	}
	if got.Status.Fingerprint == "" {
		t.Fatalf("expected Fingerprint populated")
	}
}

func TestKeycloakRealmReconciler_HappyPath(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.KeycloakRealm{
		ObjectMeta: newClusterMeta("realm1"),
		Spec:       novanasv1alpha1.KeycloakRealmSpec{DisplayName: "Realm 1"},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &KeycloakRealmReconciler{BaseReconciler: newPart2Base(c, s, "KeycloakRealm"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("realm1"))
	var got novanasv1alpha1.KeycloakRealm
	_ = c.Get(context.Background(), client.ObjectKey{Name: "realm1"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if got.Status.LastSync == nil {
		t.Fatalf("expected LastSync populated")
	}
}
