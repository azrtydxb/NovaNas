import { useQuery } from "@tanstack/react-query";
import { alerts } from "../../api/observability";
import { Icon } from "../../components/Icon";

export default function Receivers() {
  const q = useQuery({
    queryKey: ["alerts", "receivers"],
    queryFn: () => alerts.listReceivers(),
  });
  const list = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          <Icon name="plus" size={11} /> Add receiver
        </button>
        <span className="muted" style={{ marginLeft: "auto" }}>
          {list.length} receivers
        </span>
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
                  <div className="row gap-4">
                    {(r.integrations ?? []).map((i, idx) => (
                      <span key={idx} className="pill pill--info">
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
