import { useQuery } from "@tanstack/react-query";
import { network } from "../../api/network";
import { Icon } from "../../components/Icon";

export function Bonds() {
  const q = useQuery({ queryKey: ["network", "bonds"], queryFn: () => network.listBonds() });
  if (q.isLoading) return <div className="empty-hint">Loading bonds…</div>;
  if (q.isError) return <div className="empty-hint" style={{ color: "var(--err)" }}>Failed: {(q.error as Error).message}</div>;
  const rows = q.data ?? [];
  if (rows.length === 0)
    return (
      <div className="empty-hint" style={{ padding: 24 }}>
        <Icon name="net" size={20} /> No bonds configured. Use Interfaces → Add to create a bond.
      </div>
    );
  return (
    <table className="tbl">
      <thead>
        <tr>
          <th>Name</th>
          <th>Mode</th>
          <th>Slaves</th>
          <th>State</th>
          <th>MTU</th>
          <th>MAC</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((b: Record<string, unknown>) => {
          const name = String(b.name ?? "");
          const mode = String(b.mode ?? b.bondMode ?? "—");
          const slaves = Array.isArray(b.slaves) ? (b.slaves as string[]).join(", ") : (b.slaves as string) ?? "—";
          const state = String(b.state ?? "—");
          const mtu = String(b.mtu ?? "—");
          const mac = String(b.mac ?? "—");
          return (
            <tr key={name}>
              <td className="mono">{name}</td>
              <td>{mode}</td>
              <td className="mono small">{slaves}</td>
              <td>
                <span className={`pill pill--${/up|active|online/i.test(state) ? "ok" : "warn"}`}>
                  <span className="dot" />
                  {state}
                </span>
              </td>
              <td className="mono small">{mtu}</td>
              <td className="mono small">{mac}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

export default Bonds;
