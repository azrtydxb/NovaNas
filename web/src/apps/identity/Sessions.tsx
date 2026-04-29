import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { identity, type AuthSession } from "../../api/identity";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

function fmt(s: AuthSession, key: "ip" | "user" | "client" | "started"): string {
  switch (key) {
    case "ip":
      return s.ip ?? s.ipAddress ?? "—";
    case "user":
      return s.user ?? s.username ?? "—";
    case "client":
      return s.client ?? s.userAgent ?? "—";
    case "started":
      return s.started ?? s.startedAt ?? "—";
  }
}

function Sect({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="sect">
      <div className="sect__head">
        <div className="sect__title">{title}</div>
      </div>
      <div className="sect__body">{children}</div>
    </div>
  );
}

export function Sessions() {
  const qc = useQueryClient();
  const [sel, setSel] = useState<string | null>(null);
  const q = useQuery({ queryKey: ["auth", "sessions"], queryFn: () => identity.sessions() });
  const revoke = useMutation({
    meta: { label: "Revoke session failed" },
    mutationFn: (id: string) => identity.revokeSession(id),
    onSuccess: (_d, id) => {
      qc.invalidateQueries({ queryKey: ["auth", "sessions"] });
      if (sel === id) setSel(null);
      toastSuccess("Session revoked");
    },
  });

  const items = q.data ?? [];
  const cur = items.find((s) => s.id === sel) ?? null;

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 320px", height: "100%" }}>
      <div style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <button className="btn btn--sm" onClick={() => q.refetch()} disabled={q.isFetching}>
            <Icon name="refresh" size={11} />
            Refresh
          </button>
          <span className="muted" style={{ fontSize: 11, marginLeft: "auto" }}>
            {items.length} session{items.length === 1 ? "" : "s"}
          </span>
        </div>
        {q.isLoading && <div className="muted">Loading sessions…</div>}
        {q.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed to load: {(q.error as Error).message}
          </div>
        )}
        {q.data && q.data.length === 0 && <div className="muted">No active sessions.</div>}
        {q.data && q.data.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>Session</th>
                <th>User</th>
                <th>IP</th>
                <th>Client</th>
                <th>Started</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((s) => (
                <tr
                  key={s.id}
                  onClick={() => setSel(s.id)}
                  className={sel === s.id ? "is-on" : ""}
                >
                  <td className="mono" style={{ fontSize: 11 }}>
                    {s.id}
                    {s.current && (
                      <span className="pill pill--ok" style={{ marginLeft: 6 }}>
                        current
                      </span>
                    )}
                  </td>
                  <td>{fmt(s, "user")}</td>
                  <td className="mono">{fmt(s, "ip")}</td>
                  <td className="muted">{fmt(s, "client")}</td>
                  <td className="muted">{fmt(s, "started")}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={s.current || revoke.isPending}
                      onClick={(e) => {
                        e.stopPropagation();
                        if (window.confirm(`Revoke session ${s.id}?`)) revoke.mutate(s.id);
                      }}
                    >
                      {revoke.isPending && revoke.variables === s.id ? "Revoking…" : "Revoke"}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {cur && (
        <div className="side-detail">
          <div className="side-detail__head">
            <div>
              <div className="muted mono" style={{ fontSize: 10 }}>SESSION</div>
              <div className="side-detail__title mono" style={{ fontSize: 13 }}>
                {cur.id.slice(0, 12)}…
              </div>
            </div>
            <button
              className="modal__close"
              onClick={() => setSel(null)}
              aria-label="Close"
              style={{ marginLeft: "auto" }}
            >
              <Icon name="x" size={12} />
            </button>
          </div>
          <Sect title="Identity">
            <dl className="kv">
              <dt>User</dt>
              <dd>{fmt(cur, "user")}</dd>
              <dt>Current</dt>
              <dd>{cur.current ? "yes" : "no"}</dd>
            </dl>
          </Sect>
          <Sect title="Client">
            <dl className="kv">
              <dt>IP</dt>
              <dd className="mono">{fmt(cur, "ip")}</dd>
              <dt>Agent</dt>
              <dd className="muted" style={{ fontSize: 11 }}>{fmt(cur, "client")}</dd>
            </dl>
          </Sect>
          <Sect title="Lifetime">
            <dl className="kv">
              <dt>Started</dt>
              <dd>{fmt(cur, "started")}</dd>
              <dt>Expires</dt>
              <dd>{cur.expiresAt ?? "—"}</dd>
            </dl>
          </Sect>
          <div
            className="row gap-8"
            style={{ padding: "10px 12px", borderTop: "1px solid var(--line)" }}
          >
            <button
              className="btn btn--sm btn--danger"
              disabled={cur.current || revoke.isPending}
              onClick={() => {
                if (window.confirm(`Revoke session ${cur.id}?`)) revoke.mutate(cur.id);
              }}
              style={{ marginLeft: "auto" }}
            >
              <Icon name="trash" size={11} />
              Revoke session
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
