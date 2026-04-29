import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { vms, type VMSnapshot } from "../../api/vms";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

export function VMSnapshots() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["vms", "snapshots"],
    queryFn: () => vms.snapshots(),
    retry: false,
  });
  const restore = useMutation({
    meta: { label: "Restore failed" },
    mutationFn: (s: { name: string; namespace?: string }) => vms.restore(s.name, s.namespace),
    onSuccess: (_d, s) => {
      qc.invalidateQueries({ queryKey: ["vms"] });
      toastSuccess(`Restore from ${s.name} started`);
    },
  });
  const del = useMutation({
    meta: { label: "Delete snapshot failed" },
    mutationFn: (s: VMSnapshot) =>
      vms.deleteSnapshot(s.namespace ?? "default", s.name),
    onSuccess: (_d, s) => {
      qc.invalidateQueries({ queryKey: ["vms", "snapshots"] });
      toastSuccess(`Snapshot ${s.name} deleted`);
    },
  });

  if (q.isError) {
    const err = q.error as ApiError | Error;
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 503) {
      return (
        <div style={{ padding: 24 }}>
          <div className="discover__msg muted">
            Snapshots unavailable; KubeVirt is not yet ready.
          </div>
          <button className="btn btn--sm" style={{ marginTop: 10 }} onClick={() => q.refetch()}>
            <Icon name="refresh" size={11} />
            Retry
          </button>
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
      <div className="tbar">
        <span className="muted" style={{ fontSize: 11 }}>
          {items.length} snapshot{items.length === 1 ? "" : "s"}
        </span>
        <button
          className="btn btn--sm"
          onClick={() => q.refetch()}
          disabled={q.isFetching}
          style={{ marginLeft: "auto" }}
        >
          <Icon name="refresh" size={11} />
          Refresh
        </button>
      </div>
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
              <th>Namespace</th>
              <th className="num">Size</th>
              <th>Created</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {items.map((s) => (
              <tr key={`${s.namespace ?? "default"}/${s.name}`}>
                <td className="mono" style={{ fontSize: 11 }}>{s.name}</td>
                <td>{s.vm ?? s.vmName ?? "—"}</td>
                <td className="muted mono" style={{ fontSize: 11 }}>
                  {s.namespace ?? "default"}
                </td>
                <td className="num mono">{s.size ?? "—"}</td>
                <td className="muted">{s.t ?? s.created ?? s.createdAt ?? "—"}</td>
                <td className="num">
                  <div className="row gap-8" style={{ justifyContent: "flex-end" }}>
                    <button
                      className="btn btn--sm"
                      disabled={restore.isPending}
                      onClick={() => {
                        if (
                          window.confirm(
                            `Restore VM from snapshot ${s.name}? This will create a restore object.`,
                          )
                        ) {
                          restore.mutate({ name: s.name, namespace: s.namespace });
                        }
                      }}
                    >
                      <Icon name="refresh" size={10} />
                      Restore
                    </button>
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={del.isPending}
                      onClick={() => {
                        if (window.confirm(`Delete snapshot ${s.name}?`)) del.mutate(s);
                      }}
                    >
                      <Icon name="trash" size={10} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {(restore.isError || del.isError) && (
        <div className="muted" style={{ color: "var(--err)", fontSize: 11, padding: 8 }}>
          {((restore.error || del.error) as Error)?.message}
        </div>
      )}
    </div>
  );
}
