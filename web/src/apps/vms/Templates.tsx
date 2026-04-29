import { useQuery } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { vms } from "../../api/vms";
import { Icon } from "../../components/Icon";

export function Templates() {
  const q = useQuery({
    queryKey: ["vms", "templates"],
    queryFn: () => vms.templates(),
    retry: false,
  });

  if (q.isError) {
    const err = q.error as ApiError | Error;
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 503) {
      return (
        <div style={{ padding: 14 }} className="muted">
          Templates unavailable; KubeClient not yet wired.
        </div>
      );
    }
    return (
      <div style={{ padding: 14, color: "var(--err)" }}>
        Failed to load templates: {err.message}
      </div>
    );
  }

  const items = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          <Icon name="plus" size={11} />
          New template
        </button>
      </div>
      {q.isLoading && <div className="muted">Loading templates…</div>}
      {!q.isLoading && items.length === 0 && (
        <div className="muted" style={{ padding: 12 }}>
          No VM templates defined.
        </div>
      )}
      {items.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Template</th>
              <th>OS</th>
              <th className="num">vCPU</th>
              <th className="num">RAM</th>
              <th className="num">Disk</th>
              <th>Source</th>
            </tr>
          </thead>
          <tbody>
            {items.map((t) => (
              <tr key={`${t.namespace ?? "default"}/${t.name}`}>
                <td>{t.name}</td>
                <td className="muted">{t.os ?? "—"}</td>
                <td className="num mono">{t.cpu ?? "—"}</td>
                <td className="num mono">
                  {t.ram ? `${(t.ram / 1024).toFixed(0)} GiB` : "—"}
                </td>
                <td className="num mono">{t.disk ? `${t.disk} GiB` : "—"}</td>
                <td>
                  <span className="pill">{t.source ?? "—"}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
