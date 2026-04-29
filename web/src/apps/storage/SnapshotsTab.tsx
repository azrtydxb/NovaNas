import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { storage, type Snapshot } from "../../api/storage";
import { formatBytes } from "../../lib/format";

function snapKey(s: Snapshot): string {
  return s.fullname ?? s.name;
}

export function SnapshotsTab() {
  const [filter, setFilter] = useState("");
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["snapshots"], queryFn: () => storage.listSnapshots() });

  const delMut = useMutation({
    mutationFn: (full: string) => storage.deleteSnapshot(full),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["snapshots"] }),
  });

  const list = (q.data ?? []).filter((s) =>
    filter ? snapKey(s).toLowerCase().includes(filter.toLowerCase()) : true
  );

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          {/* TODO: phase 3 — open create-snapshot dialog */}
          <Icon name="plus" size={11} />
          New snapshot
        </button>
        <button className="btn">
          <Icon name="refresh" size={11} />
          Send/Receive
        </button>
        <input
          className="input"
          placeholder="Filter snapshots…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          style={{ marginLeft: "auto", width: 180 }}
        />
      </div>
      {q.isLoading && <div className="empty-hint">Loading snapshots…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && list.length === 0 && (
        <div className="empty-hint">No snapshots.</div>
      )}
      {list.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Snapshot</th>
              <th>Pool</th>
              <th className="num">Size</th>
              <th>Schedule</th>
              <th>Hold</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.map((s) => {
              const k = snapKey(s);
              const size = s.size ?? s.used ?? 0;
              return (
                <tr key={k}>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {s.name}
                  </td>
                  <td className="muted mono">{s.pool ?? "—"}</td>
                  <td className="num mono">{formatBytes(size)}</td>
                  <td className="muted">{s.schedule ?? "—"}</td>
                  <td>
                    {s.hold ? (
                      <Icon name="shield" size={11} />
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                  <td className="muted">{s.created ?? "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={delMut.isPending}
                      onClick={() => delMut.mutate(k)}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}

export default SnapshotsTab;
