import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares } from "../../api/shares";
import { formatBytes } from "../../lib/format";

export function ISCSI() {
  const [sel, setSel] = useState<string | null>(null);
  const q = useQuery({
    queryKey: ["iscsi-targets"],
    queryFn: () => shares.listIscsi(),
  });
  const list = q.data ?? [];
  const cur = list.find((t) => t.iqn === sel);

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
            {/* TODO: phase 3 — open create-iSCSI dialog */}
            <Icon name="plus" size={11} />
            New target
          </button>
        </div>
        {q.isLoading && <div className="empty-hint">Loading targets…</div>}
        {q.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(q.error as Error).message}
          </div>
        )}
        {q.data && list.length === 0 && (
          <div className="empty-hint">No iSCSI targets.</div>
        )}
        {list.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>IQN</th>
                <th className="num">LUNs</th>
                <th>Portals</th>
                <th className="num">ACLs</th>
                <th>State</th>
              </tr>
            </thead>
            <tbody>
              {list.map((t) => (
                <tr
                  key={t.iqn}
                  className={sel === t.iqn ? "is-on" : ""}
                  onClick={() => setSel(t.iqn)}
                  style={{ cursor: "pointer" }}
                >
                  <td className="mono" style={{ fontSize: 11 }}>
                    {t.iqn}
                  </td>
                  <td className="num mono">{t.luns ?? 0}</td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {(t.portals ?? []).join(", ")}
                  </td>
                  <td className="num mono">{t.acls ?? 0}</td>
                  <td>
                    <span className="pill pill--ok">
                      <span className="dot" />
                      {t.state ?? "up"}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {cur && <TargetDetail iqn={cur.iqn} onClose={() => setSel(null)} />}
    </div>
  );
}

function TargetDetail({ iqn, onClose }: { iqn: string; onClose: () => void }) {
  const lunsQ = useQuery({
    queryKey: ["iscsi-luns", iqn],
    queryFn: () => shares.listIscsiLuns(iqn),
  });
  const portalsQ = useQuery({
    queryKey: ["iscsi-portals", iqn],
    queryFn: () => shares.listIscsiPortals(iqn),
  });
  const aclsQ = useQuery({
    queryKey: ["iscsi-acls", iqn],
    queryFn: () => shares.listIscsiAcls(iqn),
  });

  return (
    <div className="side-detail">
      <div className="side-detail__head">
        <div>
          <div className="muted mono" style={{ fontSize: 10 }}>
            ISCSI TARGET
          </div>
          <div className="side-detail__title" style={{ wordBreak: "break-all" }}>
            {iqn}
          </div>
        </div>
        <button className="btn btn--sm" onClick={onClose}>
          <Icon name="close" size={10} />
        </button>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">LUNs</div>
        </div>
        <div className="sect__body">
          {lunsQ.isLoading && <div className="muted">Loading…</div>}
          {lunsQ.data && lunsQ.data.length === 0 && (
            <div className="muted">No LUNs.</div>
          )}
          {lunsQ.data && lunsQ.data.length > 0 && (
            <table className="tbl tbl--compact">
              <thead>
                <tr>
                  <th>LUN</th>
                  <th>Backing</th>
                  <th className="num">Size</th>
                </tr>
              </thead>
              <tbody>
                {lunsQ.data.map((l) => (
                  <tr key={l.id}>
                    <td className="mono">{l.lun ?? "—"}</td>
                    <td className="mono" style={{ fontSize: 11 }}>
                      {l.backing ?? "—"}
                    </td>
                    <td className="num mono">
                      {l.size ? formatBytes(l.size) : "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Portals</div>
        </div>
        <div className="sect__body">
          {portalsQ.isLoading && <div className="muted">Loading…</div>}
          {portalsQ.data && portalsQ.data.length === 0 && (
            <div className="muted">No portals.</div>
          )}
          {portalsQ.data && portalsQ.data.length > 0 && (
            <table className="tbl tbl--compact">
              <tbody>
                {portalsQ.data.map((p, i) => (
                  <tr key={p.id ?? i}>
                    <td className="mono">{p.ip ?? "—"}</td>
                    <td className="mono">{p.port ?? "—"}</td>
                    <td className="muted mono">tag {p.tag ?? "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">ACLs</div>
        </div>
        <div className="sect__body">
          {aclsQ.isLoading && <div className="muted">Loading…</div>}
          {aclsQ.data && aclsQ.data.length === 0 && (
            <div className="muted">No ACLs.</div>
          )}
          {aclsQ.data && aclsQ.data.length > 0 && (
            <table className="tbl tbl--compact">
              <tbody>
                {aclsQ.data.map((a, i) => (
                  <tr key={i}>
                    <td className="mono" style={{ fontSize: 11 }}>
                      {a.initiator}
                    </td>
                    <td className="muted">{a.user ?? "—"}</td>
                    <td className="muted">{a.authMethod ?? "—"}</td>
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

export default ISCSI;
