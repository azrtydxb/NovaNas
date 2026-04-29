import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { notifications, type NotificationEvent } from "../../api/observability";
import { Icon } from "../../components/Icon";

function sevDot(sev?: string): string {
  if (sev === "error" || sev === "critical") return "sdot sdot--err";
  if (sev === "warn" || sev === "warning") return "sdot sdot--warn";
  if (sev === "ok" || sev === "success") return "sdot sdot--ok";
  return "sdot sdot--info";
}

function fmtAt(at?: string): string {
  if (!at) return "—";
  const d = new Date(at);
  if (isNaN(d.getTime())) return at;
  const diff = Date.now() - d.getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return d.toLocaleString(undefined, { hour12: false });
}

export default function Notifications() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["notifications", "events"],
    queryFn: () => notifications.list({ limit: 100 }),
    refetchInterval: 10_000,
    // TODO(phase-3): switch to SSE on /api/v1/notifications/events/stream.
  });

  const readAll = useMutation({
    mutationFn: () => notifications.readAll(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notifications"] }),
  });
  const markRead = useMutation({
    mutationFn: (id: string) => notifications.markRead(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notifications"] }),
  });
  const snooze = useMutation({
    mutationFn: ({ id, minutes }: { id: string; minutes: number }) =>
      notifications.snooze(id, minutes),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notifications"] }),
  });

  const list: NotificationEvent[] = q.data ?? [];
  const unread = list.filter((n) => !n.read).length;

  return (
    <div className="app-storage">
      <div className="win-body" style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <span className="muted" style={{ fontSize: 11 }}>
            {unread} unread · {list.length} total
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
            No notifications.
          </div>
        )}

        {list.length > 0 && (
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
                <tr
                  key={n.id}
                  style={{ opacity: n.read ? 0.55 : 1 }}
                  onClick={() => {
                    if (!n.read) markRead.mutate(n.id);
                  }}
                >
                  <td>
                    <span className={sevDot(n.severity)} />
                  </td>
                  <td className="muted">{fmtAt(n.at)}</td>
                  <td>
                    <span className="pill" style={{ fontSize: 9 }}>
                      {n.source ?? "system"}
                    </span>
                  </td>
                  <td>{n.title ?? n.message ?? "—"}</td>
                  <td className="muted">{n.actor ?? "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm"
                      onClick={(e) => {
                        e.stopPropagation();
                        snooze.mutate({ id: n.id, minutes: 60 });
                      }}
                    >
                      Snooze
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
