import { useQuery } from "@tanstack/react-query";
import { marketplaces, type Marketplace } from "../../api/plugins";
import { pemFingerprint } from "../../lib/format";

// Backend returns the full PEM but no derived fingerprint. We hash
// client-side. Result is cached on `m._fingerprint` per row.
type MarketplaceRow = Marketplace & { _fingerprint?: string };

export function Marketplaces() {
  const q = useQuery<MarketplaceRow[]>({
    queryKey: ["marketplaces"],
    queryFn: async () => {
      const list = await marketplaces.list();
      return Promise.all(
        list.map(async (m) => ({
          ...m,
          _fingerprint: m.trustKeyPem ? await pemFingerprint(m.trustKeyPem) : undefined,
        }))
      );
    },
  });

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
              <td className="mono muted small">{m.indexUrl}</td>
              <td className="mono muted small" title={m._fingerprint ?? ""}>
                {m._fingerprint ? truncate(m._fingerprint, 24) : <span className="muted">—</span>}
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

function truncate(s: string, max: number): string {
  if (s.length <= max) return s;
  return s.slice(0, max - 1) + "…";
}
