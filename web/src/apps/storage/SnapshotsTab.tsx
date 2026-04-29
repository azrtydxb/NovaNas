import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { storage, type Snapshot } from "../../api/storage";
import { formatBytes } from "../../lib/format";
import { toastSuccess } from "../../store/toast";
import { Modal } from "./Modal";

function snapKey(s: Snapshot): string {
  return s.fullname ?? s.name;
}

export function SnapshotsTab() {
  const [filter, setFilter] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [holdFor, setHoldFor] = useState<string | null>(null);
  const [diffSel, setDiffSel] = useState<string[]>([]);
  const [showDiff, setShowDiff] = useState(false);
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["snapshots"], queryFn: () => storage.listSnapshots() });

  const inval = () => qc.invalidateQueries({ queryKey: ["snapshots"] });

  const delMut = useMutation({
    meta: { label: "Delete failed" },
    mutationFn: (full: string) => storage.deleteSnapshot(full),
    onSuccess: (_d, full) => { inval(); toastSuccess("Snapshot deleted", full); },
  });

  const toggleDiff = (k: string) => {
    setDiffSel((prev) => prev.includes(k) ? prev.filter((x) => x !== k) : [...prev, k].slice(-2));
  };

  const list = (q.data ?? []).filter((s) =>
    filter ? snapKey(s).toLowerCase().includes(filter.toLowerCase()) : true
  );

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setShowCreate(true)}>
          <Icon name="plus" size={11} />
          Take snapshot
        </button>
        <button
          className="btn"
          disabled={diffSel.length !== 2}
          onClick={() => setShowDiff(true)}
          title="Pick exactly two snapshots of the same dataset, then click Diff"
        >
          <Icon name="files" size={11} />
          Diff ({diffSel.length}/2)
        </button>
        <input
          className="input"
          placeholder="Filter snapshots…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          style={{ marginLeft: "auto", width: 220 }}
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
              <th>Diff</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.map((s) => {
              const k = snapKey(s);
              const size = s.size ?? s.used ?? 0;
              return (
                <tr key={k}>
                  <td className="mono" style={{ fontSize: 11 }}>{s.name}</td>
                  <td className="muted mono">{s.pool ?? "—"}</td>
                  <td className="num mono">{formatBytes(size)}</td>
                  <td className="muted">{s.schedule ?? "—"}</td>
                  <td>
                    {s.hold ? <Icon name="shield" size={11} /> : <span className="muted">—</span>}
                  </td>
                  <td className="muted">{s.created ?? "—"}</td>
                  <td>
                    <input
                      type="checkbox"
                      checked={diffSel.includes(k)}
                      onChange={() => toggleDiff(k)}
                    />
                  </td>
                  <td className="num">
                    <button className="btn btn--sm" onClick={() => setHoldFor(k)}>
                      Holds
                    </button>{" "}
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={delMut.isPending}
                      onClick={() => {
                        if (window.confirm(`Delete snapshot ${k}?`)) delMut.mutate(k);
                      }}
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

      {showCreate && <CreateSnapshotModal onClose={() => setShowCreate(false)} onDone={inval} />}
      {holdFor && <HoldsModal fullname={holdFor} onClose={() => setHoldFor(null)} />}
      {showDiff && diffSel.length === 2 && (
        <DiffModal a={diffSel[0]!} b={diffSel[1]!} onClose={() => setShowDiff(false)} />
      )}
    </div>
  );
}

function DiffModal({ a, b, onClose }: { a: string; b: string; onClose: () => void }) {
  // a and b are full snapshot names like "pool/ds@snap1". The /datasets/{full}/diff endpoint
  // is keyed on the dataset name; both snapshots must share the same dataset.
  const dsA = a.split("@")[0] ?? a;
  const dsB = b.split("@")[0] ?? b;
  const sameDataset = dsA === dsB;
  const [data, setData] = useState<unknown>(null);
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Diff failed" },
    mutationFn: () => storage.diffDataset(dsA, { from: a, to: b }),
    onSuccess: (d) => setData(d),
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Snapshot diff" sub={`${a}  →  ${b}`} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Close</button>
          <button
            className="btn btn--primary"
            disabled={!sameDataset || m.isPending}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Comparing…" : "Compare"}
          </button>
        </>
      }
    >
      {!sameDataset && (
        <div className="modal__err">
          Snapshots must belong to the same dataset to diff.<br/>
          Got <code>{dsA}</code> vs <code>{dsB}</code>.
        </div>
      )}
      {err && <div className="modal__err">{err}</div>}
      {data != null && (
        <pre
          className="mono"
          style={{
            fontSize: 11,
            maxHeight: 360,
            overflow: "auto",
            background: "var(--bg-2)",
            padding: 10,
            border: "1px solid var(--line)",
            borderRadius: 6,
          }}
        >
          {typeof data === "string" ? data : JSON.stringify(data, null, 2)}
        </pre>
      )}
    </Modal>
  );
}

