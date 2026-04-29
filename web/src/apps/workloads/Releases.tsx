import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { workloads, type HelmRelease } from "../../api/workloads";
import { Icon } from "../../components/Icon";

function ns(w: HelmRelease) {
  return w.namespace ?? w.ns ?? "—";
}
function rel(w: HelmRelease) {
  return w.release ?? w.name ?? "—";
}

function statusPill(status?: string) {
  const s = (status ?? "").toLowerCase();
  if (s === "deployed") return "pill pill--ok";
  if (s === "pending" || s.includes("pending")) return "pill pill--info";
  if (s === "failed" || s.includes("err")) return "pill pill--err";
  return "pill";
}

function Sect({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="sect">
      <div className="sect__head">
        <div className="sect__title">{title}</div>
      </div>
      <div className="sect__body">{children}</div>
    </div>
  );
}

export function Releases() {
  const [sel, setSel] = useState<string | null>(null);
  const list = useQuery({
    queryKey: ["workloads", "list"],
    queryFn: () => workloads.list(),
    retry: false,
  });

  if (list.isError) {
    const err = list.error as ApiError | Error;
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 503) {
      return (
        <div style={{ padding: 24, color: "var(--fg-2)" }}>
          <div className="row gap-8" style={{ marginBottom: 8 }}>
            <Icon name="alert" size={14} />
            <strong>Workloads service unavailable</strong>
          </div>
          <div className="muted" style={{ fontSize: 12 }}>
            k3s is being set up. Helm releases will appear here once the cluster is ready.
          </div>
        </div>
      );
    }
    return (
      <div style={{ padding: 14, color: "var(--err)" }}>
        Failed to load releases: {err.message}
      </div>
    );
  }

  const items = list.data ?? [];
  const cur = items.find((w) => rel(w) === sel) ?? items[0];

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 320px", height: "100%" }}>
      <div style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <button className="btn btn--primary">
            <Icon name="plus" size={11} />
            Install chart
          </button>
          <button className="btn">Upgrade all</button>
        </div>
        {list.isLoading && <div className="muted">Loading releases…</div>}
        {!list.isLoading && items.length === 0 && (
          <div className="muted" style={{ padding: 12 }}>
            No Helm releases installed.
          </div>
        )}
        {items.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>Release</th>
                <th>Chart</th>
                <th>Version</th>
                <th>Namespace</th>
                <th className="num">Pods</th>
                <th className="num">CPU</th>
                <th className="num">Memory</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {items.map((w) => (
                <tr
                  key={rel(w)}
                  onClick={() => setSel(rel(w))}
                  className={(cur && rel(cur) === rel(w)) ? "is-on" : ""}
                >
                  <td>
                    <Icon
                      name="apps"
                      size={12}
                      style={{ verticalAlign: "-2px", marginRight: 6, opacity: 0.6 }}
                    />
                    {rel(w)}
                  </td>
                  <td className="muted mono" style={{ fontSize: 11 }}>
                    {w.chart ?? "—"}
                  </td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {w.version ?? "—"}
                  </td>
                  <td className="muted mono" style={{ fontSize: 11 }}>
                    {ns(w)}
                  </td>
                  <td className="num mono">{w.pods ?? "—"}</td>
                  <td className="num mono">{w.cpu ?? "—"}</td>
                  <td className="num mono">{w.mem ?? w.memory ?? "—"}</td>
                  <td>
                    <span className={statusPill(w.status)}>
                      <span className="dot" />
                      {w.status ?? "—"}
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
              <div className="muted mono" style={{ fontSize: 10 }}>
                RELEASE
              </div>
              <div className="side-detail__title">{rel(cur)}</div>
            </div>
          </div>
          <Sect title="Chart">
            <dl className="kv">
              <dt>Chart</dt>
              <dd className="mono">{cur.chart ?? "—"}</dd>
              <dt>Version</dt>
              <dd className="mono">{cur.version ?? "—"}</dd>
              <dt>Namespace</dt>
              <dd className="mono">{ns(cur)}</dd>
              <dt>Updated</dt>
              <dd>{cur.updated ?? cur.updatedAt ?? "—"}</dd>
            </dl>
          </Sect>
          <Sect title="Resources">
            <dl className="kv">
              <dt>Pods</dt>
              <dd className="mono">{cur.pods ?? "—"}</dd>
              <dt>CPU</dt>
              <dd className="mono">{cur.cpu ?? "—"}</dd>
              <dt>Memory</dt>
              <dd className="mono">{cur.mem ?? cur.memory ?? "—"}</dd>
            </dl>
          </Sect>
          <div
            className="row gap-8"
            style={{
              padding: "10px 12px",
              borderTop: "1px solid var(--line)",
              flexWrap: "wrap",
            }}
          >
            <button className="btn btn--sm btn--primary">Upgrade</button>
            <button className="btn btn--sm">Edit values</button>
            <button className="btn btn--sm">Rollback…</button>
            <button className="btn btn--sm btn--danger" style={{ marginLeft: "auto" }}>
              Uninstall
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
