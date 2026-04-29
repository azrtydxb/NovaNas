import { useQuery } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { replication } from "../../api/replication";

export function Scrub() {
  const q = useQuery({
    queryKey: ["scrub-policies"],
    queryFn: () => replication.listScrubPolicies(),
  });
  const policies = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          {/* TODO: phase 3 — open create-policy dialog */}
          <Icon name="plus" size={11} />
          New policy
        </button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && policies.length === 0 && (
        <div className="empty-hint">No scrub policies.</div>
      )}
      {policies.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Name</th>
              <th>Pools</th>
              <th>Cron</th>
              <th>Priority</th>
              <th>Type</th>
            </tr>
          </thead>
          <tbody>
            {policies.map((p) => (
              <tr key={p.id}>
                <td>{p.name ?? p.id}</td>
                <td className="mono muted" style={{ fontSize: 11 }}>
                  {(p.pools ?? []).join(", ")}
                </td>
                <td className="mono">{p.cron ?? "—"}</td>
                <td>
                  <span
                    className={`pill pill--${
                      p.priority === "high"
                        ? "warn"
                        : p.priority === "low"
                          ? ""
                          : "info"
                    }`}
                  >
                    {p.priority ?? "—"}
                  </span>
                </td>
                <td className="muted">{p.builtin ? "built-in" : "custom"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

export default Scrub;
