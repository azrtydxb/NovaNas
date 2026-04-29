import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { system } from "../../api/system";
import { Icon } from "../../components/Icon";
import { formatBytes } from "../../lib/format";
import { toastSuccess } from "../../store/toast";

function formatUptime(v: string | number | undefined): string {
  if (v === undefined || v === null) return "—";
  if (typeof v === "string") return v;
  const s = Math.floor(v);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatMem(v: string | number | undefined): string {
  if (v === undefined || v === null) return "—";
  if (typeof v === "string") return v;
  return formatBytes(v);
}

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
        {action && (
          <div className="row gap-8" style={{ marginLeft: "auto" }}>
            {action}
          </div>
        )}
      </div>
      <div className="sect__body">{children}</div>
    </div>
  );
}

export function Overview() {
  const info = useQuery({
    queryKey: ["system", "info"],
    queryFn: () => system.info(),
  });

  const [editHost, setEditHost] = useState(false);
  const [pwr, setPwr] = useState<null | "reboot" | "shutdown">(null);

  if (info.isLoading) return <div className="muted">Loading system info…</div>;
  if (info.isError)
    return (
      <div className="muted" style={{ color: "var(--err)" }}>
        Failed to load: {(info.error as Error).message}
      </div>
    );

  const d = info.data ?? {};
  return (
    <>
      <Sect
        title="Host"
        action={
          <>
            <button className="btn btn--sm" onClick={() => setEditHost(true)}>
              <Icon name="edit" size={11} /> Hostname
            </button>
            <button className="btn btn--sm btn--danger" onClick={() => setPwr("reboot")}>
              <Icon name="refresh" size={11} /> Reboot
            </button>
            <button className="btn btn--sm btn--danger" onClick={() => setPwr("shutdown")}>
              <Icon name="power" size={11} /> Shutdown
            </button>
          </>
        }
      >
        <dl className="kv">
          <dt>Hostname</dt>
          <dd className="mono">{d.hostname ?? "—"}</dd>
          <dt>Version</dt>
          <dd>{d.version ?? "—"}</dd>
          <dt>Kernel</dt>
          <dd className="mono">{d.kernel ?? "—"}</dd>
          <dt>OS</dt>
          <dd>{d.os ?? "—"}</dd>
          <dt>Uptime</dt>
          <dd>{formatUptime(d.uptime)}</dd>
          <dt>Timezone</dt>
          <dd className="mono">{d.timezone ?? "—"}</dd>
        </dl>
      </Sect>
      <Sect title="Hardware">
        <dl className="kv">
          <dt>CPU</dt>
          <dd>{d.cpu ?? "—"}</dd>
          <dt>Cores / threads</dt>
          <dd className="mono">
            {d.cores ?? "—"} / {d.threads ?? "—"}
          </dd>
          <dt>Memory</dt>
          <dd className="mono">{formatMem(d.memory)}</dd>
          <dt>BMC</dt>
          <dd className="mono">{d.bmc ?? "—"}</dd>
        </dl>
      </Sect>
      <Sect title="Security">
        <dl className="kv">
          <dt>TPM</dt>
          <dd>{d.tpm ?? "—"}</dd>
          <dt>Secure Boot</dt>
          <dd>{d.secureBoot ?? "—"}</dd>
        </dl>
      </Sect>

      {editHost && (
        <HostnameModal current={d.hostname ?? ""} onClose={() => setEditHost(false)} />
      )}
      {pwr && <PowerModal action={pwr} onClose={() => setPwr(null)} />}
    </>
  );
}

function HostnameModal({ current, onClose }: { current: string; onClose: () => void }) {
  const qc = useQueryClient();
  const [val, setVal] = useState(current);
  const save = useMutation({
    meta: { label: "Set hostname failed" },
    mutationFn: () => system.setHostname(val.trim()),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["system"] });
      toastSuccess("Hostname updated");
      onClose();
    },
  });
  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon"><Icon name="edit" size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">Edit hostname</div>
            <div className="muted modal__sub">Sets the system hostname.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Hostname</label>
            <input className="input" value={val} onChange={(e) => setVal(e.target.value)} placeholder="novanas" autoFocus />
          </div>
          {save.isError && <div className="modal__err">Failed: {(save.error as Error).message}</div>}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={save.isPending}>Cancel</button>
          <button className="btn btn--primary" disabled={!val.trim() || save.isPending} onClick={() => save.mutate()}>
            <Icon name="check" size={11} />{save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

function PowerModal({ action, onClose }: { action: "reboot" | "shutdown"; onClose: () => void }) {
  const qc = useQueryClient();
  const [reason, setReason] = useState("");
  const [confirmText, setConfirmText] = useState("");
  const isReboot = action === "reboot";
  const verb = isReboot ? "Reboot" : "Shutdown";

  useEffect(() => undefined, []);

  const run = useMutation({
    meta: { label: `${verb} failed` },
    mutationFn: () =>
      isReboot
        ? system.reboot(reason || "operator initiated")
        : system.shutdown(reason || "operator initiated"),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["system"] });
      toastSuccess(isReboot ? "Reboot initiated" : "Shutdown initiated");
      onClose();
    },
  });

  const can = confirmText.trim().toUpperCase() === verb.toUpperCase();

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon"><Icon name={isReboot ? "refresh" : "power"} size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">{verb} system</div>
            <div className="muted modal__sub">All services will be {isReboot ? "restarted" : "stopped"}.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Reason (audit log)</label>
            <input className="input" value={reason} onChange={(e) => setReason(e.target.value)} placeholder={`${verb} for maintenance window`} autoFocus />
          </div>
          <div className="field">
            <label className="field__label">Type <span className="mono">{verb.toUpperCase()}</span> to confirm</label>
            <input className="input" value={confirmText} onChange={(e) => setConfirmText(e.target.value)} />
          </div>
          {run.isError && <div className="modal__err">Failed: {(run.error as Error).message}</div>}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={run.isPending}>Cancel</button>
          <button className="btn btn--danger" disabled={!can || run.isPending} onClick={() => run.mutate()}>
            <Icon name={isReboot ? "refresh" : "power"} size={11} />{run.isPending ? `${verb}ing…` : verb}
          </button>
        </div>
      </div>
    </div>
  );
}

export default Overview;
