import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { replication, type ReplicationTarget } from "../../api/replication";
import { Modal } from "./Modal";

export function Targets() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["replication-targets"],
    queryFn: () => replication.listTargets(),
  });
  const targets = q.data ?? [];
  const [edit, setEdit] = useState<ReplicationTarget | "new" | null>(null);

  const inval = () => qc.invalidateQueries({ queryKey: ["replication-targets"] });
  const delMut = useMutation({
    mutationFn: (id: string) => replication.deleteTarget(id),
    onSuccess: inval,
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setEdit("new")}>
          <Icon name="plus" size={11} />
          Add target
        </button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && targets.length === 0 && (
        <div className="empty-hint">No targets.</div>
      )}
      {targets.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Name</th>
              <th>Protocol</th>
              <th>Host</th>
              <th>Details</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {targets.map((t) => (
              <tr key={t.id}>
                <td>{t.name ?? t.id}</td>
                <td>
                  <span className="pill pill--info">{t.protocol ?? "—"}</span>
                </td>
                <td className="mono">{t.host ?? "—"}</td>
                <td className="muted mono" style={{ fontSize: 11 }}>
                  {t.protocol === "s3"
                    ? `region=${t.region ?? "—"} bucket=${t.bucket ?? "—"}`
                    : `user=${t.ssh_user ?? "—"}, port=${t.port ?? "—"}`}
                </td>
                <td className="num">
                  <button className="btn btn--sm" onClick={() => setEdit(t)}>Edit</button>{" "}
                  <button
                    className="btn btn--sm btn--danger"
                    disabled={delMut.isPending}
                    onClick={() => {
                      if (window.confirm(`Delete target ${t.name ?? t.id}?`)) delMut.mutate(t.id);
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
        <TargetModal
          init={edit === "new" ? null : edit}
          onClose={() => setEdit(null)}
          onDone={inval}
        />
      )}
    </div>
  );
}

function TargetModal({
  init,
  onClose,
  onDone,
}: {
  init: ReplicationTarget | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(init?.name ?? "");
  const [protocol, setProtocol] = useState(init?.protocol ?? "ssh+zfs");
  const [host, setHost] = useState(init?.host ?? "");
  const [port, setPort] = useState<number | "">(init?.port ?? "");
  const [sshUser, setSshUser] = useState(init?.ssh_user ?? "");
  const [region, setRegion] = useState(init?.region ?? "");
  const [bucket, setBucket] = useState(init?.bucket ?? "");
  const [err, setErr] = useState<string | null>(null);

  const body = (): Partial<ReplicationTarget> => ({
    name,
    protocol,
    host: host || undefined,
    port: port === "" ? undefined : Number(port),
    ssh_user: sshUser || undefined,
    region: region || undefined,
    bucket: bucket || undefined,
  });

  const m = useMutation({
    mutationFn: () => init
      ? replication.updateTarget(init.id, body())
      : replication.createTarget(body()),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal title={init ? `Edit target · ${init.name ?? init.id}` : "Add target"} onClose={onClose}
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
        <label className="field__label">Protocol</label>
        <select className="input" value={protocol} onChange={(e) => setProtocol(e.target.value)}>
          <option value="ssh+zfs">ssh+zfs</option>
          <option value="s3">s3</option>
        </select>
      </div>
      {protocol === "s3" ? (
        <>
          <div className="field">
            <label className="field__label">Endpoint host</label>
            <input className="input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="s3.amazonaws.com" />
          </div>
          <div className="field">
            <label className="field__label">Region</label>
            <input className="input" value={region} onChange={(e) => setRegion(e.target.value)} />
          </div>
          <div className="field">
            <label className="field__label">Bucket</label>
            <input className="input" value={bucket} onChange={(e) => setBucket(e.target.value)} />
          </div>
        </>
      ) : (
        <>
          <div className="field">
            <label className="field__label">Host</label>
            <input className="input" value={host} onChange={(e) => setHost(e.target.value)} />
          </div>
          <div className="field">
            <label className="field__label">SSH user</label>
            <input className="input" value={sshUser} onChange={(e) => setSshUser(e.target.value)} />
          </div>
          <div className="field">
            <label className="field__label">Port</label>
            <input
              className="input"
              type="number"
              value={port}
              onChange={(e) => setPort(e.target.value === "" ? "" : Number(e.target.value))}
              placeholder="22"
            />
          </div>
        </>
      )}
    </Modal>
  );
}

export default Targets;
