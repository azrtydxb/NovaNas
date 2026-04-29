import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { identity, type LoginEvent } from "../../api/identity";
import { Icon } from "../../components/Icon";

function when(e: LoginEvent): string {
  return e.at ?? e.timestamp ?? "—";
}
function user(e: LoginEvent): string {
  return e.user ?? e.username ?? "—";
}

type Filter = "all" | "success" | "fail";

export function LoginHistory() {
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const q = useQuery({
    queryKey: ["auth", "login-history"],
    queryFn: () => identity.loginHistory(),
  });

  const filtered = useMemo(() => {
    const data = q.data ?? [];
    const s = search.trim().toLowerCase();
    return data.filter((e) => {
      if (filter === "success" && e.result !== "success") return false;
      if (filter === "fail" && e.result === "success") return false;
      if (s && !user(e).toLowerCase().includes(s)) return false;
      return true;
    });
  }, [q.data, search, filter]);

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <input
          className="input"
          placeholder="Search by username…"
          style={{ width: 240 }}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <div className="row gap-8" style={{ marginLeft: 8 }}>
          {(["all", "success", "fail"] as Filter[]).map((f) => (
            <button
              key={f}
              className={`btn btn--sm ${filter === f ? "btn--primary" : ""}`}
              onClick={() => setFilter(f)}
            >
              {f}
            </button>
          ))}
        </div>
        <button
          className="btn btn--sm"
          onClick={() => q.refetch()}
          disabled={q.isFetching}
          style={{ marginLeft: "auto" }}
        >
          <Icon name="refresh" size={11} />
          Refresh
        </button>
      </div>
      {q.isLoading && <div className="muted">Loading login history…</div>}
      {q.isError && (
        <div className="muted" style={{ color: "var(--err)" }}>
          Failed to load: {(q.error as Error).message}
        </div>
      )}
      {q.data && filtered.length === 0 && (
        <div className="muted" style={{ padding: 12 }}>
          No login events match.
        </div>
      )}
      {filtered.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>When</th>
              <th>User</th>
              <th>IP</th>
              <th>Method</th>
              <th>Result</th>
              <th>Reason</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((h, i) => (
              <tr key={i}>
                <td className="muted mono" style={{ fontSize: 11 }}>{when(h)}</td>
                <td>{user(h)}</td>
                <td className="mono">{h.ip ?? "—"}</td>
                <td className="muted mono" style={{ fontSize: 11 }}>{h.method ?? "—"}</td>
                <td>
                  {h.result === "success" ? (
                    <span className="pill pill--ok">
                      <span className="dot" />
                      ok
                    </span>
                  ) : (
                    <span className="pill pill--err">
                      <span className="dot" />
                      {h.result ?? "fail"}
                    </span>
                  )}
                </td>
                <td className="muted" style={{ fontSize: 11 }}>{h.reason ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
