import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { vms } from "../../api/vms";

export function VMSnapshots() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["vms", "snapshots"],
    queryFn: () => vms.snapshots(),
    retry: false,
  });
  const restore = useMutation({
    mutationFn: (s: { name: string; namespace?: string }) => vms.restore(s.name, s.namespace),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["vms", "list"] }),
  });

  if (q.isError) {
    const err = q.error as ApiError | Error;
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 503) {
      return (
        <div style={{ padding: 14 }} className="muted">
          Snapshots unavailable; KubeClient not yet wired.
        </div>
      );
    }
    return (
      <div style={{ padding: 14, color: "var(--err)" }}>
        Failed to load snapshots: {err.message}
      </div>
    );
  }

  const items = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      {q.isLoading && <div className="muted">Loading snapshots…</div>}
      {!q.isLoading && items.length === 0 && (
        <div className="muted" style={{ padding: 12 }}>
          No VM snapshots.
        </div>
      )}
      {items.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Snapshot</th>
              <th>VM</th>
              <th className="num">Size</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {items.map((s) => (
              <tr key={`${s.namespace ?? "default"}/${s.name}`}>
                <td className="mono" style={{ fontSize: 11 }}>
                  {s.name}
                </td>
                <td>{s.vm ?? s.vmName ?? "—"}</td>
                <td className="num mono">{s.size ?? "—"}</td>
                <td className="muted">{s.t ?? s.created ?? s.createdAt ?? "—"}</td>
                <td className="num">
                  <button
                    className="btn btn--sm"
                    disabled={restore.isPending}
                    onClick={() => restore.mutate({ name: s.name, namespace: s.namespace })}
                  >
                    Restore
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
