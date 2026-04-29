import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { system, type NtpConfig } from "../../api/system";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

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

export function Timezone() {
  const info = useQuery({ queryKey: ["system", "info"], queryFn: () => system.info() });
  const ntp = useQuery({ queryKey: ["system", "ntp"], queryFn: () => system.ntp() });
  const [editTz, setEditTz] = useState(false);
  const [editNtp, setEditNtp] = useState(false);

  const tz = info.data?.timezone ?? "—";
  const drift = ntp.data?.drift ?? "—";
  const servers = (ntp.data?.servers ?? []).join(", ") || "—";
  const status = ntp.data?.enabled
    ? `active · ${servers}`
    : `disabled · ${servers}`;

  return (
    <>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Time</div>
          <div className="row gap-8" style={{ marginLeft: "auto" }}>
            <button className="btn btn--sm" onClick={() => setEditTz(true)}>
              <Icon name="edit" size={11} /> Timezone
            </button>
            <button className="btn btn--sm" onClick={() => setEditNtp(true)}>
              <Icon name="edit" size={11} /> NTP
            </button>
          </div>
        </div>
        <dl className="kv">
          <dt>Timezone</dt>
          <dd className="mono">{tz}</dd>
          <dt>NTP</dt>
          <dd>{status}</dd>
          <dt>Drift</dt>
          <dd className="mono">{drift}</dd>
        </dl>
      </div>

      {editTz && (
        <TimezoneModal current={info.data?.timezone ?? "UTC"} onClose={() => setEditTz(false)} />
      )}
      {editNtp && (
        <NtpModal current={ntp.data ?? {}} onClose={() => setEditNtp(false)} />
      )}
    </>
  );
}

function TimezoneModal({ current, onClose }: { current: string; onClose: () => void }) {
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
          <div className="modal__icon"><Icon name="globe" size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">Edit timezone</div>
            <div className="muted modal__sub">IANA tz name.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Timezone</label>
            <select className="input" value={val} onChange={(e) => setVal(e.target.value)}>
              {list.map((t) => <option key={t} value={t}>{t}</option>)}
            </select>
          </div>
          <div className="field">
            <label className="field__label">Custom (IANA)</label>
            <input className="input" value={val} onChange={(e) => setVal(e.target.value)} placeholder="Europe/Brussels" />
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

function NtpModal({ current, onClose }: { current: NtpConfig; onClose: () => void }) {
  const qc = useQueryClient();
  const [enabled, setEnabled] = useState(!!current.enabled);
  const [servers, setServers] = useState((current.servers ?? []).join(", "));
  const save = useMutation({
    meta: { label: "Save NTP failed" },
    mutationFn: () =>
      system.setNtp({
        enabled,
        servers: servers.split(",").map((s) => s.trim()).filter(Boolean),
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
          <div className="modal__icon"><Icon name="settings" size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">NTP configuration</div>
            <div className="muted modal__sub">Configure time synchronization servers.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="modal__checkbox">
              <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />{" "}
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
          {save.isError && <div className="modal__err">Failed: {(save.error as Error).message}</div>}
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

export default Timezone;
