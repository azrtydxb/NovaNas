// Package jobs defines async task types and the dispatch/worker plumbing.
package jobs

import (
	"github.com/google/uuid"

	"github.com/novanas/nova-nas/internal/host/iscsi"
	"github.com/novanas/nova-nas/internal/host/krb5"
	"github.com/novanas/nova-nas/internal/host/network"
	"github.com/novanas/nova-nas/internal/host/nfs"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
	"github.com/novanas/nova-nas/internal/host/protocolshare"
	"github.com/novanas/nova-nas/internal/host/samba"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
)

type Kind string

const (
	KindPoolCreate       Kind = "pool.create"
	KindPoolDestroy      Kind = "pool.destroy"
	KindPoolScrub        Kind = "pool.scrub"
	KindPoolReplace      Kind = "pool.replace"
	KindPoolOffline      Kind = "pool.offline"
	KindPoolOnline       Kind = "pool.online"
	KindPoolClear        Kind = "pool.clear"
	KindPoolAttach       Kind = "pool.attach"
	KindPoolDetach       Kind = "pool.detach"
	KindPoolAdd          Kind = "pool.add"
	KindPoolExport       Kind = "pool.export"
	KindPoolImport       Kind = "pool.import"
	KindDatasetCreate    Kind = "dataset.create"
	KindDatasetSet       Kind = "dataset.set"
	KindDatasetDestroy   Kind = "dataset.destroy"
	KindDatasetRename    Kind = "dataset.rename"
	KindDatasetClone     Kind = "dataset.clone"
	KindDatasetPromote   Kind = "dataset.promote"
	KindDatasetLoadKey   Kind = "dataset.load_key"
	KindDatasetUnloadKey Kind = "dataset.unload_key"
	KindDatasetChangeKey Kind = "dataset.change_key"
	KindPoolTrim         Kind = "pool.trim"
	KindPoolSetProps     Kind = "pool.set_props"
	KindSnapshotCreate   Kind = "snapshot.create"
	KindSnapshotDestroy  Kind = "snapshot.destroy"
	KindSnapshotRollback Kind = "snapshot.rollback"

	KindDatasetBookmark        Kind = "dataset.bookmark"
	KindDatasetDestroyBookmark Kind = "dataset.destroy_bookmark"
	KindSnapshotHold           Kind = "snapshot.hold"
	KindSnapshotRelease        Kind = "snapshot.release"
	KindPoolCheckpoint         Kind = "pool.checkpoint"
	KindPoolDiscardCheckpoint  Kind = "pool.discard_checkpoint"
	KindPoolUpgrade            Kind = "pool.upgrade"
	KindPoolReguid             Kind = "pool.reguid"

	// iSCSI
	KindIscsiTargetCreate  Kind = "iscsi.target.create"
	KindIscsiTargetDestroy Kind = "iscsi.target.destroy"
	KindIscsiPortalCreate  Kind = "iscsi.portal.create"
	KindIscsiPortalDelete  Kind = "iscsi.portal.delete"
	KindIscsiLUNCreate     Kind = "iscsi.lun.create"
	KindIscsiLUNDelete     Kind = "iscsi.lun.delete"
	KindIscsiACLCreate     Kind = "iscsi.acl.create"
	KindIscsiACLDelete     Kind = "iscsi.acl.delete"
	KindIscsiSaveConfig    Kind = "iscsi.saveconfig"

	// NVMe-oF
	KindNvmeofSubsystemCreate  Kind = "nvmeof.subsystem.create"
	KindNvmeofSubsystemDestroy Kind = "nvmeof.subsystem.destroy"
	KindNvmeofNamespaceAdd     Kind = "nvmeof.namespace.add"
	KindNvmeofNamespaceRemove  Kind = "nvmeof.namespace.remove"
	KindNvmeofHostAllow        Kind = "nvmeof.host.allow"
	KindNvmeofHostDisallow     Kind = "nvmeof.host.disallow"
	KindNvmeofPortCreate       Kind = "nvmeof.port.create"
	KindNvmeofPortDelete       Kind = "nvmeof.port.delete"
	KindNvmeofPortLink         Kind = "nvmeof.port.link"
	KindNvmeofPortUnlink       Kind = "nvmeof.port.unlink"
	KindNvmeofSetHostDHChap    Kind = "nvmeof.host.dhchap.set"
	KindNvmeofClearHostDHChap  Kind = "nvmeof.host.dhchap.clear"
	KindNvmeofSaveConfig       Kind = "nvmeof.saveconfig"

	// NFS
	KindNfsExportCreate Kind = "nfs.export.create"
	KindNfsExportUpdate Kind = "nfs.export.update"
	KindNfsExportDelete Kind = "nfs.export.delete"
	KindNfsReload       Kind = "nfs.reload"

	// Kerberos
	KindKrb5SetConfig     Kind = "krb5.config.set"
	KindKrb5SetIdmapd     Kind = "krb5.idmapd.set"
	KindKrb5UploadKeytab  Kind = "krb5.keytab.upload"
	KindKrb5DeleteKeytab  Kind = "krb5.keytab.delete"

	// Samba
	KindSambaShareCreate     Kind = "samba.share.create"
	KindSambaShareUpdate     Kind = "samba.share.update"
	KindSambaShareDelete     Kind = "samba.share.delete"
	KindSambaReload          Kind = "samba.reload"
	KindSambaUserAdd         Kind = "samba.user.add"
	KindSambaUserDelete      Kind = "samba.user.delete"
	KindSambaUserSetPassword Kind = "samba.user.set_password"

	// SMART
	KindSmartRunSelfTest Kind = "smart.selftest.run"
	KindSmartEnable      Kind = "smart.enable"

	// Scheduler (dispatched by the tick loop, not HTTP)
	KindSchedSnapshotFire    Kind = "scheduler.snapshot.fire"
	KindSchedReplicationFire Kind = "scheduler.replication.fire"

	// Network
	KindNetworkInterfaceApply  Kind = "network.interface.apply"
	KindNetworkInterfaceDelete Kind = "network.interface.delete"
	KindNetworkVLANApply       Kind = "network.vlan.apply"
	KindNetworkBondApply       Kind = "network.bond.apply"
	KindNetworkReload          Kind = "network.reload"

	// System
	KindSystemSetHostname    Kind = "system.hostname.set"
	KindSystemSetTimezone    Kind = "system.timezone.set"
	KindSystemSetNTP         Kind = "system.ntp.set"
	KindSystemReboot         Kind = "system.reboot"
	KindSystemShutdown       Kind = "system.shutdown"
	KindSystemCancelShutdown Kind = "system.cancel_shutdown"

	// ProtocolShare + ACL
	KindProtocolShareCreate Kind = "protocolshare.create"
	KindProtocolShareUpdate Kind = "protocolshare.update"
	KindProtocolShareDelete Kind = "protocolshare.delete"
	KindDatasetSetACL       Kind = "dataset.acl.set"
	KindDatasetAppendACE    Kind = "dataset.acl.append"
	KindDatasetRemoveACE    Kind = "dataset.acl.remove"
	KindSambaSetGlobals     Kind = "samba.globals.set"
)

