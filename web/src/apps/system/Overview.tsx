import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { system, type NtpConfig } from "../../api/system";
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

const COMMON_TZ = [
  "UTC",
  "Europe/Brussels",
  "Europe/Amsterdam",
  "Europe/London",
  "Europe/Paris",
  "Europe/Berlin",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "Asia/Tokyo",
  "Asia/Shanghai",
  "Australia/Sydney",
];

export function Overview() {
  const info = useQuery({
    queryKey: ["system", "info"],
    queryFn: () => system.info(),
  });
  const ntp = useQuery({
    queryKey: ["system", "ntp"],
    queryFn: () => system.ntp(),
  });

  const [editHost, setEditHost] = useState(false);
  const [editTz, setEditTz] = useState(false);
  const [editNtp, setEditNtp] = useState(false);
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
            <button className="btn btn--sm" onClick={() => setEditTz(true)}>
              <Icon name="edit" size={11} /> Timezone
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
          <dt>ZFS</dt>
          <dd className="mono">{d.zfsVersion ?? "—"}</dd>
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
      <Sect
        title="Time / NTP"
        action={
          <button className="btn btn--sm" onClick={() => setEditNtp(true)}>
            <Icon name="edit" size={11} /> Edit
          </button>
        }
      >
        {ntp.isLoading && <div className="muted">Loading NTP…</div>}
        {ntp.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed: {(ntp.error as Error).message}
          </div>
        )}
        {ntp.data && (
          <dl className="kv">
            <dt>Status</dt>
            <dd>
              <span
                className={`pill ${ntp.data.enabled ? "pill--ok" : ""}`}
              >
                <span className="dot" />
                {ntp.data.enabled ? "enabled" : "disabled"}
              </span>{" "}
              {ntp.data.active !== undefined && (
                <span className="muted">
                  {ntp.data.active ? "active" : "inactive"}
                </span>
              )}
            </dd>
            <dt>Servers</dt>
            <dd className="mono" style={{ fontSize: 11 }}>
              {(ntp.data.servers ?? []).join(", ") || "—"}
            </dd>
            <dt>Drift</dt>
            <dd className="mono">{ntp.data.drift ?? "—"}</dd>
          </dl>
        )}
      </Sect>
      <Sect title="Power">
        <div className="row gap-8">
          <button
            className="btn btn--danger"
            onClick={() => setPwr("reboot")}
          >
            <Icon name="refresh" size={11} /> Reboot
          </button>
          <button
            className="btn btn--danger"
            onClick={() => setPwr("shutdown")}
          >
            <Icon name="power" size={11} /> Shutdown
          </button>
          <CancelShutdownBtn />
        </div>
      </Sect>

      {editHost && (
        <HostnameModal
          current={d.hostname ?? ""}
          onClose={() => setEditHost(false)}
        />
      )}
      {editTz && (
        <TimezoneModal
          current={d.timezone ?? "UTC"}
          onClose={() => setEditTz(false)}
        />
      )}
      {editNtp && (
        <NtpModal
          current={ntp.data ?? {}}
          onClose={() => setEditNtp(false)}
        />
      )}
      {pwr && <PowerModal action={pwr} onClose={() => setPwr(null)} />}
    </>
  );
}

function CancelShutdownBtn() {
  const qc = useQueryClient();
  const cancel = useMutation({
    meta: { label: "Cancel shutdown failed" },
    mutationFn: () => system.cancelShutdown(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["system"] });
      toastSuccess("Scheduled shutdown cancelled");
    },
  });
  return (
    <button
      className="btn"
      onClick={() => cancel.mutate()}
      disabled={cancel.isPending}
      style={{ marginLeft: "auto" }}
    >
      {cancel.isPending ? "Cancelling…" : "Cancel scheduled shutdown"}
    </button>
  );
}

