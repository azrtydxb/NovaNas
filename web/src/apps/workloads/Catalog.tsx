import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { workloads, type ChartIndexEntry } from "../../api/workloads";
import { Icon } from "../../components/Icon";

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

function InstallChartModal({
  entry,
  onClose,
}: {
  entry: ChartIndexEntry;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [release, setRelease] = useState(entry.name);
  const [namespace, setNamespace] = useState("default");
  const [version, setVersion] = useState(entry.version ?? "");
  const [values, setValues] = useState("");

  const mut = useMutation({
    mutationFn: () =>
      workloads.install({
        chart: entry.name,
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
          <div
            className="modal__icon"
            style={{
              background: entry.color
                ? `linear-gradient(135deg, ${entry.color}, ${entry.color})`
                : undefined,
              color: entry.color ? "white" : undefined,
            }}
          >
            {(entry.displayName ?? entry.name).slice(0, 2).toUpperCase()}
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Install {entry.displayName ?? entry.name}</div>
            <div className="muted modal__sub">
              {entry.description ?? `${entry.category ?? "chart"} · v${entry.version ?? "—"}`}
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <Field label="Release name">
            <input
              className="input"
              value={release}
              onChange={(e) => setRelease(e.target.value)}
              autoFocus
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
          <Field label="Version" hint="Leave blank for the latest published version.">
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
              placeholder="# overrides"
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
            disabled={!release.trim() || mut.isPending}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? "Installing…" : "Install"}
          </button>
        </div>
      </div>
    </div>
  );
}

export function Catalog() {
  const qc = useQueryClient();
  const [filter, setFilter] = useState("");
  const [installing, setInstalling] = useState<ChartIndexEntry | null>(null);

  const q = useQuery({
    queryKey: ["workloads", "index"],
    queryFn: () => workloads.index(),
    retry: false,
  });

  const reload = useMutation({
    mutationFn: () => workloads.reloadIndex(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workloads", "index"] }),
  });

  if (q.isError) {
    const err = q.error as ApiError | Error;
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 503) {
      return (
        <div style={{ padding: 24 }}>
          <div className="discover__msg muted">
            Catalog index unavailable while k3s initializes.
          </div>
          <button className="btn btn--sm" style={{ marginTop: 10 }} onClick={() => q.refetch()}>
            <Icon name="refresh" size={11} />
            Retry
          </button>
        </div>
      );
    }
    return (
      <div style={{ padding: 14, color: "var(--err)" }}>
        Failed to load catalog: {err.message}
      </div>
    );
  }

  const entries: ChartIndexEntry[] = q.data ?? [];
  const filtered = filter
    ? entries.filter((a) =>
        (a.displayName ?? a.name).toLowerCase().includes(filter.toLowerCase()),
      )
    : entries;

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <input
          className="input"
          placeholder="Search catalog…"
          style={{ width: 240 }}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <span className="muted" style={{ fontSize: 11, marginLeft: "auto" }}>
          {entries.length} chart{entries.length === 1 ? "" : "s"}
        </span>
        <button
          className="btn btn--sm"
          onClick={() => reload.mutate()}
          disabled={reload.isPending}
        >
          <Icon name="refresh" size={11} />
          {reload.isPending ? "Reloading…" : "Reload index"}
        </button>
      </div>
      {reload.isError && (
        <div className="muted" style={{ color: "var(--err)", padding: 8, fontSize: 11 }}>
          Reload failed: {(reload.error as Error).message}
        </div>
      )}
      {q.isLoading && <div className="muted">Loading catalog…</div>}
      {!q.isLoading && filtered.length === 0 && (
        <div className="muted" style={{ padding: 12 }}>
          No charts in the catalog.
        </div>
      )}
      {filtered.length > 0 && (
        <div className="appcards">
          {filtered.slice(0, 60).map((a) => (
            <div key={a.name} className="appcard">
              <div
                className="appcard__icon"
                style={{
                  background: a.color
                    ? `linear-gradient(135deg, ${a.color}, ${a.color})`
                    : "linear-gradient(135deg, var(--accent), var(--accent-2, var(--accent)))",
                }}
              >
                {(a.displayName ?? a.name).slice(0, 2).toUpperCase()}
              </div>
              <div className="appcard__name">{a.displayName ?? a.name}</div>
              <div className="appcard__cat muted">
                {(a.category ?? "chart")} · v{a.version ?? "—"}
              </div>
              {a.installed ? (
                <button
                  className="btn btn--sm"
                  style={{ marginTop: "auto" }}
                  disabled
                >
                  Installed
                </button>
              ) : (
                <button
                  className="btn btn--sm btn--primary"
                  style={{ marginTop: "auto" }}
                  onClick={() => setInstalling(a)}
                >
                  Install
                </button>
              )}
            </div>
          ))}
        </div>
      )}
      {installing && (
        <InstallChartModal entry={installing} onClose={() => setInstalling(null)} />
      )}
    </div>
  );
}