// ---------- iSCSI payloads ----------

type IscsiTargetCreatePayload struct {
	IQN string `json:"iqn"`
}

type IscsiTargetDestroyPayload struct {
	IQN string `json:"iqn"`
}

type IscsiPortalCreatePayload struct {
	IQN    string       `json:"iqn"`
	Portal iscsi.Portal `json:"portal"`
}

type IscsiPortalDeletePayload struct {
	IQN    string       `json:"iqn"`
	Portal iscsi.Portal `json:"portal"`
}

type IscsiLUNCreatePayload struct {
	IQN string    `json:"iqn"`
	LUN iscsi.LUN `json:"lun"`
}

type IscsiLUNDeletePayload struct {
	IQN string `json:"iqn"`
	ID  int    `json:"id"`
}

type IscsiACLCreatePayload struct {
	IQN string    `json:"iqn"`
	ACL iscsi.ACL `json:"acl"`
}

type IscsiACLDeletePayload struct {
	IQN          string `json:"iqn"`
	InitiatorIQN string `json:"initiatorIqn"`
}

type IscsiSaveConfigPayload struct{}

// ---------- NVMe-oF payloads ----------

type NvmeofSubsystemCreatePayload struct {
	Subsystem nvmeof.Subsystem `json:"subsystem"`
}

type NvmeofSubsystemDestroyPayload struct {
	NQN string `json:"nqn"`
}

type NvmeofNamespaceAddPayload struct {
	NQN       string           `json:"nqn"`
	Namespace nvmeof.Namespace `json:"namespace"`
}

type NvmeofNamespaceRemovePayload struct {
	NQN  string `json:"nqn"`
	NSID int    `json:"nsid"`
}

type NvmeofHostAllowPayload struct {
	NQN     string `json:"nqn"`
	HostNQN string `json:"hostNqn"`
}

type NvmeofHostDisallowPayload struct {
	NQN     string `json:"nqn"`
	HostNQN string `json:"hostNqn"`
}

type NvmeofPortCreatePayload struct {
	Port nvmeof.Port `json:"port"`
}

type NvmeofPortDeletePayload struct {
	ID int `json:"id"`
}

