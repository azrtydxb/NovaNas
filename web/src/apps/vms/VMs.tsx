import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { vms, type VM } from "../../api/vms";
import { Icon } from "../../components/Icon";

function vmKey(v: VM) {
  return `${v.namespace ?? v.ns ?? "default"}/${v.name}`;
}

function stateDot(state?: string) {
  const s = (state ?? "").toLowerCase();
  if (s === "running") return "sdot sdot--ok";
  if (s === "paused") return "sdot sdot--warn";
  return "sdot";
}
function statePill(state?: string) {
  const s = (state ?? "").toLowerCase();
  if (s === "running") return "pill pill--ok";
  if (s === "paused") return "pill pill--warn";
  return "pill";
}

export function VMs() {
  const qc = useQueryClient();
  const [sel, setSel] = useState<string | null>(null);

  const list = useQuery({
    queryKey: ["vms", "list"],
    queryFn: () => vms.list(),
    retry: false,
  });

  const action = useMutation({
    mutationFn: async (a: { ns: string; name: string; verb: "start" | "stop" | "restart" | "pause" | "unpause" }) => {
      switch (a.verb) {
        case "start": return vms.start(a.ns, a.name);
        case "stop": return vms.stop(a.ns, a.name);
        case "restart": return vms.restart(a.ns, a.name);
        case "pause": return vms.pause(a.ns, a.name);
        case "unpause": return vms.unpause(a.ns, a.name);
      }
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["vms", "list"] }),
  });

  if (list.isError) {
    const err = list.error as ApiError | Error;
    const status = err instanceof ApiError ? err.status : 0;
    if (status === 503) {
      return (
        <div style={{ padding: 24, color: "var(--fg-2)" }}>
          <div className="row gap-8" style={{ marginBottom: 8 }}>
            <Icon name="alert" size={14} />
            <strong>Virtualization service unavailable</strong>
          </div>
          <div className="muted" style={{ fontSize: 12 }}>
            KubeClient not yet wired. Once KubeVirt is online, your VMs will appear here.
          </div>
        </div>
      );
    }
    return (
      <div style={{ padding: 14, color: "var(--err)" }}>
        Failed to load VMs: {err.message}
      </div>
    );
  }

  const items = list.data ?? [];
  const cur = items.find((v) => vmKey(v) === sel) ?? items[0];

  return (
    <div style={{ display: "grid", gridTemplateColumns: "200px 1fr", height: "100%" }}>
      <div style={{ borderRight: "1px solid var(--line)", overflow: "auto", padding: 6 }}>
        <div className="vlist__title" style={{ padding: "4px 8px" }}>
          VIRTUAL MACHINES
        </div>
        {list.isLoading && <div className="muted" style={{ padding: 8 }}>Loading…</div>}
        {!list.isLoading && items.length === 0 && (
          <div className="muted" style={{ padding: 8, fontSize: 11 }}>
            No virtual machines.
          </div>
        )}
        {items.map((v) => {
          const k = vmKey(v);
          const isOn = (cur && vmKey(cur) === k) ? "is-on" : "";
          return (
            <button
              key={k}
              className={`vlist__item ${isOn}`}
              onClick={() => setSel(k)}
            >
              <span className={stateDot(v.state ?? v.status)} />
              <div style={{ flex: 1, minWidth: 0, textAlign: "left" }}>
                <div style={{ color: "var(--fg-1)", fontSize: 11 }}>{v.name}</div>
                <div className="muted mono" style={{ fontSize: 9 }}>
                  {v.os ?? v.namespace ?? v.ns ?? "—"}
                </div>
              </div>
            </button>
          );
        })}
      </div>
      <div
        style={{
          padding: 14,
          overflow: "auto",
          display: "flex",
          flexDirection: "column",
          gap: 12,
        }}
      >
        {!cur && !list.isLoading && (
          <div className="muted" style={{ padding: 12 }}>
            Select a VM from the list.
          </div>
        )}
        {cur && (
          <>
            <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
              <span className={statePill(cur.state ?? cur.status)}>
                <span className="dot" />
                {cur.state ?? cur.status ?? "—"}
              </span>
              {cur.ip && (
                <span className="pill pill--info mono" style={{ fontSize: 10 }}>
                  {cur.ip}
                </span>
              )}
              <span className="muted" style={{ marginLeft: "auto", fontSize: 11 }}>
                {cur.uptime ? `uptime ${cur.uptime}` : ""}
              </span>
            </div>
            <div className="vm-console" style={{ minHeight: 160 }}>
              {(cur.state ?? cur.status) === "Running" ? (
                <>
                  <div style={{ opacity: 0.6, fontSize: 10, marginBottom: 8 }}>
                    {cur.os ?? "vm"} · console
                  </div>
                  <div>{cur.name} login: <span className="vm-console__cursor" /></div>
                </>
              ) : (
                <div style={{ opacity: 0.4, padding: 30, textAlign: "center" }}>
                  ● Powered off
                </div>
              )}
            </div>
            <div className="row gap-8" style={{ flexWrap: "wrap" }}>
              {(cur.state ?? cur.status) === "Running" ? (
                <button
                  className="btn btn--sm btn--primary"
                  disabled={action.isPending}
                  onClick={() =>
                    action.mutate({ ns: cur.namespace ?? cur.ns ?? "default", name: cur.name, verb: "pause" })
                  }
                >
                  <Icon name="pause" size={11} />
                  Pause
                </button>
              ) : (cur.state ?? cur.status) === "Paused" ? (
                <button
                  className="btn btn--sm btn--primary"
                  disabled={action.isPending}
                  onClick={() =>
                    action.mutate({ ns: cur.namespace ?? cur.ns ?? "default", name: cur.name, verb: "unpause" })
                  }
                >
                  <Icon name="play" size={11} />
                  Resume
                </button>
              ) : (
                <button
                  className="btn btn--sm btn--primary"
                  disabled={action.isPending}
                  onClick={() =>
                    action.mutate({ ns: cur.namespace ?? cur.ns ?? "default", name: cur.name, verb: "start" })
                  }
                >
                  <Icon name="play" size={11} />
                  Start
                </button>
              )}
              <button
                className="btn btn--sm"
                disabled={action.isPending}
                onClick={() =>
                  action.mutate({ ns: cur.namespace ?? cur.ns ?? "default", name: cur.name, verb: "stop" })
                }
              >
                <Icon name="stop" size={11} />
                Shutdown
              </button>
              <button
                className="btn btn--sm"
                disabled={action.isPending}
                onClick={() =>
                  action.mutate({ ns: cur.namespace ?? cur.ns ?? "default", name: cur.name, verb: "restart" })
                }
              >
                <Icon name="refresh" size={11} />
                Restart
              </button>
              <button className="btn btn--sm">Snapshot</button>
              <button className="btn btn--sm" style={{ marginLeft: "auto" }}>
                <Icon name="terminal" size={11} />
                Serial console
              </button>
            </div>
            <div className="row gap-12" style={{ flexWrap: "wrap" }}>
              <div className="kpi">
                <div className="kpi__lbl">vCPU</div>
                <div className="kpi__val mono">{cur.cpu ?? cur.vcpu ?? "—"}</div>
              </div>
              <div className="kpi">
                <div className="kpi__lbl">Memory</div>
                <div className="kpi__val mono">
                  {cur.ram ? `${(cur.ram / 1024).toFixed(0)} ` : cur.memory ? `${(cur.memory / 1024).toFixed(0)} ` : "—"}
                  <span className="muted">GiB</span>
                </div>
              </div>
              <div className="kpi">
                <div className="kpi__lbl">Disk</div>
                <div className="kpi__val mono">{cur.disk ?? "—"}</div>
              </div>
              <div className="kpi">
                <div className="kpi__lbl">MAC</div>
                <div className="kpi__val mono" style={{ fontSize: 11 }}>
                  {cur.mac ?? "—"}
                </div>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
