import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { replication, type ReplicationJob } from "../../api/replication";
import { formatBytes } from "../../lib/format";

function statePill(state: string | undefined): string {
  if (!state) return "";
  const s = state.toUpperCase();
  if (s === "OK" || s === "SUCCESS" || s === "DONE") return "ok";
  if (s === "RUNNING" || s === "ACTIVE") return "info";
  if (s === "FAILED" || s === "ERROR") return "err";
  return "";
}

export function Jobs() {
  const [sel, setSel] = useState<string | null>(null);
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["replication-jobs"],
    queryFn: () => replication.listJobs(),
  });
  const jobs = q.data ?? [];
  const cur = jobs.find((j) => j.id === sel);

  const runMut = useMutation({
    mutationFn: (id: string) => replication.runJob(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["replication-jobs"] }),
  });

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: cur ? "1fr 320px" : "1fr",
        height: "100%",
      }}
    >
      <div style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <button className="btn btn--primary">
            {/* TODO: phase 3 — open create-job dialog */}
            <Icon name="plus" size={11} />
            New job
          </button>
        </div>
        {q.isLoading && <div className="empty-hint">Loading jobs…</div>}
        {q.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(q.error as Error).message}
          </div>
        )}
        {q.data && jobs.length === 0 && (
          <div className="empty-hint">No replication jobs.</div>
        )}
        {jobs.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>Job</th>
                <th>Source</th>
                <th>Target</th>
                <th>Schedule</th>
                <th>State</th>
                <th>Last run</th>
                <th className="num">Bytes</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((j) => (
                <tr
                  key={j.id}
                  className={sel === j.id ? "is-on" : ""}
                  onClick={() => setSel(j.id)}
                  style={{ cursor: "pointer" }}
                >
                  <td>{j.name ?? j.id}</td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {j.source ?? "—"}
                  </td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {j.target ?? "—"}
                  </td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {j.schedule ?? "—"}
                  </td>
                  <td>
                    <span className={`pill pill--${statePill(j.state)}`}>
                      <span className="dot" />
                      {j.state ?? "—"}
                    </span>
                  </td>
                  <td className="muted">{j.lastRun ?? "—"}</td>
                  <td className="num mono">
                    {j.lastBytes ? formatBytes(j.lastBytes) : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {cur && <JobDetail job={cur} runPending={runMut.isPending} onRun={(id) => runMut.mutate(id)} />}
    </div>
  );
}

function JobDetail({
  job,
  runPending,
  onRun,
}: {
  job: ReplicationJob;
  runPending: boolean;
  onRun: (id: string) => void;
}) {
  const runsQ = useQuery({
    queryKey: ["replication-runs", job.id],
    queryFn: () => replication.listRuns(job.id),
  });
  return (
    <div className="side-detail">
      <div className="side-detail__head">
        <div>
          <div className="muted mono" style={{ fontSize: 10 }}>
            JOB · {job.id}
          </div>
          <div className="side-detail__title">{job.name ?? job.id}</div>
        </div>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Status</div>
        </div>
        <div className="sect__body">
          <span className={`pill pill--${statePill(job.state)}`}>
            <span className="dot" />
            {job.state ?? "—"}
          </span>
          {job.error && (
            <div
              className="muted"
              style={{ color: "var(--err)", fontSize: 11, marginTop: 8 }}
            >
              {job.error}
            </div>
          )}
        </div>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Configuration</div>
        </div>
        <div className="sect__body">
          <dl className="kv">
            <dt>Source</dt>
            <dd>{job.source ?? "—"}</dd>
            <dt>Target</dt>
            <dd>{job.target ?? "—"}</dd>
            <dt>Schedule</dt>
            <dd>{job.schedule ?? "—"}</dd>
            <dt>Last bytes</dt>
            <dd>{job.lastBytes ? formatBytes(job.lastBytes) : "—"}</dd>
            <dt>Last duration</dt>
            <dd>{job.lastDur ?? job.lastDuration ?? "—"}</dd>
          </dl>
        </div>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Runs</div>
        </div>
        <div className="sect__body">
          {runsQ.isLoading && <div className="muted" style={{ fontSize: 11 }}>Loading…</div>}
          {runsQ.data && runsQ.data.length === 0 && (
            <div className="muted" style={{ fontSize: 11 }}>No runs yet.</div>
          )}
          {runsQ.data && runsQ.data.length > 0 && (
            <table className="tbl tbl--compact">
              <thead>
                <tr>
                  <th>Started</th>
                  <th>State</th>
                  <th className="num">Bytes</th>
                </tr>
              </thead>
              <tbody>
                {runsQ.data.slice(0, 10).map((r) => (
                  <tr key={r.id}>
                    <td className="muted">{r.startedAt ?? "—"}</td>
                    <td>
                      <span className={`pill pill--${statePill(r.state)}`}>
                        <span className="dot" />
                        {r.state ?? "—"}
                      </span>
                    </td>
                    <td className="num mono">
                      {r.bytes ? formatBytes(r.bytes) : "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      <div
        className="row gap-8"
        style={{ padding: "10px 12px", borderTop: "1px solid var(--line)" }}
      >
        <button
          className="btn btn--sm btn--primary"
          disabled={runPending}
          onClick={() => onRun(job.id)}
        >
          Run now
        </button>
        {/* TODO: phase 3 — wire edit/delete */}
        <button className="btn btn--sm">Edit</button>
        <button
          className="btn btn--sm btn--danger"
          style={{ marginLeft: "auto" }}
        >
          Delete
        </button>
      </div>
    </div>
  );
}

export default Jobs;
