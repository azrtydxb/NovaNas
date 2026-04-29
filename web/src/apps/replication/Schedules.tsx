import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import {
  replication,
  type ReplicationSchedule,
  type SnapshotSchedule,
} from "../../api/replication";
import { Modal } from "./Modal";

export function Schedules() {
  const qc = useQueryClient();
  const snapQ = useQuery({
    queryKey: ["snapshot-schedules"],
    queryFn: () => replication.listSnapshotSchedules(),
  });
  const replQ = useQuery({
    queryKey: ["replication-schedules"],
    queryFn: () => replication.listReplicationSchedules(),
  });

  const snaps = snapQ.data ?? [];
  const repls = replQ.data ?? [];

  const [editSnap, setEditSnap] = useState<SnapshotSchedule | "new" | null>(null);
  const [editRepl, setEditRepl] = useState<ReplicationSchedule | "new" | null>(null);

  const invalSnap = () => qc.invalidateQueries({ queryKey: ["snapshot-schedules"] });
  const invalRepl = () => qc.invalidateQueries({ queryKey: ["replication-schedules"] });

  const delSnap = useMutation({
    mutationFn: (id: string) => replication.deleteSnapshotSchedule(id),
    onSuccess: invalSnap,
  });
  const delRepl = useMutation({
    mutationFn: (id: string) => replication.deleteReplicationSchedule(id),
    onSuccess: invalRepl,
  });
  const togSnap = useMutation({
    mutationFn: (s: SnapshotSchedule) =>
      replication.updateSnapshotSchedule(s.id, { ...s, enabled: !s.enabled }),
    onSuccess: invalSnap,
  });
  const togRepl = useMutation({
    mutationFn: (s: ReplicationSchedule) =>
      replication.updateReplicationSchedule(s.id, { ...s, enabled: !s.enabled }),
    onSuccess: invalRepl,
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="sect">
        <div className="sect__head row gap-8" style={{ alignItems: "center" }}>
          <div className="sect__title">Snapshot schedules</div>
          <button
            className="btn btn--sm btn--primary"
            style={{ marginLeft: "auto" }}
            onClick={() => setEditSnap("new")}
          >
            <Icon name="plus" size={10} />
            New
          </button>
        </div>
        <div className="sect__body">
          {snapQ.isLoading && <div className="muted">Loading…</div>}
          {snapQ.data && snaps.length === 0 && <div className="muted">No snapshot schedules.</div>}
          {snaps.length > 0 && (
            <table className="tbl">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Datasets</th>
                  <th>Cron</th>
                  <th className="num">Keep</th>
                  <th>Enabled</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {snaps.map((s) => (
                  <tr key={s.id}>
                    <td>{s.name ?? s.id}</td>
                    <td className="muted mono" style={{ fontSize: 11 }}>
                      {(s.datasets ?? []).join(", ")}
                    </td>
                    <td className="mono">{s.cron ?? "—"}</td>
                    <td className="num mono">{s.keep ?? "—"}</td>
                    <td>
                      <button
                        className="btn btn--sm"
                        onClick={() => togSnap.mutate(s)}
                      >
                        {s.enabled ? "on" : "off"}
                      </button>
                    </td>
                    <td className="num">
                      <button className="btn btn--sm" onClick={() => setEditSnap(s)}>Edit</button>{" "}
                      <button
                        className="btn btn--sm btn--danger"
                        onClick={() => {
                          if (window.confirm(`Delete schedule ${s.name ?? s.id}?`)) delSnap.mutate(s.id);
                        }}
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <div className="sect">
        <div className="sect__head row gap-8" style={{ alignItems: "center" }}>
          <div className="sect__title">Replication schedules</div>
          <button
            className="btn btn--sm btn--primary"
            style={{ marginLeft: "auto" }}
            onClick={() => setEditRepl("new")}
          >
            <Icon name="plus" size={10} />
            New
          </button>
        </div>
        <div className="sect__body">
          {replQ.isLoading && <div className="muted">Loading…</div>}
          {replQ.data && repls.length === 0 && <div className="muted">No replication schedules.</div>}
          {repls.length > 0 && (
            <table className="tbl">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Jobs</th>
                  <th>Cron</th>
                  <th>Enabled</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {repls.map((s) => (
                  <tr key={s.id}>
                    <td>{s.name ?? s.id}</td>
                    <td className="muted mono" style={{ fontSize: 11 }}>
                      {(s.jobIds ?? []).join(", ")}
                    </td>
                    <td className="mono">{s.cron ?? "—"}</td>
                    <td>
                      <button className="btn btn--sm" onClick={() => togRepl.mutate(s)}>
                        {s.enabled ? "on" : "off"}
                      </button>
                    </td>
                    <td className="num">
                      <button className="btn btn--sm" onClick={() => setEditRepl(s)}>Edit</button>{" "}
                      <button
                        className="btn btn--sm btn--danger"
                        onClick={() => {
                          if (window.confirm(`Delete schedule ${s.name ?? s.id}?`)) delRepl.mutate(s.id);
                        }}
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {editSnap && (
        <SnapshotScheduleModal
          init={editSnap === "new" ? null : editSnap}
          onClose={() => setEditSnap(null)}
          onDone={invalSnap}
        />
      )}
      {editRepl && (
        <ReplicationScheduleModal
          init={editRepl === "new" ? null : editRepl}
          onClose={() => setEditRepl(null)}
          onDone={invalRepl}
        />
      )}
    </div>
  );
}

function SnapshotScheduleModal({
  init,
  onClose,
  onDone,
}: {
  init: SnapshotSchedule | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(init?.name ?? "");
  const [datasets, setDatasets] = useState((init?.datasets ?? []).join(", "));
  const [cron, setCron] = useState(init?.cron ?? "0 * * * *");
  const [keep, setKeep] = useState<number | "">(init?.keep ?? 24);
  const [enabled, setEnabled] = useState(init?.enabled ?? true);
  const [err, setErr] = useState<string | null>(null);

  const body = (): Partial<SnapshotSchedule> => ({
    name,
    datasets: datasets.split(",").map((s) => s.trim()).filter(Boolean),
    cron,
    keep: keep === "" ? undefined : Number(keep),
    enabled,
  });

  const m = useMutation({
    mutationFn: () => init
      ? replication.updateSnapshotSchedule(init.id, body())
      : replication.createSnapshotSchedule(body()),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal title={init ? `Edit snapshot schedule · ${init.name ?? init.id}` : "New snapshot schedule"} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !name}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Name</label>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="field">
        <label className="field__label">Datasets (comma-separated)</label>
        <input className="input" value={datasets} onChange={(e) => setDatasets(e.target.value)} placeholder="tank/data, tank/home" />
      </div>
      <div className="field">
        <label className="field__label">Cron</label>
        <input className="input" value={cron} onChange={(e) => setCron(e.target.value)} placeholder="0 * * * *" />
      </div>
      <div className="field">
        <label className="field__label">Keep (count)</label>
        <input
          className="input"
          type="number"
          value={keep}
          onChange={(e) => setKeep(e.target.value === "" ? "" : Number(e.target.value))}
        />
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          Enabled
        </label>
      </div>
    </Modal>
  );
}

function ReplicationScheduleModal({
  init,
  onClose,
  onDone,
}: {
  init: ReplicationSchedule | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(init?.name ?? "");
  const [jobs, setJobs] = useState((init?.jobIds ?? []).join(", "));
  const [cron, setCron] = useState(init?.cron ?? "0 2 * * *");
  const [enabled, setEnabled] = useState(init?.enabled ?? true);
  const [err, setErr] = useState<string | null>(null);

  const body = (): Partial<ReplicationSchedule> => ({
    name,
    jobIds: jobs.split(",").map((s) => s.trim()).filter(Boolean),
    cron,
    enabled,
  });

  const m = useMutation({
    mutationFn: () => init
      ? replication.updateReplicationSchedule(init.id, body())
      : replication.createReplicationSchedule(body()),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal title={init ? `Edit replication schedule · ${init.name ?? init.id}` : "New replication schedule"} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !name}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Name</label>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="field">
        <label className="field__label">Job IDs (comma-separated)</label>
        <input className="input" value={jobs} onChange={(e) => setJobs(e.target.value)} placeholder="job-1, job-2" />
      </div>
      <div className="field">
        <label className="field__label">Cron</label>
        <input className="input" value={cron} onChange={(e) => setCron(e.target.value)} placeholder="0 2 * * *" />
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          Enabled
        </label>
      </div>
    </Modal>
  );
}

export default Schedules;
