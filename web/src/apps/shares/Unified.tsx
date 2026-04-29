import { useQuery } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares } from "../../api/shares";

export function Unified() {
  const q = useQuery({
    queryKey: ["protocol-shares"],
    queryFn: () => shares.listProtocolShares(),
  });
  const list = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          {/* TODO: phase 3 — open new-share dialog */}
          <Icon name="plus" size={11} />
          New share
        </button>
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
              <tr key={s.name}>
                <td>{s.name}</td>
                <td>
                  <div className="row gap-4">
                    {(s.protocols ?? []).map((p) => (
                      <span key={p} className="pill pill--info">
                        {p}
                      </span>
                    ))}
                  </div>
                </td>
                <td className="mono muted" style={{ fontSize: 11 }}>
                  {s.path ?? "—"}
                </td>
                <td className="mono muted" style={{ fontSize: 11 }}>
                  {s.clients ?? "—"}
                </td>
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
  );
}

export default Unified;
