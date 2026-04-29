import { useQuery } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { workloads, type HelmRelease, type K8sEvent } from "../../api/workloads";
import { Icon } from "../../components/Icon";

function rel(w: HelmRelease) {
  return w.release ?? w.name ?? "";
}

function eventTime(e: K8sEvent) {
  return e.t ?? e.time ?? e.timestamp ?? "—";
}
function eventKind(e: K8sEvent) {
  return e.kind ?? e.type ?? "Normal";
}
function eventObj(e: K8sEvent) {
  return e.obj ?? e.object ?? e.involvedObject ?? "—";
}
function eventMsg(e: K8sEvent) {
  return e.msg ?? e.message ?? "";
}

export function Events() {
  const list = useQuery({
    queryKey: ["workloads", "list"],
    queryFn: () => workloads.list(),
    retry: false,
  });

  const releaseNames: string[] = (list.data ?? []).map(rel).filter(Boolean);

  const events = useQuery({
    queryKey: ["workloads", "events", releaseNames],
    queryFn: async () => {
      const all: Array<K8sEvent & { release: string }> = [];
      for (const r of releaseNames.slice(0, 10)) {
        try {
          const evs = await workloads.events(r);
          for (const e of evs) all.push({ ...e, release: r });
        } catch {
          // ignore individual failures
        }
      }
      return all.sort((a, b) => {
        const ta = eventTime(a);
        const tb = eventTime(b);
        return tb.localeCompare(ta);
      });
    },
    enabled: releaseNames.length > 0,
    retry: false,
    refetchInterval: 5000,
  });

  if (list.isError) {
    const err = list.error as ApiError | Error;
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 503) {
      return (
        <div style={{ padding: 24 }}>
          <div className="discover__msg muted">
            Events unavailable while k3s initializes.
          </div>
          <button className="btn btn--sm" style={{ marginTop: 10 }} onClick={() => list.refetch()}>
            <Icon name="refresh" size={11} />
            Retry
          </button>
        </div>
      );
    }
    return (
      <div style={{ padding: 14, color: "var(--err)" }}>
        Failed to load: {err.message}
      </div>
    );
  }

  const evs = events.data ?? [];

  return (
    <div style={{ padding: 14 }}>
      {(list.isLoading || events.isLoading) && (
        <div className="muted">Loading events…</div>
      )}
      {!events.isLoading && releaseNames.length === 0 && (
        <div className="muted" style={{ padding: 12 }}>
          No releases — no events to show.
        </div>
      )}
      {!events.isLoading && releaseNames.length > 0 && evs.length === 0 && (
        <div className="muted" style={{ padding: 12 }}>
          No recent events.
        </div>
      )}
      {evs.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Time</th>
              <th>Kind</th>
              <th>Reason</th>
              <th>Object</th>
              <th>Message</th>
            </tr>
          </thead>
          <tbody>
            {evs.map((e, i) => {
              const k = eventKind(e);
              return (
                <tr key={i}>
                  <td className="muted mono" style={{ fontSize: 11 }}>{eventTime(e)}</td>
                  <td>
                    {k === "Warning" ? (
                      <span className="pill pill--warn">
                        <span className="dot" />
                        {k}
                      </span>
                    ) : (
                      <span className="pill">
                        <span className="dot" />
                        {k}
                      </span>
                    )}
                  </td>
                  <td className="mono" style={{ fontSize: 11 }}>{e.reason ?? "—"}</td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {e.release}/{eventObj(e)}
                  </td>
                  <td className="muted">{eventMsg(e)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}
