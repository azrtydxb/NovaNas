import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { storage, type Pool } from "../../api/storage";
import { formatBytes } from "../../lib/format";
import { Modal } from "./Modal";

type Props = { onPick: (name: string) => void };

function poolUsed(p: Pool): number {
  return p.used ?? p.alloc ?? 0;
}
function poolTotal(p: Pool): number {
  return p.total ?? p.size ?? 0;
}

export function PoolsTab({ onPick }: Props) {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["pools"], queryFn: () => storage.listPools() });
  const pools = q.data ?? [];
  const totalSize = pools.reduce((m, p) => m + poolTotal(p), 0);

  const [detailFor, setDetailFor] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [showImport, setShowImport] = useState(false);

  const inval = () => {
    qc.invalidateQueries({ queryKey: ["pools"] });
    if (detailFor) qc.invalidateQueries({ queryKey: ["pool", detailFor] });
  };

  const scrubMut = useMutation({ mutationFn: (n: string) => storage.scrubPool(n), onSuccess: inval });
  const trimMut = useMutation({ mutationFn: (n: string) => storage.trimPool(n), onSuccess: inval });
  const clearMut = useMutation({ mutationFn: (n: string) => storage.clearPool(n), onSuccess: inval });
  const checkpointMut = useMutation({ mutationFn: (n: string) => storage.checkpointPool(n), onSuccess: inval });
  const exportMut = useMutation({ mutationFn: (n: string) => storage.exportPool(n), onSuccess: inval });
  const syncMut = useMutation({ mutationFn: () => storage.syncPools(), onSuccess: inval });

  const detail = pools.find((p) => p.name === detailFor) ?? null;

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setShowCreate(true)}>
          <Icon name="plus" size={11} />
          Create pool
        </button>
        <button className="btn" onClick={() => setShowImport(true)}>
          <Icon name="download" size={11} />
          Import
        </button>
        <button className="btn" disabled={syncMut.isPending} onClick={() => syncMut.mutate()}>
          <Icon name="refresh" size={11} />
          Sync
        </button>
        <span className="muted" style={{ marginLeft: "auto", fontSize: 11 }}>
          {pools.length} pools · {formatBytes(totalSize)} total
        </span>
      </div>
      {q.isLoading && <div className="empty-hint">Loading pools…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && pools.length === 0 && <div className="empty-hint">No pools.</div>}
      {pools.length > 0 && (
        <div className="cards-grid">
          {pools.map((p) => {
            const used = poolUsed(p);
            const total = poolTotal(p);
            const pct = total > 0 ? used / total : 0;
            const tier = p.tier ?? "warm";
            const state = p.state ?? p.health ?? "ONLINE";
            const healthy = /online|healthy/i.test(state);
            return (
              <div
                key={p.name}
                className="pool-card"
                onClick={() => setDetailFor(p.name)}
                style={{ cursor: "pointer" }}
              >
                <div className="pool-card__head">
                  <div className="pool-card__name">
                    <Icon name="storage" size={14} />
                    <span>{p.name}</span>
                    <span className={`tier tier--${tier}`}>{tier}</span>
                  </div>
                  <span className={`pill pill--${healthy ? "ok" : "warn"}`}>
                    <span className="dot" />
                    {state}
                  </span>
                </div>
                <div className="pool-card__meta">
                  <span>
                    {p.disks ?? "—"} disks · {p.devices ?? "—"}
                  </span>
                  <span>{p.protection ?? "—"}</span>
                </div>
                <div className="bar">
                  <div className="bar__fill" style={{ width: `${pct * 100}%` }} />
                </div>
                <div className="pool-card__nums">
                  <span className="mono">
                    {formatBytes(used)} / {formatBytes(total)}
                  </span>
                  <span className="muted mono">{(pct * 100).toFixed(1)}%</span>
                </div>
                <div className="pool-card__io">
                  <div>
                    <span className="muted">R</span>{" "}
                    <span className="mono">{p.throughput?.r ?? 0} MB/s</span>
                  </div>
                  <div>
                    <span className="muted">W</span>{" "}
                    <span className="mono">{p.throughput?.w ?? 0} MB/s</span>
                  </div>
                  <div>
                    <span className="muted">IOPS</span>{" "}
                    <span className="mono">
                      {((p.iops?.r ?? 0) / 1000).toFixed(1)}k
                    </span>
                  </div>
                </div>
                <div className="pool-card__scrub">
                  <span className="muted">scrub: {p.scrubLast ?? "—"}</span>
                  <span className="muted">next: {p.scrubNext ?? "—"}</span>
                </div>
                <div
                  className="row gap-8"
                  style={{ flexWrap: "wrap", paddingTop: 6, borderTop: "1px solid var(--line)" }}
                  onClick={(e) => e.stopPropagation()}
                >
                  <button
                    className="btn btn--sm"
                    disabled={scrubMut.isPending}
                    onClick={() => scrubMut.mutate(p.name)}
                  >
                    Scrub
                  </button>
                  <button
                    className="btn btn--sm"
                    disabled={trimMut.isPending}
                    onClick={() => trimMut.mutate(p.name)}
                  >
                    Trim
                  </button>
                  <button
                    className="btn btn--sm"
                    disabled={clearMut.isPending}
                    onClick={() => clearMut.mutate(p.name)}
                  >
                    Clear
                  </button>
                  <button
                    className="btn btn--sm"
                    disabled={checkpointMut.isPending}
                    onClick={() => checkpointMut.mutate(p.name)}
                  >
                    Checkpoint
                  </button>
                  <button
                    className="btn btn--sm"
                    onClick={() => onPick(p.name)}
                  >
                    Vdevs
                  </button>
                  <button
                    className="btn btn--sm btn--danger"
                    style={{ marginLeft: "auto" }}
                    disabled={exportMut.isPending}
                    onClick={() => {
                      if (window.confirm(`Export pool "${p.name}"? It will be unmounted.`)) {
                        exportMut.mutate(p.name);
                      }
                    }}
                  >
                    Export
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {detail && (
        <PoolDetailModal
          pool={detail}
          onClose={() => setDetailFor(null)}
          onVdevs={() => {
            const n = detail.name;
            setDetailFor(null);
            onPick(n);
          }}
        />
      )}
      {showCreate && <CreatePoolModal onClose={() => setShowCreate(false)} />}
      {showImport && (
        <ImportPoolModal
          onClose={() => setShowImport(false)}
          onDone={inval}
        />
      )}
    </div>
  );
}

function PoolDetailModal({
  pool,
  onClose,
  onVdevs,
}: {
  pool: Pool;
  onClose: () => void;
  onVdevs: () => void;
}) {
  const propsQ = useQuery({
    queryKey: ["pool-props", pool.name],
    queryFn: () => storage.getPoolProperties(pool.name),
  });
  const props = (propsQ.data ?? {}) as Record<string, unknown>;
  return (
    <Modal title={`Pool · ${pool.name}`} sub={pool.state ?? pool.health ?? ""} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Close</button>
          <button className="btn btn--primary" onClick={onVdevs}>Manage VDEVs</button>
        </>
      }
    >
      <div className="sect">
        <div className="sect__title">Summary</div>
        <div className="sect__body">
          <dl className="kv">
            <dt>State</dt><dd>{pool.state ?? pool.health ?? "—"}</dd>
            <dt>Protection</dt><dd>{pool.protection ?? "—"}</dd>
            <dt>Devices</dt><dd>{pool.devices ?? "—"}</dd>
            <dt>Disks</dt><dd>{pool.disks ?? "—"}</dd>
            <dt>Used</dt><dd>{formatBytes(pool.used ?? pool.alloc ?? 0)}</dd>
            <dt>Total</dt><dd>{formatBytes(pool.total ?? pool.size ?? 0)}</dd>
            <dt>Free</dt><dd>{formatBytes(pool.free ?? 0)}</dd>
            <dt>Frag</dt><dd>{pool.fragmentation != null ? `${pool.fragmentation}%` : "—"}</dd>
          </dl>
        </div>
      </div>
      <div className="sect">
        <div className="sect__title">Properties</div>
        <div className="sect__body">
          {propsQ.isLoading && <div className="muted">Loading…</div>}
          {Object.keys(props).length === 0 && !propsQ.isLoading && (
            <div className="muted">No properties.</div>
          )}
          {Object.keys(props).length > 0 && (
            <table className="tbl tbl--compact">
              <tbody>
                {Object.entries(props).slice(0, 60).map(([k, v]) => (
                  <tr key={k}>
                    <td className="mono" style={{ fontSize: 11 }}>{k}</td>
                    <td className="mono muted" style={{ fontSize: 11 }}>{String(v)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </Modal>
  );
}

function CreatePoolModal({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Create pool" onClose={onClose}
      footer={<button className="btn" onClick={onClose}>Close</button>}
    >
      <div className="modal__err" style={{ background: "transparent", color: "var(--fg-2)" }}>
        Pool creation flow is multi-step (select disks, pick layout, set tier).
        The backend has no <code>POST /pools</code> endpoint yet — coming next.
        For now, use <strong>Import</strong> to bring an existing pool online.
      </div>
    </Modal>
  );
}

function ImportPoolModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState("");
  const [dir, setDir] = useState("");
  const [force, setForce] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: () => storage.importPool({ name: name || undefined, dir: dir || undefined, force }),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Import pool" sub="Discover and import an existing ZFS pool" onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Importing…" : "Import"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Pool name (optional)</label>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="leave blank to import all discoverable" />
      </div>
      <div className="field">
        <label className="field__label">Search directory (optional)</label>
        <input className="input" value={dir} onChange={(e) => setDir(e.target.value)} placeholder="/dev" />
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />
          Force import (use with caution)
        </label>
      </div>
    </Modal>
  );
}

export default PoolsTab;
