import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import {
  workloads,
  type HelmRelease,
  type HelmReleaseDetail,
} from "../../api/workloads";
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

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="field">
      <label className="field__label">{label}</label>
      {children}
      {hint && <div className="field__hint muted">{hint}</div>}
    </div>
  );
}

function UpgradeModal({
  release,
  current,
  onClose,
}: {
  release: string;
  current: HelmReleaseDetail | undefined;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [version, setVersion] = useState(current?.version ?? "");
  const [values, setValues] = useState(
    current?.values ? JSON.stringify(current.values, null, 2) : "",
  );
  const mut = useMutation({
    mutationFn: () =>
      workloads.upgrade(release, {
        version: version || undefined,
        values: values.trim() ? values : undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["workloads"] });
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" style={{ width: 620 }} onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="apps" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Upgrade {release}</div>
            <div className="muted modal__sub">
              Helm will reconcile the release to the new version and values.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <Field label="Chart version" hint={`Current: ${current?.version ?? "unknown"}`}>
            <input
              className="input"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              placeholder="1.2.3"
              autoFocus
            />
          </Field>
          <Field
            label="values.yaml (optional)"
            hint="Leave empty to keep existing values; YAML or JSON accepted."
          >
            <textarea
              className="input"
              rows={12}
              value={values}
              onChange={(e) => setValues(e.target.value)}
              style={{ fontFamily: "var(--font-mono)", fontSize: 11, resize: "vertical" }}
              placeholder="# values"
            />
          </Field>
          {mut.isError && (
            <div className="modal__err">Failed: {(mut.error as Error).message}</div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={mut.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={mut.isPending}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? "Upgrading…" : "Upgrade"}
          </button>
        </div>
      </div>
    </div>
  );
}

function RollbackModal({
  release,
  detail,
  onClose,
}: {
  release: string;
  detail: HelmReleaseDetail | undefined;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const history = detail?.history ?? [];
  const [revision, setRevision] = useState<number | "">(
    history.length > 1 ? history[history.length - 2].revision : "",
  );
  const mut = useMutation({
    mutationFn: () =>
      workloads.rollback(release, typeof revision === "number" ? revision : undefined),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["workloads"] });
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="refresh" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Roll back {release}</div>
            <div className="muted modal__sub">
              Pick a prior revision to roll the release back to.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          {history.length === 0 ? (
            <div className="modal__loading muted">
              No revision history available — rollback will revert to the previous revision.
            </div>
          ) : (
            <Field label="Target revision">
              <select
                className="input"
                value={revision === "" ? "" : String(revision)}
                onChange={(e) =>
                  setRevision(e.target.value === "" ? "" : Number(e.target.value))
                }
              >
                <option value="">previous</option>
                {history.map((h) => (
                  <option key={h.revision} value={h.revision}>
                    rev {h.revision} · {h.status} · {h.updated}
                  </option>
                ))}
              </select>
            </Field>
          )}
          {mut.isError && (
            <div className="modal__err">Failed: {(mut.error as Error).message}</div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={mut.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={mut.isPending}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? "Rolling back…" : "Roll back"}
          </button>
        </div>
      </div>
    </div>
  );
}

function InstallModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [chart, setChart] = useState("");
  const [release, setRelease] = useState("");
  const [namespace, setNamespace] = useState("default");
  const [version, setVersion] = useState("");
  const [values, setValues] = useState("");
  const mut = useMutation({
    mutationFn: () =>
      workloads.install({
        chart,
        release: release || undefined,
        namespace: namespace || undefined,
        version: version || undefined,
        values: values.trim() ? values : undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["workloads"] });
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="plus" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Install chart</div>
            <div className="muted modal__sub">
              Install a Helm chart from the catalog.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <Field label="Chart" hint="Catalog name (see Catalog tab).">
            <input
              className="input"
              value={chart}
              onChange={(e) => setChart(e.target.value)}
              placeholder="immich"
              autoFocus
            />
          </Field>
          <Field label="Release name" hint="Defaults to the chart name.">
            <input
              className="input"
              value={release}
              onChange={(e) => setRelease(e.target.value)}
              placeholder="immich"
            />
          </Field>
          <Field label="Namespace">
            <input
              className="input"
              value={namespace}
              onChange={(e) => setNamespace(e.target.value)}
              placeholder="default"
            />
          </Field>
          <Field label="Version" hint="Leave blank for latest.">
            <input
              className="input"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
            />
          </Field>
          <Field label="values.yaml (optional)">
            <textarea
              className="input"
              rows={8}
              value={values}
              onChange={(e) => setValues(e.target.value)}
              style={{ fontFamily: "var(--font-mono)", fontSize: 11, resize: "vertical" }}
            />
          </Field>
          {mut.isError && (
            <div className="modal__err">Failed: {(mut.error as Error).message}</div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={mut.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={!chart.trim() || mut.isPending}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? "Installing…" : "Install"}
          </button>
        </div>
      </div>
    </div>
  );
}

export function Releases() {
  const qc = useQueryClient();
  const [sel, setSel] = useState<string | null>(null);
  const [showInstall, setShowInstall] = useState(false);
  const [showUpgrade, setShowUpgrade] = useState(false);
  const [showRollback, setShowRollback] = useState(false);

  const list = useQuery({
    queryKey: ["workloads", "list"],
    queryFn: () => workloads.list(),
    retry: false,
  });

  const items = list.data ?? [];
  const cur = items.find((w) => rel(w) === sel) ?? items[0];
  const curName = cur ? rel(cur) : null;

  const detail = useQuery({
    queryKey: ["workloads", "detail", curName],
    queryFn: () => workloads.get(curName!),
    enabled: !!curName,
    retry: false,
  });

  const uninstall = useMutation({
    mutationFn: (name: string) => workloads.uninstall(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["workloads"] });
      setSel(null);
    },
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
          <div className="discover__msg muted">
            k3s is being set up. Helm releases will appear here once the cluster is ready.
          </div>
          <button className="btn btn--sm" style={{ marginTop: 10 }} onClick={() => list.refetch()}>
            <Icon name="refresh" size={11} />
            Retry
          </button>
        </div>
      );
    }
    return (
      <div style={{ padding: 14, color: "var(--err)" }}>
        Failed to load releases: {err.message}
      </div>
    );
  }

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 320px", height: "100%" }}>
      <div style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <button className="btn btn--primary" onClick={() => setShowInstall(true)}>
            <Icon name="plus" size={11} />
            Install chart
          </button>
          <button
            className="btn btn--sm"
            onClick={() => list.refetch()}
            disabled={list.isFetching}
            style={{ marginLeft: "auto" }}
          >
            <Icon name="refresh" size={11} />
            Refresh
          </button>
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
                  className={cur && rel(cur) === rel(w) ? "is-on" : ""}
                >
                  <td>
                    <Icon
                      name="apps"
                      size={12}
                      style={{ verticalAlign: "-2px", marginRight: 6, opacity: 0.6 }}
                    />
                    {rel(w)}
                  </td>
                  <td className="muted mono" style={{ fontSize: 11 }}>{w.chart ?? "—"}</td>
                  <td className="mono" style={{ fontSize: 11 }}>{w.version ?? "—"}</td>
                  <td className="muted mono" style={{ fontSize: 11 }}>{ns(w)}</td>
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
              <div className="muted mono" style={{ fontSize: 10 }}>RELEASE</div>
              <div className="side-detail__title">{rel(cur)}</div>
            </div>
          </div>
          <Sect title="Chart">
            <dl className="kv">
              <dt>Chart</dt>
              <dd className="mono">{cur.chart ?? "—"}</dd>
              <dt>Version</dt>
              <dd className="mono">{cur.version ?? "—"}</dd>
              <dt>App version</dt>
              <dd className="mono">{cur.appVersion ?? "—"}</dd>
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
              <dt>Status</dt>
              <dd>
                <span className={statusPill(cur.status)}>
                  <span className="dot" />
                  {cur.status ?? "—"}
                </span>
              </dd>
            </dl>
          </Sect>
          {detail.data?.history && detail.data.history.length > 0 && (
            <Sect title="History">
              <table className="tbl tbl--compact">
                <tbody>
                  {detail.data.history.slice(-5).reverse().map((h) => (
                    <tr key={h.revision}>
                      <td className="mono">rev {h.revision}</td>
                      <td>
                        <span className={statusPill(h.status)}>
                          <span className="dot" />
                          {h.status}
                        </span>
                      </td>
                      <td className="muted">{h.updated}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Sect>
          )}
          <div
            className="row gap-8"
            style={{
              padding: "10px 12px",
              borderTop: "1px solid var(--line)",
              flexWrap: "wrap",
            }}
          >
            <button
              className="btn btn--sm btn--primary"
              onClick={() => setShowUpgrade(true)}
            >
              Upgrade
            </button>
            <button
              className="btn btn--sm"
              onClick={() => setShowRollback(true)}
            >
              Rollback…
            </button>
            <button
              className="btn btn--sm btn--danger"
              style={{ marginLeft: "auto" }}
              disabled={uninstall.isPending}
              onClick={() => {
                if (window.confirm(`Uninstall ${rel(cur)}? This cannot be undone.`)) {
                  uninstall.mutate(rel(cur));
                }
              }}
            >
              {uninstall.isPending ? "Uninstalling…" : "Uninstall"}
            </button>
          </div>
        </div>
      )}
      {showInstall && <InstallModal onClose={() => setShowInstall(false)} />}
      {showUpgrade && curName && (
        <UpgradeModal
          release={curName}
          current={detail.data}
          onClose={() => setShowUpgrade(false)}
        />
      )}
      {showRollback && curName && (
        <RollbackModal
          release={curName}
          detail={detail.data}
          onClose={() => setShowRollback(false)}
        />
      )}
    </div>
  );
}
