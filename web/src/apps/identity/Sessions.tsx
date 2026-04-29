import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { identity, type AuthSession } from "../../api/identity";

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

export function Sessions() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["auth", "sessions"], queryFn: () => identity.sessions() });
  const revoke = useMutation({
    mutationFn: (id: string) => identity.revokeSession(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["auth", "sessions"] }),
  });

  return (
    <div style={{ padding: 14 }}>
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
              <tr key={s.id}>
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
                    onClick={() => revoke.mutate(s.id)}
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
  );
}
