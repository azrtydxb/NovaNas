import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  alerts,
  type AlertSilence,
  type AlertSilenceMatcher,
} from "../../api/observability";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

function fmtMatcher(m: AlertSilenceMatcher): string {
  const op = m.isRegex ? (m.isEqual === false ? "!~" : "=~") : m.isEqual === false ? "!=" : "=";
  return `${m.name}${op}${m.value}`;
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

function fmtTime(ts?: string): string {
  if (!ts) return "—";
  const d = new Date(ts);
  if (isNaN(d.getTime())) return ts;
  return d.toLocaleString(undefined, { hour12: false });
}

type CreateModalProps = { onClose: () => void };

function CreateModal({ onClose }: CreateModalProps) {
  const qc = useQueryClient();
  const [duration, setDuration] = useState("2h");
  const [comment, setComment] = useState("");
  const [matchers, setMatchers] = useState<AlertSilenceMatcher[]>([
    { name: "alertname", value: "", isEqual: true, isRegex: false },
  ]);

  const create = useMutation({
    meta: { label: "Create silence failed" },
    mutationFn: () => {
      const now = new Date();
      const ends = new Date(now.getTime() + parseDuration(duration));
      return alerts.createSilence({
        matchers,
        startsAt: now.toISOString(),
        endsAt: ends.toISOString(),
        createdBy: "console",
        comment: comment || "Manual silence",
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["alerts", "silences"] });
      toastSuccess("Silence created");
      onClose();
    },
  });

  const setMatcher = (i: number, patch: Partial<AlertSilenceMatcher>) =>
    setMatchers((cur) => cur.map((m, j) => (j === i ? { ...m, ...patch } : m)));
  const removeMatcher = (i: number) =>
    setMatchers((cur) => cur.filter((_, j) => j !== i));
  const addMatcher = () =>
    setMatchers((cur) => [...cur, { name: "", value: "", isEqual: true }]);

  const valid =
    matchers.length > 0 && matchers.every((m) => m.name && m.value);

  return (
    <div className="modal-bg" onMouseDown={onClose}>
      <div className="modal" onMouseDown={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon"><Icon name="alert" size={18} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">New silence</div>
            <div className="modal__sub muted">Suppress matching alerts for a time window</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
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
              <div key={i} className="row gap-4" style={{ marginBottom: 4, alignItems: "center" }}>
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
              placeholder="Why is this silenced?"
              value={comment}
              onChange={(e) => setComment(e.target.value)}
            />
          </div>
          {create.isError && (
            <div className="modal__err">{(create.error as Error).message}</div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn btn--sm" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--sm btn--primary"
            disabled={!valid || create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "Creating…" : "Create silence"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default function Silences() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["alerts", "silences"],
    queryFn: () => alerts.listSilences(),
    refetchInterval: 30_000,
  });
  const expire = useMutation({
    meta: { label: "Expire silence failed" },
    mutationFn: (id: string) => alerts.expireSilence(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["alerts", "silences"] });
      toastSuccess("Silence expired");
    },
  });
  const [showCreate, setShowCreate] = useState(false);

  const list: AlertSilence[] = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setShowCreate(true)}>
          <Icon name="plus" size={11} />New silence
        </button>
      </div>
      <table className="tbl">
        <thead>
          <tr>
            <th>ID</th>
            <th>Matchers</th>
            <th>Comment</th>
            <th>Creator</th>
            <th>Ends</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {list.map((s) => (
            <tr key={s.id}>
              <td className="mono" style={{ fontSize: 11 }}>{s.id.slice(0, 8)}</td>
              <td className="mono" style={{ fontSize: 11 }}>
                {(s.matchers ?? []).map(fmtMatcher).join(" ")}
              </td>
              <td className="muted">{s.comment ?? "—"}</td>
              <td>{s.createdBy ?? "—"}</td>
              <td className="muted">{fmtTime(s.endsAt)}</td>
              <td className="num">
                <button
                  className="btn btn--sm btn--danger"
                  disabled={expire.isPending}
                  onClick={() => {
                    if (confirm(`Expire silence ${s.id.slice(0, 8)}?`)) {
                      expire.mutate(s.id);
                    }
                  }}
                >
                  Expire
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {q.isLoading && <div className="muted" style={{ padding: 8 }}>Loading silences…</div>}
      {q.isError && (
        <div className="muted" style={{ padding: 8, color: "var(--err)" }}>
          Failed to load: {(q.error as Error).message}
        </div>
      )}
      {q.data && list.length === 0 && (
        <div className="muted" style={{ padding: 20 }}>No silences configured.</div>
      )}

      {showCreate && <CreateModal onClose={() => setShowCreate(false)} />}
    </div>
  );
}
