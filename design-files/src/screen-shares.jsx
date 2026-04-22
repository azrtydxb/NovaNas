/* globals React, Icon, Pill */
function SharesScreen() {
  const shares = [
    { name: "family-media", proto: "SMB", path: "/family-media", users: 4, acl: "posix", state: "ok" },
    { name: "family-media", proto: "NFS", path: "/mnt/family-media", users: 2, acl: "nfsv4", state: "ok" },
    { name: "pascal-docs",  proto: "SMB", path: "/pascal/docs",  users: 1, acl: "posix", state: "ok" },
    { name: "backups",      proto: "S3",  path: "s3://backups",  users: 2, acl: "iam",   state: "ok" },
    { name: "vm-store",     proto: "iSCSI", path: "iqn.2026-04.io.novanas:vm-store", users: 1, acl: "chap", state: "ok" },
  ];
  return (
    <>
      <div className="page-head">
        <div><h1>Shares</h1><div className="page-head__sub">SMB · NFS · iSCSI · S3</div></div>
        <div className="page-head__actions">
          <button className="btn"><Icon name="filter" size={13}/> Filter</button>
          <button className="btn btn--primary"><Icon name="plus" size={13}/> New share</button>
        </div>
      </div>
      <div className="card">
        <table className="tbl">
          <thead><tr><th>Name</th><th>Protocol</th><th>Path / target</th><th>ACL</th><th className="num">Users</th><th>State</th><th></th></tr></thead>
          <tbody>
            {shares.map((s, i) => (
              <tr key={i}>
                <td className="fg0">{s.name}</td>
                <td><span className="chip">{s.proto}</span></td>
                <td className="mono" style={{ color: "var(--fg-2)" }}>{s.path}</td>
                <td><span className="chip">{s.acl}</span></td>
                <td className="num">{s.users}</td>
                <td><Pill tone="ok" dot>online</Pill></td>
                <td><button className="btn btn--ghost btn--sm"><Icon name="more" size={12}/></button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
window.SharesScreen = SharesScreen;