function CreateSnapshotModal({
  onClose,
  onDone,
}: {
  onClose: () => void;
  onDone: () => void;
}) {
  const dsQ = useQuery({ queryKey: ["datasets"], queryFn: () => storage.listDatasets() });
  const datasets = dsQ.data ?? [];
  const [dataset, setDataset] = useState("");
  const [name, setName] = useState("");
  const [err, setErr] = useState<string | null>(null);

  const m = useMutation({
    meta: { label: "Snapshot failed" },
    mutationFn: () => storage.createSnapshot(dataset, name),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Snapshot taken", `${dataset}@${name}`); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal title="Take snapshot" onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !dataset || !name}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Creating…" : "Snapshot"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Dataset</label>
        <select className="input" value={dataset} onChange={(e) => setDataset(e.target.value)}>
          <option value="">— select —</option>
          {datasets.map((d) => {
            const k = d.fullname ?? d.name;
            return <option key={k} value={k}>{k}</option>;
          })}
        </select>
      </div>
      <div className="field">
        <label className="field__label">Snapshot name</label>
        <input
          className="input"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="manual-2026-04-29"
        />
      </div>
    </Modal>
  );
}

function HoldsModal({ fullname, onClose }: { fullname: string; onClose: () => void }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["holds", fullname],
    queryFn: () => storage.listHolds(fullname),
  });
  const inval = () => qc.invalidateQueries({ queryKey: ["holds", fullname] });
  const [tag, setTag] = useState("");
  const [err, setErr] = useState<string | null>(null);

  const holdMut = useMutation({
    meta: { label: "Hold failed" },
    mutationFn: () => storage.holdSnapshot(fullname, tag),
    onSuccess: () => { setTag(""); inval(); toastSuccess("Hold added"); },
    onError: (e: Error) => setErr(e.message),
  });
  const releaseMut = useMutation({
    meta: { label: "Release failed" },
    mutationFn: (t: string) => storage.releaseSnapshot(fullname, t),
    onSuccess: () => { inval(); toastSuccess("Hold released"); },
  });

  return (
    <Modal title="Holds" sub={fullname} onClose={onClose}
      footer={<button className="btn" onClick={onClose}>Close</button>}
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="sect">
        <div className="sect__title">Active holds</div>
        <div className="sect__body">
          {q.isLoading && <div className="muted">Loading…</div>}
          {q.data && q.data.length === 0 && <div className="muted">No holds.</div>}
          {q.data && q.data.length > 0 && (
            <table className="tbl tbl--compact">
              <tbody>
                {q.data.map((t) => (
                  <tr key={t}>
                    <td className="mono">{t}</td>
                    <td className="num">
                      <button
                        className="btn btn--sm btn--danger"
                        disabled={releaseMut.isPending}
                        onClick={() => releaseMut.mutate(t)}
                      >
                        Release
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
      <div className="field">
        <label className="field__label">Add hold tag</label>
        <div className="row gap-8">
          <input className="input" value={tag} onChange={(e) => setTag(e.target.value)} placeholder="legal-hold" />
          <button
            className="btn btn--sm"
            disabled={holdMut.isPending || !tag}
            onClick={() => { setErr(null); holdMut.mutate(); }}
          >
            Hold
          </button>
        </div>
      </div>
    </Modal>
  );
}

export default SnapshotsTab;
