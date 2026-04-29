import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { Icon } from "../components/Icon";

type Notif = {
  id: string;
  severity: "info" | "warning" | "error" | "critical";
  title: string;
  body?: string;
  source?: string;
  actor?: string;
  createdAt: string;
  read?: boolean;
};

export function BellDrawer({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const q = useQuery<Notif[]>({
    queryKey: ["notifications", "bell"],
    queryFn: () => api<Notif[]>("/api/v1/notifications/events?limit=20"),
    refetchInterval: 30_000,
  });
  const markAll = useMutation({
    mutationFn: () => api("/api/v1/notifications/events/read-all", { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notifications"] }),
  });

  return (
    <div className="bell-drawer-bg" onClick={onClose}>
      <div className="bell-drawer" onClick={(e) => e.stopPropagation()}>
        <div className="bell-drawer__head">
          <span className="bell-drawer__title">Notifications</span>
          <button
            className="btn btn--sm"
            onClick={() => markAll.mutate()}
            disabled={markAll.isPending}
          >
            Mark all read
          </button>
        </div>
        <div className="bell-drawer__list">
          {q.isLoading && <div className="bell-drawer__msg">Loading…</div>}
          {q.isError && <div className="bell-drawer__msg muted">No notifications service</div>}
          {q.data && q.data.length === 0 && (
            <div className="bell-drawer__msg muted">All caught up.</div>
          )}
          {q.data?.map((n) => (
            <div key={n.id} className={`notif-item${n.read ? "" : " is-unread"}`}>
              <span className={`dot dot--${n.severity}`} />
              <div className="notif-item__body">
                <div className="notif-item__title">{n.title}</div>
                {n.body && <div className="notif-item__sub muted">{n.body}</div>}
                <div className="notif-item__meta muted">
                  {new Date(n.createdAt).toLocaleString()}
                  {n.source && <> · {n.source}</>}
                </div>
              </div>
              <Icon name="chev" size={11} style={{ color: "var(--fg-3)" }} />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
