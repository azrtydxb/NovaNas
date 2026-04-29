import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares, type NfsExport } from "../../api/shares";
import { toastSuccess } from "../../store/toast";
import { Modal } from "./Modal";

export function NFS() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["nfs-exports"], queryFn: () => shares.listNfs() });
  const list = q.data ?? [];
  const [edit, setEdit] = useState<NfsExport | "new" | null>(null);

  const inval = () => qc.invalidateQueries({ queryKey: ["nfs-exports"] });
  const delMut = useMutation({
    meta: { label: "Delete export failed" },
    mutationFn: (n: string) => shares.deleteNfs(n),
    onSuccess: (_d, n) => { inval(); toastSuccess("Export deleted", n); },
  });
  const reloadMut = useMutation({
    meta: { label: "Reload failed" },
    mutationFn: () => shares.nfsReload(),
    onSuccess: () => toastSuccess("NFS reloaded"),
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setEdit("new")}>
          <Icon name="plus" size={11} />
          New export
        </button>
        <button className="btn" disabled={reloadMut.isPending} onClick={() => reloadMut.mutate()}>
          <Icon name="refresh" size={11} />
          Reload
        </button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading NFS exports…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && list.length === 0 && <div className="empty-hint">No NFS exports.</div>}
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
                <td className="mono muted" style={{ fontSize: 11 }}>{n.path ?? "—"}</td>
                <td className="mono">{n.clients ?? "—"}</td>
                <td className="mono muted" style={{ fontSize: 11 }}>{n.options ?? "—"}</td>
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
                  <button className="btn btn--sm" onClick={() => setEdit(n)}>Edit</button>{" "}
                  <button
                    className="btn btn--sm btn--danger"
                    disabled={delMut.isPending}
                    onClick={() => {
                      if (window.confirm(`Delete export ${n.name}?`)) delMut.mutate(n.name);
                    }}
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {edit && (
        <NfsModal
          init={edit === "new" ? null : edit}
          onClose={() => setEdit(null)}
          onDone={inval}
        />
      )}
    </div>
  );
}

function NfsModal({
  init,
  onClose,
  onDone,
}: {
  init: NfsExport | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(init?.name ?? "");
  const [path, setPath] = useState(init?.path ?? "");
  const [clients, setClients] = useState(init?.clients ?? "");
  const [options, setOptions] = useState(init?.options ?? "rw,sync,no_subtree_check");
  const [active, setActive] = useState(init?.active ?? true);
  const [err, setErr] = useState<string | null>(null);

  const body = (): Partial<NfsExport> => ({ name, path, clients, options, active });

  const m = useMutation({
    meta: { label: "Save export failed" },
    mutationFn: () => init
      ? shares.updateNfs(init.name, body())
      : shares.createNfs(body()),
    onSuccess: () => { onDone(); onClose(); toastSuccess(init ? "Export updated" : "Export created", name); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal title={init ? `Edit export · ${init.name}` : "New NFS export"} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !name || !path}
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
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} disabled={!!init} />
      </div>
      <div className="field">
        <label className="field__label">Path</label>
        <input className="input" value={path} onChange={(e) => setPath(e.target.value)} placeholder="/tank/export" />
      </div>
      <div className="field">
        <label className="field__label">Clients (CIDR / hostname)</label>
        <input className="input" value={clients} onChange={(e) => setClients(e.target.value)} placeholder="10.0.0.0/24" />
      </div>
      <div className="field">
        <label className="field__label">Options</label>
        <input className="input" value={options} onChange={(e) => setOptions(e.target.value)} />
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} />
          Active
        </label>
      </div>
    </Modal>
  );
}

export default NFS;
