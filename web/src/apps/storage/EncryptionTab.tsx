import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { storage, type Dataset } from "../../api/storage";
import { Modal } from "./Modal";

function dsKey(d: Dataset): string {
  return d.fullname ?? d.name;
}

type Action = { kind: "load" | "rotate"; full: string } | null;

export function EncryptionTab() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["datasets"], queryFn: () => storage.listDatasets() });
  const [action, setAction] = useState<Action>(null);

  const inval = () => qc.invalidateQueries({ queryKey: ["datasets"] });

  const unloadMut = useMutation({
    mutationFn: (full: string) => storage.unloadKey(full),
    onSuccess: inval,
  });

  const encrypted = (q.data ?? []).filter(
    (d) => d.enc || d.encrypted || (d.encryption && d.encryption !== "off")
  );

  return (
    <div style={{ padding: 14 }}>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">TPM-sealed key escrow</div>
          <span className="pill pill--ok">
            <span className="dot" />
            TPM 2.0 healthy
          </span>
        </div>
        <div className="sect__body">
          <div className="muted" style={{ fontSize: 11, marginBottom: 10 }}>
            Native ZFS encryption · keys are wrapped to PCRs of this host.
            Recovery requires admin role and is audit-logged.
          </div>
        </div>
      </div>
      {q.isLoading && <div className="empty-hint">Loading…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && encrypted.length === 0 && (
        <div className="empty-hint">No encrypted datasets.</div>
      )}
      {encrypted.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Dataset</th>
              <th>Status</th>
              <th>Encryption</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {encrypted.map((d) => {
              const k = dsKey(d);
              return (
                <tr key={k}>
                  <td>{d.name}</td>
                  <td>
                    <span className="pill pill--ok">
                      <span className="dot" />
                      available
                    </span>
                  </td>
                  <td className="mono" style={{ fontSize: 11 }}>{d.encryption ?? "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm"
                      onClick={() => setAction({ kind: "load", full: k })}
                    >
                      Load key
                    </button>{" "}
                    <button
                      className="btn btn--sm"
                      disabled={unloadMut.isPending}
                      onClick={() => unloadMut.mutate(k)}
                    >
                      Unload
                    </button>{" "}
                    <button
                      className="btn btn--sm"
                      onClick={() => setAction({ kind: "rotate", full: k })}
                    >
                      Rotate key
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {action?.kind === "load" && (
        <LoadKeyModal full={action.full} onClose={() => setAction(null)} onDone={inval} />
      )}
      {action?.kind === "rotate" && (
        <RotateKeyModal full={action.full} onClose={() => setAction(null)} onDone={inval} />
      )}
    </div>
  );
}

function LoadKeyModal({
  full,
  onClose,
  onDone,
}: {
  full: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [key, setKey] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: () => storage.loadKey(full, key || undefined),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Load encryption key" sub={full} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Loading…" : "Load"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Passphrase / key (leave blank to use TPM)</label>
        <input className="input" type="password" value={key} onChange={(e) => setKey(e.target.value)} />
      </div>
    </Modal>
  );
}

function RotateKeyModal({
  full,
  onClose,
  onDone,
}: {
  full: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [key, setKey] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: () => storage.changeKey(full, key ? { key } : {}),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Rotate encryption key" sub={full} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Rotating…" : "Rotate"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">New passphrase (leave blank to re-seal to TPM)</label>
        <input className="input" type="password" value={key} onChange={(e) => setKey(e.target.value)} />
      </div>
    </Modal>
  );
}

export default EncryptionTab;
