import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { vms, type VMTemplate } from "../../api/vms";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

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

function CreateFromTemplateModal({
  templates,
  initial,
  onClose,
}: {
  templates: VMTemplate[];
  initial?: VMTemplate;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [template, setTemplate] = useState(initial?.name ?? templates[0]?.name ?? "");
  const [namespace, setNamespace] = useState(initial?.namespace ?? "default");
  const mut = useMutation({
    meta: { label: "Create VM failed" },
    mutationFn: () =>
      vms.createFromTemplate({ name, template, namespace: namespace || undefined }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vms", "list"] });
      toastSuccess(`VM ${name} created`);
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="vm" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Create VM from template</div>
            <div className="muted modal__sub">
              Provisions a new VM with the resources defined by the template.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <Field label="VM name" hint="Must be a valid DNS-1123 label.">
            <input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="my-vm"
              autoFocus
            />
          </Field>
          <Field label="Template">
            <select
              className="input"
              value={template}
              onChange={(e) => setTemplate(e.target.value)}
            >
              {templates.map((t) => (
                <option key={t.name} value={t.name}>
                  {t.name} {t.os ? `· ${t.os}` : ""}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Namespace">
            <input
              className="input"
              value={namespace}
              onChange={(e) => setNamespace(e.target.value)}
              placeholder="default"
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
            disabled={!name.trim() || !template || mut.isPending}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? "Creating…" : "Create VM"}
          </button>
        </div>
      </div>
    </div>
  );
}

export function Templates() {
  const [creating, setCreating] = useState<{ initial?: VMTemplate } | null>(null);

  const q = useQuery({
    queryKey: ["vms", "templates"],
    queryFn: () => vms.templates(),
    retry: false,
  });

  if (q.isError) {
    const err = q.error as ApiError | Error;
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 503) {
      return (
        <div style={{ padding: 24 }}>
          <div className="discover__msg muted">
            Templates unavailable; KubeVirt is not yet ready.
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
        Failed to load templates: {err.message}
      </div>
    );
  }

  const items = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button
          className="btn btn--primary"
          onClick={() => setCreating({})}
        >
          <Icon name="plus" size={11} />
          New template
        </button>
        <button
          className="btn"
          onClick={() => q.refetch()}
          disabled={q.isFetching}
          style={{ marginLeft: "auto" }}
        >
          <Icon name="refresh" size={11} />
          Refresh
        </button>
      </div>
      {q.isLoading && <div className="muted">Loading templates…</div>}
      {!q.isLoading && items.length === 0 && (
        <div className="muted" style={{ padding: 12 }}>
          No VM templates defined.
        </div>
      )}
      {items.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Template</th>
              <th>OS</th>
              <th className="num">vCPU</th>
              <th className="num">RAM</th>
              <th className="num">Disk</th>
              <th>Source</th>
            </tr>
          </thead>
          <tbody>
            {items.map((t) => (
              <tr
                key={`${t.namespace ?? "default"}/${t.name}`}
                onClick={() => setCreating({ initial: t })}
                style={{ cursor: "pointer" }}
                title="Click to create a VM from this template"
              >
                <td>{t.name}</td>
                <td className="muted">{t.os ?? "—"}</td>
                <td className="num mono">{t.cpu ?? "—"}</td>
                <td className="num mono">
                  {t.ram ? `${(t.ram / 1024).toFixed(0)} GiB` : "—"}
                </td>
                <td className="num mono">{t.disk ? `${t.disk} GiB` : "—"}</td>
                <td>
                  <span className="pill">{t.source ?? "—"}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {creating && (
        <CreateFromTemplateModal
          templates={items}
          initial={creating.initial}
          onClose={() => setCreating(null)}
        />
      )}
    </div>
  );
}
