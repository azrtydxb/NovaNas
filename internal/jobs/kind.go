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
	KindSnapshotCreate   Kind = "snapshot.create"
	KindSnapshotDestroy  Kind = "snapshot.destroy"
	KindSnapshotRollback Kind = "snapshot.rollback"
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
