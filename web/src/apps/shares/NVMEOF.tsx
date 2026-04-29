import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares } from "../../api/shares";
import { formatBytes } from "../../lib/format";

export function NVMEOF() {
  const [sel, setSel] = useState<string | null>(null);
  const subQ = useQuery({
    queryKey: ["nvmeof-subsystems"],
    queryFn: () => shares.listNvmeofSubsystems(),
  });
  const portsQ = useQuery({
    queryKey: ["nvmeof-ports"],
    queryFn: () => shares.listNvmeofPorts(),
  });
  const subs = subQ.data ?? [];
  const cur = subs.find((s) => s.nqn === sel);

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
          <button className="btn btn--primary">
            {/* TODO: phase 3 — open create-subsystem dialog */}
            <Icon name="plus" size={11} />
            New subsystem
          </button>
        </div>
        {subQ.isLoading && <div className="empty-hint">Loading subsystems…</div>}
        {subQ.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(subQ.error as Error).message}
          </div>
        )}
        {subQ.data && subs.length === 0 && (
          <div className="empty-hint">No NVMe-oF subsystems.</div>
        )}
        {subs.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>NQN</th>
                <th className="num">NS</th>
                <th className="num">Ports</th>
                <th className="num">Hosts</th>
                <th>DH-CHAP</th>
                <th>State</th>
              </tr>
            </thead>
            <tbody>
              {subs.map((s) => (
                <tr
                  key={s.nqn}
                  className={sel === s.nqn ? "is-on" : ""}
                  onClick={() => setSel(s.nqn)}
                  style={{ cursor: "pointer" }}
                >
                  <td className="mono" style={{ fontSize: 11 }}>
                    {s.nqn}
                  </td>
                  <td className="num mono">{s.ns ?? 0}</td>
                  <td className="num mono">{s.ports ?? 0}</td>
                  <td className="num mono">{s.hosts ?? 0}</td>
                  <td>
                    {s.dhchap ? (
                      <Icon name="shield" size={11} />
                    ) : (
                      <span className="muted">off</span>
                    )}
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
        <div className="sect">
          <div className="sect__head">
            <div className="sect__title">Ports</div>
          </div>
          <div className="sect__body">
            {portsQ.isLoading && <div className="muted">Loading…</div>}
            {portsQ.data && portsQ.data.length === 0 && (
              <div className="muted">No ports.</div>
            )}
            {portsQ.data && portsQ.data.length > 0 && (
              <table className="tbl tbl--compact">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Type</th>
                    <th>Address</th>
                    <th>Service</th>
                  </tr>
                </thead>
                <tbody>
                  {portsQ.data.map((p) => (
                    <tr key={p.id}>
                      <td className="mono">{p.id}</td>
                      <td className="mono">{p.trtype ?? "—"}</td>
                      <td className="mono" style={{ fontSize: 11 }}>
                        {p.traddr ?? "—"}
                      </td>
                      <td className="mono">{p.trsvcid ?? "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      </div>
      {cur && <SubsystemDetail nqn={cur.nqn} onClose={() => setSel(null)} />}
    </div>
  );
}

function SubsystemDetail({
  nqn,
  onClose,
}: {
  nqn: string;
  onClose: () => void;
}) {
  const nsQ = useQuery({
    queryKey: ["nvmeof-ns", nqn],
    queryFn: () => shares.listNvmeofNamespaces(nqn),
  });
  return (
    <div className="side-detail">
      <div className="side-detail__head">
        <div>
          <div className="muted mono" style={{ fontSize: 10 }}>
            SUBSYSTEM
          </div>
          <div className="side-detail__title" style={{ wordBreak: "break-all" }}>
            {nqn}
          </div>
        </div>
        <button className="btn btn--sm" onClick={onClose}>
          <Icon name="close" size={10} />
        </button>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Namespaces</div>
        </div>
        <div className="sect__body">
          {nsQ.isLoading && <div className="muted">Loading…</div>}
          {nsQ.data && nsQ.data.length === 0 && (
            <div className="muted">No namespaces.</div>
          )}
          {nsQ.data && nsQ.data.length > 0 && (
            <table className="tbl tbl--compact">
              <thead>
                <tr>
                  <th>NSID</th>
                  <th>Device</th>
                  <th className="num">Size</th>
                </tr>
              </thead>
              <tbody>
                {nsQ.data.map((n) => (
                  <tr key={n.nsid}>
                    <td className="mono">{n.nsid}</td>
                    <td className="mono" style={{ fontSize: 11 }}>
                      {n.device ?? "—"}
                    </td>
                    <td className="num mono">
                      {n.size ? formatBytes(n.size) : "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}

export default NVMEOF;
