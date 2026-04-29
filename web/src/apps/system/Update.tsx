import { useQuery } from "@tanstack/react-query";
import { system } from "../../api/system";
import { Icon } from "../../components/Icon";

export function Update() {
  const upd = useQuery({
    queryKey: ["system", "updates"],
    queryFn: () => system.updates(),
  });

  if (upd.isLoading) return <div className="muted">Checking for updates…</div>;
  if (upd.isError)
    return (
      <div className="muted" style={{ color: "var(--err)" }}>
        Failed to load: {(upd.error as Error).message}
      </div>
    );

  const d = upd.data ?? {};
  const next = d.available;
  const notes = d.notes ?? [];

  return (
    <>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Channel</div>
          {d.channel && <span className="pill pill--info">{d.channel}</span>}
        </div>
        <div className="sect__body">
          <dl className="kv">
            <dt>Current version</dt>
            <dd className="mono">{d.current ?? "—"}</dd>
            <dt>Available</dt>
            <dd
              className="mono"
              style={next ? { color: "var(--ok)" } : undefined}
            >
              {next ?? "up to date"}
            </dd>
            <dt>Last check</dt>
            <dd>{d.checked ?? "—"}</dd>
          </dl>
        </div>
      </div>

      {notes.length > 0 && (
        <div className="sect">
          <div className="sect__head">
            <div className="sect__title">Changelog{next ? ` · ${next}` : ""}</div>
          </div>
          <div className="sect__body">
            <ul
              style={{
                paddingLeft: 18,
                fontSize: 11,
                color: "var(--fg-2)",
                lineHeight: 1.7,
                margin: 0,
              }}
            >
              {notes.map((n, i) => (
                <li key={i}>{n}</li>
              ))}
            </ul>
          </div>
        </div>
      )}

      <div className="row gap-8" style={{ marginTop: 8 }}>
        <button className="btn btn--primary" disabled={!next}>
          <Icon name="download" size={11} />
          {next ? `Install ${next}` : "Install"}
        </button>
        <button className="btn" onClick={() => void upd.refetch()}>
          <Icon name="refresh" size={11} /> Check for updates
        </button>
      </div>
    </>
  );
}

export default Update;
