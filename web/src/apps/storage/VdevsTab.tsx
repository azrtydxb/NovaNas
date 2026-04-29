import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { storage, type Vdev } from "../../api/storage";
import { toastSuccess } from "../../store/toast";
import { Modal } from "./Modal";

type Props = { pool: string | null; setPool: (n: string) => void };

type FlatRow = { vdev: Vdev; disk: string | null };

function flattenDisks(vs: Vdev[] | undefined): FlatRow[] {
  if (!vs) return [];
  const out: FlatRow[] = [];
  const walk = (list: Vdev[]) => {
    for (const v of list) {
      if (v.disks && v.disks.length > 0) {
        for (const d of v.disks) out.push({ vdev: v, disk: d });
      } else if (!v.children?.length) {
        out.push({ vdev: v, disk: null });
      }
      if (v.children?.length) walk(v.children);
    }
  };
  walk(vs);
  return out;
}

function flattenVdevs(vs: Vdev[] | undefined): Vdev[] {
  if (!vs) return [];
  const out: Vdev[] = [];
  const walk = (list: Vdev[]) => {
    for (const v of list) {
      out.push(v);
      if (v.children?.length) walk(v.children);
    }
  };
  walk(vs);
  return out;
}

export function VdevsTab({ pool, setPool }: Props) {
  const qc = useQueryClient();
  const poolsQ = useQuery({ queryKey: ["pools"], queryFn: () => storage.listPools() });
  const pools = poolsQ.data ?? [];
  const active = pool ?? pools[0]?.name ?? null;

  const detailQ = useQuery({
    queryKey: ["pool", active],
    queryFn: () => storage.getPool(active!),
    enabled: !!active,
  });

  const inval = () => {
    qc.invalidateQueries({ queryKey: ["pool", active] });
    qc.invalidateQueries({ queryKey: ["pools"] });
  };

  const scrubMut = useMutation({
    meta: { label: "Scrub failed" },
    mutationFn: (n: string) => storage.scrubPool(n),
    onSuccess: (_d, n) => { inval(); toastSuccess("Scrub started", `Pool ${n}`); },
  });
  const trimMut = useMutation({
    meta: { label: "Trim failed" },
    mutationFn: (n: string) => storage.trimPool(n),
    onSuccess: (_d, n) => { inval(); toastSuccess("Trim started", `Pool ${n}`); },
  });
  const waitMut = useMutation({
    meta: { label: "Wait failed" },
    mutationFn: (n: string) => storage.waitPool(n),
    onSuccess: (_d, n) => { inval(); toastSuccess("Wait complete", `Pool ${n}`); },
  });
  const onlineMut = useMutation({
    meta: { label: "Online failed" },
    mutationFn: ({ p, d }: { p: string; d: string }) => storage.onlineDevice(p, d),
    onSuccess: (_d, v) => { inval(); toastSuccess("Device online", v.d); },
  });
  const offlineMut = useMutation({
    meta: { label: "Offline failed" },
    mutationFn: ({ p, d }: { p: string; d: string }) => storage.offlineDevice(p, d),
    onSuccess: (_d, v) => { inval(); toastSuccess("Device offline", v.d); },
  });
  const detachMut = useMutation({
    meta: { label: "Detach failed" },
    mutationFn: ({ p, d }: { p: string; d: string }) => storage.detachFromPool(p, d),
    onSuccess: (_d, v) => { inval(); toastSuccess("Device detached", v.d); },
  });

  const [replaceFor, setReplaceFor] = useState<string | null>(null);
  const [attachFor, setAttachFor] = useState<string | null>(null);
  const [showAddVdev, setShowAddVdev] = useState(false);

  const cur = detailQ.data;
  const rows = flattenDisks(cur?.vdevs);
  const allVdevs = flattenVdevs(cur?.vdevs);

  return (
    <div
      style={{
        padding: 14,
        display: "grid",
        gridTemplateColumns: "160px 1fr",
        gap: 14,
      }}
    >
      <div className="vlist">
        <div className="vlist__title">POOLS</div>
        {pools.map((p) => (
          <button
            key={p.name}
            className={`vlist__item ${active === p.name ? "is-on" : ""}`}
            onClick={() => setPool(p.name)}
          >
            <span className={`tier-mark tier-mark--${p.tier ?? "warm"}`} />
            {p.name}
          </button>
        ))}
      </div>
      <div className="col gap-12">
        {!active && <div className="empty-hint">Select a pool</div>}
        {active && detailQ.isLoading && <div className="empty-hint">Loading…</div>}
        {active && detailQ.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(detailQ.error as Error).message}
          </div>
        )}
        {cur && (
          <>
            <div className="row gap-8" style={{ flexWrap: "wrap" }}>
              <span className="pill pill--ok">
                <span className="dot" />
                {cur.state ?? cur.health ?? "ONLINE"}
              </span>
              {cur.protection && <span className="pill">{cur.protection}</span>}
              {cur.devices && <span className="pill">{cur.devices}</span>}
              <button
                className="btn btn--sm"
                style={{ marginLeft: "auto" }}
                disabled={scrubMut.isPending}
                onClick={() => scrubMut.mutate(cur.name)}
              >
                <Icon name="play" size={9} />
                Scrub now
              </button>
              <button
                className="btn btn--sm"
                disabled={trimMut.isPending}
                onClick={() => trimMut.mutate(cur.name)}
              >
                <Icon name="bolt" size={9} />
                Trim
              </button>
              <button
                className="btn btn--sm"
                disabled={waitMut.isPending}
                onClick={() => waitMut.mutate(cur.name)}
                title="Wait for resilver / scrub"
              >
                Wait
              </button>
              <button
                className="btn btn--sm"
                onClick={() => setShowAddVdev(true)}
                title="Add a new VDEV to the pool"
              >
                <Icon name="plus" size={9} />
                Add VDEV
              </button>
            </div>

            <div className="sect">
              <div className="sect__head">
                <div className="sect__title">VDEV layout</div>
              </div>
              <div className="sect__body">
                <table className="tbl tbl--compact">
                  <thead>
                    <tr>
                      <th>VDEV</th>
                      <th>Type</th>
                      <th>State</th>
                      <th>Disks</th>
                    </tr>
                  </thead>
                  <tbody>
                    {allVdevs.length === 0 && (
                      <tr>
                        <td colSpan={4} className="muted">No VDEV data</td>
                      </tr>
                    )}
                    {allVdevs.map((v) => {
                      const t = v.type ?? "";
                      const pillKind = t.startsWith("mirror")
                        ? "info"
                        : t.startsWith("raidz")
                          ? "warn"
                          : "";
                      const okState = v.state === "ONLINE" || v.state === "AVAIL";
                      return (
                        <tr key={v.name}>
                          <td className="mono">{v.name}</td>
                          <td>
                            <span className={`pill pill--${pillKind}`}>{t}</span>
                          </td>
                          <td>
                            <span className={`sdot sdot--${okState ? "ok" : "warn"}`} />{" "}
                            {v.state ?? "—"}
                          </td>
                          <td className="mono" style={{ fontSize: 11 }}>
                            {(v.disks ?? []).join(" · ")}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="sect">
              <div className="sect__head">
                <div className="sect__title">Per-disk actions</div>
              </div>
              <div className="sect__body">
                {rows.length === 0 && <div className="muted">No disks listed.</div>}
                {rows.length > 0 && (
                  <table className="tbl tbl--compact">
                    <thead>
                      <tr>
                        <th>VDEV</th>
                        <th>Device</th>
                        <th>State</th>
                        <th></th>
                      </tr>
                    </thead>
                    <tbody>
                      {rows.map((r, i) => (
                        <tr key={i}>
                          <td className="mono">{r.vdev.name}</td>
                          <td className="mono" style={{ fontSize: 11 }}>{r.disk ?? "—"}</td>
                          <td>{r.vdev.state ?? "—"}</td>
                          <td className="num">
                            {r.disk && (
                              <>
                                <button
                                  className="btn btn--sm"
                                  disabled={onlineMut.isPending}
                                  onClick={() => onlineMut.mutate({ p: cur.name, d: r.disk! })}
                                >
                                  Online
                                </button>{" "}
                                <button
                                  className="btn btn--sm"
                                  disabled={offlineMut.isPending}
                                  onClick={() => offlineMut.mutate({ p: cur.name, d: r.disk! })}
                                >
                                  Offline
                                </button>{" "}
                                <button
                                  className="btn btn--sm"
                                  onClick={() => setReplaceFor(r.disk!)}
                                >
                                  Replace
                                </button>{" "}
                                <button
                                  className="btn btn--sm"
                                  onClick={() => setAttachFor(r.disk!)}
                                  title="Attach a mirror to this device"
                                >
                                  Attach
                                </button>{" "}
                                <button
                                  className="btn btn--sm btn--danger"
                                  disabled={detachMut.isPending}
                                  onClick={() => {
                                    if (window.confirm(`Detach ${r.disk} from ${cur.name}?`)) {
                                      detachMut.mutate({ p: cur.name, d: r.disk! });
                                    }
                                  }}
                                >
                                  Detach
                                </button>
                              </>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            </div>

            <div className="sect">
              <div className="sect__head">
                <div className="sect__title">I/O</div>
              </div>
              <div className="sect__body">
                <div className="row gap-12" style={{ flexWrap: "wrap" }}>
                  <div className="kpi">
                    <div className="kpi__lbl">Read</div>
                    <div className="kpi__val mono">
                      {cur.throughput?.r ?? 0} <span className="muted">MB/s</span>
                    </div>
                  </div>
                  <div className="kpi">
                    <div className="kpi__lbl">Write</div>
                    <div className="kpi__val mono">
                      {cur.throughput?.w ?? 0} <span className="muted">MB/s</span>
                    </div>
                  </div>
                  <div className="kpi">
                    <div className="kpi__lbl">Read IOPS</div>
                    <div className="kpi__val mono">
                      {((cur.iops?.r ?? 0) / 1000).toFixed(1)}k
                    </div>
                  </div>
                  <div className="kpi">
                    <div className="kpi__lbl">Write IOPS</div>
                    <div className="kpi__val mono">
                      {((cur.iops?.w ?? 0) / 1000).toFixed(1)}k
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </>
        )}
      </div>

      {replaceFor && active && (
        <ReplaceDeviceModal
          pool={active}
          oldDevice={replaceFor}
          onClose={() => setReplaceFor(null)}
          onDone={inval}
        />
      )}
      {attachFor && active && (
        <AttachDeviceModal
          pool={active}
          device={attachFor}
          onClose={() => setAttachFor(null)}
          onDone={inval}
        />
      )}
      {showAddVdev && active && (
        <AddVdevModal
          pool={active}
          onClose={() => setShowAddVdev(false)}
          onDone={inval}
        />
      )}
    </div>
  );
}

function AttachDeviceModal({
  pool,
  device,
  onClose,
  onDone,
}: {
  pool: string;
  device: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [newDev, setNewDev] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Attach failed" },
    mutationFn: () => storage.attachToPool(pool, { device, new_device: newDev }),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Device attached", newDev); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Attach mirror device" sub={`Pool ${pool} · ${device}`} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !newDev}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Attaching…" : "Attach"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">New device path</label>
        <input className="input" value={newDev} onChange={(e) => setNewDev(e.target.value)} placeholder="/dev/disk/by-id/…" />
      </div>
    </Modal>
  );
}

function AddVdevModal({
  pool,
  onClose,
  onDone,
}: {
  pool: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [type, setType] = useState("mirror");
  const [devices, setDevices] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Add VDEV failed" },
    mutationFn: () => storage.addToPool(pool, {
      type,
      devices: devices.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean),
    }),
    onSuccess: () => { onDone(); onClose(); toastSuccess("VDEV added", `Pool ${pool}`); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Add VDEV" sub={`Pool ${pool}`} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !devices}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Adding…" : "Add"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">VDEV type</label>
        <select className="input" value={type} onChange={(e) => setType(e.target.value)}>
          <option value="mirror">mirror</option>
          <option value="raidz1">raidz1</option>
          <option value="raidz2">raidz2</option>
          <option value="raidz3">raidz3</option>
          <option value="stripe">stripe</option>
          <option value="cache">cache (L2ARC)</option>
          <option value="log">log (SLOG)</option>
          <option value="spare">spare</option>
        </select>
      </div>
      <div className="field">
        <label className="field__label">Devices (whitespace or comma-separated)</label>
        <input className="input" value={devices} onChange={(e) => setDevices(e.target.value)} placeholder="/dev/sda /dev/sdb" />
      </div>
    </Modal>
  );
}

function ReplaceDeviceModal({
  pool,
  oldDevice,
  onClose,
  onDone,
}: {
  pool: string;
  oldDevice: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [newDev, setNewDev] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Replace failed" },
    mutationFn: () => storage.replaceDevice(pool, { old_device: oldDevice, new_device: newDev }),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Device replaced", newDev); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Replace device" sub={`Pool ${pool} · ${oldDevice}`} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !newDev}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Replacing…" : "Replace"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">New device path</label>
        <input className="input" value={newDev} onChange={(e) => setNewDev(e.target.value)} placeholder="/dev/disk/by-id/…" />
      </div>
    </Modal>
  );
}

export default VdevsTab;
