import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { storage, type Disk } from "../../api/storage";
import { formatBytes } from "../../lib/format";

function diskCap(d: Disk): number {
  return d.capacity ?? d.size ?? d.cap ?? 0;
}

export function DisksTab() {
  const qc = useQueryClient();
  const [sel, setSel] = useState<string | null>(null);
  const q = useQuery({ queryKey: ["disks"], queryFn: () => storage.listDisks() });
  const disks = q.data ?? [];
  const selected = disks.find((d) => d.name === sel) ?? disks[0];

  const smartQ = useQuery({
    queryKey: ["smart", selected?.name],
    queryFn: () => storage.getSmart(selected!.name),
    enabled: !!selected && selected.state !== "EMPTY",
  });

  const enableMut = useMutation({
    mutationFn: (n: string) => storage.enableSmart(n),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["smart", selected?.name] }),
  });

  const testMut = useMutation({
    mutationFn: ({ n, type }: { n: string; type: "short" | "long" | "conveyance" | "offline" }) =>
      storage.startSmartTest(n, type),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["smart", selected?.name] }),
  });

  return (
    <div className="disks-split">
      <div>
        <div className="encl-title">ENCLOSURE 0 · {disks.length}-bay</div>
        {q.isLoading && <div className="empty-hint">Loading disks…</div>}
        {q.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(q.error as Error).message}
          </div>
        )}
        {q.data && disks.length === 0 && <div className="empty-hint">No disks.</div>}
        {disks.length > 0 && (
          <div className="encl-grid">
            {disks.map((d, i) => {
              const slot = d.slot ?? i + 1;
              const empty = d.state === "EMPTY";
              const isSel = (selected?.name ?? null) === d.name;
              const cap = diskCap(d);
              const modelTail = d.model ? d.model.split(" ").slice(-1)[0] : "";
              return (
                <div
                  key={d.name}
                  className={`encl-slot ${isSel ? "is-on" : ""} ${empty ? "is-empty" : ""}`}
                  data-state={d.state}
                  onClick={() => setSel(d.name)}
                  style={{ cursor: "pointer" }}
                >
                  <div className="encl-slot__top">
                    <span className="mono num">{String(slot).padStart(2, "0")}</span>
                    <span className="led" />
                  </div>
                  <div className="encl-slot__bot">
                    {empty ? (
                      <span className="muted mono" style={{ fontSize: 9 }}>empty</span>
                    ) : (
                      <>
                        <span className="mono" style={{ fontSize: 10, color: "var(--fg-2)" }}>
                          {modelTail || d.name}
                        </span>
                        <span className="mono" style={{ fontSize: 10, color: "var(--fg-0)" }}>
                          {cap > 0 ? formatBytes(cap) : "—"}
                        </span>
                      </>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
      <div className="disk-detail">
        {!selected && <div className="empty-hint">Select a disk</div>}
        {selected && selected.state === "EMPTY" && (
          <div className="empty-hint">Empty slot · insert a disk to initialize</div>
        )}
        {selected && selected.state !== "EMPTY" && (
          <>
            <div className="disk-detail__head">
              <div>
                <div className="muted mono" style={{ fontSize: 11 }}>
                  SLOT {String(selected.slot ?? "—").padStart(2, "0")} ·{" "}
                  {selected.class ?? "—"}
                </div>
                <div className="disk-detail__title">{selected.model ?? selected.name}</div>
              </div>
              <span
                className={`pill pill--${
                  selected.state === "DEGRADED"
                    ? "warn"
                    : selected.state === "ACTIVE" || selected.state === "ONLINE"
                      ? "ok"
                      : "info"
                }`}
              >
                <span className="dot" />
                {selected.state ?? "—"}
              </span>
            </div>
            <dl className="kv">
              <dt>Name</dt><dd className="mono">{selected.name}</dd>
              <dt>Serial</dt><dd>{selected.serial ?? "—"}</dd>
              <dt>Capacity</dt><dd>{formatBytes(diskCap(selected))}</dd>
              <dt>Pool</dt><dd>{selected.pool ?? "—"}</dd>
              <dt>Temperature</dt>
              <dd>
                {selected.temperature ?? selected.temp ?? "—"}
                {(selected.temperature ?? selected.temp) != null ? "°C" : ""}
              </dd>
              <dt>Power-on hours</dt>
              <dd>{(selected.hours ?? 0).toLocaleString()}</dd>
              <dt>SMART pass</dt>
              <dd>{selected.smart?.passed ? "yes" : "no"}</dd>
              <dt>Reallocated</dt>
              <dd
                style={{
                  color: (selected.smart?.reallocated ?? 0) > 0 ? "var(--warn)" : undefined,
                }}
              >
                {selected.smart?.reallocated ?? 0}
              </dd>
              <dt>Pending</dt>
              <dd>{selected.smart?.pending ?? 0}</dd>
            </dl>

            <div className="row gap-8" style={{ flexWrap: "wrap", marginTop: 10 }}>
              <button
                className="btn btn--sm"
                disabled={testMut.isPending}
                onClick={() => testMut.mutate({ n: selected.name, type: "short" })}
              >
                SMART · short
              </button>
              <button
                className="btn btn--sm"
                disabled={testMut.isPending}
                onClick={() => testMut.mutate({ n: selected.name, type: "long" })}
              >
                SMART · long
              </button>
              <button
                className="btn btn--sm"
                disabled={testMut.isPending}
                onClick={() => testMut.mutate({ n: selected.name, type: "conveyance" })}
              >
                Conveyance
              </button>
              <button
                className="btn btn--sm"
                disabled={enableMut.isPending}
                onClick={() => enableMut.mutate(selected.name)}
              >
                Enable SMART
              </button>
            </div>

            <div className="sect" style={{ marginTop: 10 }}>
              <div className="sect__title">SMART data</div>
              <div className="sect__body">
                {smartQ.isLoading && <div className="muted">Loading SMART…</div>}
                {smartQ.isError && (
                  <div className="muted" style={{ color: "var(--err)" }}>
                    {(smartQ.error as Error).message}
                  </div>
                )}
                {smartQ.data && (
                  <>
                    <div className="muted" style={{ marginBottom: 6 }}>
                      Overall: {smartQ.data.passed ? "PASSED" : "FAILED / unknown"}
                    </div>
                    {smartQ.data.attributes && (
                      <table className="tbl tbl--compact">
                        <tbody>
                          {Object.entries(smartQ.data.attributes).slice(0, 20).map(([k, v]) => (
                            <tr key={k}>
                              <td className="mono" style={{ fontSize: 11 }}>{k}</td>
                              <td className="mono muted" style={{ fontSize: 11 }}>{String(v)}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    )}
                  </>
                )}
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

export default DisksTab;
