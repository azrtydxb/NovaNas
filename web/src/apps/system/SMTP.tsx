import { useQuery } from "@tanstack/react-query";
import { system } from "../../api/system";

export function SMTP() {
  const smtp = useQuery({
    queryKey: ["system", "smtp"],
    queryFn: () => system.getSmtp(),
  });

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
          <span className={`pill ${d.enabled ? "pill--ok" : ""}`}>
            <span className="dot" />
            {d.enabled ? "enabled" : "disabled"}
          </span>
        </div>
        <div className="sect__body">
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
      </div>
      <div className="row gap-8">
        <button className="btn btn--primary">Send test email</button>
        <button className="btn">Edit</button>
      </div>
    </>
  );
}

export default SMTP;
