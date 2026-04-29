import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { system, type SmtpConfig } from "../../api/system";
import { Icon } from "../../components/Icon";

export function SMTP() {
  const qc = useQueryClient();
  const smtp = useQuery({
    queryKey: ["system", "smtp"],
    queryFn: () => system.getSmtp(),
  });
  const [showTest, setShowTest] = useState(false);
  const [form, setForm] = useState<SmtpConfig>({});
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (smtp.data && !dirty) setForm(smtp.data);
  }, [smtp.data, dirty]);

  const save = useMutation({
    mutationFn: () => system.saveSmtp(form),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["system", "smtp"] });
      setDirty(false);
    },
  });

  function patch<K extends keyof SmtpConfig>(k: K, v: SmtpConfig[K]) {
    setForm((f) => ({ ...f, [k]: v }));
    setDirty(true);
  }

  if (smtp.isLoading) return <div className="muted">Loading SMTP config…</div>;
  if (smtp.isError)
    return (
      <div className="muted" style={{ color: "var(--err)" }}>
        Failed to load: {(smtp.error as Error).message}
      </div>
    );

  const d = form;
  return (
    <>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Outgoing relay</div>
          <span className={`pill ${d.enabled ? "pill--ok" : ""}`}>
            <span className="dot" />
            {d.enabled ? "enabled" : "disabled"}
          </span>
        </div>
        <div className="sect__body">
          <div className="field">
            <label className="modal__checkbox">
              <input
                type="checkbox"
                checked={!!d.enabled}
                onChange={(e) => patch("enabled", e.target.checked)}
              />{" "}
              Enable SMTP relay
            </label>
          </div>
          <div className="field">
            <label className="field__label">Host</label>
            <input
              className="input"
              value={d.host ?? ""}
              onChange={(e) => patch("host", e.target.value)}
              placeholder="smtp.example.com"
            />
          </div>
          <div className="field">
            <label className="field__label">Port</label>
            <input
              className="input"
              value={d.port ?? ""}
              onChange={(e) =>
                patch("port", e.target.value ? Number(e.target.value) : undefined)
              }
              placeholder="587"
              inputMode="numeric"
            />
          </div>
          <div className="field">
            <label className="field__label">Encryption</label>
            <select
              className="input"
              value={d.encryption ?? "starttls"}
              onChange={(e) => patch("encryption", e.target.value)}
            >
              <option value="none">none</option>
              <option value="starttls">STARTTLS</option>
              <option value="tls">TLS</option>
            </select>
          </div>
          <div className="field">
            <label className="field__label">From</label>
            <input
              className="input"
              value={d.from ?? ""}
              onChange={(e) => patch("from", e.target.value)}
              placeholder="alerts@novanas.local"
            />
          </div>
          <div className="field">
            <label className="field__label">Reply-to (optional)</label>
            <input
              className="input"
              value={(d.replyTo as string | undefined) ?? ""}
              onChange={(e) =>
                setForm((f) => {
                  setDirty(true);
                  return { ...f, replyTo: e.target.value };
                })
              }
              placeholder="ops@novanas.local"
            />
          </div>
          <div className="field">
            <label className="field__label">Auth user</label>
            <input
              className="input"
              value={d.user ?? ""}
              onChange={(e) => patch("user", e.target.value)}
              placeholder="user@example.com"
              autoComplete="off"
            />
          </div>
          <div className="field">
            <label className="field__label">Auth password</label>
            <input
              className="input"
              type="password"
              value={(d.password as string | undefined) ?? ""}
              onChange={(e) => patch("password", e.target.value)}
              placeholder="••••••••"
              autoComplete="new-password"
            />
          </div>
          <div className="field">
            <dl className="kv" style={{ margin: 0 }}>
              <dt>Last test</dt>
              <dd>{d.lastTest ?? "—"}</dd>
            </dl>
          </div>
        </div>
      </div>
      {save.isError && (
        <div className="modal__err">
          Save failed: {(save.error as Error).message}
        </div>
      )}
      <div className="row gap-8" style={{ padding: "10px 16px" }}>
        <button
          className="btn btn--primary"
          disabled={!dirty || save.isPending}
          onClick={() => save.mutate()}
        >
          <Icon name="check" size={11} />
          {save.isPending ? "Saving…" : "Save"}
        </button>
        <button
          className="btn"
          onClick={() => {
            if (smtp.data) {
              setForm(smtp.data);
              setDirty(false);
            }
          }}
          disabled={!dirty}
        >
          Reset
        </button>
        <button
          className="btn"
          style={{ marginLeft: "auto" }}
          onClick={() => setShowTest(true)}
        >
          <Icon name="bell" size={11} /> Send test email
        </button>
      </div>
      {showTest && <TestModal onClose={() => setShowTest(false)} />}
    </>
  );
}

function TestModal({ onClose }: { onClose: () => void }) {
  const [to, setTo] = useState("");
  const send = useMutation({
    mutationFn: () => system.testSmtp(to.trim()),
  });
  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="bell" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Send test email</div>
            <div className="muted modal__sub">
              Sends a test message using the saved SMTP config.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Recipient</label>
            <input
              className="input"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              placeholder="you@example.com"
              autoFocus
            />
          </div>
          {send.isError && (
            <div className="modal__err">
              Failed: {(send.error as Error).message}
            </div>
          )}
          {send.isSuccess && (
            <div
              className="muted"
              style={{
                color: "var(--ok)",
                fontSize: 11,
                padding: "0 16px 8px",
              }}
            >
              Test sent. Check the inbox.
            </div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={send.isPending}>
            Close
          </button>
          <button
            className="btn btn--primary"
            disabled={!to.trim() || send.isPending}
            onClick={() => send.mutate()}
          >
            <Icon name="bell" size={11} />
            {send.isPending ? "Sending…" : "Send test"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default SMTP;
