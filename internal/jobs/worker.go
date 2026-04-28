package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
	"github.com/novanas/nova-nas/internal/host/zfs/pool"
	"github.com/novanas/nova-nas/internal/host/zfs/snapshot"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

type WorkerDeps struct {
	Logger    *slog.Logger
	Queries   *storedb.Queries
	Redis     *redis.Client
	Pools     *pool.Manager
	Datasets  *dataset.Manager
	Snapshots *snapshot.Manager
}

func NewServeMux(d WorkerDeps) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(string(KindPoolCreate), d.handlePoolCreate)
	mux.HandleFunc(string(KindPoolDestroy), d.handlePoolDestroy)
	mux.HandleFunc(string(KindPoolScrub), d.handlePoolScrub)
	mux.HandleFunc(string(KindPoolReplace), d.handlePoolReplace)
	mux.HandleFunc(string(KindPoolOffline), d.handlePoolOffline)
	mux.HandleFunc(string(KindPoolOnline), d.handlePoolOnline)
	mux.HandleFunc(string(KindPoolClear), d.handlePoolClear)
	mux.HandleFunc(string(KindPoolAttach), d.handlePoolAttach)
	mux.HandleFunc(string(KindPoolDetach), d.handlePoolDetach)
	mux.HandleFunc(string(KindPoolAdd), d.handlePoolAdd)
	mux.HandleFunc(string(KindPoolExport), d.handlePoolExport)
	mux.HandleFunc(string(KindPoolImport), d.handlePoolImport)
	mux.HandleFunc(string(KindDatasetCreate), d.handleDatasetCreate)
	mux.HandleFunc(string(KindDatasetSet), d.handleDatasetSet)
	mux.HandleFunc(string(KindDatasetDestroy), d.handleDatasetDestroy)
	mux.HandleFunc(string(KindDatasetRename), d.handleDatasetRename)
	mux.HandleFunc(string(KindDatasetClone), d.handleDatasetClone)
	mux.HandleFunc(string(KindDatasetPromote), d.handleDatasetPromote)
	mux.HandleFunc(string(KindDatasetLoadKey), d.handleDatasetLoadKey)
	mux.HandleFunc(string(KindDatasetUnloadKey), d.handleDatasetUnloadKey)
	mux.HandleFunc(string(KindDatasetChangeKey), d.handleDatasetChangeKey)
	mux.HandleFunc(string(KindPoolTrim), d.handlePoolTrim)
	mux.HandleFunc(string(KindPoolSetProps), d.handlePoolSetProps)
	mux.HandleFunc(string(KindSnapshotCreate), d.handleSnapshotCreate)
	mux.HandleFunc(string(KindSnapshotDestroy), d.handleSnapshotDestroy)
	mux.HandleFunc(string(KindSnapshotRollback), d.handleSnapshotRollback)
	mux.HandleFunc(string(KindDatasetBookmark), d.handleDatasetBookmark)
	mux.HandleFunc(string(KindDatasetDestroyBookmark), d.handleDatasetDestroyBookmark)
	mux.HandleFunc(string(KindSnapshotHold), d.handleSnapshotHold)
	mux.HandleFunc(string(KindSnapshotRelease), d.handleSnapshotRelease)
	mux.HandleFunc(string(KindPoolCheckpoint), d.handlePoolCheckpoint)
	mux.HandleFunc(string(KindPoolDiscardCheckpoint), d.handlePoolDiscardCheckpoint)
	mux.HandleFunc(string(KindPoolUpgrade), d.handlePoolUpgrade)
	mux.HandleFunc(string(KindPoolReguid), d.handlePoolReguid)
	return mux
}

func (d WorkerDeps) decode(t *asynq.Task, payload any) (uuid.UUID, error) {
	var body TaskBody
	if err := json.Unmarshal(t.Payload(), &body); err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.Parse(body.JobID)
	if err != nil {
		return uuid.Nil, err
	}
	if err := json.Unmarshal(body.Payload, payload); err != nil {
		return id, err
	}
	return id, nil
}

func (d WorkerDeps) markRunning(ctx context.Context, id uuid.UUID) error {
	return d.Queries.MarkJobRunning(ctx, pgtype.UUID{Bytes: id, Valid: true})
}

func (d WorkerDeps) finish(ctx context.Context, id uuid.UUID, runErr error) {
	state := "succeeded"
	stderr := ""
	stdout := ""
	var exitCode *int32
	var errMsg *string
	if runErr != nil {
		state = "failed"
		s := runErr.Error()
		errMsg = &s
		var he *exec.HostError
		if errors.As(runErr, &he) {
			stderr = he.Stderr
			ec := int32(he.ExitCode)
			exitCode = &ec
		}
	}
	_ = d.Queries.MarkJobFinished(ctx, storedb.MarkJobFinishedParams{
		ID:       pgtype.UUID{Bytes: id, Valid: true},
		State:    state,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Error:    errMsg,
	})
	_ = d.Redis.Publish(ctx, "job:"+id.String()+":update", state).Err()
}

