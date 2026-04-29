import { useQuery } from "@tanstack/react-query";
import { network } from "../../api/network";
import { Icon } from "../../components/Icon";

// Routes are derived from interface metadata: each NetInterface or
// NetConfig carries a `routes` array (gateway, destination, prefix,
// metric, dev). The backend doesn't yet expose a dedicated /routes
// listing, so we flatten across interface configs. Adding /network/routes
// later will be a one-line swap.
type RouteRow = {
  dst: string;
  gateway?: string;
  dev: string;
  metric?: number;
  proto?: string;
  scope?: string;
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
        proto: o.proto as string | undefined,
        scope: o.scope as string | undefined,
      });
    }
  }

  if (ifaces.isLoading) return <div className="empty-hint">Loading routes…</div>;
  if (ifaces.isError)
    return (
      <div className="empty-hint" style={{ color: "var(--err)" }}>
        Failed to load: {(ifaces.error as Error).message}
      </div>
    );
  if (rows.length === 0)
    return (
      <div className="empty-hint" style={{ padding: 24 }}>
        <Icon name="net" size={20} /> No route table is exposed by the backend yet.
        <br />
        <span className="muted small">
          Routes will appear here once the API surfaces them in interface metadata
          or via a dedicated <code>/network/routes</code> endpoint.
        </span>
      </div>
    );

  return (
    <table className="tbl">
      <thead>
        <tr>
          <th>Destination</th>
          <th>Gateway</th>
          <th>Device</th>
          <th>Proto</th>
          <th>Scope</th>
          <th className="num">Metric</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={i}>
            <td className="mono">{r.dst}</td>
            <td className="mono small">{r.gateway ?? "—"}</td>
            <td className="mono small">{r.dev}</td>
            <td className="muted small">{r.proto ?? "—"}</td>
            <td className="muted small">{r.scope ?? "—"}</td>
            <td className="num mono">{r.metric ?? "—"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export default Routes;
