import { useQuery } from "@tanstack/react-query";
import { identity, type Me } from "../../api/identity";
import { Icon } from "../../components/Icon";

function pickUsername(m: Me): string {
  return (m.preferred_username as string) || (m.username as string) || (m.name as string) || (m.sub as string) || "—";
}
function pickRole(m: Me): string {
  if (Array.isArray(m.roles) && m.roles.length > 0) return m.roles[0];
  if (Array.isArray(m.groups) && m.groups.length > 0) return m.groups[0];
  return "user";
}

export function Users() {
  const me = useQuery({ queryKey: ["auth", "me"], queryFn: () => identity.me() });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <a
          className="btn btn--primary"
          href="/auth/admin/"
          target="_blank"
          rel="noreferrer"
        >
          <Icon name="plus" size={11} />
          New user
        </a>
        <span className="muted" style={{ fontSize: 11, marginLeft: "auto" }}>
          User management is delegated to Keycloak
        </span>
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
              <th>Last login</th>
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
                <span className={`pill pill--${pickRole(me.data) === "nova-admin" ? "warn" : "info"}`}>
                  {pickRole(me.data)}
                </span>
              </td>
              <td className="muted">{me.data.email ?? "—"}</td>
              <td>
                {me.data.mfa ? <Icon name="shield" size={11} /> : <span className="muted">no</span>}
              </td>
              <td className="muted">{me.data.lastLogin ?? "now"}</td>
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
    </div>
  );
}
