import { useState } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { storage, type Dataset, type EncryptionStatus } from "../../api/storage";
import { toastSuccess } from "../../store/toast";
import { Modal } from "./Modal";

function dsKey(d: Dataset): string {
  return d.fullname ?? d.name;
}

type Action = { kind: "load" | "rotate" | "recover"; full: string } | null;

export function EncryptionTab() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["datasets"], queryFn: () => storage.listDatasets() });
  const [action, setAction] = useState<Action>(null);

  const inval = () => qc.invalidateQueries({ queryKey: ["datasets"] });

  const unloadMut = useMutation({
    meta: { label: "Unload key failed" },
    mutationFn: (full: string) => storage.unloadKey(full),
    onSuccess: (_d, full) => { inval(); toastSuccess("Key unloaded", full); },
  });

  const encrypted = (q.data ?? []).filter(
    (d) => d.enc || d.encrypted || (d.encryption && d.encryption !== "off")
  );

  const encQs = useQueries({
    queries: encrypted.map((d) => ({
      queryKey: ["encryption", dsKey(d)],
      queryFn: () => storage.getEncryption(dsKey(d)),
    })),
  });
  const encByKey: Record<string, EncryptionStatus | undefined> = {};
  encrypted.forEach((d, i) => {
    encByKey[dsKey(d)] = encQs[i]?.data;
  });

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
              <th>Format</th>
              <th>Key location</th>
              <th>Last rotated</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {encrypted.map((d) => {
              const k = dsKey(d);
              const enc = encByKey[k];
              const status = enc?.keystatus ?? enc?.status ?? "available";
              const isLoaded = status === "available" || status === "loaded";
              return (
                <tr key={k}>
                  <td>{d.name}</td>
                  <td>
                    <span className={`pill pill--${isLoaded ? "ok" : "warn"}`}>
                      <span className="dot" />
                      {status}
                    </span>
                  </td>
                  <td className="mono" style={{ fontSize: 11 }}>{enc?.keyformat ?? d.encryption ?? "—"}</td>
                  <td className="mono" style={{ fontSize: 11 }}>{enc?.keylocation ?? "—"}</td>
                  <td className="muted">{enc?.rotated ?? "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm"
                      onClick={() => isLoaded ? unloadMut.mutate(k) : setAction({ kind: "load", full: k })}
                      disabled={unloadMut.isPending}
                    >
                      {isLoaded ? "Unload" : "Load"}
                    </button>{" "}
                    <button
                      className="btn btn--sm"
                      onClick={() => setAction({ kind: "rotate", full: k })}
                    >
                      Rotate
                    </button>{" "}
                    <button
                      className="btn btn--sm btn--danger"
                      onClick={() => setAction({ kind: "recover", full: k })}
                      title="Recover via TPM-sealed escrow"
                    >
                      Recover
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
      {action?.kind === "recover" && (
        <RecoverKeyModal full={action.full} onClose={() => setAction(null)} onDone={inval} />
      )}
    </div>
  );
}

function RecoverKeyModal({
  full,
  onClose,
  onDone,
}: {
  full: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [escrowToken, setEscrowToken] = useState("");
  const [adminConfirm, setAdminConfirm] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Recovery failed" },
    mutationFn: () => storage.recoverKey(full, {
      escrow_token: escrowToken || undefined,
      admin_confirm: adminConfirm,
    }),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Key recovered", full); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Recover encryption key" sub={full} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !adminConfirm}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Recovering…" : "Recover"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="modal__err" style={{ background: "transparent", color: "var(--fg-2)" }}>
        Recovery is audit-logged and requires admin confirmation. Use this when the
        TPM seal is broken (firmware update, mainboard swap) or the passphrase is lost.
      </div>
      <div className="field">
        <label className="field__label">Escrow token (optional, for off-host recovery)</label>
        <input className="input" type="password" value={escrowToken} onChange={(e) => setEscrowToken(e.target.value)} />
      </div>
      <div className="field">
        <label className="field__label">Type "RECOVER" to confirm</label>
        <input className="input" value={adminConfirm} onChange={(e) => setAdminConfirm(e.target.value)} placeholder="RECOVER" />
      </div>
    </Modal>
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
    meta: { label: "Load key failed" },
    mutationFn: () => storage.loadKey(full, key || undefined),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Key loaded", full); },
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
    meta: { label: "Rotate key failed" },
    mutationFn: () => storage.changeKey(full, key ? { key } : {}),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Key rotated", full); },
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
