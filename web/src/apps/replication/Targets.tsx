import { useQuery } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { replication } from "../../api/replication";

export function Targets() {
  const q = useQuery({
    queryKey: ["replication-targets"],
    queryFn: () => replication.listTargets(),
  });
  const targets = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          {/* TODO: phase 3 — open add-target dialog */}
          <Icon name="plus" size={11} />
          Add target
        </button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && targets.length === 0 && (
        <div className="empty-hint">No targets.</div>
      )}
      {targets.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Name</th>
              <th>Protocol</th>
              <th>Host</th>
              <th>Details</th>
            </tr>
          </thead>
          <tbody>
            {targets.map((t) => (
              <tr key={t.id}>
                <td>{t.name ?? t.id}</td>
                <td>
                  <span className="pill pill--info">{t.protocol ?? "—"}</span>
                </td>
                <td className="mono">{t.host ?? "—"}</td>
                <td className="muted mono" style={{ fontSize: 11 }}>
                  {t.protocol === "s3"
                    ? `region=${t.region ?? "—"}`
                    : `user=${t.ssh_user ?? "—"}, port=${t.port ?? "—"}`}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

export default Targets;
