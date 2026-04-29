import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  notifications,
  type NotificationEvent,
} from "../../api/observability";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

type FilterMode = "all" | "unread" | "snoozed";

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
      <div
        className="modal"
        style={{ width: 380 }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="bell" size={18} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Snooze notification</div>
            <div className="modal__sub muted">
              {event.title ?? event.message ?? event.id}
            </div>
          </div>
          <button className="modal__close" onClick={onClose}>
            <Icon name="x" size={14} />
          </button>
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
          <button className="btn btn--sm" onClick={onClose}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}

export default function Notifications() {
  const qc = useQueryClient();
  const [filter, setFilter] = useState<FilterMode>("all");
  const [snoozeFor, setSnoozeFor] = useState<NotificationEvent | null>(null);

  const q = useQuery({
    queryKey: ["notifications", "events", filter],
    queryFn: () =>
      notifications.list({
        unread: filter === "unread" ? true : undefined,
        limit: 200,
      }),
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
  const markRead = useMutation({
    meta: { label: "Mark read failed" },
    mutationFn: (id: string) => notifications.markRead(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notifications"] }),
  });
  const dismiss = useMutation({
    meta: { label: "Dismiss failed" },
    mutationFn: (id: string) => notifications.dismiss(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notifications"] });
      toastSuccess("Notification dismissed");
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

  const all: NotificationEvent[] = q.data ?? [];

  const list = useMemo(() => {
    if (filter === "snoozed") {
      return all.filter((n) => {
        const r = n as NotificationEvent & { snoozedUntil?: string };
        return !!r.snoozedUntil;
      });
    }
    return all;
  }, [all, filter]);

  const unread = all.filter((n) => !n.read).length;

  return (
    <div className="app-storage">
      <div className="win-body" style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <div className="row gap-4">
            {(["all", "unread", "snoozed"] as const).map((f) => (
              <button
                key={f}
                className={`btn btn--sm ${filter === f ? "btn--primary" : ""}`}
                onClick={() => setFilter(f)}
              >
                {f}
              </button>
            ))}
          </div>
          <span className="muted" style={{ fontSize: 11 }}>
            {unread} unread · {all.length} total
          </span>
          <button
            className="btn btn--sm"
            disabled={readAll.isPending || unread === 0}
            onClick={() => readAll.mutate()}
          >
            <Icon name="check" size={11} /> Mark all read
          </button>
          <button
            className="btn btn--sm"
            style={{ marginLeft: "auto" }}
            onClick={() => qc.invalidateQueries({ queryKey: ["notifications"] })}
          >
            <Icon name="refresh" size={11} /> Refresh
          </button>
        </div>

        {q.isLoading && <div className="muted">Loading notifications…</div>}
        {q.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed to load: {(q.error as Error).message}
          </div>
        )}
        {q.data && list.length === 0 && (
          <div className="muted" style={{ padding: "20px 0" }}>
            {filter === "unread"
              ? "No unread notifications."
              : filter === "snoozed"
                ? "No snoozed notifications."
                : "No notifications."}
          </div>
        )}

        {list.length > 0 && (
          <div className="notif-list">
            {list.map((n) => (
              <div
                key={n.id}
                className={`notif-item ${n.read ? "" : "is-unread"}`}
                onClick={() => {
                  if (!n.read) markRead.mutate(n.id);
                }}
              >
                <span
                  className={sevDot(n.severity)}
                  style={{ marginTop: 4 }}
                />
                <div className="notif-item__body">
                  <div className="notif-item__head">
                    <div className="notif-item__title">
                      {n.title ?? n.message ?? "(no title)"}
                    </div>
                    <span className="notif-item__time">{fmtAgo(n.at)}</span>
                  </div>
                  {n.title && n.message && n.title !== n.message && (
                    <div className="notif-item__sub muted">{n.message}</div>
                  )}
                  <div className="notif-item__meta muted">
                    <span>{n.source ?? "system"}</span>
                    {n.actor && <span> · {n.actor}</span>}
                  </div>
                  <div className="notif-item__actions">
                    {!n.read && (
                      <button
                        className="btn btn--sm"
                        onClick={(e) => {
                          e.stopPropagation();
                          markRead.mutate(n.id);
                        }}
                      >
                        <Icon name="check" size={10} /> Read
                      </button>
                    )}
                    <button
                      className="btn btn--sm"
                      onClick={(e) => {
                        e.stopPropagation();
                        setSnoozeFor(n);
                      }}
                    >
                      <Icon name="pause" size={10} /> Snooze
                    </button>
                    <button
                      className="btn btn--sm btn--danger"
                      onClick={(e) => {
                        e.stopPropagation();
                        dismiss.mutate(n.id);
                      }}
                    >
                      <Icon name="x" size={10} /> Dismiss
                    </button>
                    {n.url && (
                      <a
                        className="btn btn--sm"
                        href={n.url}
                        target="_blank"
                        rel="noreferrer"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <Icon name="external" size={10} /> Open
                      </a>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {snoozeFor && (
        <SnoozeModal
          event={snoozeFor}
          onClose={() => setSnoozeFor(null)}
          pending={snooze.isPending}
          onPick={(minutes) =>
            snooze.mutate({ id: snoozeFor.id, minutes })
          }
        />
      )}
    </div>
  );
}
