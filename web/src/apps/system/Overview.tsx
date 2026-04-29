import { useQuery } from "@tanstack/react-query";
import { system } from "../../api/system";
import { formatBytes } from "../../lib/format";

function formatUptime(v: string | number | undefined): string {
  if (v === undefined || v === null) return "—";
  if (typeof v === "string") return v;
  // assume seconds
  const s = Math.floor(v);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatMem(v: string | number | undefined): string {
  if (v === undefined || v === null) return "—";
  if (typeof v === "string") return v;
  return formatBytes(v);
}

function Sect({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="sect">
      <div className="sect__head">
        <div className="sect__title">{title}</div>
      </div>
      <div className="sect__body">{children}</div>
    </div>
  );
}

export function Overview() {
  const info = useQuery({
    queryKey: ["system", "info"],
    queryFn: () => system.info(),
  });

  if (info.isLoading) return <div className="muted">Loading system info…</div>;
  if (info.isError)
    return (
      <div className="muted" style={{ color: "var(--err)" }}>
        Failed to load: {(info.error as Error).message}
      </div>
    );

  const d = info.data ?? {};
  return (
    <>
      <Sect title="Host">
        <dl className="kv">
          <dt>Hostname</dt>
          <dd className="mono">{d.hostname ?? "—"}</dd>
          <dt>Version</dt>
          <dd>{d.version ?? "—"}</dd>
          <dt>Kernel</dt>
          <dd className="mono">{d.kernel ?? "—"}</dd>
          <dt>OS</dt>
          <dd>{d.os ?? "—"}</dd>
          <dt>Uptime</dt>
          <dd>{formatUptime(d.uptime)}</dd>
          <dt>Timezone</dt>
          <dd className="mono">{d.timezone ?? "—"}</dd>
          <dt>ZFS</dt>
          <dd className="mono">{d.zfsVersion ?? "—"}</dd>
        </dl>
      </Sect>
      <Sect title="Hardware">
        <dl className="kv">
          <dt>CPU</dt>
          <dd>{d.cpu ?? "—"}</dd>
          <dt>Cores / threads</dt>
          <dd className="mono">
            {d.cores ?? "—"} / {d.threads ?? "—"}
          </dd>
          <dt>Memory</dt>
          <dd className="mono">{formatMem(d.memory)}</dd>
          <dt>BMC</dt>
          <dd className="mono">{d.bmc ?? "—"}</dd>
        </dl>
      </Sect>
      <Sect title="Security">
        <dl className="kv">
          <dt>TPM</dt>
          <dd>{d.tpm ?? "—"}</dd>
          <dt>Secure Boot</dt>
          <dd>{d.secureBoot ?? "—"}</dd>
        </dl>
      </Sect>
    </>
  );
}

export default Overview;