type NvmeofPortLinkPayload struct {
	PortID int    `json:"portId"`
	NQN    string `json:"nqn"`
}

type NvmeofPortUnlinkPayload struct {
	PortID int    `json:"portId"`
	NQN    string `json:"nqn"`
}

type NvmeofSetHostDHChapPayload struct {
	HostNQN string              `json:"hostNqn"`
	Config  nvmeof.DHChapConfig `json:"config"`
}

type NvmeofClearHostDHChapPayload struct {
	HostNQN string `json:"hostNqn"`
}

// NvmeofSaveConfigPayload carries the path to write the JSON snapshot to.
// Empty Path means "use the binary's default" (the worker resolves this
// against the configured /etc/nova-nas/nvmet-config.json path).
type NvmeofSaveConfigPayload struct {
	Path string `json:"path,omitempty"`
}

// PoolCreatePayload carries the full pool spec to the worker. Name is
// duplicated at the top level so the dispatcher can use it as a
// concurrency-key and for log/audit fields without unmarshalling Spec.
type PoolCreatePayload struct {
	Name string          `json:"name"`
	Spec pool.CreateSpec `json:"spec"`
}

type PoolDestroyPayload struct {
	Name string `json:"name"`
}

type PoolScrubPayload struct {
	Name   string           `json:"name"`
	Action pool.ScrubAction `json:"action"`
}

type DatasetCreatePayload struct {
	Spec dataset.CreateSpec `json:"spec"`
}

type DatasetSetPayload struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

type DatasetDestroyPayload struct {
	Name      string `json:"name"`
	Recursive bool   `json:"recursive"`
}

type SnapshotCreatePayload struct {
	Dataset   string `json:"dataset"`
	ShortName string `json:"shortName"`
	Recursive bool   `json:"recursive"`
}

type SnapshotDestroyPayload struct {
	Name string `json:"name"`
}

type SnapshotRollbackPayload struct {
	Snapshot string `json:"snapshot"`
}

type PoolReplacePayload struct {
	Name    string `json:"name"`
	OldDisk string `json:"oldDisk"`
	NewDisk string `json:"newDisk"`
}

type PoolOfflinePayload struct {
	Name      string `json:"name"`
	Disk      string `json:"disk"`
	Temporary bool   `json:"temporary"`
}

type PoolOnlinePayload struct {
	Name string `json:"name"`
	Disk string `json:"disk"`
}

type PoolClearPayload struct {
	Name string `json:"name"`
	Disk string `json:"disk"` // optional
}

type PoolAttachPayload struct {
	Name     string `json:"name"`
	Existing string `json:"existing"`
	NewDisk  string `json:"newDisk"`
}

type PoolDetachPayload struct {
	Name string `json:"name"`
	Disk string `json:"disk"`
}

type PoolAddPayload struct {
	Name string       `json:"name"`
	Spec pool.AddSpec `json:"spec"`
}

type PoolExportPayload struct {
	Name  string `json:"name"`
	Force bool   `json:"force"`
}

type PoolImportPayload struct {
	Name string `json:"name"`
}

type DatasetRenamePayload struct {
	OldName   string `json:"oldName"`
	NewName   string `json:"newName"`
	Recursive bool   `json:"recursive"`
}

type DatasetClonePayload struct {
	Snapshot   string            `json:"snapshot"`
	Target     string            `json:"target"`
	Properties map[string]string `json:"properties,omitempty"`
}

type DatasetPromotePayload struct {
	Name string `json:"name"`
}

type DatasetLoadKeyPayload struct {
	Name        string `json:"name"`
	Keylocation string `json:"keylocation,omitempty"`
	Recursive   bool   `json:"recursive"`
}

type DatasetUnloadKeyPayload struct {
	Name      string `json:"name"`
	Recursive bool   `json:"recursive"`
}

type DatasetChangeKeyPayload struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

type PoolTrimPayload struct {
	Name   string `json:"name"`
	Action string `json:"action"` // "start" | "stop"
	Disk   string `json:"disk,omitempty"`
}

type PoolSetPropsPayload struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

type DatasetBookmarkPayload struct {
	Snapshot string `json:"snapshot"`
	Bookmark string `json:"bookmark"`
}

type DatasetDestroyBookmarkPayload struct {
	Bookmark string `json:"bookmark"`
}

type SnapshotHoldPayload struct {
	Snapshot  string `json:"snapshot"`
	Tag       string `json:"tag"`
	Recursive bool   `json:"recursive"`
}

type SnapshotReleasePayload struct {
	Snapshot  string `json:"snapshot"`
	Tag       string `json:"tag"`
	Recursive bool   `json:"recursive"`
}

type PoolCheckpointPayload struct {
	Name string `json:"name"`
}

