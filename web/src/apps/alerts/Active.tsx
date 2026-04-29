import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  alerts,
  type Alert,
  type AlertSilenceMatcher,
} from "../../api/observability";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

function severityOf(a: Alert): string {
  return a.labels?.severity ?? "info";
}

function pillClass(sev: string): string {
  if (sev === "critical") return "pill pill--err";
  if (sev === "warning") return "pill pill--warn";
  if (sev === "info") return "pill pill--info";
  return "pill";
}

function fmtSince(ts?: string): string {
  if (!ts) return "—";
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  const diff = Date.now() - d.getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

function alertName(a: Alert): string {
  return a.labels?.alertname ?? a.fingerprint.slice(0, 8);
}

type SilenceModalProps = {
  alert: Alert;
  onClose: () => void;
};

function SilenceModal({ alert, onClose }: SilenceModalProps) {
  const qc = useQueryClient();
  const [duration, setDuration] = useState("2h");
  const [comment, setComment] = useState("");
  const initial: AlertSilenceMatcher[] = Object.entries(alert.labels ?? {}).map(
    ([k, v]) => ({ name: k, value: v, isRegex: false, isEqual: true })
  );
  const [matchers, setMatchers] = useState<AlertSilenceMatcher[]>(initial);

  const create = useMutation({
    meta: { label: "Create silence failed" },
    mutationFn: () => {
      const now = new Date();
      const ms = parseDuration(duration);
      const ends = new Date(now.getTime() + ms);
      return alerts.createSilence({
        matchers,
        startsAt: now.toISOString(),
        endsAt: ends.toISOString(),
        createdBy: "console",
        comment: comment || `Silenced ${alertName(alert)}`,
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["alerts"] });
      toastSuccess("Silence created");
      onClose();
    },
  });

  const setMatcher = (i: number, patch: Partial<AlertSilenceMatcher>) => {
    setMatchers((cur) => cur.map((m, j) => (j === i ? { ...m, ...patch } : m)));
  };
  const removeMatcher = (i: number) =>
    setMatchers((cur) => cur.filter((_, j) => j !== i));
  const addMatcher = () =>
    setMatchers((cur) => [...cur, { name: "", value: "", isEqual: true }]);

  return (
    <div className="modal-bg" onMouseDown={onClose}>
      <div className="modal" onMouseDown={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="alert" size={18} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Silence alert</div>
            <div className="modal__sub muted">{alertName(alert)}</div>
          </div>
          <button className="modal__close" onClick={onClose}>
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Duration</label>
            <div className="row gap-4">
              {["15m", "1h", "2h", "12h", "1d"].map((d) => (
                <button
                  key={d}
                  className={`btn btn--sm ${duration === d ? "btn--primary" : ""}`}
                  onClick={() => setDuration(d)}
                  type="button"
                >
                  {d}
                </button>
              ))}
              <input
                className="input"
                value={duration}
                onChange={(e) => setDuration(e.target.value)}
                style={{ width: 80 }}
              />
            </div>
          </div>
          <div className="field">
            <label className="field__label">Matchers</label>
            {matchers.map((m, i) => (
              <div
                key={i}
                className="row gap-4"
                style={{ marginBottom: 4, alignItems: "center" }}
              >
                <input
                  className="input input--mono"
                  placeholder="name"
                  value={m.name}
                  onChange={(e) => setMatcher(i, { name: e.target.value })}
                  style={{ flex: 1 }}
                />
                <span className="mono muted">{m.isRegex ? "=~" : "="}</span>
                <input
                  className="input input--mono"
                  placeholder="value"
                  value={m.value}
                  onChange={(e) => setMatcher(i, { value: e.target.value })}
                  style={{ flex: 2 }}
                />
                <button
                  className="btn btn--sm"
                  onClick={() => setMatcher(i, { isRegex: !m.isRegex })}
                  type="button"
                >
                  {m.isRegex ? "regex" : "exact"}
                </button>
                <button
                  className="btn btn--sm btn--danger"
                  onClick={() => removeMatcher(i)}
                  type="button"
                >
                  <Icon name="x" size={10} />
                </button>
              </div>
            ))}
            <button
              className="btn btn--sm"
              onClick={addMatcher}
              type="button"
              style={{ marginTop: 4 }}
            >
              <Icon name="plus" size={10} /> Add matcher
            </button>
          </div>
          <div className="field">
            <label className="field__label">Comment</label>
            <input
              className="input"
              placeholder="Why is this being silenced?"
              value={comment}
              onChange={(e) => setComment(e.target.value)}
            />
          </div>
          {create.isError && (
            <div className="modal__err">
              {(create.error as Error).message}
            </div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn btn--sm" onClick={onClose}>
            Cancel
          </button>
          <button
            className="btn btn--sm btn--primary"
            disabled={create.isPending || matchers.length === 0}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "Creating…" : "Create silence"}
          </button>
        </div>
      </div>
    </div>
  );
}

function parseDuration(s: string): number {
  const m = s.trim().match(/^(\d+)\s*([smhd])$/i);
  if (!m) return 2 * 3600 * 1000;
  const n = Number(m[1]);
  const unit = m[2].toLowerCase();
  const mult =
    unit === "s" ? 1000 : unit === "m" ? 60_000 : unit === "h" ? 3_600_000 : 86_400_000;
  return n * mult;
}

export default function Active() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["alerts", "list"],
    queryFn: () => alerts.list(),
    refetchInterval: 5000,
  });
  const list: Alert[] = q.data ?? [];
  const [sel, setSel] = useState<string | null>(null);
  const [silenceFor, setSilenceFor] = useState<Alert | null>(null);
  const cur = list.find((a) => a.fingerprint === sel) ?? list[0];

  const counts = {
    critical: list.filter((a) => severityOf(a) === "critical").length,
    warning: list.filter((a) => severityOf(a) === "warning").length,
    info: list.filter((a) => severityOf(a) === "info").length,
  };

  const runbook =
    cur?.annotations?.runbook_url ??
    cur?.annotations?.runbook ??
    cur?.annotations?.runbookURL;

  return (
    <div style={{ display: "grid", gridTemplateColumns: "1fr 320px", height: "100%" }}>
      <div style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <span className="pill pill--err">
            <span className="dot" /> {counts.critical} critical
          </span>
          <span className="pill pill--warn">
            <span className="dot" /> {counts.warning} warning
          </span>
          <span className="pill pill--info">
            <span className="dot" /> {counts.info} info
          </span>
          <button
            className="btn btn--sm"
            style={{ marginLeft: "auto" }}
            onClick={() => qc.invalidateQueries({ queryKey: ["alerts", "list"] })}
          >
            <Icon name="refresh" size={11} /> Refresh
          </button>
        </div>

        {q.isLoading && <div className="muted">Loading alerts…</div>}
        {q.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed to load: {(q.error as Error).message}
          </div>
        )}
        {q.data && list.length === 0 && (
          <div className="muted" style={{ padding: "20px 0" }}>
            No active alerts.
          </div>
        )}

        {list.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>Alert</th>
                <th>Severity</th>
                <th>Since</th>
                <th>Labels</th>
              </tr>
            </thead>
            <tbody>
              {list.map((a) => {
                const sev = severityOf(a);
                const isOn = (cur && cur.fingerprint === a.fingerprint) || false;
                return (
                  <tr
                    key={a.fingerprint}
                    className={isOn ? "is-on" : ""}
                    onClick={() => setSel(a.fingerprint)}
                  >
                    <td>{alertName(a)}</td>
                    <td>
                      <span className={pillClass(sev)}>
                        <span className="dot" /> {sev}
                      </span>
                    </td>
                    <td className="muted">{fmtSince(a.startsAt)}</td>
                    <td className="mono muted" style={{ fontSize: 10 }}>
                      {Object.entries(a.labels ?? {})
                        .filter(([k]) => k !== "alertname" && k !== "severity")
                        .slice(0, 4)
                        .map(([k, v]) => `${k}=${v}`)
                        .join(" ")}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {cur && (
        <div className="side-detail">
          <div className="side-detail__head">
            <div>
              <div className="muted mono" style={{ fontSize: 10 }}>
                ALERT · {cur.fingerprint.slice(0, 12)}
              </div>
              <div className="side-detail__title">{alertName(cur)}</div>
            </div>
          </div>

          <div className="sect">
            <div className="sect__head">
              <div className="sect__title">Summary</div>
            </div>
            <div className="sect__body" style={{ fontSize: 12 }}>
              {cur.annotations?.summary ??
                cur.annotations?.description ??
                "No summary."}
            </div>
          </div>

          <div className="sect">
            <div className="sect__head">
              <div className="sect__title">Labels</div>
            </div>
            <div className="sect__body chip-row">
              {Object.entries(cur.labels ?? {}).map(([k, v]) => (
                <span key={k} className="chip">
                  {k}={v}
                </span>
              ))}
            </div>
          </div>

          {cur.annotations && Object.keys(cur.annotations).length > 0 && (
            <div className="sect">
              <div className="sect__head">
                <div className="sect__title">Annotations</div>
              </div>
              <table className="tbl tbl--compact">
                <tbody>
                  {Object.entries(cur.annotations).map(([k, v]) => (
                    <tr key={k}>
                      <td className="mono">{k}</td>
                      <td className="mono">{v}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="sect">
            <div className="sect__head">
              <div className="sect__title">State</div>
            </div>
            <dl className="kv">
              <dt>Severity</dt>
              <dd>{severityOf(cur)}</dd>
              <dt>State</dt>
              <dd>{cur.status?.state ?? "—"}</dd>
              <dt>Since</dt>
              <dd>{fmtSince(cur.startsAt)}</dd>
            </dl>
          </div>

          <div
            className="row gap-8"
            style={{ padding: "10px 12px", borderTop: "1px solid var(--line)" }}
          >
            <button
              className="btn btn--sm btn--primary"
              onClick={() => setSilenceFor(cur)}
            >
              Silence…
            </button>
            {runbook && (
              <a
                className="btn btn--sm"
                href={runbook}
                target="_blank"
                rel="noreferrer"
              >
                <Icon name="external" size={10} /> Runbook
              </a>
            )}
            {cur.generatorURL && (
              <a
                className="btn btn--sm"
                href={cur.generatorURL}
                target="_blank"
                rel="noreferrer"
              >
                Source
              </a>
            )}
          </div>
        </div>
      )}

      {silenceFor && (
        <SilenceModal alert={silenceFor} onClose={() => setSilenceFor(null)} />
      )}
    </div>
  );
}
