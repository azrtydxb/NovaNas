import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { replication, type ScrubPolicy } from "../../api/replication";
import { toastSuccess } from "../../store/toast";
import { Modal } from "./Modal";

export function Scrub() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["scrub-policies"],
    queryFn: () => replication.listScrubPolicies(),
  });
  const policies = q.data ?? [];
  const [edit, setEdit] = useState<ScrubPolicy | "new" | null>(null);

  const inval = () => qc.invalidateQueries({ queryKey: ["scrub-policies"] });
  const delMut = useMutation({
    meta: { label: "Delete scrub policy failed" },
    mutationFn: (id: string) => replication.deleteScrubPolicy(id),
    onSuccess: () => { inval(); toastSuccess("Scrub policy deleted"); },
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setEdit("new")}>
          <Icon name="plus" size={11} />
          New policy
        </button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && policies.length === 0 && (
        <div className="empty-hint">No scrub policies.</div>
      )}
      {policies.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Name</th>
              <th>Pools</th>
              <th>Cron</th>
              <th>Priority</th>
              <th>Type</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {policies.map((p) => (
              <tr key={p.id}>
                <td>{p.name ?? p.id}</td>
                <td className="mono muted" style={{ fontSize: 11 }}>
                  {(p.pools ?? []).join(", ")}
                </td>
                <td className="mono">{p.cron ?? "—"}</td>
                <td>
                  <span
                    className={`pill pill--${
                      p.priority === "high"
                        ? "warn"
                        : p.priority === "low"
                          ? ""
                          : "info"
                    }`}
                  >
                    {p.priority ?? "—"}
                  </span>
                </td>
                <td className="muted">{p.builtin ? "built-in" : "custom"}</td>
                <td className="num">
                  {!p.builtin && (
                    <>
                      <button className="btn btn--sm" onClick={() => setEdit(p)}>Edit</button>{" "}
                      <button
                        className="btn btn--sm btn--danger"
                        disabled={delMut.isPending}
                        onClick={() => {
                          if (window.confirm(`Delete policy ${p.name ?? p.id}?`)) delMut.mutate(p.id);
                        }}
                      >
                        Delete
                      </button>
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {edit && (
        <ScrubPolicyModal
          init={edit === "new" ? null : edit}
          onClose={() => setEdit(null)}
          onDone={inval}
        />
      )}
    </div>
  );
}

function ScrubPolicyModal({
  init,
  onClose,
  onDone,
}: {
  init: ScrubPolicy | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(init?.name ?? "");
  const [pools, setPools] = useState((init?.pools ?? []).join(", "));
  const [cron, setCron] = useState(init?.cron ?? "0 3 * * 0");
  const [priority, setPriority] = useState(init?.priority ?? "normal");
  const [err, setErr] = useState<string | null>(null);

  const body = (): Partial<ScrubPolicy> => ({
    name,
    pools: pools.split(",").map((s) => s.trim()).filter(Boolean),
    cron,
    priority,
  });

  const m = useMutation({
    meta: { label: "Save scrub policy failed" },
    mutationFn: () => init
      ? replication.updateScrubPolicy(init.id, body())
      : replication.createScrubPolicy(body()),
    onSuccess: () => { onDone(); onClose(); toastSuccess(init ? "Scrub policy updated" : "Scrub policy created", name); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal title={init ? `Edit scrub policy · ${init.name ?? init.id}` : "New scrub policy"} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !name}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Name</label>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="field">
        <label className="field__label">Pools (comma-separated)</label>
        <input className="input" value={pools} onChange={(e) => setPools(e.target.value)} placeholder="tank, fast" />
      </div>
      <div className="field">
        <label className="field__label">Cron</label>
        <input className="input" value={cron} onChange={(e) => setCron(e.target.value)} placeholder="0 3 * * 0" />
      </div>
      <div className="field">
        <label className="field__label">Priority</label>
        <select className="input" value={priority} onChange={(e) => setPriority(e.target.value)}>
          <option value="low">low</option>
          <option value="normal">normal</option>
          <option value="high">high</option>
        </select>
      </div>
    </Modal>
  );
}

export default Scrub;