function HostnameModal({
  current,
  onClose,
}: {
  current: string;
  onClose: () => void;
}) {
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
          <div className="modal__icon">
            <Icon name="edit" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Edit hostname</div>
            <div className="muted modal__sub">
              Sets the system hostname. Existing sessions keep the old name
              until they reconnect.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Hostname</label>
            <input
              className="input"
              value={val}
              onChange={(e) => setVal(e.target.value)}
              placeholder="novanas"
              autoFocus
            />
          </div>
          {save.isError && (
            <div className="modal__err">
              Failed: {(save.error as Error).message}
            </div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={save.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={!val.trim() || save.isPending}
            onClick={() => save.mutate()}
          >
            <Icon name="check" size={11} />
            {save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

function TimezoneModal({
  current,
  onClose,
}: {
  current: string;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [val, setVal] = useState(current);
  const save = useMutation({
    meta: { label: "Set timezone failed" },
    mutationFn: () => system.setTimezone(val),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["system"] });
      toastSuccess("Timezone updated");
      onClose();
    },
  });
  const list = COMMON_TZ.includes(val) ? COMMON_TZ : [val, ...COMMON_TZ];
  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="globe" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Edit timezone</div>
            <div className="muted modal__sub">
              IANA tz name. Affects log timestamps and scheduled jobs.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Timezone</label>
            <select
              className="input"
              value={val}
              onChange={(e) => setVal(e.target.value)}
            >
              {list.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </div>
          <div className="field">
            <label className="field__label">Custom (IANA)</label>
            <input
              className="input"
              value={val}
              onChange={(e) => setVal(e.target.value)}
              placeholder="Europe/Brussels"
            />
          </div>
          {save.isError && (
            <div className="modal__err">
              Failed: {(save.error as Error).message}
            </div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={save.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={!val.trim() || save.isPending}
            onClick={() => save.mutate()}
          >
            <Icon name="check" size={11} />
            {save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

function NtpModal({
  current,
  onClose,
}: {
  current: NtpConfig;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [enabled, setEnabled] = useState(!!current.enabled);
  const [servers, setServers] = useState((current.servers ?? []).join(", "));
  const save = useMutation({
    meta: { label: "Save NTP failed" },
    mutationFn: () =>
      system.setNtp({
        enabled,
        servers: servers
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["system"] });
      toastSuccess("NTP configuration saved");
      onClose();
    },
  });
  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="settings" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">NTP configuration</div>
            <div className="muted modal__sub">
              Configure time synchronization servers.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="modal__checkbox">
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />{" "}
              Enable NTP synchronization
            </label>
          </div>
          <div className="field">
            <label className="field__label">Servers</label>
            <input
              className="input"
              value={servers}
              onChange={(e) => setServers(e.target.value)}
              placeholder="pool.ntp.org, time.cloudflare.com"
            />
            <div className="field__hint muted">Comma separated.</div>
          </div>
          {save.isError && (
            <div className="modal__err">
              Failed: {(save.error as Error).message}
            </div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={save.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={save.isPending}
            onClick={() => save.mutate()}
          >
            <Icon name="check" size={11} />
            {save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

function PowerModal({
  action,
  onClose,
}: {
  action: "reboot" | "shutdown";
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [reason, setReason] = useState("");
  const [confirmText, setConfirmText] = useState("");
  const isReboot = action === "reboot";
  const verb = isReboot ? "Reboot" : "Shutdown";

  // suppress unused warning in some configs
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
          <div className="modal__icon">
            <Icon name={isReboot ? "refresh" : "power"} size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">{verb} system</div>
            <div className="muted modal__sub">
              All services will be {isReboot ? "restarted" : "stopped"}. In-flight
              jobs may be lost.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Reason (audit log)</label>
            <input
              className="input"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={`${verb} for maintenance window`}
              autoFocus
            />
          </div>
          <div className="field">
            <label className="field__label">
              Type <span className="mono">{verb.toUpperCase()}</span> to confirm
            </label>
            <input
              className="input"
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
            />
          </div>
          {run.isError && (
            <div className="modal__err">
              Failed: {(run.error as Error).message}
            </div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={run.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--danger"
            disabled={!can || run.isPending}
            onClick={() => run.mutate()}
          >
            <Icon name={isReboot ? "refresh" : "power"} size={11} />
            {run.isPending ? `${verb}ing…` : verb}
          </button>
        </div>
      </div>
    </div>
  );
}

export default Overview;
