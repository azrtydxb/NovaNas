import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { system } from "../../api/system";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

export function Update() {
  const qc = useQueryClient();
  const upd = useQuery({
    queryKey: ["system", "updates"],
    queryFn: () => system.updates(),
  });

  const apply = useMutation({
    meta: { label: "Apply update failed" },
    mutationFn: () => system.applyUpdate(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["system", "updates"] });
      toastSuccess("Update started");
    },
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
        <dl className="kv">
          <dt>Current version</dt>
          <dd className="mono">{d.current ?? "—"}</dd>
          <dt>Available</dt>
          <dd className="mono" style={next ? { color: "var(--ok)" } : undefined}>
            {next ?? "up to date"}
          </dd>
          <dt>Last check</dt>
          <dd>{d.checked ?? "—"}</dd>
        </dl>
      </div>
      <div className="sect">
        <div className="sect__head">
          <div className="sect__title">Changelog{next ? ` · ${next}` : ""}</div>
        </div>
        <ul style={{ paddingLeft: 18, fontSize: 11, color: "var(--fg-2)", lineHeight: 1.7, margin: 0 }}>
          {notes.length > 0 ? (
            notes.map((n, i) => <li key={i}>{n}</li>)
          ) : (
            <li className="muted">No release notes.</li>
          )}
        </ul>
      </div>
      <div className="row gap-8" style={{ marginTop: 8 }}>
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
          {apply.isPending ? "Applying…" : next ? `Install ${next}` : "Install"}
        </button>
        <button className="btn" onClick={() => void upd.refetch()}>
          Check for updates
        </button>
      </div>
      {apply.isError && (
        <div className="modal__err" style={{ marginTop: 8 }}>
          Failed to apply: {(apply.error as Error).message}
        </div>
      )}
    </>
  );
}

export default Update;
