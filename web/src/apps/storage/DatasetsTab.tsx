import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import {
  storage,
  type Dataset,
  type DatasetMetadata,
} from "../../api/storage";
import { formatBytes } from "../../lib/format";
import { Modal } from "./Modal";

function dsKey(d: Dataset): string {
  return d.fullname ?? d.name;
}

type ActionKind =
  | "rollback"
  | "clone"
  | "promote"
  | "rename"
  | "send"
  | "receive"
  | null;

export function DatasetsTab() {
  const [sel, setSel] = useState<string | null>(null);
  const q = useQuery({ queryKey: ["datasets"], queryFn: () => storage.listDatasets() });
  const datasets = q.data ?? [];

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: sel ? "1fr 360px" : "1fr",
        height: "100%",
      }}
    >
      <div style={{ overflow: "auto", padding: 14 }}>
        <div className="tbar">
          <button className="btn btn--primary" disabled title="Backend POST /datasets is missing">
            <Icon name="plus" size={11} />
            New dataset
          </button>
        </div>
        {q.isLoading && <div className="empty-hint">Loading datasets…</div>}
        {q.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(q.error as Error).message}
          </div>
        )}
        {q.data && datasets.length === 0 && <div className="empty-hint">No datasets.</div>}
        {datasets.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>Dataset</th>
                <th>Pool</th>
                <th>Protocol</th>
                <th className="num">Used</th>
                <th>Quota</th>
                <th className="num">Snaps</th>
                <th>Comp</th>
                <th>Enc</th>
              </tr>
            </thead>
            <tbody>
              {datasets.map((d) => {
                const k = dsKey(d);
                const used = d.used ?? 0;
                const quota = d.quota ?? 0;
                const pct = quota > 0 ? used / quota : 0;
                const enc = d.enc ?? d.encrypted ?? !!d.encryption;
                const snap = d.snap ?? d.snapshots ?? 0;
                return (
                  <tr
                    key={k}
                    onClick={() => setSel(k)}
                    className={sel === k ? "is-on" : ""}
                    style={{ cursor: "pointer" }}
                  >
                    <td>
                      <Icon name="files" size={12} style={{ verticalAlign: "-2px", marginRight: 6, opacity: 0.6 }} />
                      {d.name}
                    </td>
                    <td className="muted mono">{d.pool ?? "—"}</td>
                    <td className="muted">{d.proto ?? "—"}</td>
                    <td className="num mono">{formatBytes(used)}</td>
                    <td>
                      {quota > 0 ? (
                        <div className="cap">
                          <div className="cap__bar">
                            <div style={{ width: `${pct * 100}%` }} />
                          </div>
                          <span className="mono" style={{ fontSize: 11, color: "var(--fg-3)" }}>
                            {formatBytes(quota)}
                          </span>
                        </div>
                      ) : (
                        <span className="muted">—</span>
                      )}
                    </td>
                    <td className="num mono">{snap}</td>
                    <td className="muted mono" style={{ fontSize: 11 }}>{d.comp ?? d.compression ?? "—"}</td>
                    <td>{enc ? <Icon name="shield" size={12} /> : <span className="muted">—</span>}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
      {sel && (
        <DatasetDetail
          fullname={sel}
          fallback={datasets.find((d) => dsKey(d) === sel)}
          onClose={() => setSel(null)}
        />
      )}
    </div>
  );
}

type SubTab = "general" | "props" | "quota" | "policy" | "sharing" | "acl" | "meta";

function DatasetDetail({
  fullname,
  fallback,
  onClose,
}: {
  fullname: string;
  fallback?: Dataset;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [tab, setTab] = useState<SubTab>("general");
  const [action, setAction] = useState<ActionKind>(null);

  const q = useQuery({
    queryKey: ["dataset", fullname],
    queryFn: () => storage.getDataset(fullname),
  });
  const d = q.data ?? fallback;

  const inval = () => {
    qc.invalidateQueries({ queryKey: ["dataset", fullname] });
    qc.invalidateQueries({ queryKey: ["datasets"] });
  };

  const promoteMut = useMutation({
    mutationFn: () => storage.promoteDataset(fullname),
    onSuccess: inval,
  });

  if (!d) {
    return (
      <div className="side-detail">
        <div className="side-detail__head">
          <div>
            <div className="muted mono" style={{ fontSize: 10 }}>DATASET</div>
            <div className="side-detail__title">{fullname}</div>
          </div>
          <button className="btn btn--sm" onClick={onClose}>
            <Icon name="close" size={10} />
          </button>
        </div>
        <div className="empty-hint">{q.isLoading ? "Loading…" : "No data"}</div>
      </div>
    );
  }

  const used = d.used ?? 0;
  const quota = d.quota ?? 0;
  const pct = quota > 0 ? used / quota : 0;
  const enc = d.enc ?? d.encrypted ?? !!d.encryption;
  const snap = d.snap ?? d.snapshots ?? 0;

  return (
    <div className="side-detail">
      <div className="side-detail__head">
        <div>
          <div className="muted mono" style={{ fontSize: 10 }}>DATASET</div>
          <div className="side-detail__title">{d.name}</div>
        </div>
        <button className="btn btn--sm" onClick={onClose}>
          <Icon name="close" size={10} />
        </button>
      </div>

      <div className="win-tabs" style={{ overflowX: "auto" }}>
        {(["general", "props", "quota", "policy", "sharing", "acl", "meta"] as const).map((t) => (
          <button key={t} className={tab === t ? "is-on" : ""} onClick={() => setTab(t)}>
            {t}
          </button>
        ))}
      </div>

      {tab === "general" && (
        <div className="sect">
          <div className="sect__title">Capacity</div>
          <div className="sect__body">
            <div className="bar">
              <div className="bar__fill" style={{ width: `${pct * 100}%` }} />
            </div>
            <div className="row" style={{ justifyContent: "space-between", fontSize: 11, marginTop: 4 }}>
              <span className="mono">{formatBytes(used)}</span>
              <span className="muted mono">/ {formatBytes(quota)}</span>
            </div>
            <dl className="kv" style={{ marginTop: 8 }}>
              <dt>Pool</dt><dd>{d.pool ?? "—"}</dd>
              <dt>Mountpoint</dt><dd className="mono" style={{ fontSize: 11 }}>{d.mountpoint ?? "—"}</dd>
              <dt>Protocol</dt><dd>{d.proto ?? "—"}</dd>
              <dt>Snapshots</dt><dd>{snap}</dd>
              <dt>Encrypted</dt><dd>{enc ? "yes" : "no"}</dd>
            </dl>
          </div>
        </div>
      )}

      {tab === "props" && (
        <div className="sect">
          <div className="sect__title">Properties</div>
          <div className="sect__body">
            <dl className="kv">
              <dt>Compression</dt><dd>{d.comp ?? d.compression ?? "—"}</dd>
              <dt>Recordsize</dt><dd>{d.recordsize ?? "—"}</dd>
              <dt>Atime</dt><dd>{d.atime ?? "—"}</dd>
              <dt>Encryption</dt><dd>{d.encryption ?? "—"}</dd>
              <dt>Referenced</dt><dd>{d.referenced ? formatBytes(d.referenced) : "—"}</dd>
              <dt>Available</dt><dd>{d.available ? formatBytes(d.available) : "—"}</dd>
            </dl>
          </div>
        </div>
      )}

      {tab === "quota" && (
        <div className="sect">
          <div className="sect__title">Quota</div>
          <div className="sect__body">
            <dl className="kv">
              <dt>Used</dt><dd>{formatBytes(used)}</dd>
              <dt>Quota</dt><dd>{quota > 0 ? formatBytes(quota) : "none"}</dd>
              <dt>Available</dt><dd>{d.available ? formatBytes(d.available) : "—"}</dd>
            </dl>
            <div className="muted small" style={{ marginTop: 6 }}>
              Editing quotas requires the property setter (TODO: backend missing for direct PUT).
            </div>
          </div>
        </div>
      )}

      {tab === "policy" && (
        <div className="sect">
          <div className="sect__title">Snapshot policy</div>
          <div className="sect__body">
            <div className="muted small">
              Snapshot schedules are managed in the <strong>Replication</strong> app.
              This dataset would inherit any schedule whose datasets list contains <code>{fullname}</code>.
            </div>
          </div>
        </div>
      )}

      {tab === "sharing" && (
        <div className="sect">
          <div className="sect__title">Sharing</div>
          <div className="sect__body">
            <div className="muted small">
              Manage SMB/NFS/iSCSI/NVMe-oF exports for this path in the <strong>Shares</strong> app.
            </div>
            <dl className="kv" style={{ marginTop: 6 }}>
              <dt>Path</dt><dd className="mono" style={{ fontSize: 11 }}>{d.mountpoint ?? "—"}</dd>
              <dt>Active proto</dt><dd>{d.proto ?? "—"}</dd>
            </dl>
          </div>
        </div>
      )}

      {tab === "acl" && <AclPanel fullname={fullname} />}
      {tab === "meta" && <MetadataPanel fullname={fullname} />}

      <div
        className="row gap-8"
        style={{ padding: "10px 12px", borderTop: "1px solid var(--line)", flexWrap: "wrap" }}
      >
        <button className="btn btn--sm" onClick={() => setAction("rollback")}>Rollback</button>
        <button className="btn btn--sm" onClick={() => setAction("clone")}>Clone</button>
        <button
          className="btn btn--sm"
          disabled={promoteMut.isPending}
          onClick={() => {
            if (window.confirm(`Promote ${fullname}?`)) promoteMut.mutate();
          }}
        >
          Promote
        </button>
        <button className="btn btn--sm" onClick={() => setAction("rename")}>Rename</button>
        <button className="btn btn--sm" onClick={() => setAction("send")}>Send…</button>
        <button className="btn btn--sm" onClick={() => setAction("receive")}>Receive…</button>
      </div>

      {action === "rollback" && (
        <RollbackModal fullname={fullname} onClose={() => setAction(null)} onDone={inval} />
      )}
      {action === "clone" && (
        <CloneModal fullname={fullname} onClose={() => setAction(null)} onDone={inval} />
      )}
      {action === "rename" && (
        <RenameModal fullname={fullname} onClose={() => setAction(null)} onDone={inval} />
      )}
      {action === "send" && (
        <SendReceiveModal mode="send" fullname={fullname} onClose={() => setAction(null)} />
      )}
      {action === "receive" && (
        <SendReceiveModal mode="receive" fullname={fullname} onClose={() => setAction(null)} />
      )}
    </div>
  );
}

function AclPanel({ fullname }: { fullname: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["acl", fullname],
    queryFn: () => storage.getAcl(fullname),
  });
  const inval = () => qc.invalidateQueries({ queryKey: ["acl", fullname] });
  const removeMut = useMutation({
    mutationFn: (i: number) => storage.removeAcl(fullname, i),
    onSuccess: inval,
  });
  const [tag, setTag] = useState("user");
  const [who, setWho] = useState("");
  const [perms, setPerms] = useState("rwx");
  const [err, setErr] = useState<string | null>(null);

  const appendMut = useMutation({
    mutationFn: () => storage.appendAcl(fullname, { tag, who, permissions: perms }),
    onSuccess: () => { setWho(""); inval(); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <div className="sect">
      <div className="sect__title">ACL</div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {q.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>{(q.error as Error).message}</div>
        )}
        {q.data && q.data.length === 0 && <div className="muted">No ACL entries.</div>}
        {q.data && q.data.length > 0 && (
          <table className="tbl tbl--compact">
            <thead>
              <tr>
                <th>Tag</th>
                <th>Who</th>
                <th>Perms</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((a, i) => (
                <tr key={i}>
                  <td className="mono">{a.tag ?? "—"}</td>
                  <td className="mono" style={{ fontSize: 11 }}>{a.who ?? "—"}</td>
                  <td className="mono">{a.permissions ?? a.flags ?? "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={removeMut.isPending}
                      onClick={() => removeMut.mutate(i)}
                    >
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="row gap-8" style={{ marginTop: 8, flexWrap: "wrap" }}>
          <select className="input" value={tag} onChange={(e) => setTag(e.target.value)} style={{ width: 90 }}>
            <option value="user">user</option>
            <option value="group">group</option>
            <option value="everyone">everyone@</option>
            <option value="owner">owner@</option>
          </select>
          <input
            className="input"
            placeholder="who"
            value={who}
            onChange={(e) => setWho(e.target.value)}
            style={{ flex: 1, minWidth: 90 }}
          />
          <input
            className="input"
            placeholder="rwx"
            value={perms}
            onChange={(e) => setPerms(e.target.value)}
            style={{ width: 80 }}
          />
          <button
            className="btn btn--sm"
            disabled={appendMut.isPending}
            onClick={() => { setErr(null); appendMut.mutate(); }}
          >
            Add
          </button>
        </div>
        {err && <div className="modal__err" style={{ marginTop: 6 }}>{err}</div>}
      </div>
    </div>
  );
}

function MetadataPanel({ fullname }: { fullname: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["meta", fullname],
    queryFn: () => storage.getMetadata(fullname),
  });
  const [draft, setDraft] = useState<DatasetMetadata>({});
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (q.data) setDraft({ ...q.data });
  }, [q.data]);

  const saveMut = useMutation({
    mutationFn: () => storage.putMetadata(fullname, draft),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["meta", fullname] }),
    onError: (e: Error) => setErr(e.message),
  });

  const [k, setK] = useState("");
  const [v, setV] = useState("");

  return (
    <div className="sect">
      <div className="sect__title">Metadata</div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {Object.keys(draft).length === 0 && !q.isLoading && (
          <div className="muted">No metadata.</div>
        )}
        {Object.keys(draft).length > 0 && (
          <table className="tbl tbl--compact">
            <tbody>
              {Object.entries(draft).map(([key, val]) => (
                <tr key={key}>
                  <td className="mono" style={{ fontSize: 11 }}>{key}</td>
                  <td>
                    <input
                      className="input"
                      value={val}
                      onChange={(e) => setDraft((d) => ({ ...d, [key]: e.target.value }))}
                    />
                  </td>
                  <td className="num">
                    <button
                      className="btn btn--sm btn--danger"
                      onClick={() => setDraft((d) => {
                        const c = { ...d }; delete c[key]; return c;
                      })}
                    >
                      ×
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="row gap-8" style={{ marginTop: 8 }}>
          <input className="input" placeholder="key" value={k} onChange={(e) => setK(e.target.value)} />
          <input className="input" placeholder="value" value={v} onChange={(e) => setV(e.target.value)} />
          <button
            className="btn btn--sm"
            disabled={!k}
            onClick={() => { setDraft((d) => ({ ...d, [k]: v })); setK(""); setV(""); }}
          >
            +
          </button>
        </div>
        <div className="row gap-8" style={{ marginTop: 8 }}>
          <button
            className="btn btn--sm btn--primary"
            disabled={saveMut.isPending}
            onClick={() => { setErr(null); saveMut.mutate(); }}
          >
            {saveMut.isPending ? "Saving…" : "Save"}
          </button>
        </div>
        {err && <div className="modal__err" style={{ marginTop: 6 }}>{err}</div>}
      </div>
    </div>
  );
}

function RollbackModal({ fullname, onClose, onDone }: { fullname: string; onClose: () => void; onDone: () => void }) {
  const [snap, setSnap] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: () => storage.rollbackDataset(fullname, snap || undefined),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Rollback dataset" sub={fullname} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Rolling back…" : "Rollback"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Snapshot (optional, defaults to latest)</label>
        <input className="input" value={snap} onChange={(e) => setSnap(e.target.value)} placeholder="snap-name" />
      </div>
    </Modal>
  );
}

function CloneModal({ fullname, onClose, onDone }: { fullname: string; onClose: () => void; onDone: () => void }) {
  const [snap, setSnap] = useState("");
  const [target, setTarget] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: () => storage.cloneDataset(fullname, { snapshot: snap, target }),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Clone dataset" sub={fullname} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !snap || !target}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Cloning…" : "Clone"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Source snapshot</label>
        <input className="input" value={snap} onChange={(e) => setSnap(e.target.value)} placeholder="snapname" />
      </div>
      <div className="field">
        <label className="field__label">Target dataset (full path)</label>
        <input className="input" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="pool/clone-name" />
      </div>
    </Modal>
  );
}

function RenameModal({ fullname, onClose, onDone }: { fullname: string; onClose: () => void; onDone: () => void }) {
  const [target, setTarget] = useState(fullname);
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: () => storage.renameDataset(fullname, target),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Rename dataset" sub={fullname} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !target || target === fullname}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Renaming…" : "Rename"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">New full name</label>
        <input className="input" value={target} onChange={(e) => setTarget(e.target.value)} />
      </div>
    </Modal>
  );
}

function SendReceiveModal({
  mode,
  fullname,
  onClose,
}: {
  mode: "send" | "receive";
  fullname: string;
  onClose: () => void;
}) {
  return (
    <Modal title={mode === "send" ? "Send dataset" : "Receive dataset"} sub={fullname} onClose={onClose}
      footer={<button className="btn" onClick={onClose}>Close</button>}
    >
      <div className="modal__err" style={{ background: "transparent", color: "var(--fg-2)" }}>
        Streaming send/receive is a multi-step flow (target selection, encryption,
        resume tokens). The endpoint exists at{" "}
        <code>POST /api/v1/datasets/{fullname}/{mode}</code> — wiring the
        actual streaming UI is coming next.
      </div>
    </Modal>
  );
}

export default DatasetsTab;
