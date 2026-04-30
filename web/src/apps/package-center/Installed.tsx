import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { plugins, type Installation } from "../../api/plugins";
import { api } from "../../api/client";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

export function Installed() {
  const qc = useQueryClient();
  const q = useQuery<Installation[]>({
    queryKey: ["plugins", "installed"],
    queryFn: () => plugins.listInstalled(),
  });
  const [pickedName, setPickedName] = useState<string | null>(null);
  const [showLogs, setShowLogs] = useState(false);

  const cur = (q.data ?? []).find((p) => p.name === pickedName) ?? (q.data ?? [])[0];
  const curName = cur?.name;

  const deps = useQuery({
    queryKey: ["plugin-deps", curName],
    enabled: !!curName,
    queryFn: () => plugins.listDependencies(curName!),
  });
  const dependents = useQuery({
    queryKey: ["plugin-dependents", curName],
    enabled: !!curName,
    queryFn: () => plugins.listDependents(curName!),
  });

  const uninstall = useMutation({
    meta: { label: "Uninstall failed" },
    mutationFn: (name: string) =>
      api(`/api/v1/plugins/${encodeURIComponent(name)}`, { method: "DELETE" }),
    onSuccess: (_d, name) => {
      qc.invalidateQueries({ queryKey: ["plugins"] });
      toastSuccess("Plugin uninstalled", name);
    },
  });

  const restart = useMutation({
    meta: { label: "Restart failed" },
    mutationFn: (name: string) => plugins.restart(name),
    onSuccess: (_d, name) => toastSuccess("Plugin restarted", name),
  });

  if (q.isLoading) return <div className="discover__msg">Loading installed plugins…</div>;
  if (q.isError)
    return (
      <div className="discover__msg discover__msg--err">
        Failed to load: {(q.error as Error).message}
      </div>
    );
  if (!q.data || q.data.length === 0)
    return (
      <div className="discover__msg muted">
        No plugins installed yet. Browse the Discover tab to install one.
      </div>
    );

  return (
    <div className="installed">
      <aside className="installed__list">
        <div className="vlist__title">INSTALLED</div>
        {q.data.map((p) => (
          <button
            key={p.name}
            className={cur?.name === p.name ? "is-on" : ""}
            onClick={() => setPickedName(p.name)}
          >
            <Icon name="package" size={12} />
            <span>{p.name}</span>
            <span className="muted mono small">{p.version}</span>
          </button>
        ))}
      </aside>
      {cur && (
        <main className="installed__detail">
          <div className="installed__head">
            <div className="installed__icon">
              {cur.name.split("-").slice(-1)[0].slice(0, 2).toUpperCase()}
            </div>
            <div className="installed__head-meta">
              <div className="installed__title">{cur.name}</div>
              <div className="muted">
                v{cur.version}
                {cur.manifest?.metadata?.vendor && <> · {cur.manifest.metadata.vendor}</>}
              </div>
            </div>
          </div>
          <Sect title="Status">
            <span className="pill pill--ok">
              <span className="dot" /> running
            </span>
          </Sect>
          {(() => {
            const spec = (cur.manifest?.spec ?? {}) as Record<string, unknown>;
            const desc = spec.description as string | undefined;
            return desc ? (
              <Sect title="Description">
                <div className="muted">{desc}</div>
              </Sect>
            ) : null;
          })()}
          {deps.data && deps.data.length > 0 && (
            <Sect title="Depends on">
              {deps.data.map((d) => (
                <div key={d.name} className="perm-row">
                  <Icon name="package" size={11} />
                  <span className="perm-row__name">{d.name}</span>
                  <span className="perm-row__desc mono">{d.version}</span>
                </div>
              ))}
            </Sect>
          )}
          {dependents.data && dependents.data.length > 0 && (
            <Sect title="Required by">
              {dependents.data.map((d) => (
                <div key={d.name} className="perm-row">
                  <Icon name="package" size={11} />
                  <span className="perm-row__name">{d.name}</span>
                  <span className="perm-row__desc mono">{d.version}</span>
                </div>
              ))}
            </Sect>
          )}
          <div className="installed__actions">
            <button
              className="btn"
              disabled={restart.isPending}
              onClick={() => restart.mutate(cur.name)}
            >
              <Icon name="refresh" size={11} /> {restart.isPending ? "Restarting…" : "Restart"}
            </button>
            <button className="btn" onClick={() => setShowLogs(true)}>
              <Icon name="log" size={11} /> Logs
            </button>
            <button
              className="btn btn--danger"
              style={{ marginLeft: "auto" }}
              disabled={uninstall.isPending}
              onClick={() => {
                if (window.confirm(`Uninstall ${cur.name}?`)) {
                  uninstall.mutate(cur.name);
                }
              }}
            >
              <Icon name="trash" size={11} /> Uninstall
            </button>
          </div>
        </main>
      )}
      {showLogs && curName && (
        <PluginLogsModal name={curName} onClose={() => setShowLogs(false)} />
      )}
    </div>
  );
}

function PluginLogsModal({ name, onClose }: { name: string; onClose: () => void }) {
  const q = useQuery({
    queryKey: ["plugin-logs", name],
    queryFn: () => plugins.getLogs(name, 500),
    refetchInterval: 5_000,
  });
  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" style={{ width: 760, maxWidth: "92vw" }} onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="log" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Logs · {name}</div>
            <div className="muted modal__sub">journalctl -u nova-plugin-{name} · refresh every 5s</div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body" style={{ maxHeight: "60vh", overflow: "auto" }}>
          {q.isLoading && <div className="discover__msg muted">Loading logs…</div>}
          {q.isError && (
            <div className="discover__msg discover__msg--err">
              Failed to load: {(q.error as Error).message}
            </div>
          )}
          {q.data && (
            <pre
              style={{
                margin: 0,
                padding: 12,
                fontFamily: "var(--font-mono)",
                fontSize: 11,
                color: "var(--fg-1)",
                background: "var(--bg-0)",
                whiteSpace: "pre-wrap",
                wordBreak: "break-word",
              }}
            >
              {q.data.lines.join("\n")}
            </pre>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  );
}

function Sect({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="sect">
      <div className="sect__title">{title}</div>
      <div className="sect__body">{children}</div>
    </div>
  );
}
