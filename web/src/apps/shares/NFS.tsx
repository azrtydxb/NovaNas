import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares } from "../../api/shares";

export function NFS() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["nfs-exports"],
    queryFn: () => shares.listNfs(),
  });
  const list = q.data ?? [];

  const delMut = useMutation({
    mutationFn: (name: string) => shares.deleteNfs(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["nfs-exports"] }),
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          {/* TODO: phase 3 — open create-NFS dialog */}
          <Icon name="plus" size={11} />
          New export
        </button>
        <button className="btn">Reload</button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading NFS exports…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && list.length === 0 && (
        <div className="empty-hint">No NFS exports.</div>
      )}
      {list.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Export</th>
              <th>Path</th>
              <th>Clients</th>
              <th>Options</th>
              <th>Active</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.map((n) => (
              <tr key={n.name}>
                <td>{n.name}</td>
                <td className="mono muted" style={{ fontSize: 11 }}>
                  {n.path ?? "—"}
                </td>
                <td className="mono">{n.clients ?? "—"}</td>
                <td className="mono muted" style={{ fontSize: 11 }}>
                  {n.options ?? "—"}
                </td>
                <td>
                  {n.active ? (
                    <span className="pill pill--ok">
                      <span className="dot" />
                      up
                    </span>
                  ) : (
                    <span className="muted">off</span>
                  )}
                </td>
                <td className="num">
                  <button
                    className="btn btn--sm btn--danger"
                    disabled={delMut.isPending}
                    onClick={() => delMut.mutate(n.name)}
                  >
                    Delete
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

export default NFS;
