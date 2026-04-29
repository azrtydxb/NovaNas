import { useQuery, useQueryClient } from "@tanstack/react-query";
import { alerts } from "../../api/observability";
import { Icon } from "../../components/Icon";

export default function Receivers() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["alerts", "receivers"],
    queryFn: () => alerts.listReceivers(),
    refetchInterval: 60_000,
  });
  const list = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <span className="muted" style={{ fontSize: 11 }}>
          Receivers are configured in Alertmanager. This view is read-only.
        </span>
        <button
          className="btn btn--sm"
          style={{ marginLeft: "auto" }}
          onClick={() => qc.invalidateQueries({ queryKey: ["alerts", "receivers"] })}
        >
          <Icon name="refresh" size={11} /> Refresh
        </button>
      </div>

      {q.isLoading && <div className="muted">Loading receivers…</div>}
      {q.isError && (
        <div className="muted" style={{ color: "var(--err)" }}>
          Failed to load: {(q.error as Error).message}
        </div>
      )}
      {q.data && list.length === 0 && (
        <div className="muted" style={{ padding: "20px 0" }}>
          No receivers configured.
        </div>
      )}

      {list.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Receiver</th>
              <th>Integrations</th>
            </tr>
          </thead>
          <tbody>
            {list.map((r) => (
              <tr key={r.name}>
                <td>{r.name}</td>
                <td>
                  <div className="chip-row">
                    {(r.integrations ?? []).map((i, idx) => (
                      <span key={idx} className="chip chip--accent">
                        {i.type ?? i.name ?? "integration"}
                      </span>
                    ))}
                    {(r.integrations ?? []).length === 0 && (
                      <span className="muted">—</span>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
