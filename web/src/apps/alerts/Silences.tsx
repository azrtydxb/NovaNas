import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { alerts, type AlertSilence } from "../../api/observability";
import { Icon } from "../../components/Icon";

function fmtMatchers(s: AlertSilence): string {
  return (s.matchers ?? [])
    .map((m) => `${m.name}${m.isRegex ? "=~" : "="}${m.value}`)
    .join(" ");
}

export default function Silences() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["alerts", "silences"],
    queryFn: () => alerts.listSilences(),
    refetchInterval: 30000,
  });

  const expire = useMutation({
    mutationFn: (id: string) => alerts.expireSilence(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alerts", "silences"] }),
  });

  const list = q.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary">
          <Icon name="plus" size={11} /> New silence
        </button>
        <span className="muted" style={{ marginLeft: "auto" }}>
          {list.length} silences
        </span>
      </div>

      {q.isLoading && <div className="muted">Loading silences…</div>}
      {q.isError && (
        <div className="muted" style={{ color: "var(--err)" }}>
          Failed to load: {(q.error as Error).message}
        </div>
      )}
      {q.data && list.length === 0 && (
        <div className="muted" style={{ padding: "20px 0" }}>
          No silences configured.
        </div>
      )}

      {list.length > 0 && (
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
                <td className="mono" style={{ fontSize: 11 }}>
                  {s.id.slice(0, 8)}
                </td>
                <td className="mono" style={{ fontSize: 11 }}>
                  {fmtMatchers(s)}
                </td>
                <td className="muted">{s.comment ?? "—"}</td>
                <td>{s.createdBy ?? "—"}</td>
                <td className="muted">{s.endsAt ?? "—"}</td>
                <td className="num">
                  <button
                    className="btn btn--sm btn--danger"
                    disabled={expire.isPending}
                    onClick={() => expire.mutate(s.id)}
                  >
                    Expire
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
