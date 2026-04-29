import { useQuery } from "@tanstack/react-query";
import { network } from "../../api/network";
import { Icon } from "../../components/Icon";

export function VLANs() {
  const q = useQuery({ queryKey: ["network", "vlans"], queryFn: () => network.listVlans() });
  if (q.isLoading) return <div className="empty-hint">Loading VLANs…</div>;
  if (q.isError) return <div className="empty-hint" style={{ color: "var(--err)" }}>Failed: {(q.error as Error).message}</div>;
  const rows = q.data ?? [];
  if (rows.length === 0)
    return (
      <div className="empty-hint" style={{ padding: 24 }}>
        <Icon name="net" size={20} /> No VLANs configured. Use Interfaces → Add to create one.
      </div>
    );
  return (
    <table className="tbl">
      <thead>
        <tr>
          <th>Name</th>
          <th>Parent</th>
          <th>VID</th>
          <th>State</th>
          <th>IPv4</th>
          <th>MTU</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((v: Record<string, unknown>) => {
          const name = String(v.name ?? "");
          const parent = String(v.parent ?? v.link ?? "—");
          const vid = String(v.vid ?? v.id ?? "—");
          const state = String(v.state ?? "—");
          const ipv4 = Array.isArray(v.ipv4) ? (v.ipv4 as string[]).join(", ") : (v.ipv4 as string) ?? "—";
          const mtu = String(v.mtu ?? "—");
          return (
            <tr key={name}>
              <td className="mono">{name}</td>
              <td className="mono small">{parent}</td>
              <td className="mono">{vid}</td>
              <td>
                <span className={`pill pill--${/up|active|online/i.test(state) ? "ok" : "warn"}`}>
                  <span className="dot" />
                  {state}
                </span>
              </td>
              <td className="mono small">{ipv4}</td>
              <td className="mono small">{mtu}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

export default VLANs;
