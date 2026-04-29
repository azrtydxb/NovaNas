import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { storage, type Disk } from "../../api/storage";
import { formatBytes } from "../../lib/format";

function diskCap(d: Disk): number {
  return d.capacity ?? d.size ?? d.cap ?? 0;
}

export function DisksTab() {
  const [sel, setSel] = useState<string | null>(null);
  const q = useQuery({ queryKey: ["disks"], queryFn: () => storage.listDisks() });
  const disks = q.data ?? [];
  const selected = disks.find((d) => d.name === sel) ?? disks[0];

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
        {q.data && disks.length === 0 && (
          <div className="empty-hint">No disks.</div>
        )}
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
                    <span className="mono num">
                      {String(slot).padStart(2, "0")}
                    </span>
                    <span className="led" />
                  </div>
                  <div className="encl-slot__bot">
                    {empty ? (
                      <span
                        className="muted mono"
                        style={{ fontSize: 9 }}
                      >
                        empty
                      </span>
                    ) : (
                      <>
                        <span
                          className="mono"
                          style={{ fontSize: 10, color: "var(--fg-2)" }}
                        >
                          {modelTail || d.name}
                        </span>
                        <span
                          className="mono"
                          style={{ fontSize: 10, color: "var(--fg-0)" }}
                        >
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
                <div
                  className="muted mono"
                  style={{ fontSize: 11 }}
                >
                  SLOT {String(selected.slot ?? "—").padStart(2, "0")} ·{" "}
                  {selected.class ?? "—"}
                </div>
                <div className="disk-detail__title">
                  {selected.model ?? selected.name}
                </div>
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
              <dt>Name</dt>
              <dd className="mono">{selected.name}</dd>
              <dt>Serial</dt>
              <dd>{selected.serial ?? "—"}</dd>
              <dt>Capacity</dt>
              <dd>{formatBytes(diskCap(selected))}</dd>
              <dt>Pool</dt>
              <dd>{selected.pool ?? "—"}</dd>
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
                  color:
                    (selected.smart?.reallocated ?? 0) > 0
                      ? "var(--warn)"
                      : undefined,
                }}
              >
                {selected.smart?.reallocated ?? 0}
              </dd>
              <dt>Pending</dt>
              <dd>{selected.smart?.pending ?? 0}</dd>
            </dl>
            <div className="row gap-8" style={{ flexWrap: "wrap" }}>
              {/* TODO: phase 3 — wire SMART tests, locate, eject */}
              <button className="btn btn--sm">Run SMART · short</button>
              <button className="btn btn--sm">Run SMART · long</button>
              <button className="btn btn--sm">Locate</button>
              <button className="btn btn--sm btn--danger">Eject</button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

export default DisksTab;
