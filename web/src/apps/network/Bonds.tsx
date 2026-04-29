import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { network } from "../../api/network";
import { Icon } from "../../components/Icon";
import { toastSuccess } from "../../store/toast";

export function Bonds() {
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const q = useQuery({ queryKey: ["network", "bonds"], queryFn: () => network.listBonds() });

  const Toolbar = (
    <div className="tbar">
      <button className="btn btn--primary" onClick={() => setShowCreate(true)}>
        <Icon name="plus" size={11} /> Create bond
      </button>
      <button
        className="btn btn--sm"
        style={{ marginLeft: "auto" }}
        onClick={() => qc.invalidateQueries({ queryKey: ["network", "bonds"] })}
      >
        <Icon name="refresh" size={11} /> Refresh
      </button>
    </div>
  );

  const body = (() => {
    if (q.isLoading) return <div className="empty-hint">Loading bonds…</div>;
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
          <Icon name="net" size={20} /> No bonds configured. Click Create bond above.
        </div>
      );
    return (
      <table className="tbl">
        <thead>
          <tr>
            <th>Name</th>
            <th>Mode</th>
            <th>Slaves</th>
            <th>State</th>
            <th>MTU</th>
            <th>MAC</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((b: Record<string, unknown>) => {
            const name = String(b.name ?? "");
            const mode = String(b.mode ?? b.bondMode ?? "—");
            const slaves = Array.isArray(b.slaves)
              ? (b.slaves as string[]).join(", ")
              : Array.isArray(b.members)
                ? (b.members as string[]).join(", ")
                : (b.slaves as string) ?? "—";
            const state = String(b.state ?? "—");
            const mtu = String(b.mtu ?? "—");
            const mac = String(b.mac ?? "—");
            return (
              <tr key={name}>
                <td className="mono">{name}</td>
                <td>{mode}</td>
                <td className="mono small">{slaves}</td>
                <td>
                  <span className={`pill pill--${/up|active|online/i.test(state) ? "ok" : "warn"}`}>
                    <span className="dot" />
                    {state}
                  </span>
                </td>
                <td className="mono small">{mtu}</td>
                <td className="mono small">{mac}</td>
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
      {showCreate && <CreateBondModal onClose={() => setShowCreate(false)} />}
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
        members: members
          .split(",")
          .map((m) => m.trim())
          .filter(Boolean),
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
          <div className="modal__icon">
            <Icon name="plus" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Create bond</div>
            <div className="muted modal__sub">Aggregate two or more interfaces into a single bond.</div>
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
              placeholder="bond0"
              autoFocus
            />
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
            <input
              className="input"
              value={members}
              onChange={(e) => setMembers(e.target.value)}
              placeholder="ens3f0, ens3f1"
            />
            <div className="field__hint muted">Comma separated.</div>
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

export default Bonds;
