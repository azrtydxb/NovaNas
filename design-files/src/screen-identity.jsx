/* globals React, Icon, Pill */
function IdentityScreen() {
  const users = [
    { name: "pascal", role: "admin", source: "local", mfa: "WebAuthn", lastLogin: "14:01" },
    { name: "eli", role: "member", source: "local", mfa: "TOTP", lastLogin: "yesterday" },
    { name: "family", role: "member", source: "local", mfa: "—", lastLogin: "3 days ago" },
    { name: "svc-backup", role: "service", source: "local", mfa: "—", lastLogin: "13:58" },
    { name: "j.doe", role: "viewer", source: "OIDC (Google)", mfa: "—", lastLogin: "last week" },
  ];
  return (
    <>
      <div className="page-head">
        <div><h1>Identity</h1><div className="page-head__sub">Keycloak · OIDC · WebAuthn · TPM-unsealed OpenBao</div></div>
        <div className="page-head__actions">
          <button className="btn"><Icon name="key" size={13}/> API tokens</button>
          <button className="btn btn--primary"><Icon name="plus" size={13}/> Invite user</button>
        </div>
      </div>
      <div className="card">
        <table className="tbl">
          <thead><tr><th>User</th><th>Role</th><th>Source</th><th>MFA</th><th>Last login</th><th></th></tr></thead>
          <tbody>
            {users.map(u => (
              <tr key={u.name}>
                <td><div className="row gap-8">
                  <div className="user-chip__avatar" style={{ width: 22, height: 22 }}>{u.name.slice(0,2).toUpperCase()}</div>
                  <span className="fg0">{u.name}</span>
                </div></td>
                <td><span className="chip">{u.role}</span></td>
                <td className="mono muted">{u.source}</td>
                <td>{u.mfa !== "—" ? <Pill tone="ok" dot>{u.mfa}</Pill> : <span className="muted">—</span>}</td>
                <td className="mono muted">{u.lastLogin}</td>
                <td><button className="btn btn--ghost btn--sm"><Icon name="more" size={12}/></button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
window.IdentityScreen = IdentityScreen;
