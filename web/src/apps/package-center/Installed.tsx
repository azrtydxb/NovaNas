import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { plugins, type PluginManifest } from "../../api/plugins";
import { api } from "../../api/client";
import { Icon } from "../../components/Icon";

export function Installed() {
  const qc = useQueryClient();
  const q = useQuery<PluginManifest[]>({
    queryKey: ["plugins", "installed"],
    queryFn: () => plugins.listInstalled(),
  });
  const [pickedName, setPickedName] = useState<string | null>(null);

  const cur = (q.data ?? []).find((p) => p.metadata.name === pickedName) ?? (q.data ?? [])[0];
  const curName = cur?.metadata.name;

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
    mutationFn: (name: string) =>
      api(`/api/v1/plugins/${encodeURIComponent(name)}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["plugins"] }),
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
            key={p.metadata.name}
            className={cur?.metadata.name === p.metadata.name ? "is-on" : ""}
            onClick={() => setPickedName(p.metadata.name)}
          >
            <Icon name="package" size={12} />
            <span>{p.metadata.name}</span>
            <span className="muted mono small">{p.metadata.version}</span>
          </button>
        ))}
      </aside>
      {cur && (
        <main className="installed__detail">
          <div className="installed__head">
            <div className="installed__icon">
              {cur.metadata.name.split("-").slice(-1)[0].slice(0, 2).toUpperCase()}
            </div>
            <div className="installed__head-meta">
              <div className="installed__title">{cur.metadata.name}</div>
              <div className="muted">
                v{cur.metadata.version}
                {cur.metadata.vendor && <> · {cur.metadata.vendor}</>}
              </div>
            </div>
          </div>
          <Sect title="Status">
            <span className="pill pill--ok">
              <span className="dot" /> running
            </span>
          </Sect>
          {(() => {
            const spec = cur.spec as Record<string, unknown>;
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
            <button className="btn">
              <Icon name="refresh" size={11} /> Restart
            </button>
            <button className="btn">
              <Icon name="log" size={11} /> Logs
            </button>
            <button
              className="btn btn--danger"
              style={{ marginLeft: "auto" }}
              disabled={uninstall.isPending}
              onClick={() => {
                if (window.confirm(`Uninstall ${cur.metadata.name}?`)) {
                  uninstall.mutate(cur.metadata.name);
                }
              }}
            >
              <Icon name="trash" size={11} /> Uninstall
            </button>
          </div>
        </main>
      )}
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
