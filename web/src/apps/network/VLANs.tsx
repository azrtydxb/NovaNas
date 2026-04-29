import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { network } from "../../api/network";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

export function VLANs() {
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const q = useQuery({ queryKey: ["network", "vlans"], queryFn: () => network.listVlans() });

  const Toolbar = (
    <div className="tbar">
      <button className="btn btn--primary" onClick={() => setShowCreate(true)}>
        <Icon name="plus" size={11} /> Create VLAN
      </button>
      <button
        className="btn btn--sm"
        style={{ marginLeft: "auto" }}
        onClick={() => qc.invalidateQueries({ queryKey: ["network", "vlans"] })}
      >
        <Icon name="refresh" size={11} /> Refresh
      </button>
    </div>
  );

  const body = (() => {
    if (q.isLoading) return <div className="empty-hint">Loading VLANs…</div>;
    if (q.isError)
      return (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      );
    const rows = q.data ?? [];
    if (rows.length === 0)
      return (
        <div className="empty-hint" style={{ padding: 24 }}>
          <Icon name="net" size={20} /> No VLANs configured. Click Create VLAN above.
        </div>
      );
    return (
      <table className="tbl">
        <thead>
          <tr>
            <th>Name</th>
            <th>Parent</th>
            <th>VID</th>
            <th>State</th>
            <th>IPv4</th>
            <th>MTU</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((v: Record<string, unknown>) => {
            const name = String(v.name ?? "");
            const parent = String(v.parent ?? v.link ?? "—");
            const vid = String(v.vid ?? v.id ?? "—");
            const state = String(v.state ?? "—");
            const ipv4 = Array.isArray(v.ipv4) ? (v.ipv4 as string[]).join(", ") : (v.ipv4 as string) ?? "—";
            const mtu = String(v.mtu ?? "—");
            return (
              <tr key={name}>
                <td className="mono">{name}</td>
                <td className="mono small">{parent}</td>
                <td className="mono">{vid}</td>
                <td>
                  <span className={`pill pill--${/up|active|online/i.test(state) ? "ok" : "warn"}`}>
                    <span className="dot" />
                    {state}
                  </span>
                </td>
                <td className="mono small">{ipv4}</td>
                <td className="mono small">{mtu}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    );
  })();

  return (
    <div style={{ padding: 14 }}>
      {Toolbar}
      {body}
      {showCreate && <CreateVlanModal onClose={() => setShowCreate(false)} />}
    </div>
  );
}

function CreateVlanModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [parent, setParent] = useState("");
  const [vid, setVid] = useState("");
  const [ipv4, setIpv4] = useState("");

  const create = useMutation({
    meta: { label: "Create VLAN failed" },
    mutationFn: () =>
      network.createVlan({
        name,
        parent,
        vid: vid ? Number(vid) : undefined,
        ipv4: ipv4 || undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["network"] });
      toastSuccess(`VLAN ${name} created`);
      onClose();
    },
  });

  const valid = name.trim() && parent.trim() && vid.trim();

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="plus" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Create VLAN</div>
            <div className="muted modal__sub">Tagged virtual interface on top of a parent device.</div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Name</label>
            <input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="vlan20"
              autoFocus
            />
          </div>
          <div className="field">
            <label className="field__label">Parent</label>
            <input
              className="input"
              value={parent}
              onChange={(e) => setParent(e.target.value)}
              placeholder="bond0"
            />
          </div>
          <div className="field">
            <label className="field__label">VLAN ID</label>
            <input
              className="input"
              value={vid}
              onChange={(e) => setVid(e.target.value)}
              placeholder="20"
              inputMode="numeric"
            />
          </div>
          <div className="field">
            <label className="field__label">IPv4 (CIDR, optional)</label>
            <input
              className="input"
              value={ipv4}
              onChange={(e) => setIpv4(e.target.value)}
              placeholder="192.168.20.10/24"
            />
          </div>
          {create.isError && (
            <div className="modal__err">Failed: {(create.error as Error).message}</div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={create.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={!valid || create.isPending}
            onClick={() => create.mutate()}
          >
            <Icon name="plus" size={11} />
            {create.isPending ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default VLANs;