func (d WorkerDeps) handlePoolCreate(ctx context.Context, t *asynq.Task) error {
	var p PoolCreatePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Create(ctx, p.Spec)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolDestroy(ctx context.Context, t *asynq.Task) error {
	var p PoolDestroyPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Destroy(ctx, p.Name)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolScrub(ctx context.Context, t *asynq.Task) error {
	var p PoolScrubPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Scrub(ctx, p.Name, p.Action)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetCreate(ctx context.Context, t *asynq.Task) error {
	var p DatasetCreatePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.Create(ctx, p.Spec)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetSet(ctx context.Context, t *asynq.Task) error {
	var p DatasetSetPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.SetProps(ctx, p.Name, p.Properties)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetDestroy(ctx context.Context, t *asynq.Task) error {
	var p DatasetDestroyPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.Destroy(ctx, p.Name, p.Recursive)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleSnapshotCreate(ctx context.Context, t *asynq.Task) error {
	var p SnapshotCreatePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Snapshots.Create(ctx, p.Dataset, p.ShortName, p.Recursive)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleSnapshotDestroy(ctx context.Context, t *asynq.Task) error {
	var p SnapshotDestroyPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Snapshots.Destroy(ctx, p.Name)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleSnapshotRollback(ctx context.Context, t *asynq.Task) error {
	var p SnapshotRollbackPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Snapshots.Rollback(ctx, p.Snapshot)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolReplace(ctx context.Context, t *asynq.Task) error {
	var p PoolReplacePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Replace(ctx, p.Name, p.OldDisk, p.NewDisk)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolOffline(ctx context.Context, t *asynq.Task) error {
	var p PoolOfflinePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Offline(ctx, p.Name, p.Disk, p.Temporary)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolOnline(ctx context.Context, t *asynq.Task) error {
	var p PoolOnlinePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Online(ctx, p.Name, p.Disk)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolClear(ctx context.Context, t *asynq.Task) error {
	var p PoolClearPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Clear(ctx, p.Name, p.Disk)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolAttach(ctx context.Context, t *asynq.Task) error {
	var p PoolAttachPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Attach(ctx, p.Name, p.Existing, p.NewDisk)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolDetach(ctx context.Context, t *asynq.Task) error {
	var p PoolDetachPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Detach(ctx, p.Name, p.Disk)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolAdd(ctx context.Context, t *asynq.Task) error {
	var p PoolAddPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Add(ctx, p.Name, p.Spec)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolExport(ctx context.Context, t *asynq.Task) error {
	var p PoolExportPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Export(ctx, p.Name, p.Force)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolImport(ctx context.Context, t *asynq.Task) error {
	var p PoolImportPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Import(ctx, p.Name)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetRename(ctx context.Context, t *asynq.Task) error {
	var p DatasetRenamePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.Rename(ctx, p.OldName, p.NewName, p.Recursive)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetClone(ctx context.Context, t *asynq.Task) error {
	var p DatasetClonePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.Clone(ctx, p.Snapshot, p.Target, p.Properties)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetPromote(ctx context.Context, t *asynq.Task) error {
	var p DatasetPromotePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.Promote(ctx, p.Name)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetLoadKey(ctx context.Context, t *asynq.Task) error {
	var p DatasetLoadKeyPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.LoadKey(ctx, p.Name, p.Keylocation, p.Recursive)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetUnloadKey(ctx context.Context, t *asynq.Task) error {
	var p DatasetUnloadKeyPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.UnloadKey(ctx, p.Name, p.Recursive)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetChangeKey(ctx context.Context, t *asynq.Task) error {
	var p DatasetChangeKeyPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.ChangeKey(ctx, p.Name, p.Properties)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolTrim(ctx context.Context, t *asynq.Task) error {
	var p PoolTrimPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	var action pool.TrimAction
	switch p.Action {
	case "start":
		action = pool.TrimStart
	case "stop":
		action = pool.TrimStop
	default:
		err = fmt.Errorf("invalid trim action %q", p.Action)
		d.finish(ctx, id, err)
		return err
	}
	err = d.Pools.Trim(ctx, p.Name, action, p.Disk)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolSetProps(ctx context.Context, t *asynq.Task) error {
	var p PoolSetPropsPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.SetProps(ctx, p.Name, p.Properties)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetBookmark(ctx context.Context, t *asynq.Task) error {
	var p DatasetBookmarkPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.Bookmark(ctx, p.Snapshot, p.Bookmark)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleDatasetDestroyBookmark(ctx context.Context, t *asynq.Task) error {
	var p DatasetDestroyBookmarkPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Datasets.DestroyBookmark(ctx, p.Bookmark)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleSnapshotHold(ctx context.Context, t *asynq.Task) error {
	var p SnapshotHoldPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Snapshots.Hold(ctx, p.Snapshot, p.Tag, p.Recursive)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handleSnapshotRelease(ctx context.Context, t *asynq.Task) error {
	var p SnapshotReleasePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Snapshots.Release(ctx, p.Snapshot, p.Tag, p.Recursive)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolCheckpoint(ctx context.Context, t *asynq.Task) error {
	var p PoolCheckpointPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Checkpoint(ctx, p.Name)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolDiscardCheckpoint(ctx context.Context, t *asynq.Task) error {
	var p PoolDiscardCheckpointPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.DiscardCheckpoint(ctx, p.Name)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolUpgrade(ctx context.Context, t *asynq.Task) error {
	var p PoolUpgradePayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Upgrade(ctx, p.Name, p.All)
	d.finish(ctx, id, err)
	return err
}

func (d WorkerDeps) handlePoolReguid(ctx context.Context, t *asynq.Task) error {
	var p PoolReguidPayload
	id, err := d.decode(t, &p)
	if err != nil {
		return err
	}
	_ = d.markRunning(ctx, id)
	err = d.Pools.Reguid(ctx, p.Name)
	d.finish(ctx, id, err)
	return err
}
