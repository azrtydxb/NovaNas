import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { network, type NetConfig, type NetInterface } from "../../api/network";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

type AddType = "eth" | "bond" | "vlan" | "bridge";

export function Interfaces() {
  const qc = useQueryClient();
  const [showAddIface, setShowAddIface] = useState(false);
  const [showAddVlan, setShowAddVlan] = useState(false);
  const [showAddBond, setShowAddBond] = useState(false);
  const [editName, setEditName] = useState<string | null>(null);

  const ifaces = useQuery({
    queryKey: ["network", "interfaces"],
    queryFn: () => network.listInterfaces(),
  });

  const reload = useMutation({
    meta: { label: "Network reload failed" },
    mutationFn: () => network.reload(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["network"] });
      toastSuccess("Network configuration reloaded");
    },
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setShowAddIface(true)}>
          <Icon name="plus" size={11} />Add interface
        </button>
        <button className="btn" onClick={() => setShowAddVlan(true)}>Add VLAN</button>
        <button className="btn" onClick={() => setShowAddBond(true)}>Add bond</button>
        <button
          className="btn"
          style={{ marginLeft: "auto" }}
          onClick={() => reload.mutate()}
          disabled={reload.isPending}
        >
          <Icon name="refresh" size={11} />{reload.isPending ? "Reloading…" : "Reload"}
        </button>
      </div>
      <table className="tbl">
        <thead>
          <tr>
            <th>Interface</th>
            <th>Type</th>
            <th>State</th>
            <th>IPv4</th>
            <th>MAC</th>
            <th className="num">MTU</th>
            <th>Speed</th>
          </tr>
        </thead>
        <tbody>
          {(ifaces.data ?? []).map((i: NetInterface) => {
            const state = (i.state ?? i.link ?? "").toString().toUpperCase();
            const ipv4 =
              i.ipv4 ??
              (i.addresses ?? []).find((a) => /^\d+\.\d+\.\d+\.\d+/.test(a));
            return (
              <tr key={i.name} onClick={() => setEditName(i.name)} style={{ cursor: "pointer" }}>
                <td className="mono">{i.name}</td>
                <td>
                  {i.type ? <span className="pill pill--info">{i.type}</span> : <span className="muted">—</span>}
                </td>
                <td>
                  <span className={`sdot sdot--${state === "UP" || state === "ACTIVE" ? "ok" : "warn"}`} />{" "}
                  {state || "—"}
                </td>
                <td className="mono" style={{ fontSize: 11 }}>
                  {ipv4 ?? <span className="muted">—</span>}
                </td>
                <td className="mono muted" style={{ fontSize: 11 }}>{i.mac ?? "—"}</td>
                <td className="num mono">{i.mtu ?? "—"}</td>
                <td className="mono" style={{ fontSize: 11 }}>{i.speed ?? "—"}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {ifaces.isLoading && <div className="muted" style={{ padding: 8 }}>Loading interfaces…</div>}
      {ifaces.isError && (
        <div className="muted" style={{ padding: 8, color: "var(--err)" }}>
          Failed to load: {(ifaces.error as Error).message}
        </div>
      )}
      {ifaces.data && ifaces.data.length === 0 && (
        <div className="muted" style={{ padding: 20 }}>No interfaces found.</div>
      )}

      {showAddIface && <AddInterfaceModal onClose={() => setShowAddIface(false)} />}
      {showAddVlan && <CreateVlanModal onClose={() => setShowAddVlan(false)} />}
      {showAddBond && <CreateBondModal onClose={() => setShowAddBond(false)} />}
      {editName && <EditConfigModal name={editName} onClose={() => setEditName(null)} />}
    </div>
  );
}

function AddInterfaceModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [type, setType] = useState<AddType>("eth");
  const [ipv4, setIpv4] = useState("");
  const [mtu, setMtu] = useState("");

  const create = useMutation({
    meta: { label: "Create interface failed" },
    mutationFn: async () => {
      const body: NetConfig = { name, type, enabled: true };
      if (ipv4) body.ipv4 = ipv4;
      if (mtu) body.mtu = Number(mtu);
      return network.createConfig(body);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["network"] });
      toastSuccess(`Interface ${name} created`);
      onClose();
    },
  });

  const valid = name.trim();

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon"><Icon name="plus" size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">Add interface</div>
            <div className="muted modal__sub">Create a new network interface configuration.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Name</label>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="ens1" autoFocus />
          </div>
          <div className="field">
            <label className="field__label">Type</label>
            <select className="input" value={type} onChange={(e) => setType(e.target.value as AddType)}>
              <option value="eth">Ethernet</option>
              <option value="bridge">Bridge</option>
            </select>
          </div>
          <div className="field">
            <label className="field__label">IPv4 (CIDR, optional)</label>
            <input className="input" value={ipv4} onChange={(e) => setIpv4(e.target.value)} placeholder="192.168.10.10/24" />
          </div>
          <div className="field">
            <label className="field__label">MTU (optional)</label>
            <input className="input" value={mtu} onChange={(e) => setMtu(e.target.value)} placeholder="9000" inputMode="numeric" />
          </div>
          {create.isError && <div className="modal__err">Failed: {(create.error as Error).message}</div>}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={create.isPending}>Cancel</button>
          <button className="btn btn--primary" disabled={!valid || create.isPending} onClick={() => create.mutate()}>
            <Icon name="plus" size={11} />{create.isPending ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
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
          <div className="modal__icon"><Icon name="plus" size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">Create VLAN</div>
            <div className="muted modal__sub">Tagged virtual interface on top of a parent device.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Name</label>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="vlan20" autoFocus />
          </div>
          <div className="field">
            <label className="field__label">Parent</label>
            <input className="input" value={parent} onChange={(e) => setParent(e.target.value)} placeholder="bond0" />
          </div>
          <div className="field">
            <label className="field__label">VLAN ID</label>
            <input className="input" value={vid} onChange={(e) => setVid(e.target.value)} placeholder="20" inputMode="numeric" />
          </div>
          <div className="field">
            <label className="field__label">IPv4 (CIDR, optional)</label>
            <input className="input" value={ipv4} onChange={(e) => setIpv4(e.target.value)} placeholder="192.168.20.10/24" />
          </div>
          {create.isError && <div className="modal__err">Failed: {(create.error as Error).message}</div>}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={create.isPending}>Cancel</button>
          <button className="btn btn--primary" disabled={!valid || create.isPending} onClick={() => create.mutate()}>
            <Icon name="plus" size={11} />{create.isPending ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}

function CreateBondModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [mode, setMode] = useState("active-backup");
  const [members, setMembers] = useState("");

  const create = useMutation({
    meta: { label: "Create bond failed" },
    mutationFn: () =>
      network.createBond({
        name,
        mode,
        members: members.split(",").map((m) => m.trim()).filter(Boolean),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["network"] });
      toastSuccess(`Bond ${name} created`);
      onClose();
    },
  });

  const valid = name.trim() && members.trim();

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon"><Icon name="plus" size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">Create bond</div>
            <div className="muted modal__sub">Aggregate two or more interfaces into a single bond.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          <div className="field">
            <label className="field__label">Name</label>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="bond0" autoFocus />
          </div>
          <div className="field">
            <label className="field__label">Mode</label>
            <select className="input" value={mode} onChange={(e) => setMode(e.target.value)}>
              <option value="active-backup">active-backup</option>
              <option value="balance-rr">balance-rr</option>
              <option value="802.3ad">802.3ad (LACP)</option>
              <option value="balance-xor">balance-xor</option>
            </select>
          </div>
          <div className="field">
            <label className="field__label">Members</label>
            <input className="input" value={members} onChange={(e) => setMembers(e.target.value)} placeholder="ens3f0, ens3f1" />
            <div className="field__hint muted">Comma separated.</div>
          </div>
          {create.isError && <div className="modal__err">Failed: {(create.error as Error).message}</div>}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={create.isPending}>Cancel</button>
          <button className="btn btn--primary" disabled={!valid || create.isPending} onClick={() => create.mutate()}>
            <Icon name="plus" size={11} />{create.isPending ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}

