import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares } from "../../api/shares";

export function Unified() {
  const [sel, setSel] = useState<string | null>(null);
  const q = useQuery({
    queryKey: ["protocol-shares"],
    queryFn: () => shares.listProtocolShares(),
  });
  const list = q.data ?? [];
  const cur = list.find((s) => s.name === sel) ?? null;

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
          <div className="muted small" style={{ marginLeft: "auto" }}>
            Create per-protocol shares from the SMB / NFS / iSCSI / NVMe-oF tabs.
          </div>
        </div>
        {q.isLoading && <div className="empty-hint">Loading shares…</div>}
        {q.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(q.error as Error).message}
          </div>
        )}
        {q.data && list.length === 0 && <div className="empty-hint">No shares.</div>}
        {list.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>Share</th>
                <th>Protocols</th>
                <th>Path</th>
                <th>Clients</th>
                <th>State</th>
              </tr>
            </thead>
            <tbody>
              {list.map((s) => (
                <tr
                  key={s.name}
                  className={sel === s.name ? "is-on" : ""}
                  style={{ cursor: "pointer" }}
                  onClick={() => setSel(s.name)}
                >
                  <td>{s.name}</td>
                  <td>
                    <div className="row gap-8">
                      {(s.protocols ?? []).map((p) => (
                        <span key={p} className="pill pill--info">{p}</span>
                      ))}
                    </div>
                  </td>
                  <td className="mono muted" style={{ fontSize: 11 }}>{s.path ?? "—"}</td>
                  <td className="mono muted" style={{ fontSize: 11 }}>{s.clients ?? "—"}</td>
                  <td>
                    <span className="pill pill--ok">
                      <span className="dot" />
                      {s.state ?? "up"}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {cur && (
        <div className="side-detail">
          <div className="side-detail__head">
            <div>
              <div className="muted mono" style={{ fontSize: 10 }}>SHARE</div>
              <div className="side-detail__title">{cur.name}</div>
            </div>
            <button className="btn btn--sm" onClick={() => setSel(null)}>
              <Icon name="close" size={10} />
            </button>
          </div>
          <div className="sect">
            <div className="sect__title">Detail</div>
            <div className="sect__body">
              <dl className="kv">
                <dt>Path</dt><dd className="mono" style={{ fontSize: 11 }}>{cur.path ?? "—"}</dd>
                <dt>Clients</dt><dd className="mono" style={{ fontSize: 11 }}>{cur.clients ?? "—"}</dd>
                <dt>State</dt><dd>{cur.state ?? "up"}</dd>
                <dt>Protocols</dt><dd>{(cur.protocols ?? []).join(", ")}</dd>
              </dl>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default Unified;
