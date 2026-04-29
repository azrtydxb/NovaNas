import { useQuery } from "@tanstack/react-query";
import { jobs, type Job } from "../../api/observability";

function statePill(state?: string): string {
  if (state === "ok" || state === "completed" || state === "succeeded")
    return "pill pill--ok";
  if (state === "running" || state === "active") return "pill pill--info";
  if (state === "failed" || state === "error") return "pill pill--err";
  return "pill";
}

function progressPct(j: Job): number {
  if (typeof j.progress === "number")
    return Math.max(0, Math.min(1, j.progress > 1 ? j.progress / 100 : j.progress));
  if (typeof j.pct === "number") {
    const v = j.pct > 1 ? j.pct / 100 : j.pct;
    return Math.max(0, Math.min(1, v));
  }
  return 0;
}

function isRunning(j: Job): boolean {
  return j.state === "running" || j.state === "active";
}

export default function Jobs() {
  const q = useQuery({
    queryKey: ["jobs", "list"],
    queryFn: () => jobs.list(),
    refetchInterval: 5000,
  });
  const list: Job[] = q.data ?? [];

  const counts = {
    running: list.filter(isRunning).length,
    queued: list.filter(
      (j) => j.state === "queued" || j.state === "pending" || j.state === "scheduled"
    ).length,
    done: list.filter(
      (j) => j.state === "ok" || j.state === "completed" || j.state === "succeeded"
    ).length,
  };

  return (
    <div className="app-storage">
      <div className="win-body" style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <span className="pill pill--info"><span className="dot" />{counts.running} running</span>
          <span className="pill"><span className="dot" />{counts.queued} queued</span>
          <span className="pill pill--ok"><span className="dot" />{counts.done} done</span>
        </div>
        <table className="tbl">
          <thead>
            <tr>
              <th>Job</th>
              <th>Kind</th>
              <th>Target</th>
              <th>Progress</th>
              <th>ETA</th>
              <th>State</th>
            </tr>
          </thead>
          <tbody>
            {list.map((j) => {
              const pct = progressPct(j);
              const running = isRunning(j);
              return (
                <tr key={j.id}>
                  <td className="mono" style={{ fontSize: 11 }}>{j.id.slice(0, 12)}</td>
                  <td className="mono" style={{ fontSize: 11 }}>{j.kind ?? "—"}</td>
                  <td className="muted">{j.target ?? "—"}</td>
                  <td>
                    {running ? (
                      <div className="cap">
                        <div className="cap__bar"><div style={{ width: `${pct * 100}%` }} /></div>
                        <span className="mono" style={{ fontSize: 11 }}>{Math.round(pct * 100)}%</span>
                      </div>
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                  <td className="muted mono" style={{ fontSize: 11 }}>{j.eta ?? "—"}</td>
                  <td>
                    <span className={statePill(j.state)}>
                      <span className="dot" />{j.state ?? "unknown"}
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {q.isLoading && <div className="muted" style={{ padding: 8 }}>Loading jobs…</div>}
        {q.isError && (
          <div className="muted" style={{ padding: 8, color: "var(--err)" }}>
            Failed to load: {(q.error as Error).message}
          </div>
        )}
        {q.data && list.length === 0 && (
          <div className="muted" style={{ padding: 20 }}>No jobs.</div>
        )}
      </div>
    </div>
  );
}
