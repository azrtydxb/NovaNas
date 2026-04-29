import { useQuery } from "@tanstack/react-query";
import { identity, type LoginEvent } from "../../api/identity";

function when(e: LoginEvent): string {
  return e.at ?? e.timestamp ?? "—";
}
function user(e: LoginEvent): string {
  return e.user ?? e.username ?? "—";
}

export function LoginHistory() {
  const q = useQuery({
    queryKey: ["auth", "login-history"],
    queryFn: () => identity.loginHistory(),
  });

  return (
    <div style={{ padding: 14 }}>
      {q.isLoading && <div className="muted">Loading login history…</div>}
      {q.isError && (
        <div className="muted" style={{ color: "var(--err)" }}>
          Failed to load: {(q.error as Error).message}
        </div>
      )}
      {q.data && q.data.length === 0 && <div className="muted">No login events recorded.</div>}
      {q.data && q.data.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>When</th>
              <th>User</th>
              <th>IP</th>
              <th>Method</th>
              <th>Result</th>
            </tr>
          </thead>
          <tbody>
            {q.data.map((h, i) => (
              <tr key={i}>
                <td className="muted">{when(h)}</td>
                <td>{user(h)}</td>
                <td className="mono">{h.ip ?? "—"}</td>
                <td className="muted mono" style={{ fontSize: 11 }}>
                  {h.method ?? "—"}
                </td>
                <td>
                  {h.result === "success" ? (
                    <span className="pill pill--ok">
                      <span className="dot" />
                      ok
                    </span>
                  ) : (
                    <span className="pill pill--err">
                      <span className="dot" />
                      {h.result ?? "fail"}
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
