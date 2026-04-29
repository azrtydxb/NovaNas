import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { system } from "../../api/system";
import { Icon } from "../../components/Icon";

export function Update() {
  const qc = useQueryClient();
  const upd = useQuery({
    queryKey: ["system", "updates"],
    queryFn: () => system.updates(),
  });

  const apply = useMutation({
    mutationFn: () => system.applyUpdate(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["system", "updates"] }),
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

      {next && (
        <div
          className="sect"
          style={{
            background: "var(--accent-soft)",
            borderLeft: "3px solid var(--accent)",
          }}
        >
          <div className="sect__body">
            <div style={{ color: "var(--fg-0)", fontSize: 12 }}>
              Update <span className="mono">{next}</span> is available.
            </div>
          </div>
        </div>
      )}

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

      {apply.isError && (
        <div className="modal__err" style={{ marginTop: 8 }}>
          Failed to apply: {(apply.error as Error).message}
        </div>
      )}
      {apply.isSuccess && (
        <div
          className="muted"
          style={{ marginTop: 8, color: "var(--ok)", fontSize: 11 }}
        >
          Update started. The system will reboot once installation completes.
        </div>
      )}

      <div className="row gap-8" style={{ marginTop: 8, padding: "0 16px 14px" }}>
        <button
          className="btn btn--primary"
          disabled={!next || apply.isPending}
          onClick={() => {
            if (
              confirm(
                `Apply update ${next}? The system may reboot once installation completes.`
              )
            )
              apply.mutate();
          }}
        >
          <Icon name="download" size={11} />
          {apply.isPending
            ? "Applying…"
            : next
              ? `Install ${next}`
              : "Install"}
        </button>
        <button className="btn" onClick={() => void upd.refetch()}>
          <Icon name="refresh" size={11} /> Check for updates
        </button>
      </div>
    </>
  );
}

export default Update;
