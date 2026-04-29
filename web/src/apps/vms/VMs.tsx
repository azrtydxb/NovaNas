import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { vms, type VM } from "../../api/vms";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

function vmKey(v: VM) {
  return `${v.namespace ?? v.ns ?? "default"}/${v.name}`;
}
function vmNs(v: VM) {
  return v.namespace ?? v.ns ?? "default";
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

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="field">
      <label className="field__label">{label}</label>
      {children}
      {hint && <div className="field__hint muted">{hint}</div>}
    </div>
  );
}

function MigrateModal({
  vm,
  onClose,
}: {
  vm: VM;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [target, setTarget] = useState("");
  const mut = useMutation({
    meta: { label: "Migrate failed" },
    mutationFn: () => vms.migrate(vmNs(vm), vm.name, target || undefined),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vms", "list"] });
      toastSuccess(`${vm.name} migration started`);
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="vm" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Migrate {vm.name}</div>
            <div className="muted modal__sub">
              Live-migrate this VM to another node in the cluster.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <Field
            label="Target node"
            hint="Leave blank to let the scheduler pick a node."
          >
            <input
              className="input"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              placeholder="nas2.lan"
              autoFocus
            />
          </Field>
          <div className="muted" style={{ padding: "0 16px 12px", fontSize: 11 }}>
            Current node: <span className="mono">{vm.node ?? "—"}</span>
          </div>
          {mut.isError && (
            <div className="modal__err">Failed: {(mut.error as Error).message}</div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={mut.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={mut.isPending}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? "Migrating…" : "Migrate"}
          </button>
        </div>
      </div>
    </div>
  );
}

function SnapshotModal({
  vm,
  onClose,
}: {
  vm: VM;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(`${vm.name}-${new Date().toISOString().slice(0, 10)}`);
  const mut = useMutation({
    meta: { label: "Snapshot failed" },
    mutationFn: () =>
      vms.createSnapshot({ name, vmName: vm.name, namespace: vmNs(vm) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vms", "snapshots"] });
      toastSuccess(`Snapshot ${name} created`);
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="vm" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Snapshot {vm.name}</div>
            <div className="muted modal__sub">
              Captures the current disk and memory state.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <Field label="Snapshot name">
            <input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
          </Field>
          {mut.isError && (
            <div className="modal__err">Failed: {(mut.error as Error).message}</div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={mut.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={!name.trim() || mut.isPending}
            onClick={() => mut.mutate()}
          >
            {mut.isPending ? "Creating…" : "Create snapshot"}
          </button>
        </div>
      </div>
    </div>
  );
}

export function VMs() {
  const qc = useQueryClient();
  const [sel, setSel] = useState<string | null>(null);
  const [showMigrate, setShowMigrate] = useState(false);
  const [showSnapshot, setShowSnapshot] = useState(false);

  const list = useQuery({
    queryKey: ["vms", "list"],
    queryFn: () => vms.list(),
    retry: false,
  });

  const verbLabel: Record<
    "start" | "stop" | "restart" | "pause" | "unpause",
    string
  > = {
    start: "started",
    stop: "shut down",
    restart: "restarting",
    pause: "paused",
    unpause: "resumed",
  };
  const action = useMutation({
    meta: { label: "VM action failed" },
    mutationFn: async (a: {
      ns: string;
      name: string;
      verb: "start" | "stop" | "restart" | "pause" | "unpause";
    }) => {
      switch (a.verb) {
        case "start":
          return vms.start(a.ns, a.name);
        case "stop":
          return vms.stop(a.ns, a.name);
        case "restart":
          return vms.restart(a.ns, a.name);
        case "pause":
          return vms.pause(a.ns, a.name);
        case "unpause":
          return vms.unpause(a.ns, a.name);
      }
    },
    onSuccess: (_d, a) => {
      qc.invalidateQueries({ queryKey: ["vms", "list"] });
      toastSuccess(`${a.name} ${verbLabel[a.verb]}`);
    },
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
          <div className="discover__msg muted">
            KubeVirt is not yet ready. VMs will appear here once the cluster is online.
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
        Failed to load VMs: {err.message}
      </div>
    );
  }

  const items = list.data ?? [];
  const cur = items.find((v) => vmKey(v) === sel) ?? items[0];
  const state = (cur?.state ?? cur?.status ?? "").toLowerCase();

  return (
    <div style={{ display: "grid", gridTemplateColumns: "180px 1fr", height: "100%" }}>
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
          const isOn = cur && vmKey(cur) === k ? "is-on" : "";
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
                  {v.os ?? vmNs(v)}
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
              <span className="muted mono" style={{ fontSize: 10 }}>
                {vmNs(cur)}/{cur.name}
              </span>
              {cur.node && (
                <span className="muted mono" style={{ fontSize: 10 }}>
                  on {cur.node}
                </span>
              )}
              <span className="muted" style={{ marginLeft: "auto", fontSize: 11 }}>
                {cur.uptime ? `uptime ${cur.uptime}` : ""}
              </span>
            </div>
            <div className="vm-console" style={{ minHeight: 160 }}>
              {state === "running" ? (
                <>
                  <div style={{ opacity: 0.6, fontSize: 10, marginBottom: 8 }}>
                    {cur.os ?? "vm"} · console
                  </div>
                  <div>
                    {cur.name} login: <span className="vm-console__cursor" />
                  </div>
                </>
              ) : (
                <div style={{ opacity: 0.4, padding: 30, textAlign: "center" }}>
                  ● {state === "paused" ? "Paused" : "Powered off"}
                </div>
              )}
            </div>
            <div className="row gap-8" style={{ flexWrap: "wrap" }}>
              {state === "running" ? (
                <button
                  className="btn btn--sm btn--primary"
                  disabled={action.isPending}
                  onClick={() =>
                    action.mutate({ ns: vmNs(cur), name: cur.name, verb: "pause" })
                  }
                >
                  <Icon name="pause" size={11} />
                  Pause
                </button>
              ) : state === "paused" ? (
                <button
                  className="btn btn--sm btn--primary"
                  disabled={action.isPending}
                  onClick={() =>
                    action.mutate({ ns: vmNs(cur), name: cur.name, verb: "unpause" })
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
                    action.mutate({ ns: vmNs(cur), name: cur.name, verb: "start" })
                  }
                >
                  <Icon name="play" size={11} />
                  Start
                </button>
              )}
              <button
                className="btn btn--sm"
                disabled={action.isPending}
                onClick={() => {
                  if (state === "running") {
                    if (window.confirm(`Shut down ${cur.name}?`)) {
                      action.mutate({ ns: vmNs(cur), name: cur.name, verb: "stop" });
                    }
                  } else {
                    action.mutate({ ns: vmNs(cur), name: cur.name, verb: "start" });
                  }
                }}
              >
                <Icon name="power" size={11} />
                {state === "running" ? "Shutdown" : "Boot"}
              </button>
              <button
                className="btn btn--sm"
                disabled={action.isPending || state !== "running"}
                onClick={() => {
                  if (window.confirm(`Restart ${cur.name}?`)) {
                    action.mutate({ ns: vmNs(cur), name: cur.name, verb: "restart" });
                  }
                }}
              >
                <Icon name="refresh" size={11} />
                Restart
              </button>
              <button
                className="btn btn--sm"
                disabled={state !== "running"}
                onClick={() => setShowMigrate(true)}
              >
                Migrate…
              </button>
              <button
                className="btn btn--sm"
                onClick={() => setShowSnapshot(true)}
              >
                Snapshot
              </button>
              <button
                className="btn btn--sm"
                disabled={state !== "running"}
                onClick={() =>
                  window.open(
                    vms.serialUrl(vmNs(cur), cur.name),
                    `vm-serial-${cur.name}`,
                    "width=900,height=600",
                  )
                }
                style={{ marginLeft: "auto" }}
              >
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
                  {cur.ram
                    ? `${(cur.ram / 1024).toFixed(0)} `
                    : cur.memory
                      ? `${(cur.memory / 1024).toFixed(0)} `
                      : "—"}
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
            {action.isError && (
              <div className="muted" style={{ color: "var(--err)", fontSize: 11 }}>
                Action failed: {(action.error as Error).message}
              </div>
            )}
          </>
        )}
      </div>
      {showMigrate && cur && <MigrateModal vm={cur} onClose={() => setShowMigrate(false)} />}
      {showSnapshot && cur && <SnapshotModal vm={cur} onClose={() => setShowSnapshot(false)} />}
    </div>
  );
}
