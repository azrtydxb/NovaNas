import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { storage, type Dataset } from "../../api/storage";
import { formatBytes } from "../../lib/format";

function dsKey(d: Dataset): string {
  return d.fullname ?? d.name;
}

export function DatasetsTab() {
  const [sel, setSel] = useState<string | null>(null);
  const q = useQuery({ queryKey: ["datasets"], queryFn: () => storage.listDatasets() });
  const datasets = q.data ?? [];

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: sel ? "1fr 320px" : "1fr",
        height: "100%",
      }}
    >
      <div style={{ overflow: "auto", padding: 14 }}>
        <div className="tbar">
          <button className="btn btn--primary">
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
        {q.data && datasets.length === 0 && (
          <div className="empty-hint">No datasets.</div>
        )}
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
                      <Icon
                        name="files"
                        size={12}
                        style={{
                          verticalAlign: "-2px",
                          marginRight: 6,
                          opacity: 0.6,
                        }}
                      />
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
                          <span
                            className="mono"
                            style={{ fontSize: 11, color: "var(--fg-3)" }}
                          >
                            {formatBytes(quota)}
                          </span>
                        </div>
                      ) : (
                        <span className="muted">—</span>
                      )}
                    </td>
                    <td className="num mono">{snap}</td>
                    <td className="muted mono" style={{ fontSize: 11 }}>
                      {d.comp ?? d.compression ?? "—"}
                    </td>
                    <td>
                      {enc ? (
                        <Icon name="shield" size={12} />
                      ) : (
                        <span className="muted">—</span>
                      )}
                    </td>
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

function DatasetDetail({
  fullname,
  fallback,
  onClose,
}: {
  fullname: string;
  fallback?: Dataset;
  onClose: () => void;
}) {
  const q = useQuery({
    queryKey: ["dataset", fullname],
    queryFn: () => storage.getDataset(fullname),
  });
  const d = q.data ?? fallback;
  if (!d) {
    return (
      <div className="side-detail">
        <div className="side-detail__head">
          <div>
            <div className="muted mono" style={{ fontSize: 10 }}>
              DATASET
            </div>
            <div className="side-detail__title">{fullname}</div>
          </div>
          <button className="btn btn--sm" onClick={onClose}>
            <Icon name="close" size={10} />
          </button>
        </div>
        <div className="empty-hint">
          {q.isLoading ? "Loading…" : "No data"}
        </div>
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
          <div className="muted mono" style={{ fontSize: 10 }}>
            DATASET
          </div>
          <div className="side-detail__title">{d.name}</div>
        </div>
        <button className="btn btn--sm" onClick={onClose}>
          <Icon name="close" size={10} />
        </button>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Capacity</div>
        </div>
        <div className="sect__body">
          <div className="bar">
            <div style={{ width: `${pct * 100}%` }} />
          </div>
          <div
            className="row"
            style={{ justifyContent: "space-between", fontSize: 11, marginTop: 4 }}
          >
            <span className="mono">{formatBytes(used)}</span>
            <span className="muted mono">/ {formatBytes(quota)}</span>
          </div>
        </div>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Properties</div>
        </div>
        <div className="sect__body">
          <dl className="kv">
            <dt>Pool</dt>
            <dd>{d.pool ?? "—"}</dd>
            <dt>Protocol</dt>
            <dd>{d.proto ?? "—"}</dd>
            <dt>Compression</dt>
            <dd>{d.comp ?? d.compression ?? "—"}</dd>
            <dt>Recordsize</dt>
            <dd>{d.recordsize ?? "—"}</dd>
            <dt>Atime</dt>
            <dd>{d.atime ?? "—"}</dd>
            <dt>Encrypted</dt>
            <dd>{enc ? "yes" : "no"}</dd>
            <dt>Snapshots</dt>
            <dd>{snap}</dd>
          </dl>
        </div>
      </div>
      <div
        className="row gap-8"
        style={{ padding: "10px 12px", borderTop: "1px solid var(--line)" }}
      >
        {/* TODO: phase 3 — wire snapshot/send/destroy */}
        <button className="btn btn--sm">Snapshot</button>
        <button className="btn btn--sm">Send…</button>
        <button
          className="btn btn--sm btn--danger"
          style={{ marginLeft: "auto" }}
        >
          Destroy
        </button>
      </div>
    </div>
  );
}

export default DatasetsTab;
