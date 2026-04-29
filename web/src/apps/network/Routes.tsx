import { useQuery } from "@tanstack/react-query";
import { network } from "../../api/network";

type RouteRow = {
  dst: string;
  gateway?: string;
  dev: string;
  metric?: number;
};

export function Routes() {
  const ifaces = useQuery({
    queryKey: ["network", "interfaces"],
    queryFn: () => network.listInterfaces(),
  });

  const rows: RouteRow[] = [];
  for (const i of ifaces.data ?? []) {
    const r = (i as unknown as { routes?: unknown[] }).routes;
    if (!Array.isArray(r)) continue;
    for (const rt of r) {
      const o = rt as Record<string, unknown>;
      rows.push({
        dst: String(o.dst ?? o.destination ?? "default"),
        gateway: o.gateway as string | undefined,
        dev: String(o.dev ?? i.name ?? "—"),
        metric: o.metric as number | undefined,
      });
    }
  }

  return (
    <div style={{ padding: 14 }}>
      <table className="tbl">
        <thead>
          <tr>
            <th>Destination</th>
            <th>Gateway</th>
            <th>Interface</th>
            <th className="num">Metric</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td className="mono">{r.dst}</td>
              <td className="mono">{r.gateway ?? <span className="muted">link</span>}</td>
              <td className="mono">{r.dev}</td>
              <td className="num mono">{r.metric ?? "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {ifaces.isLoading && <div className="muted" style={{ padding: 8 }}>Loading routes…</div>}
      {ifaces.isError && (
        <div className="muted" style={{ padding: 8, color: "var(--err)" }}>
          Failed to load: {(ifaces.error as Error).message}
        </div>
      )}
      {ifaces.data && rows.length === 0 && (
        <div className="muted" style={{ padding: 20 }}>
          No route table is exposed by the backend yet.
        </div>
      )}
    </div>
  );
}

export default Routes;
