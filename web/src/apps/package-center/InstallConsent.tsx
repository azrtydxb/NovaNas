import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { plugins, type PluginIndexEntry } from "../../api/plugins";
import { api } from "../../api/client";
import { Icon } from "../../components/Icon";
import { formatBytes } from "../../lib/format";

type Props = {
  plugin: PluginIndexEntry;
  marketplaceId?: string;
  onClose: () => void;
};

// Backend's permissions summary returns the Go const names from
// internal/auth/rbac.go (e.g. "PermPluginsRead") rather than the
// nova:domain:verb scope strings. We accept both forms here so the
// dialog stays correct if the backend's emit format ever shifts.
const PERM_DESCS: Record<string, string> = {
  PermStorageRead: "Read pools, vdevs, datasets, capacity",
  PermStorageWrite: "Create/modify pools, vdevs, datasets",
  PermNetworkRead: "Read interfaces, routes, RDMA state",
  PermNetworkWrite: "Configure interfaces, routes, RDMA",
  PermSystemRead: "Read host info — hostname, version, uptime",
  PermSystemWrite: "Modify system configuration",
  PermSystemAdmin: "Reboot or shut down the host",
  PermAuditRead: "Read the audit log",
  PermSchedulerRead: "Read scheduled jobs",
  PermSchedulerWrite: "Create or modify scheduled jobs",
  PermNotificationsRead: "Read notifications",
  PermNotificationsWrite: "Send notifications",
  PermNotificationsEventsRead: "Read notification event stream",
  PermNotificationsEventsWrite: "Emit notification events",
  PermPoolEncryptionRead: "Read encryption status",
  PermPoolEncryptionWrite: "Rotate encryption keys",
  PermPoolEncryptionRecover: "Unlock encrypted pools/datasets",
  PermReplicationRead: "Read replication jobs and targets",
  PermReplicationWrite: "Create or modify replication jobs",
  PermScrubRead: "Read scrub status and policies",
  PermScrubWrite: "Trigger scrubs or change policies",
  PermAlertsRead: "Read fired alerts and silences",
  PermAlertsWrite: "Acknowledge or silence alerts",
  PermLogsRead: "Read system logs",
  PermSessionsRead: "Read active user sessions",
  PermSessionsAdmin: "Revoke user sessions",
  PermVMRead: "Read VM list and state",
  PermVMWrite: "Start, stop, or modify VMs",
  PermWorkloadsRead: "Read Helm workloads",
  PermWorkloadsWrite: "Install or modify workloads",
  PermPluginsRead: "Read installed plugins",
  PermPluginsWrite: "Install or remove plugins",
  PermPluginsAdmin: "Modify plugin engine configuration",
  PermMarketplacesRead: "Read configured marketplaces",
  PermMarketplacesAdmin: "Add or remove marketplaces",
  "nova:storage:read": "Read pools, vdevs, datasets, capacity",
  "nova:storage:write": "Create/modify pools, vdevs, datasets",
  "nova:network:read": "Read interfaces, routes, RDMA state",
  "nova:network:write": "Configure interfaces, routes, RDMA",
  "nova:system:read": "Read host info — hostname, version, uptime",
  "nova:system:write": "Modify system configuration",
  "nova:system:admin": "Reboot or shut down the host",
  "nova:audit:read": "Read the audit log",
  "nova:scheduler:read": "Read scheduled jobs",
  "nova:scheduler:write": "Create or modify scheduled jobs",
  "nova:notifications:read": "Read notifications",
  "nova:notifications:write": "Send notifications",
  "nova:encryption:read": "Read encryption status",
  "nova:encryption:write": "Rotate encryption keys",
  "nova:encryption:recover": "Unlock encrypted pools/datasets",
  "nova:replication:read": "Read replication jobs and targets",
  "nova:replication:write": "Create or modify replication jobs",
  "nova:scrub:read": "Read scrub status and policies",
  "nova:scrub:write": "Trigger scrubs or change policies",
  "nova:alerts:read": "Read fired alerts and silences",
  "nova:alerts:write": "Acknowledge or silence alerts",
  "nova:logs:read": "Read system logs",
  "nova:sessions:read": "Read active user sessions",
  "nova:sessions:admin": "Revoke user sessions",
  "nova:vm:read": "Read VM list and state",
  "nova:vm:write": "Start, stop, or modify VMs",
  "nova:workloads:read": "Read Helm workloads",
  "nova:workloads:write": "Install or modify workloads",
  "nova:plugins:read": "Read installed plugins",
  "nova:plugins:write": "Install or remove plugins",
  "nova:plugins:admin": "Modify plugin engine configuration",
  "nova:marketplaces:read": "Read configured marketplaces",
  "nova:marketplaces:admin": "Add or remove marketplaces",
};

