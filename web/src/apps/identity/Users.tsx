import { useQuery } from "@tanstack/react-query";
import { identity, type AuthSession, type Me } from "../../api/identity";
import { Icon } from "../../components/Icon";

function pickUsername(m: Me): string {
  return (m.preferred_username as string) || (m.username as string) || (m.name as string) || (m.sub as string) || "—";
}
function pickRoles(m: Me): string {
  if (Array.isArray(m.roles) && m.roles.length > 0) return m.roles.join(", ");
  if (Array.isArray(m.groups) && m.groups.length > 0) return m.groups.join(", ");
  return "user";
}

function fmtIp(s: AuthSession): string {
  return s.ip ?? s.ipAddress ?? "—";
}
function fmtClient(s: AuthSession): string {
  return s.client ?? s.userAgent ?? "—";
}
function fmtStarted(s: AuthSession): string {
  return s.started ?? s.startedAt ?? "—";
}

export function Users() {
  const me = useQuery({ queryKey: ["auth", "me"], queryFn: () => identity.me() });
  const userId =
    (me.data?.sub as string | undefined) ??
    (me.data?.preferred_username as string | undefined) ??
    "";
  const userSessions = useQuery({
    queryKey: ["auth", "user-sessions", userId],
    queryFn: () => identity.userSessions(userId),
    enabled: !!userId,
    retry: false,
  });

  return (
    <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 12 }}>
      <div
        className="row gap-8"
        style={{
          padding: "10px 12px",
          border: "1px solid var(--line)",
          borderRadius: "var(--r-sm)",
          background: "var(--bg-1)",
        }}
      >
        <Icon name="info" size={14} />
        <div style={{ fontSize: 11, lineHeight: 1.5 }}>
          <strong>User management is delegated to Keycloak.</strong>
          <div className="muted">
            Create, edit, disable, and assign roles to users in the Keycloak admin console.
            This panel surfaces the current operator (from <span className="mono">/auth/me</span>)
            and active sessions for visibility.
          </div>
        </div>
        <a
          className="btn btn--sm"
          href="/auth/admin/"
          target="_blank"
          rel="noreferrer"
          style={{ marginLeft: "auto" }}
        >
          <Icon name="external" size={11} />
          Open Keycloak
        </a>
      </div>

      {me.isLoading && <div className="muted" style={{ padding: 12 }}>Loading…</div>}
      {me.isError && (
        <div className="muted" style={{ padding: 12, color: "var(--err)" }}>
          Failed to load: {(me.error as Error).message}
        </div>
      )}
      {me.data && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Username</th>
              <th>Role</th>
              <th>Email</th>
              <th>MFA</th>
              <th>Subject</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>
                <div className="row gap-8">
                  <div className="avatar">{pickUsername(me.data).slice(0, 2).toUpperCase()}</div>
                  {pickUsername(me.data)}
                </div>
              </td>
              <td>
                <span className="pill pill--info">{pickRoles(me.data)}</span>
              </td>
              <td className="muted">{me.data.email ?? "—"}</td>
              <td>
                {me.data.mfa ? <Icon name="shield" size={11} /> : <span className="muted">no</span>}
              </td>
              <td className="muted mono" style={{ fontSize: 11 }}>{me.data.sub ?? "—"}</td>
              <td>
                <span className="pill pill--ok">
                  <span className="dot" />
                  active
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      )}

      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">My active sessions</div>
        </div>
        <div className="sect__body">
          {userSessions.isLoading && <div className="muted">Loading sessions…</div>}
          {userSessions.isError && (
            <div className="muted" style={{ color: "var(--err)" }}>
              Sessions unavailable: {(userSessions.error as Error).message}
            </div>
          )}
          {userSessions.data && userSessions.data.length === 0 && (
            <div className="muted">No active sessions for this user.</div>
          )}
          {userSessions.data && userSessions.data.length > 0 && (
            <table className="tbl tbl--compact">
              <thead>
                <tr>
                  <th>Session</th>
                  <th>IP</th>
                  <th>Client</th>
                  <th>Started</th>
                </tr>
              </thead>
              <tbody>
                {userSessions.data.map((s) => (
                  <tr key={s.id}>
                    <td className="mono" style={{ fontSize: 11 }}>
                      {s.id}
                      {s.current && (
                        <span className="pill pill--ok" style={{ marginLeft: 6 }}>
                          current
                        </span>
                      )}
                    </td>
                    <td className="mono">{fmtIp(s)}</td>
                    <td className="muted">{fmtClient(s)}</td>
                    <td className="muted">{fmtStarted(s)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}
