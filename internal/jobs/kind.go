// Package jobs defines async task types and the dispatch/worker plumbing.
package jobs

import (
	"github.com/novanas/nova-nas/internal/host/iscsi"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
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
