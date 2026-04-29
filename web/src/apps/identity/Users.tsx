import { useQuery } from "@tanstack/react-query";
import { identity, type Me } from "../../api/identity";
import { Icon } from "../../components/Icon";

// TODO: Keycloak admin pass-through for user listing.
// Until then, render the current user from /auth/me as the only row.

function pickUsername(m: Me): string {
  return (m.preferred_username as string) || (m.username as string) || (m.name as string) || (m.sub as string) || "—";
}

function pickRoles(m: Me): string {
  if (Array.isArray(m.roles) && m.roles.length > 0) return m.roles.join(", ");
  if (Array.isArray(m.groups) && m.groups.length > 0) return m.groups.join(", ");
  return "user";
}

export function Users() {
  const me = useQuery({ queryKey: ["auth", "me"], queryFn: () => identity.me() });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          <Icon name="plus" size={11} />
          New user
        </button>
        <span className="muted" style={{ fontSize: 11, marginLeft: "auto" }}>
          User management is delegated to Keycloak admin
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
                <span className="pill pill--info">{pickRoles(me.data)}</span>
              </td>
              <td className="muted">{me.data.email ?? "—"}</td>
              <td>
                {me.data.mfa ? <Icon name="shield" size={11} /> : <span className="muted">no</span>}
              </td>
              <td className="muted">{me.data.lastLogin ?? "—"}</td>
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
