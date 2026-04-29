import { useQuery } from "@tanstack/react-query";
import { marketplaces } from "../../api/plugins";

export function Marketplaces() {
  const q = useQuery({ queryKey: ["marketplaces"], queryFn: () => marketplaces.list() });

  if (q.isLoading) return <div className="discover__msg">Loading marketplaces…</div>;
  if (q.isError)
    return (
      <div className="discover__msg discover__msg--err">
        Failed to load: {(q.error as Error).message}
      </div>
    );

  return (
    <div className="marketplaces">
      <div className="marketplaces__bar">
        <button className="btn btn--primary">Add marketplace</button>
        <span className="muted">{q.data?.length ?? 0} configured</span>
      </div>
      <table className="tbl">
        <thead>
          <tr>
            <th>Name</th>
            <th>URL</th>
            <th>Trust key</th>
            <th>Added</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {(q.data ?? []).map((m) => (
            <tr key={m.id}>
              <td>
                <div className="row gap-8">
                  {m.name}
                  {m.locked && <span className="trust-badge trust-badge--official">locked</span>}
                </div>
              </td>
              <td className="mono muted">{m.indexUrl}</td>
              <td className="mono muted small">
                {m.trustKeyFingerprint ?? <span className="muted">—</span>}
              </td>
              <td className="muted">{m.addedAt ? new Date(m.addedAt).toLocaleDateString() : "—"}</td>
              <td>
                <span className={`pill pill--${m.enabled ? "ok" : "warn"}`}>
                  <span className="dot" /> {m.enabled ? "enabled" : "disabled"}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