function EditConfigModal({ name, onClose }: { name: string; onClose: () => void }) {
  const qc = useQueryClient();
  const cfg = useQuery({
    queryKey: ["network", "configs", name],
    queryFn: () => network.getConfig(name),
  });
  const [text, setText] = useState<string>("");
  const [parseErr, setParseErr] = useState<string | null>(null);

  useEffect(() => {
    if (cfg.data && text === "") {
      try {
        setText(JSON.stringify(cfg.data, null, 2));
      } catch {
        /* noop */
      }
    }
  }, [cfg.data, text]);

  const save = useMutation({
    meta: { label: "Update interface failed" },
    mutationFn: async () => {
      let body: NetConfig;
      try {
        body = JSON.parse(text) as NetConfig;
        setParseErr(null);
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        setParseErr(msg);
        throw new Error("Invalid JSON: " + msg);
      }
      return network.updateConfig(name, body);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["network"] });
      toastSuccess(`Config saved for ${name}`);
      onClose();
    },
  });

  const remove = useMutation({
    meta: { label: "Delete interface failed" },
    mutationFn: () => network.deleteConfig(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["network"] });
      toastSuccess("Interface deleted");
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon"><Icon name="edit" size={16} /></div>
          <div className="modal__head-meta">
            <div className="modal__title">Edit · {name}</div>
            <div className="muted modal__sub">Edit the raw config JSON. Run Reload after save.</div>
          </div>
          <button className="modal__close" onClick={onClose}><Icon name="x" size={14} /></button>
        </div>
        <div className="modal__body">
          {cfg.isLoading && <div className="modal__loading muted">Loading config…</div>}
          {cfg.isError && <div className="modal__err">Failed: {(cfg.error as Error).message}</div>}
          {cfg.data && (
            <div className="field">
              <label className="field__label">Config (JSON)</label>
              <textarea
                className="input"
                rows={18}
                value={text}
                onChange={(e) => setText(e.target.value)}
                style={{ fontFamily: "var(--font-mono)", fontSize: 11, resize: "vertical" }}
              />
              {parseErr && <div className="field__hint" style={{ color: "var(--err)" }}>{parseErr}</div>}
            </div>
          )}
          {save.isError && !parseErr && <div className="modal__err">Failed: {(save.error as Error).message}</div>}
        </div>
        <div className="modal__foot">
          <button
            className="btn btn--danger"
            onClick={() => {
              if (confirm(`Delete config for ${name}?`)) remove.mutate();
            }}
            disabled={remove.isPending}
            style={{ marginRight: "auto" }}
          >
            <Icon name="trash" size={11} />Delete
          </button>
          <button className="btn" onClick={onClose} disabled={save.isPending}>Cancel</button>
          <button className="btn btn--primary" disabled={!cfg.data || save.isPending} onClick={() => save.mutate()}>
            <Icon name="check" size={11} />{save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default Interfaces;
