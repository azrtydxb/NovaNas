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
	KindDatasetCreate    Kind = "dataset.create"
	KindDatasetSet       Kind = "dataset.set"
	KindDatasetDestroy   Kind = "dataset.destroy"
	KindSnapshotCreate   Kind = "snapshot.create"
	KindSnapshotDestroy  Kind = "snapshot.destroy"
	KindSnapshotRollback Kind = "snapshot.rollback"
)

type PoolCreatePayload struct {
	Name string         `json:"name"`
	Spec pool.CreateSpec `json:"spec"`
}

type PoolDestroyPayload struct {
	Name string `json:"name"`
}

type PoolScrubPayload struct {
	Name   string            `json:"name"`
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
