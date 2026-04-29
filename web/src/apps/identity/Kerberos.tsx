import { useMutation, useQuery } from "@tanstack/react-query";
import { identity } from "../../api/identity";
import { Icon } from "../../components/Icon";

function Sect({
  title,
  action,
  children,
}: {
  title: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="sect">
      <div className="sect__head">
        <div className="sect__title">{title}</div>
        {action}
      </div>
      <div className="sect__body">{children}</div>
    </div>
  );
}

export function Kerberos() {
  const status = useQuery({
    queryKey: ["krb5", "kdc-status"],
    queryFn: () => identity.krb5KdcStatus(),
    retry: false,
  });
  const config = useQuery({
    queryKey: ["krb5", "config"],
    queryFn: () => identity.krb5Config(),
    retry: false,
  });
  const principals = useQuery({
    queryKey: ["krb5", "principals"],
    queryFn: () => identity.krb5Principals(),
    retry: false,
  });
  const refreshKeytab = useMutation({
    mutationFn: (name: string) => identity.krb5RefreshKeytab(name),
  });

  const online = status.data?.online ?? status.data?.status === "online";
  const pillClass = status.isError ? "pill pill--err" : online ? "pill pill--ok" : "pill pill--warn";
  const pillText = status.isError ? "KDC error" : online ? "KDC online" : status.data?.status ?? "unknown";

  return (
    <div style={{ padding: 14 }}>
      <Sect
        title="Realm"
        action={
          <span className={pillClass}>
            <span className="dot" />
            {pillText}
          </span>
        }
      >
        <dl className="kv">
          <dt>Realm</dt>
          <dd>{config.data?.realm ?? status.data?.realm ?? "—"}</dd>
          <dt>KDC</dt>
          <dd>{config.data?.kdc ?? status.data?.kdc ?? "—"}</dd>
          <dt>Admin server</dt>
          <dd>{config.data?.adminServer ?? status.data?.adminServer ?? "—"}</dd>
          <dt>Idmap</dt>
          <dd>{config.data ? "cfg loaded" : "—"}</dd>
        </dl>
      </Sect>

      <Sect
        title="Principals"
        action={
          <button className="btn btn--sm btn--primary">
            <Icon name="plus" size={9} />
            New
          </button>
        }
      >
        {principals.isLoading && <div className="muted">Loading principals…</div>}
        {principals.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed to load: {(principals.error as Error).message}
          </div>
        )}
        {principals.data && principals.data.length === 0 && (
          <div className="muted">No principals configured.</div>
        )}
        {principals.data && principals.data.length > 0 && (
          <table className="tbl tbl--compact">
            <thead>
              <tr>
                <th>Principal</th>
                <th>Type</th>
                <th className="num">KVNO</th>
                <th>Created</th>
                <th>Expires</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {principals.data.map((p) => (
                <tr key={p.name}>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {p.name}
                  </td>
                  <td>
                    <span className="pill">{p.type ?? "—"}</span>
                  </td>
                  <td className="num mono">{p.kvno ?? p.keyver ?? "—"}</td>
                  <td className="muted">{p.created ?? p.createdAt ?? "—"}</td>
                  <td className="muted">{p.expires ?? p.expiresAt ?? "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm"
                      disabled={refreshKeytab.isPending}
                      onClick={() => refreshKeytab.mutate(p.name)}
                    >
                      Keytab
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Sect>
    </div>
  );
}
