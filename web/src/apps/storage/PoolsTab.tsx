import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { storage, type Pool, type PoolDependent, type PoolDependentKind } from "../../api/storage";
import { formatBytes } from "../../lib/format";
import { toastSuccess } from "../../store/toast";
import { Modal } from "./Modal";

function poolUsed(p: Pool): number {
  return p.used ?? p.alloc ?? 0;
}
function poolTotal(p: Pool): number {
  return p.total ?? p.size ?? 0;
}

export function PoolsTab() {
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

  // Per-pool actions (Scrub/Trim/Clear/Checkpoint/Wait/Discard/Export)
  // moved into <PoolDetailModal> to match the design — pool cards are
  // informational only, click opens detail.
  const syncMut = useMutation({
    meta: { label: "Sync failed" },
    mutationFn: () => storage.syncPools(),
    onSuccess: () => { inval(); toastSuccess("Pools refreshed"); },
  });

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
                {/* Per design: pool cards are informational. Actions
                    live on the detail modal opened by clicking the
                    card, and on the Vdev tab toolbar. */}
              </div>
            );
          })}
        </div>
      )}

      {detail && (
        <PoolDetailModal
          pool={detail}
          onClose={() => setDetailFor(null)}
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
}: {
  pool: Pool;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const propsQ = useQuery({
    queryKey: ["pool-props", pool.name],
    queryFn: () => storage.getPoolProperties(pool.name),
  });
  const props = (propsQ.data ?? {}) as Record<string, unknown>;

  const inval = () => qc.invalidateQueries({ queryKey: ["pools"] });
  const m = (label: string, fn: () => Promise<unknown>, ok: string, andClose = false) =>
    useMutation({
      meta: { label: `${label} failed` },
      mutationFn: fn,
      onSuccess: () => {
        inval();
        toastSuccess(ok, `Pool ${pool.name}`);
        if (andClose) onClose();
      },
    });
  const scrub = m("Scrub", () => storage.scrubPool(pool.name), "Scrub started");
  const trim = m("Trim", () => storage.trimPool(pool.name), "Trim started");
  const clear = m("Clear", () => storage.clearPool(pool.name), "Errors cleared");
  const exportPool = m("Export", () => storage.exportPool(pool.name), "Pool exported", true);
  const [showDelete, setShowDelete] = useState(false);

  return (
    <Modal title={`Pool · ${pool.name}`} sub={pool.state ?? pool.health ?? ""} onClose={onClose}
      footer={<button className="btn" onClick={onClose}>Close</button>}
    >
      <div className="sect">
        <div className="sect__title">Actions</div>
        <div className="sect__body">
          <div className="row gap-8" style={{ flexWrap: "wrap" }}>
            <button className="btn btn--sm" disabled={scrub.isPending} onClick={() => scrub.mutate()}>Scrub</button>
            <button className="btn btn--sm" disabled={trim.isPending} onClick={() => trim.mutate()}>Trim</button>
            <button className="btn btn--sm" disabled={clear.isPending} onClick={() => clear.mutate()}>Clear errors</button>
            <button
              className="btn btn--sm btn--danger"
              style={{ marginLeft: "auto" }}
              disabled={exportPool.isPending}
              onClick={() => {
                if (window.confirm(`Export pool "${pool.name}"? It will be unmounted but data is preserved.`)) exportPool.mutate();
              }}
            >Export</button>
            <button
              className="btn btn--sm btn--danger"
              onClick={() => setShowDelete(true)}
            >Delete pool</button>
          </div>
        </div>
      </div>
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
      {showDelete && (
        <DeletePoolModal
          pool={pool.name}
          onClose={() => setShowDelete(false)}
          onDeleted={() => {
            setShowDelete(false);
            onClose();
          }}
        />
      )}
    </Modal>
  );
}

const DEPENDENT_LABELS: Record<PoolDependentKind, string> = {
  "dataset": "Datasets",
  "share": "Shares",
  "iscsi-target": "iSCSI targets",
  "replication-job": "Replication jobs",
  "replication-schedule": "Replication schedules",
  "snapshot-schedule": "Snapshot schedules",
  "scrub-policy": "Scrub policies",
  "plugin": "Plugins",
};

const DEPENDENT_ORDER: PoolDependentKind[] = [
  "plugin",
  "share",
  "iscsi-target",
  "replication-job",
  "replication-schedule",
  "snapshot-schedule",
  "scrub-policy",
  "dataset",
];

function DeletePoolModal({
  pool,
  onClose,
  onDeleted,
}: {
  pool: string;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const qc = useQueryClient();
  const [confirmText, setConfirmText] = useState("");
  const depsQ = useQuery({
    queryKey: ["pool-dependents", pool],
    queryFn: () => storage.getPoolDependents(pool),
    refetchInterval: 5_000,
  });
  const dependents = depsQ.data?.dependents ?? [];
  const blocking = dependents.filter((d) => d.blocking);
  const grouped = new Map<PoolDependentKind, PoolDependent[]>();
  for (const d of dependents) {
    const arr = grouped.get(d.kind) ?? [];
    arr.push(d);
    grouped.set(d.kind, arr);
  }

  const deletePool = useMutation({
    meta: { label: "Delete pool failed" },
    mutationFn: () => storage.deletePool(pool),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["pools"] });
      toastSuccess("Pool deleted", pool);
      onDeleted();
    },
  });

  const canConfirm =
    !depsQ.isLoading &&
    blocking.length === 0 &&
    confirmText === pool &&
    !deletePool.isPending;

  return (
    <Modal
      title={`Delete pool · ${pool}`}
      sub="Permanently destroy the pool and all its data"
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--danger"
            disabled={!canConfirm}
            onClick={() => deletePool.mutate()}
          >
            {deletePool.isPending ? "Deleting…" : "Destroy pool"}
          </button>
        </>
      }
    >
      <div className="sect">
        <div className="sect__title">Dependents</div>
        <div className="sect__body">
          {depsQ.isLoading && <div className="muted">Checking what uses this pool…</div>}
          {depsQ.isError && (
            <div className="discover__msg discover__msg--err">
              Failed to load: {(depsQ.error as Error).message}
            </div>
          )}
          {!depsQ.isLoading && !depsQ.isError && dependents.length === 0 && (
            <div className="muted">Nothing references this pool.</div>
          )}
          {DEPENDENT_ORDER.map((kind) => {
            const items = grouped.get(kind);
            if (!items || items.length === 0) return null;
            const isBlocking = items.some((i) => i.blocking);
            return (
              <div key={kind} style={{ marginBottom: 12 }}>
                <div
                  className="sect__title"
                  style={{ color: isBlocking ? "var(--danger, #ef4444)" : undefined, fontSize: 11 }}
                >
                  {DEPENDENT_LABELS[kind]} ({items.length})
                </div>
                <table className="tbl tbl--compact">
                  <tbody>
                    {items.map((d) => (
                      <tr key={`${d.kind}:${d.id}`}>
                        <td className="mono" style={{ fontSize: 11 }}>{d.name}</td>
                        <td className="mono muted" style={{ fontSize: 11 }}>{d.detail ?? ""}</td>
                        <td style={{ fontSize: 11 }}>
                          {d.enabled === false && <span className="muted">disabled</span>}
                          {d.enabled === true && <span style={{ color: "var(--danger, #ef4444)" }}>enabled</span>}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            );
          })}
          {blocking.length > 0 && (
            <div className="muted" style={{ marginTop: 12, fontSize: 11 }}>
              Remove or disable the {blocking.length} blocking dependent{blocking.length === 1 ? "" : "s"} above before destroying the pool. This list refreshes every 5 seconds.
            </div>
          )}
        </div>
      </div>
      <div className="sect">
        <div className="sect__title">Confirm</div>
        <div className="sect__body">
          <div className="muted" style={{ marginBottom: 8, fontSize: 11 }}>
            This <strong>permanently destroys all data</strong> in pool <code>{pool}</code> and cannot be undone. Type the pool name to confirm.
          </div>
          <input
            type="text"
            className="input"
            placeholder={pool}
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            disabled={blocking.length > 0}
            autoFocus
          />
        </div>
      </div>
    </Modal>
  );
}

type Vdev = { type: string; devices: string };

function CreatePoolModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [mountpoint, setMountpoint] = useState("");
  const [ashift, setAshift] = useState("12");
  const [autotrim, setAutotrim] = useState(true);
  const [force, setForce] = useState(false);
  const [vdevs, setVdevs] = useState<Vdev[]>([{ type: "stripe", devices: "" }]);
  const [err, setErr] = useState<string | null>(null);

  const updateVdev = (i: number, patch: Partial<Vdev>) =>
    setVdevs((vs) => vs.map((v, idx) => (idx === i ? { ...v, ...patch } : v)));
  const removeVdev = (i: number) => setVdevs((vs) => vs.filter((_, idx) => idx !== i));
  const addVdev = () => setVdevs((vs) => [...vs, { type: "mirror", devices: "" }]);

  const m = useMutation({
    meta: { label: "Create pool failed" },
    mutationFn: () => {
      const v = vdevs
        .filter((x) => x.devices.trim().length > 0)
        .map((x) => ({
          type: x.type,
          devices: x.devices.split(/[\s,]+/).map((d) => d.trim()).filter(Boolean),
        }));
      return storage.createPool({
        name,
        vdevs: v,
        properties: { ashift, ...(autotrim ? { autotrim: "on" } : {}) },
        mountpoint: mountpoint || undefined,
        force,
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["pools"] });
      toastSuccess("Pool created", name);
      onClose();
    },
    onError: (e: Error) => setErr(e.message),
  });

  const valid = name.trim() && vdevs.some((v) => v.devices.trim());
  return (
    <Modal
      title="Create pool"
      sub="Compose vdevs from raw block devices"
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose} disabled={m.isPending}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={!valid || m.isPending}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Creating…" : "Create pool"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Pool name</label>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="tank" autoFocus />
      </div>
      <div className="field">
        <label className="field__label">Mountpoint (optional)</label>
        <input className="input" value={mountpoint} onChange={(e) => setMountpoint(e.target.value)} placeholder="/mnt/tank" />
      </div>
      <div className="field">
        <label className="field__label">vdevs</label>
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {vdevs.map((v, i) => (
            <div key={i} className="row gap-8" style={{ alignItems: "stretch" }}>
              <select
                className="input"
                style={{ flex: "0 0 130px" }}
                value={v.type}
                onChange={(e) => updateVdev(i, { type: e.target.value })}
              >
                <option value="stripe">stripe</option>
                <option value="mirror">mirror</option>
                <option value="raidz1">raidz1</option>
                <option value="raidz2">raidz2</option>
                <option value="raidz3">raidz3</option>
                <option value="log">log (SLOG)</option>
                <option value="cache">cache (L2ARC)</option>
                <option value="spare">spare</option>
                <option value="special">special</option>
                <option value="dedup">dedup</option>
              </select>
              <input
                className="input"
                value={v.devices}
                onChange={(e) => updateVdev(i, { devices: e.target.value })}
                placeholder="/dev/sda /dev/sdb"
                style={{ flex: 1 }}
              />
              {vdevs.length > 1 && (
                <button className="btn btn--sm" onClick={() => removeVdev(i)}>
                  <Icon name="trash" size={11} />
                </button>
              )}
            </div>
          ))}
          <button className="btn btn--sm" style={{ alignSelf: "flex-start" }} onClick={addVdev}>
            <Icon name="plus" size={11} /> Add vdev
          </button>
        </div>
      </div>
      <div className="field">
        <label className="field__label">ashift</label>
        <select className="input" value={ashift} onChange={(e) => setAshift(e.target.value)}>
          <option value="9">9 (512B sectors)</option>
          <option value="12">12 (4K sectors — recommended)</option>
          <option value="13">13 (8K sectors)</option>
        </select>
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={autotrim} onChange={(e) => setAutotrim(e.target.checked)} />
          autotrim=on
        </label>
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />
          Force create (use with caution — will overwrite existing data on disks)
        </label>
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
    meta: { label: "Import failed" },
    mutationFn: () => storage.importPool({ name: name || undefined, dir: dir || undefined, force }),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Pool imported", name || "auto-discovered"); },
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
