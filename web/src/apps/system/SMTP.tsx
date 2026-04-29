import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { system, type SmtpConfig } from "../../api/system";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

export function SMTP() {
  const smtp = useQuery({
    queryKey: ["system", "smtp"],
    queryFn: () => system.getSmtp(),
  });
  const [showTest, setShowTest] = useState(false);
  const [showEdit, setShowEdit] = useState(false);

  if (smtp.isLoading) return <div className="muted">Loading SMTP config…</div>;
  if (smtp.isError)
    return (
      <div className="muted" style={{ color: "var(--err)" }}>
        Failed to load: {(smtp.error as Error).message}
      </div>
    );

  const d = smtp.data ?? {};

  return (
    <>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Outgoing relay</div>
          <span className={`pill pill--${d.enabled ? "ok" : ""}`}>
            <span className="dot" />
            {d.enabled ? "enabled" : "disabled"}
          </span>
        </div>
        <dl className="kv">
          <dt>Host</dt>
          <dd className="mono">{d.host ?? "—"}</dd>
          <dt>Port</dt>
          <dd className="mono">{d.port ?? "—"}</dd>
          <dt>Encryption</dt>
          <dd>{d.encryption ?? "—"}</dd>
          <dt>From</dt>
          <dd className="mono">{d.from ?? "—"}</dd>
          <dt>Auth</dt>
          <dd className="mono">{d.user ?? "—"}</dd>
          <dt>Last test</dt>
          <dd>{d.lastTest ?? "—"}</dd>
        </dl>
      </div>
      <div className="row gap-8">
        <button className="btn btn--primary" onClick={() => setShowTest(true)}>
          Send test email
        </button>
        <button className="btn" onClick={() => setShowEdit(true)}>Edit</button>
      </div>

      {showTest && <TestModal onClose={() => setShowTest(false)} />}
      {showEdit && <EditModal current={d} onClose={() => setShowEdit(false)} />}
    </>
  );
}

function EditModal({ current, onClose }: { current: SmtpConfig; onClose: () => void }) {
  const qc = useQueryClient();
  const [form, setForm] = useState<SmtpConfig>(current);

  useEffect(() => setForm(current), [current]);

  const save = useMutation({
    meta: { label: "Save SMTP failed" },
    mutationFn: () => system.saveSmtp(form),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["system", "smtp"] });
      toastSuccess("SMTP configuration saved");
      onClose();
    },
  });

  function patch<K extends keyof SmtpConfig>(k: K, v: SmtpConfig[K]) {
    setForm((f) => ({ ...f, [k]: v }));
  }

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon"><Icon name="edit" size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">Edit SMTP relay</div>
            <div className="muted modal__sub">Configure outgoing email server.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="modal__checkbox">
              <input type="checkbox" checked={!!form.enabled} onChange={(e) => patch("enabled", e.target.checked)} />{" "}
              Enable SMTP relay
            </label>
          </div>
          <div className="field">
            <label className="field__label">Host</label>
            <input className="input" value={form.host ?? ""} onChange={(e) => patch("host", e.target.value)} placeholder="smtp.example.com" />
          </div>
          <div className="field">
            <label className="field__label">Port</label>
            <input
              className="input"
              value={form.port ?? ""}
              onChange={(e) => patch("port", e.target.value ? Number(e.target.value) : undefined)}
              placeholder="587"
              inputMode="numeric"
            />
          </div>
          <div className="field">
            <label className="field__label">Encryption</label>
            <select className="input" value={form.encryption ?? "starttls"} onChange={(e) => patch("encryption", e.target.value)}>
              <option value="none">none</option>
              <option value="starttls">STARTTLS</option>
              <option value="tls">TLS</option>
            </select>
          </div>
          <div className="field">
            <label className="field__label">From</label>
            <input className="input" value={form.from ?? ""} onChange={(e) => patch("from", e.target.value)} placeholder="alerts@novanas.local" />
          </div>
          <div className="field">
            <label className="field__label">Auth user</label>
            <input className="input" value={form.user ?? ""} onChange={(e) => patch("user", e.target.value)} autoComplete="off" />
          </div>
          <div className="field">
            <label className="field__label">Auth password</label>
            <input
              className="input"
              type="password"
              value={(form.password as string | undefined) ?? ""}
              onChange={(e) => patch("password", e.target.value)}
              placeholder="••••••••"
              autoComplete="new-password"
            />
          </div>
          {save.isError && <div className="modal__err">Save failed: {(save.error as Error).message}</div>}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={save.isPending}>Cancel</button>
          <button className="btn btn--primary" disabled={save.isPending} onClick={() => save.mutate()}>
            <Icon name="check" size={11} />{save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

function TestModal({ onClose }: { onClose: () => void }) {
  const [to, setTo] = useState("");
  const send = useMutation({
    meta: { label: "Send test email failed" },
    mutationFn: () => system.testSmtp(to.trim()),
    onSuccess: () => toastSuccess("Test email sent"),
  });
  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon"><Icon name="bell" size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">Send test email</div>
            <div className="muted modal__sub">Sends a test message using the saved SMTP config.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Recipient</label>
            <input className="input" value={to} onChange={(e) => setTo(e.target.value)} placeholder="you@example.com" autoFocus />
          </div>
          {send.isError && <div className="modal__err">Failed: {(send.error as Error).message}</div>}
          {send.isSuccess && (
            <div className="muted" style={{ color: "var(--ok)", fontSize: 11, padding: "0 16px 8px" }}>
              Test sent. Check the inbox.
            </div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={send.isPending}>Close</button>
          <button className="btn btn--primary" disabled={!to.trim() || send.isPending} onClick={() => send.mutate()}>
            <Icon name="bell" size={11} />{send.isPending ? "Sending…" : "Send test"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default SMTP;
