// Package jobs defines async task types and the dispatch/worker plumbing.
package jobs

import (
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
)

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
