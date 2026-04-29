import { useQuery } from "@tanstack/react-query";
import { network } from "../../api/network";

export function RDMA() {
  const rdma = useQuery({
    queryKey: ["network", "rdma"],
    queryFn: () => network.listRdma(),
  });

  return (
    <div style={{ padding: 14 }}>
      {rdma.isLoading && <div className="muted">Loading RDMA devices…</div>}
      {rdma.isError && (
        <div className="muted" style={{ color: "var(--err)" }}>
          Failed to load: {(rdma.error as Error).message}
        </div>
      )}
      {rdma.data && rdma.data.length === 0 && (
        <div className="muted">No RDMA devices detected.</div>
      )}
      {rdma.data && rdma.data.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Device</th>
              <th>Port</th>
              <th>State</th>
              <th>Speed</th>
              <th>LID</th>
              <th>GID</th>
            </tr>
          </thead>
          <tbody>
            {rdma.data.map((r, i) => {
              const state = (r.state ?? "").toString().toUpperCase();
              return (
                <tr key={`${r.name}-${i}`}>
                  <td className="mono">{r.name}</td>
                  <td className="mono">{r.port ?? "—"}</td>
                  <td>
                    <span
                      className={`sdot sdot--${state === "ACTIVE" || state === "UP" ? "ok" : "warn"}`}
                    />{" "}
                    {state || "—"}
                  </td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {r.speed ?? "—"}
                  </td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {r.lid ?? "—"}
                  </td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {r.gid ?? "—"}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}

export default RDMA;
