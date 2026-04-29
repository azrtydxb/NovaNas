import { useQuery } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { replication } from "../../api/replication";

export function Schedules() {
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

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          {/* TODO: phase 3 — open create-schedule dialog */}
          <Icon name="plus" size={11} />
          New schedule
        </button>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Snapshot schedules</div>
        </div>
        <div className="sect__body">
          {snapQ.isLoading && <div className="muted">Loading…</div>}
          {snapQ.isError && (
            <div className="muted" style={{ color: "var(--err)" }}>
              Failed: {(snapQ.error as Error).message}
            </div>
          )}
          {snapQ.data && snaps.length === 0 && (
            <div className="muted">No snapshot schedules.</div>
          )}
          {snaps.length > 0 && (
            <table className="tbl">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Datasets</th>
                  <th>Cron</th>
                  <th className="num">Keep</th>
                  <th>Enabled</th>
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
                      {s.enabled ? (
                        <Icon name="check" size={11} />
                      ) : (
                        <span className="muted">off</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Replication schedules</div>
        </div>
        <div className="sect__body">
          {replQ.isLoading && <div className="muted">Loading…</div>}
          {replQ.data && repls.length === 0 && (
            <div className="muted">No replication schedules.</div>
          )}
          {repls.length > 0 && (
            <table className="tbl">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Jobs</th>
                  <th>Cron</th>
                  <th>Enabled</th>
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
                      {s.enabled ? (
                        <Icon name="check" size={11} />
                      ) : (
                        <span className="muted">off</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}

export default Schedules;
