import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { network, type NetConfig, type NetInterface } from "../../api/network";
import { Icon } from "../../components/Icon";

type AddType = "eth" | "bond" | "vlan" | "bridge";

export function Interfaces() {
  const qc = useQueryClient();
  const [selected, setSelected] = useState<string | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [editName, setEditName] = useState<string | null>(null);

  const ifaces = useQuery({
    queryKey: ["network", "interfaces"],
    queryFn: () => network.listInterfaces(),
  });

  const reload = useMutation({
    mutationFn: () => network.reload(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["network"] });
    },
  });

  const remove = useMutation({
    mutationFn: (name: string) => network.deleteConfig(name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["network"] });
      setSelected(null);
    },
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setShowAdd(true)}>
          <Icon name="plus" size={11} /> Add interface
        </button>
        <button
          className="btn"
          style={{ marginLeft: "auto" }}
          onClick={() => reload.mutate()}
          disabled={reload.isPending}
        >
          <Icon name="refresh" size={11} />{" "}
          {reload.isPending ? "Reloading…" : "Reload"}
        </button>
      </div>

      {reload.isError && (
        <div className="muted" style={{ color: "var(--err)", marginTop: 8 }}>
          Reload failed: {(reload.error as Error).message}
        </div>
      )}

      {ifaces.isLoading && <div className="muted">Loading interfaces…</div>}
      {ifaces.isError && (
        <div className="muted" style={{ color: "var(--err)" }}>
          Failed to load: {(ifaces.error as Error).message}
        </div>
      )}
      {ifaces.data && ifaces.data.length === 0 && (
        <div className="muted">No interfaces found.</div>
      )}
      {ifaces.data && ifaces.data.length > 0 && (
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
              <th></th>
            </tr>
          </thead>
          <tbody>
            {ifaces.data.map((i: NetInterface) => {
              const state = (i.state ?? i.link ?? "").toString().toUpperCase();
              const ipv4 =
                i.ipv4 ??
                (i.addresses ?? []).find((a) => /^\d+\.\d+\.\d+\.\d+/.test(a));
              const isOn = selected === i.name;
              return (
                <tr
                  key={i.name}
                  className={isOn ? "is-on" : ""}
                  onClick={() => setSelected(i.name)}
                  style={{ cursor: "pointer" }}
                >
                  <td className="mono">{i.name}</td>
                  <td>
                    {i.type ? (
                      <span className="pill pill--info">{i.type}</span>
                    ) : (
                      <span className="muted">—</span>
                    )}
                  </td>
                  <td>
                    <span
                      className={`sdot sdot--${state === "UP" || state === "ACTIVE" ? "ok" : "warn"}`}
                    />{" "}
                    {state || "—"}
                  </td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {ipv4 ?? <span className="muted">—</span>}
                  </td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {i.mac ?? "—"}
                  </td>
                  <td className="num mono">{i.mtu ?? "—"}</td>
                  <td className="mono" style={{ fontSize: 11 }}>
                    {i.speed ?? "—"}
                  </td>
                  <td className="mono" style={{ textAlign: "right" }}>
                    <button
                      className="btn btn--sm"
                      onClick={(e) => {
                        e.stopPropagation();
                        setEditName(i.name);
                      }}
                    >
                      <Icon name="edit" size={11} />
                    </button>{" "}
                    <button
                      className="btn btn--sm btn--danger"
                      onClick={(e) => {
                        e.stopPropagation();
                        if (confirm(`Delete config for ${i.name}?`))
                          remove.mutate(i.name);
                      }}
                    >
                      <Icon name="trash" size={11} />
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {selected && (
        <ConfigDetail
          name={selected}
          onClose={() => setSelected(null)}
          onEdit={() => setEditName(selected)}
          onDelete={() => {
            if (confirm(`Delete config for ${selected}?`)) remove.mutate(selected);
          }}
        />
      )}

      {showAdd && <AddInterfaceModal onClose={() => setShowAdd(false)} />}
      {editName && (
        <EditConfigModal name={editName} onClose={() => setEditName(null)} />
      )}
    </div>
  );
}

function ConfigDetail({
  name,
  onClose,
  onEdit,
  onDelete,
}: {
  name: string;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const cfg = useQuery({
    queryKey: ["network", "configs", name],
    queryFn: () => network.getConfig(name),
  });

  return (
    <div className="sect" style={{ marginTop: 12 }}>
      <div className="sect__head">
        <div className="sect__title">Config · {name}</div>
        <div className="row gap-8" style={{ marginLeft: "auto" }}>
          <button className="btn btn--sm" onClick={onEdit}>
            <Icon name="edit" size={11} /> Edit
          </button>
          <button className="btn btn--sm btn--danger" onClick={onDelete}>
            <Icon name="trash" size={11} /> Delete
          </button>
          <button className="btn btn--sm" onClick={onClose}>
            <Icon name="x" size={11} />
          </button>
        </div>
      </div>
      <div className="sect__body">
        {cfg.isLoading && <div className="muted">Loading…</div>}
        {cfg.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>
            Failed: {(cfg.error as Error).message}
          </div>
        )}
        {cfg.data && (
          <pre
            className="mono"
            style={{
              fontSize: 11,
              padding: 10,
              background: "var(--bg-2)",
              borderRadius: "var(--r-md)",
              border: "1px solid var(--line)",
              overflowX: "auto",
              margin: 0,
            }}
          >
            {JSON.stringify(cfg.data, null, 2)}
          </pre>
        )}
      </div>
    </div>
  );
}

function AddInterfaceModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [type, setType] = useState<AddType>("eth");
  const [parent, setParent] = useState("");
  const [vid, setVid] = useState("");
  const [members, setMembers] = useState("");
  const [mode, setMode] = useState("active-backup");
  const [ipv4, setIpv4] = useState("");
  const [mtu, setMtu] = useState("");

  const create = useMutation({
    mutationFn: async () => {
      if (type === "bond") {
        return network.createBond({
          name,
          mode,
          members: members
            .split(",")
            .map((m) => m.trim())
            .filter(Boolean),
        });
      }
      if (type === "vlan") {
        return network.createVlan({
          name,
          parent,
          vid: vid ? Number(vid) : undefined,
          ipv4: ipv4 || undefined,
        });
      }
      const body: NetConfig = {
        name,
        type,
        enabled: true,
      };
      if (ipv4) body.ipv4 = ipv4;
      if (mtu) body.mtu = Number(mtu);
      return network.createConfig(body);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["network"] });
      onClose();
    },
  });

  const valid =
    name.trim() &&
    (type !== "vlan" || (parent.trim() && vid.trim())) &&
    (type !== "bond" || members.trim());

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="plus" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Add interface</div>
            <div className="muted modal__sub">
              Create a new network interface configuration.
            </div>
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
              placeholder="bond0 / vlan20 / br0"
              autoFocus
            />
          </div>
          <div className="field">
            <label className="field__label">Type</label>
            <select
              className="input"
              value={type}
              onChange={(e) => setType(e.target.value as AddType)}
            >
              <option value="eth">Ethernet</option>
              <option value="bond">Bond</option>
              <option value="vlan">VLAN</option>
              <option value="bridge">Bridge</option>
            </select>
          </div>
          {type === "bond" && (
            <>
              <div className="field">
                <label className="field__label">Mode</label>
                <select
                  className="input"
                  value={mode}
                  onChange={(e) => setMode(e.target.value)}
                >
                  <option value="active-backup">active-backup</option>
                  <option value="balance-rr">balance-rr</option>
                  <option value="802.3ad">802.3ad (LACP)</option>
                  <option value="balance-xor">balance-xor</option>
                </select>
              </div>
              <div className="field">
                <label className="field__label">Members</label>
                <input
                  className="input"
                  value={members}
                  onChange={(e) => setMembers(e.target.value)}
                  placeholder="ens3f0, ens3f1"
                />
                <div className="field__hint muted">Comma separated.</div>
              </div>
            </>
          )}
          {type === "vlan" && (
            <>
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
            </>
          )}
          {(type === "eth" || type === "bridge" || type === "vlan") && (
            <div className="field">
              <label className="field__label">IPv4 (CIDR, optional)</label>
              <input
                className="input"
                value={ipv4}
                onChange={(e) => setIpv4(e.target.value)}
                placeholder="192.168.10.10/24"
              />
            </div>
          )}
          {(type === "eth" || type === "bridge") && (
            <div className="field">
              <label className="field__label">MTU (optional)</label>
              <input
                className="input"
                value={mtu}
                onChange={(e) => setMtu(e.target.value)}
                placeholder="9000"
                inputMode="numeric"
              />
            </div>
          )}
          {create.isError && (
            <div className="modal__err">
              Failed: {(create.error as Error).message}
            </div>
          )}
        </div>
        <div className="modal__foot">
          <button
            className="btn"
            onClick={onClose}
            disabled={create.isPending}
          >
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

function EditConfigModal({
  name,
  onClose,
}: {
  name: string;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const cfg = useQuery({
    queryKey: ["network", "configs", name],
    queryFn: () => network.getConfig(name),
  });
  const [text, setText] = useState<string>("");
  const [parseErr, setParseErr] = useState<string | null>(null);

  // initialize text once data arrives
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
      onClose();
    },
  });

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="edit" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Edit · {name}</div>
            <div className="muted modal__sub">
              Edit the raw config JSON. Backend will apply on save; run Reload
              to bring changes online.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          {cfg.isLoading && (
            <div className="modal__loading muted">Loading config…</div>
          )}
          {cfg.isError && (
            <div className="modal__err">
              Failed: {(cfg.error as Error).message}
            </div>
          )}
          {cfg.data && (
            <div className="field">
              <label className="field__label">Config (JSON)</label>
              <textarea
                className="input"
                rows={18}
                value={text}
                onChange={(e) => setText(e.target.value)}
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 11,
                  resize: "vertical",
                }}
              />
              {parseErr && (
                <div className="field__hint" style={{ color: "var(--err)" }}>
                  {parseErr}
                </div>
              )}
            </div>
          )}
          {save.isError && !parseErr && (
            <div className="modal__err">
              Failed: {(save.error as Error).message}
            </div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={save.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={!cfg.data || save.isPending}
            onClick={() => save.mutate()}
          >
            <Icon name="check" size={11} />
            {save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default Interfaces;
