import { useQuery } from "@tanstack/react-query";
import { network } from "../../api/network";
import { Icon } from "../../components/Icon";

export function Interfaces() {
  const ifaces = useQuery({
    queryKey: ["network", "interfaces"],
    queryFn: () => network.listInterfaces(),
  });

  const refetch = () => {
    void ifaces.refetch();
  };

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          <Icon name="plus" size={11} /> Add interface
        </button>
        <button className="btn">Add VLAN</button>
        <button className="btn">Add bond</button>
        <button className="btn" style={{ marginLeft: "auto" }} onClick={refetch}>
          <Icon name="refresh" size={11} /> Reload
        </button>
      </div>

      {ifaces.isLoading && <div className="muted">Loading interfaces…</div>}
      {ifaces.isError && (
        <div className="muted" style={{ color: "var(--err)" }}>
          Failed to load: {(ifaces.error as Error).message}
        </div>
      )}
      {ifaces.data && ifaces.data.length === 0 && (
        <div className="muted">No interfaces found.</div>
      )}
      {ifaces.data && ifaces.data.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Interface</th>
              <th>Type</th>
              <th>State</th>
              <th>IPv4</th>
              <th>MAC</th>
              <th className="num">MTU</th>
              <th>Speed</th>
            </tr>
          </thead>
          <tbody>
            {ifaces.data.map((i) => {
              const state = (i.state ?? i.link ?? "").toString().toUpperCase();
              const ipv4 =
                i.ipv4 ??
                (i.addresses ?? []).find((a) => /^\d+\.\d+\.\d+\.\d+/.test(a));
              return (
                <tr key={i.name}>
                  <td className="mono">{i.name}</td>
                  <td>
                    {i.type ? (
                      <span className="pill pill--info">{i.type}</span>
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                  <td>
                    <span
                      className={`sdot sdot--${state === "UP" || state === "ACTIVE" ? "ok" : "warn"}`}
                    />{" "}
                    {state || "—"}
                  </td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {ipv4 ?? <span className="muted">—</span>}
                  </td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {i.mac ?? "—"}
                  </td>
                  <td className="num mono">{i.mtu ?? "—"}</td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {i.speed ?? "—"}
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

export default Interfaces;
