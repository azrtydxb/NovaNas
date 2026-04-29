import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { storage, type Vdev } from "../../api/storage";

type Props = { pool: string | null; setPool: (n: string) => void };

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

  const scrubMut = useMutation({
    mutationFn: (name: string) => storage.scrubPool(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pool", active] }),
  });

  const cur = detailQ.data;
  const vdevs = flattenVdevs(cur?.vdevs);

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
              <button className="btn btn--sm">
                <Icon name="bolt" size={9} />
                Trim
              </button>
              <button className="btn btn--sm">
                <Icon name="more" size={11} />
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
                    {vdevs.length === 0 && (
                      <tr>
                        <td colSpan={4} className="muted">
                          No VDEV data
                        </td>
                      </tr>
                    )}
                    {vdevs.map((v) => {
                      const t = v.type ?? "";
                      const pillKind = t.startsWith("mirror")
                        ? "info"
                        : t.startsWith("raidz")
                          ? "warn"
                          : "";
                      const okState =
                        v.state === "ONLINE" || v.state === "AVAIL";
                      return (
                        <tr key={v.name}>
                          <td className="mono">{v.name}</td>
                          <td>
                            <span className={`pill pill--${pillKind}`}>{t}</span>
                          </td>
                          <td>
                            <span
                              className={`sdot sdot--${okState ? "ok" : "warn"}`}
                            />{" "}
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
    </div>
  );
}

export default VdevsTab;
