import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  notifications,
  type NotificationEvent,
} from "../../api/observability";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

function sevDot(sev?: string): string {
  if (sev === "error" || sev === "critical") return "sdot sdot--err";
  if (sev === "warn" || sev === "warning") return "sdot sdot--warn";
  if (sev === "ok" || sev === "success") return "sdot sdot--ok";
  return "sdot sdot--info";
}

function fmtAgo(at?: string): string {
  if (!at) return "—";
  const d = new Date(at);
  if (isNaN(d.getTime())) return at;
  const diff = Date.now() - d.getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const days = Math.floor(h / 24);
  if (days < 7) return `${days}d ago`;
  return d.toLocaleDateString();
}

const SNOOZE_OPTIONS: { label: string; minutes: number }[] = [
  { label: "15 minutes", minutes: 15 },
  { label: "1 hour", minutes: 60 },
  { label: "4 hours", minutes: 240 },
  { label: "1 day", minutes: 60 * 24 },
];

type SnoozeModalProps = {
  event: NotificationEvent;
  onClose: () => void;
  onPick: (minutes: number) => void;
  pending: boolean;
};

function SnoozeModal({ event, onClose, onPick, pending }: SnoozeModalProps) {
  return (
    <div className="modal-bg" onMouseDown={onClose}>
      <div className="modal" style={{ width: 380 }} onMouseDown={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon"><Icon name="bell" size={18} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">Snooze notification</div>
            <div className="modal__sub muted">{event.title ?? event.message ?? event.id}</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body" style={{ padding: 12 }}>
          <div className="row gap-8" style={{ flexWrap: "wrap" }}>
            {SNOOZE_OPTIONS.map((o) => (
              <button
                key={o.minutes}
                className="btn btn--sm"
                disabled={pending}
                onClick={() => onPick(o.minutes)}
                style={{ flex: "1 1 45%" }}
              >
                {o.label}
              </button>
            ))}
          </div>
        </div>
        <div className="modal__foot">
          <button className="btn btn--sm" onClick={onClose}>Cancel</button>
        </div>
      </div>
    </div>
  );
}

export default function Notifications() {
  const qc = useQueryClient();
  const [snoozeFor, setSnoozeFor] = useState<NotificationEvent | null>(null);
  const [showSettings, setShowSettings] = useState(false);

  const q = useQuery({
    queryKey: ["notifications", "events"],
    queryFn: () => notifications.list({ limit: 200 }),
    refetchInterval: 10_000,
  });

  const readAll = useMutation({
    meta: { label: "Mark all read failed" },
    mutationFn: () => notifications.readAll(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notifications"] });
      toastSuccess("All notifications marked read");
    },
  });
  const snooze = useMutation({
    meta: { label: "Snooze failed" },
    mutationFn: ({ id, minutes }: { id: string; minutes: number }) =>
      notifications.snooze(id, minutes),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notifications"] });
      toastSuccess("Notification snoozed");
      setSnoozeFor(null);
    },
  });

  const list: NotificationEvent[] = q.data ?? [];
  const unread = list.filter((n) => !n.read).length;

  return (
    <div className="app-storage">
      <div className="win-body" style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <span className="muted" style={{ fontSize: 11 }}>{unread} unread</span>
          <button
            className="btn btn--sm"
            disabled={readAll.isPending || unread === 0}
            onClick={() => readAll.mutate()}
          >
            Mark all read
          </button>
          <button
            className="btn btn--sm"
            style={{ marginLeft: "auto" }}
            onClick={() => setShowSettings(true)}
          >
            Settings
          </button>
        </div>
        <table className="tbl">
          <thead>
            <tr>
              <th></th>
              <th>Time</th>
              <th>Source</th>
              <th>Message</th>
              <th>Actor</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.map((n) => (
              <tr key={n.id} style={{ opacity: n.read ? 0.55 : 1 }}>
                <td><span className={sevDot(n.severity)} /></td>
                <td className="muted">{fmtAgo(n.at)}</td>
                <td><span className="pill" style={{ fontSize: 9 }}>{n.source ?? "system"}</span></td>
                <td>{n.title ?? n.message ?? "(no title)"}</td>
                <td className="muted">{n.actor ?? ""}</td>
                <td className="num">
                  <button className="btn btn--sm" onClick={() => setSnoozeFor(n)}>Snooze</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {q.isLoading && <div className="muted" style={{ padding: 8 }}>Loading notifications…</div>}
        {q.isError && (
          <div className="muted" style={{ padding: 8, color: "var(--err)" }}>
            Failed to load: {(q.error as Error).message}
          </div>
        )}
        {q.data && list.length === 0 && (
          <div className="muted" style={{ padding: 20 }}>No notifications.</div>
        )}
      </div>

      {snoozeFor && (
        <SnoozeModal
          event={snoozeFor}
          onClose={() => setSnoozeFor(null)}
          pending={snooze.isPending}
          onPick={(minutes) => snooze.mutate({ id: snoozeFor.id, minutes })}
        />
      )}
      {showSettings && (
        <div className="modal-bg" onMouseDown={() => setShowSettings(false)}>
          <div className="modal" onMouseDown={(e) => e.stopPropagation()}>
            <div className="modal__head">
              <div className="modal__icon"><Icon name="bell" size={18} /></div>
              <div className="modal__head-meta">
                <div className="modal__title">Notification settings</div>
                <div className="modal__sub muted">Configured in System · SMTP and channels.</div>
              </div>
              <button className="modal__close" onClick={() => setShowSettings(false)}>
                <Icon name="x" size={14} />
              </button>
            </div>
            <div className="modal__body">
              <div className="muted" style={{ padding: 16, fontSize: 11 }}>
                Open the System app to configure SMTP relay and notification channels.
              </div>
            </div>
            <div className="modal__foot">
              <button className="btn btn--sm" onClick={() => setShowSettings(false)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
