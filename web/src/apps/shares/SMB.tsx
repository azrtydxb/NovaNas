import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares } from "../../api/shares";

export function SMB() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["smb-shares"],
    queryFn: () => shares.listSmb(),
  });
  const list = q.data ?? [];

  const delMut = useMutation({
    mutationFn: (name: string) => shares.deleteSmb(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["smb-shares"] }),
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          {/* TODO: phase 3 — open create-SMB dialog */}
          <Icon name="plus" size={11} />
          New SMB share
        </button>
        <button className="btn">Globals…</button>
        <button className="btn">Users…</button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading SMB shares…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && list.length === 0 && (
        <div className="empty-hint">No SMB shares.</div>
      )}
      {list.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Share</th>
              <th>Path</th>
              <th>Users</th>
              <th>Guest</th>
              <th>Recycle</th>
              <th>VFS</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.map((s) => (
              <tr key={s.name}>
                <td>{s.name}</td>
                <td className="mono muted" style={{ fontSize: 11 }}>
                  {s.path ?? "—"}
                </td>
                <td className="mono">{s.users ?? "—"}</td>
                <td>
                  {s.guest ? (
                    <Icon name="check" size={11} />
                  ) : (
                    <span className="muted">no</span>
                  )}
                </td>
                <td>
                  {s.recycle ? (
                    <Icon name="check" size={11} />
                  ) : (
                    <span className="muted">no</span>
                  )}
                </td>
                <td className="mono muted" style={{ fontSize: 11 }}>
                  {s.vfs ?? "—"}
                </td>
                <td className="num">
                  <button
                    className="btn btn--sm btn--danger"
                    disabled={delMut.isPending}
                    onClick={() => delMut.mutate(s.name)}
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

export default SMB;
