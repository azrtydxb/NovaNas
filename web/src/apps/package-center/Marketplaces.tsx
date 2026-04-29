import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { marketplaces, type Marketplace } from "../../api/plugins";
import { api } from "../../api/client";
import { pemFingerprint } from "../../lib/format";
import { Icon } from "../../components/Icon";
import { AddMarketplace } from "./AddMarketplace";

type MarketplaceRow = Marketplace & { _fingerprint?: string };

export function Marketplaces() {
  const qc = useQueryClient();
  const [adding, setAdding] = useState(false);
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

  const remove = useMutation({
    mutationFn: (id: string) => api(`/api/v1/marketplaces/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["marketplaces"] }),
  });
  const refresh = useMutation({
    mutationFn: (id: string) =>
      api(`/api/v1/marketplaces/${id}/refresh-trust-key`, { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["marketplaces"] }),
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
        <button className="btn btn--primary" onClick={() => setAdding(true)}>
          <Icon name="plus" size={11} /> Add marketplace
        </button>
        <span className="muted">
          {q.data?.length ?? 0} configured · {q.data?.filter((m) => m.enabled).length ?? 0} enabled
        </span>
      </div>
      <table className="tbl">
        <thead>
          <tr>
            <th>Name</th>
            <th>URL</th>
            <th>Trust key</th>
            <th>Added</th>
            <th>Status</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {(q.data ?? []).map((m) => (
            <tr key={m.id}>
              <td>
                <div className="row gap-8">
                  {m.locked && <Icon name="lock" size={11} style={{ color: "var(--accent)" }} />}
                  {m.name}
                  {m.locked && <span className="trust-badge trust-badge--official">locked</span>}
                </div>
              </td>
              <td className="mono muted small" style={{ maxWidth: 320, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {m.indexUrl}
              </td>
              <td className="mono muted small" title={m._fingerprint ?? ""}>
                {m._fingerprint ? truncate(m._fingerprint, 24) : <span className="muted">—</span>}
              </td>
              <td className="muted">
                {m.addedAt ? new Date(m.addedAt).toLocaleDateString() : "—"}
              </td>
              <td>
                <span className={`pill pill--${m.enabled ? "ok" : "warn"}`}>
                  <span className="dot" />
                  {m.enabled ? "enabled" : "disabled"}
                </span>
              </td>
              <td>
                <div className="row gap-8">
                  <button
                    className="btn btn--sm"
                    title="Refresh trust key"
                    onClick={() => refresh.mutate(m.id)}
                    disabled={refresh.isPending}
                  >
                    <Icon name="refresh" size={11} />
                  </button>
                  {!m.locked && (
                    <button
                      className="btn btn--sm btn--danger"
                      title="Remove"
                      onClick={() => {
                        if (window.confirm(`Remove marketplace ${m.name}?`)) {
                          remove.mutate(m.id);
                        }
                      }}
                      disabled={remove.isPending}
                    >
                      <Icon name="trash" size={11} />
                    </button>
                  )}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {adding && <AddMarketplace onClose={() => setAdding(false)} />}
    </div>
  );
}

function truncate(s: string, max: number): string {
  if (s.length <= max) return s;
  return s.slice(0, max - 1) + "…";
}
