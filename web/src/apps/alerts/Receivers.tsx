import { useQuery } from "@tanstack/react-query";
import { alerts } from "../../api/observability";
import { Icon } from "../../components/Icon";

export default function Receivers() {
  const q = useQuery({
    queryKey: ["alerts", "receivers"],
    queryFn: () => alerts.listReceivers(),
    refetchInterval: 60_000,
  });
  const list = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary"><Icon name="plus" size={11} />Add receiver</button>
      </div>
      <table className="tbl">
        <thead><tr><th>Receiver</th><th>Integrations</th></tr></thead>
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
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {q.isLoading && <div className="muted" style={{ padding: 8 }}>Loading receivers…</div>}
      {q.isError && (
        <div className="muted" style={{ padding: 8, color: "var(--err)" }}>
          Failed to load: {(q.error as Error).message}
        </div>
      )}
      {q.data && list.length === 0 && (
        <div className="muted" style={{ padding: 20 }}>No receivers configured.</div>
      )}
    </div>
  );
}
