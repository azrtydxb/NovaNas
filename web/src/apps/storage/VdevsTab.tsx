import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { storage, type Vdev } from "../../api/storage";
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

  const scrubMut = useMutation({ mutationFn: (n: string) => storage.scrubPool(n), onSuccess: inval });
  const trimMut = useMutation({ mutationFn: (n: string) => storage.trimPool(n), onSuccess: inval });
  const onlineMut = useMutation({
    mutationFn: ({ p, d }: { p: string; d: string }) => storage.onlineDevice(p, d),
    onSuccess: inval,
  });
  const offlineMut = useMutation({
    mutationFn: ({ p, d }: { p: string; d: string }) => storage.offlineDevice(p, d),
    onSuccess: inval,
  });

  const [replaceFor, setReplaceFor] = useState<string | null>(null);

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
    </div>
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
    mutationFn: () => storage.replaceDevice(pool, { old_device: oldDevice, new_device: newDev }),
    onSuccess: () => { onDone(); onClose(); },
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
