import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { identity, type Krb5Principal } from "../../api/identity";
import { Icon } from "../../components/Icon";

function Sect({
  title,
  action,
  children,
}: {
  title: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="sect">
      <div className="sect__head">
        <div className="sect__title">{title}</div>
        {action}
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

function PrincipalModal({
  mode,
  initial,
  onClose,
}: {
  mode: "create" | "edit";
  initial?: Krb5Principal;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(initial?.name ?? "");
  const [type, setType] = useState(initial?.type ?? "user");
  const [password, setPassword] = useState("");

  const mut = useMutation({
    mutationFn: () =>
      mode === "create"
        ? identity.krb5CreatePrincipal({ name, password: password || undefined, type })
        : identity.krb5UpdatePrincipal(initial!.name, {
            password: password || undefined,
            type,
          }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["krb5", "principals"] });
      onClose();
    },
  });

  const valid = mode === "edit" || (name.trim().length > 0);

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="kerberos" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">
              {mode === "create" ? "Add Kerberos principal" : `Edit ${initial?.name}`}
            </div>
            <div className="muted modal__sub">
              Principals are stored in the local KDC database.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <Field label="Principal name" hint="e.g. host/nas.example.com or user@REALM">
            <input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="user@EXAMPLE.COM"
              disabled={mode === "edit"}
              autoFocus={mode === "create"}
            />
          </Field>
          <Field label="Type">
            <select
              className="input"
              value={type}
              onChange={(e) => setType(e.target.value)}
            >
              <option value="user">user</option>
              <option value="service">service</option>
              <option value="host">host</option>
            </select>
          </Field>
          <Field
            label={mode === "create" ? "Password" : "New password (optional)"}
            hint={
              mode === "create"
                ? "Leave blank to generate a random key."
                : "Leave blank to keep current key."
            }
          >
            <input
              className="input"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              autoFocus={mode === "edit"}
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
            disabled={!valid || mut.isPending}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? "Saving…" : mode === "create" ? "Create" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

function ConfigModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const cfg = useQuery({
    queryKey: ["krb5", "config"],
    queryFn: () => identity.krb5Config(),
    retry: false,
  });
  const [raw, setRaw] = useState("");

  useEffect(() => {
    if (cfg.data) {
      setRaw(
        (cfg.data.raw as string | undefined) ??
          (cfg.data.config as string | undefined) ??
          JSON.stringify(cfg.data, null, 2),
      );
    }
  }, [cfg.data]);

  const save = useMutation({
    mutationFn: () => identity.krb5UpdateConfig({ raw }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["krb5", "config"] });
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" style={{ width: 720 }} onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="settings" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Edit krb5.conf</div>
            <div className="muted modal__sub">
              Realm, KDC, and admin server settings used by Kerberos clients on this host.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          {cfg.isLoading && <div className="modal__loading muted">Loading config…</div>}
          {cfg.isError && (
            <div className="modal__err">Failed: {(cfg.error as Error).message}</div>
          )}
          {!cfg.isLoading && (
            <Field label="krb5.conf">
              <textarea
                className="input"
                rows={18}
                value={raw}
                onChange={(e) => setRaw(e.target.value)}
                style={{ fontFamily: "var(--font-mono)", fontSize: 11, resize: "vertical" }}
              />
            </Field>
          )}
          {save.isError && (
            <div className="modal__err">Save failed: {(save.error as Error).message}</div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={save.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={save.isPending || !raw.trim()}
            onClick={() => save.mutate()}
          >
            {save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

export function Kerberos() {
  const qc = useQueryClient();
  const [editing, setEditing] = useState<{ mode: "create" | "edit"; principal?: Krb5Principal } | null>(null);
  const [showConfig, setShowConfig] = useState(false);

  const status = useQuery({
    queryKey: ["krb5", "kdc-status"],
    queryFn: () => identity.krb5KdcStatus(),
    retry: false,
  });
  const config = useQuery({
    queryKey: ["krb5", "config"],
    queryFn: () => identity.krb5Config(),
    retry: false,
  });
  const idmap = useQuery({
    queryKey: ["krb5", "idmapd"],
    queryFn: () => identity.krb5Idmapd(),
    retry: false,
  });
  const principals = useQuery({
    queryKey: ["krb5", "principals"],
    queryFn: () => identity.krb5Principals(),
    retry: false,
  });
  const refreshKeytab = useMutation({
    mutationFn: (name: string) => identity.krb5RefreshKeytab(name),
  });
  const del = useMutation({
    mutationFn: (name: string) => identity.krb5DeletePrincipal(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["krb5", "principals"] }),
  });

  const online = status.data?.online ?? status.data?.status === "online";
  const pillClass = status.isError ? "pill pill--err" : online ? "pill pill--ok" : "pill pill--warn";
  const pillText = status.isError ? "KDC error" : online ? "KDC online" : status.data?.status ?? "unknown";

  return (
    <div style={{ padding: 14 }}>
      {status.data?.message && (
        <div
          className="row gap-8"
          style={{
            padding: "8px 12px",
            marginBottom: 10,
            border: "1px solid var(--line)",
            borderRadius: "var(--r-sm)",
            background: "var(--bg-1)",
            fontSize: 11,
          }}
        >
          <Icon name="info" size={12} />
          <span className="muted">{status.data.message}</span>
        </div>
      )}

      <Sect
        title="Realm"
        action={
          <div className="row gap-8">
            <span className={pillClass}>
              <span className="dot" />
              {pillText}
            </span>
            <button className="btn btn--sm" onClick={() => setShowConfig(true)}>
              <Icon name="edit" size={10} />
              Edit krb5.conf
            </button>
          </div>
        }
      >
        <dl className="kv">
          <dt>Realm</dt>
          <dd>{config.data?.realm ?? status.data?.realm ?? "—"}</dd>
          <dt>KDC</dt>
          <dd>{config.data?.kdc ?? status.data?.kdc ?? "—"}</dd>
          <dt>Admin server</dt>
          <dd>{config.data?.adminServer ?? status.data?.adminServer ?? "—"}</dd>
          <dt>Idmap domain</dt>
          <dd>{idmap.data?.domain ?? "—"}</dd>
        </dl>
      </Sect>

      <Sect
        title="Principals"
        action={
          <div className="row gap-8">
            <button
              className="btn btn--sm"
              onClick={() => principals.refetch()}
              disabled={principals.isFetching}
            >
              <Icon name="refresh" size={10} />
              Refresh
            </button>
            <button
              className="btn btn--sm btn--primary"
              onClick={() => setEditing({ mode: "create" })}
            >
              <Icon name="plus" size={9} />
              New
            </button>
          </div>
        }
      >
        {principals.isLoading && <div className="muted">Loading principals…</div>}
        {principals.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed to load: {(principals.error as Error).message}
          </div>
        )}
        {principals.data && principals.data.length === 0 && (
          <div className="muted">No principals configured.</div>
        )}
        {principals.data && principals.data.length > 0 && (
          <table className="tbl tbl--compact">
            <thead>
              <tr>
                <th>Principal</th>
                <th>Type</th>
                <th className="num">KVNO</th>
                <th>Created</th>
                <th>Expires</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {principals.data.map((p) => (
                <tr key={p.name}>
                  <td className="mono" style={{ fontSize: 11 }}>{p.name}</td>
                  <td>
                    <span className="pill">{p.type ?? "—"}</span>
                  </td>
                  <td className="num mono">{p.kvno ?? p.keyver ?? "—"}</td>
                  <td className="muted">{p.created ?? p.createdAt ?? "—"}</td>
                  <td className="muted">{p.expires ?? p.expiresAt ?? "—"}</td>
                  <td className="num">
                    <div className="row gap-8" style={{ justifyContent: "flex-end" }}>
                      <button
                        className="btn btn--sm"
                        disabled={refreshKeytab.isPending}
                        onClick={() => refreshKeytab.mutate(p.name)}
                        title="Refresh keytab"
                      >
                        <Icon name="key" size={10} />
                        Keytab
                      </button>
                      <button
                        className="btn btn--sm"
                        onClick={() => setEditing({ mode: "edit", principal: p })}
                        title="Edit"
                      >
                        <Icon name="edit" size={10} />
                      </button>
                      <button
                        className="btn btn--sm btn--danger"
                        disabled={del.isPending}
                        onClick={() => {
                          if (window.confirm(`Delete principal ${p.name}?`)) del.mutate(p.name);
                        }}
                        title="Delete"
                      >
                        <Icon name="trash" size={10} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Sect>

      {editing && (
        <PrincipalModal
          mode={editing.mode}
          initial={editing.principal}
          onClose={() => setEditing(null)}
        />
      )}
      {showConfig && <ConfigModal onClose={() => setShowConfig(false)} />}
    </div>
  );
}