export function InstallConsent({ plugin, marketplaceId, onClose }: Props) {
  const v = plugin.versions[0];
  const [agreed, setAgreed] = useState(false);
  const qc = useQueryClient();

  const preview = useQuery({
    queryKey: ["plugin-preview", plugin.name, v?.version],
    enabled: !!v,
    queryFn: () => plugins.getManifestPreview(plugin.name, v.version),
  });

  const install = useMutation({
    mutationFn: () =>
      api(`/api/v1/plugins`, {
        method: "POST",
        body: JSON.stringify({
          name: plugin.name,
          version: v?.version,
          marketplaceId: marketplaceId ?? plugin.marketplace,
          accepted: true,
        }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["plugins"] });
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            {plugin.name.split("-").slice(-1)[0].slice(0, 2).toUpperCase()}
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Install {plugin.displayName ?? plugin.name}?</div>
            <div className="muted modal__sub">
              v{v?.version ?? "—"} · from{" "}
              <span style={{ color: "var(--fg-1)" }}>
                {marketplaceId ?? plugin.marketplace ?? "(default)"}
              </span>
              {v && <> · {formatBytes(v.size)}</>}
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>

        <div className="modal__body">
          {plugin.description && <p className="modal__desc">{plugin.description}</p>}

          {preview.isLoading && (
            <div className="modal__loading">Verifying signature and reading manifest…</div>
          )}
          {preview.isError && (
            <div className="modal__err">
              Preview failed: {(preview.error as Error).message}
            </div>
          )}

          {preview.data && (
            <>
              <Sect title={`Permissions (${preview.data.permissions.scopes.length})`}>
                {preview.data.permissions.scopes.length === 0 ? (
                  <div className="muted">No new permissions requested.</div>
                ) : (
                  preview.data.permissions.scopes.map((p) => (
                    <div key={p} className="perm-row">
                      <Icon name="shield" size={11} />
                      <span className="perm-row__name mono">{p}</span>
                      <span className="perm-row__desc">{PERM_DESCS[p] ?? "—"}</span>
                    </div>
                  ))
                )}
              </Sect>

              {preview.data.permissions.willCreate.length > 0 && (
                <Sect title="Will create">
                  {preview.data.permissions.willCreate.map((c, i) => (
                    <div key={i} className="perm-row">
                      <Icon name={iconFor(c.kind)} size={11} />
                      <span className="perm-row__name">{c.what}</span>
                      {c.destructive && (
                        <span className="pill pill--err" style={{ marginLeft: "auto" }}>
                          destructive
                        </span>
                      )}
                    </div>
                  ))}
                </Sect>
              )}

              {preview.data.permissions.willMount.length > 0 && (
                <Sect title="Mounts API routes">
                  {preview.data.permissions.willMount.map((r) => (
                    <div key={r} className="perm-row">
                      <Icon name="external" size={11} />
                      <span className="perm-row__name mono">{r}</span>
                    </div>
                  ))}
                </Sect>
              )}

              {preview.data.permissions.willOpen.length > 0 && (
                <Sect title="Opens ports">
                  {preview.data.permissions.willOpen.map((p) => (
                    <div key={p} className="perm-row">
                      <Icon name="net" size={11} />
                      <span className="perm-row__name mono">{p}</span>
                    </div>
                  ))}
                </Sect>
              )}

              <Sect title="Trust">
                <div className="muted modal__trust">
                  Tarball verified by cosign against marketplace{" "}
                  <span className="mono" style={{ color: "var(--fg-1)" }}>
                    {marketplaceId ?? plugin.marketplace}
                  </span>
                  <br />
                  sha256: <span className="mono">{v?.sha256}</span>
                </div>
              </Sect>
            </>
          )}

          <label className="modal__checkbox">
            <input
              type="checkbox"
              checked={agreed}
              onChange={(e) => setAgreed(e.target.checked)}
            />
            I trust this source and grant the requested permissions.
          </label>
        </div>

        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={install.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            onClick={() => install.mutate()}
            disabled={!agreed || install.isPending || !preview.data}
          >
            <Icon name="download" size={11} />
            {install.isPending ? "Installing…" : "Install"}
          </button>
        </div>

        {install.isError && (
          <div className="modal__err" style={{ margin: "0 16px 12px" }}>
            Install failed: {(install.error as Error).message}
          </div>
        )}
      </div>
    </div>
  );
}

function Sect({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="sect">
      <div className="sect__title">{title}</div>
      <div className="sect__body">{children}</div>
    </div>
  );
}

function iconFor(kind: string): "storage" | "shield" | "key" | "user" | "external" | "info" {
  switch (kind) {
    case "dataset":
      return "storage";
    case "tlsCert":
      return "shield";
    case "oidcClient":
      return "key";
    case "permission":
      return "user";
    default:
      return "info";
  }
}