type PoolDiscardCheckpointPayload struct {
	Name string `json:"name"`
}

type PoolUpgradePayload struct {
	Name string `json:"name"`
	All  bool   `json:"all"`
}

type PoolReguidPayload struct {
	Name string `json:"name"`
}

// ---------- NFS payloads ----------

type NfsExportCreatePayload struct {
	Export nfs.Export `json:"export"`
}

type NfsExportUpdatePayload struct {
	Export nfs.Export `json:"export"`
}

type NfsExportDeletePayload struct {
	Name string `json:"name"`
}

type NfsReloadPayload struct{}

// ---------- Kerberos payloads ----------

type Krb5SetConfigPayload struct {
	Config krb5.Config `json:"config"`
}

type Krb5SetIdmapdPayload struct {
	Config krb5.IdmapdConfig `json:"config"`
}

// Krb5UploadKeytabPayload carries the raw keytab bytes. encoding/json
// emits []byte as base64, so the on-the-wire shape is `{"data":"<base64>"}`.
type Krb5UploadKeytabPayload struct {
	Data []byte `json:"data"`
}

type Krb5DeleteKeytabPayload struct{}

// ---------- Samba payloads ----------

type SambaShareCreatePayload struct {
	Share samba.Share `json:"share"`
}

type SambaShareUpdatePayload struct {
	Share samba.Share `json:"share"`
}

type SambaShareDeletePayload struct {
	Name string `json:"name"`
}

type SambaReloadPayload struct{}

type SambaUserAddPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type SambaUserDeletePayload struct {
	Username string `json:"username"`
}

type SambaUserSetPasswordPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ---------- SMART payloads ----------

type SmartRunSelfTestPayload struct {
	DevicePath string `json:"devicePath"`
	TestType   string `json:"testType"`
}

type SmartEnablePayload struct {
	DevicePath string `json:"devicePath"`
}

// ---------- Scheduler payloads ----------

// SchedSnapshotFirePayload identifies a single snapshot schedule to fire.
// The handler re-fetches by ID so the latest config is used.
type SchedSnapshotFirePayload struct {
	ScheduleID uuid.UUID `json:"scheduleId"`
}

// SchedReplicationFirePayload identifies a single replication schedule.
type SchedReplicationFirePayload struct {
	ScheduleID uuid.UUID `json:"scheduleId"`
}

// ---------- Network payloads ----------

type NetworkInterfaceApplyPayload struct {
	Config network.InterfaceConfig `json:"config"`
}

type NetworkInterfaceDeletePayload struct {
	Name   string `json:"name"`
	DryRun bool   `json:"dryRun,omitempty"`
}

type NetworkVLANApplyPayload struct {
	VLAN network.VLAN `json:"vlan"`
}

type NetworkBondApplyPayload struct {
	Bond network.Bond `json:"bond"`
}

type NetworkReloadPayload struct{}

// ---------- System payloads ----------

type SystemSetHostnamePayload struct {
	Hostname string `json:"hostname"`
}

type SystemSetTimezonePayload struct {
	Timezone string `json:"timezone"`
}

type SystemSetNTPPayload struct {
	Enabled bool     `json:"enabled"`
	Servers []string `json:"servers,omitempty"`
}

type SystemRebootPayload struct {
	DelaySeconds int `json:"delaySeconds"`
}

type SystemShutdownPayload struct {
	DelaySeconds int `json:"delaySeconds"`
}

type SystemCancelShutdownPayload struct{}

// ---------- ProtocolShare + ACL payloads ----------

type ProtocolShareCreatePayload struct {
	Share protocolshare.ProtocolShare `json:"share"`
}

type ProtocolShareUpdatePayload struct {
	Share protocolshare.ProtocolShare `json:"share"`
}

// ProtocolShareDeletePayload identifies the share to remove. When Pool
// and DatasetName are both set the worker performs a full teardown
// (samba + nfs + dataset destroy) via Manager.DeleteShare; otherwise it
// performs the lighter Manager.Delete which only tears down the nfs +
// samba surfaces.
type ProtocolShareDeletePayload struct {
	Name        string `json:"name"`
	Pool        string `json:"pool,omitempty"`
	DatasetName string `json:"datasetName,omitempty"`
}

type DatasetSetACLPayload struct {
	Path string        `json:"path"`
	ACEs []dataset.ACE `json:"aces"`
}

type DatasetAppendACEPayload struct {
	Path string      `json:"path"`
	ACE  dataset.ACE `json:"ace"`
}

type DatasetRemoveACEPayload struct {
	Path  string `json:"path"`
	Index int    `json:"index"`
}

type SambaSetGlobalsPayload struct {
	Opts samba.GlobalsOpts `json:"opts"`
}
