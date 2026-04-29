import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares, type NvmeofSubsystem, type NvmeofPort, type NvmeofNamespace } from "../../api/shares";
import { formatBytes } from "../../lib/format";
import { toastSuccess } from "../../store/toast";
import { Modal } from "./Modal";

type View = "subsystems" | "ports";

export function NVMEOF() {
  const [view, setView] = useState<View>("subsystems");
  const saveMut = useMutation({
    meta: { label: "Save config failed" },
    mutationFn: () => shares.nvmeofSaveConfig(),
    onSuccess: () => toastSuccess("NVMe-oF config saved"),
  });

  return (
    <div className="app-storage">
      <div className="win-tabs">
        {(["subsystems", "ports"] as const).map((v) => (
          <button key={v} className={view === v ? "is-on" : ""} onClick={() => setView(v)}>
            {v}
          </button>
        ))}
        <div style={{ marginLeft: "auto", padding: "4px 8px" }}>
          <button className="btn btn--sm" disabled={saveMut.isPending} onClick={() => saveMut.mutate()}>
            <Icon name="download" size={10} />
            Save config
          </button>
        </div>
      </div>
      <div style={{ flex: 1, minHeight: 0, overflow: "auto" }}>
        {view === "subsystems" && <SubsystemsView />}
        {view === "ports" && <PortsView />}
      </div>
    </div>
  );
}

function SubsystemsView() {
  const qc = useQueryClient();
  const [sel, setSel] = useState<string | null>(null);
  const [edit, setEdit] = useState<NvmeofSubsystem | "new" | null>(null);
  const subQ = useQuery({
    queryKey: ["nvmeof-subsystems"],
    queryFn: () => shares.listNvmeofSubsystems(),
  });
  const subs = subQ.data ?? [];
  const cur = subs.find((s) => s.nqn === sel);

  const inval = () => qc.invalidateQueries({ queryKey: ["nvmeof-subsystems"] });
  const delMut = useMutation({
    meta: { label: "Delete subsystem failed" },
    mutationFn: (nqn: string) => shares.deleteNvmeofSubsystem(nqn),
    onSuccess: (_d, nqn) => { inval(); toastSuccess("Subsystem deleted", nqn); },
  });

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: cur ? "1fr 360px" : "1fr",
        height: "100%",
      }}
    >
      <div style={{ padding: 14, overflow: "auto" }}>
        <div className="tbar">
          <button className="btn btn--primary" onClick={() => setEdit("new")}>
            <Icon name="plus" size={11} />
            New subsystem
          </button>
        </div>
        {subQ.isLoading && <div className="empty-hint">Loading subsystems…</div>}
        {subQ.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(subQ.error as Error).message}
          </div>
        )}
        {subQ.data && subs.length === 0 && (
          <div className="empty-hint">No NVMe-oF subsystems.</div>
        )}
        {subs.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>NQN</th>
                <th className="num">NS</th>
                <th className="num">Ports</th>
                <th className="num">Hosts</th>
                <th>DH-CHAP</th>
                <th>State</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {subs.map((s) => (
                <tr
                  key={s.nqn}
                  className={sel === s.nqn ? "is-on" : ""}
                  onClick={() => setSel(s.nqn)}
                  style={{ cursor: "pointer" }}
                >
                  <td className="mono" style={{ fontSize: 11 }}>{s.nqn}</td>
                  <td className="num mono">{s.ns ?? 0}</td>
                  <td className="num mono">{s.ports ?? 0}</td>
                  <td className="num mono">{s.hosts ?? 0}</td>
                  <td>{s.dhchap ? <Icon name="shield" size={11} /> : <span className="muted">off</span>}</td>
                  <td>
                    <span className="pill pill--ok">
                      <span className="dot" />
                      {s.state ?? "up"}
                    </span>
                  </td>
                  <td className="num" onClick={(e) => e.stopPropagation()}>
                    <button className="btn btn--sm" onClick={() => setEdit(s)}>Edit</button>{" "}
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={delMut.isPending}
                      onClick={() => {
                        if (window.confirm(`Delete subsystem ${s.nqn}?`)) delMut.mutate(s.nqn);
                      }}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {cur && <SubsystemDetail nqn={cur.nqn} onClose={() => setSel(null)} />}
      {edit && (
        <SubsystemModal
          init={edit === "new" ? null : edit}
          onClose={() => setEdit(null)}
          onDone={inval}
        />
      )}
    </div>
  );
}

function SubsystemModal({
  init,
  onClose,
  onDone,
}: {
  init: NvmeofSubsystem | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [nqn, setNqn] = useState(init?.nqn ?? "");
  const [serial, setSerial] = useState(init?.serial ?? "");
  const [dhchap, setDhchap] = useState(init?.dhchap ?? false);
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Save subsystem failed" },
    mutationFn: () => init
      ? shares.updateNvmeofSubsystem(init.nqn, { serial, dhchap })
      : shares.createNvmeofSubsystem({ nqn, serial, dhchap }),
    onSuccess: () => { onDone(); onClose(); toastSuccess(init ? "Subsystem updated" : "Subsystem created", init ? init.nqn : nqn); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title={init ? `Edit subsystem · ${init.nqn}` : "New NVMe-oF subsystem"} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || (!init && !nqn)}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">NQN</label>
        <input
          className="input"
          value={nqn}
          onChange={(e) => setNqn(e.target.value)}
          disabled={!!init}
          placeholder="nqn.2026-04.com.example:subsystem"
        />
      </div>
      <div className="field">
        <label className="field__label">Serial</label>
        <input className="input" value={serial} onChange={(e) => setSerial(e.target.value)} />
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={dhchap} onChange={(e) => setDhchap(e.target.checked)} />
          DH-CHAP required
        </label>
      </div>
    </Modal>
  );
}

type SubView = "namespaces" | "hosts";

function SubsystemDetail({ nqn, onClose }: { nqn: string; onClose: () => void }) {
  const [view, setView] = useState<SubView>("namespaces");
  return (
    <div className="side-detail">
      <div className="side-detail__head">
        <div>
          <div className="muted mono" style={{ fontSize: 10 }}>SUBSYSTEM</div>
          <div className="side-detail__title" style={{ wordBreak: "break-all" }}>{nqn}</div>
        </div>
        <button className="btn btn--sm" onClick={onClose}>
          <Icon name="close" size={10} />
        </button>
      </div>
      <div className="win-tabs">
        {(["namespaces", "hosts"] as const).map((v) => (
          <button key={v} className={view === v ? "is-on" : ""} onClick={() => setView(v)}>
            {v}
          </button>
        ))}
      </div>
      {view === "namespaces" && <NamespacesPanel nqn={nqn} />}
      {view === "hosts" && <HostsPanel nqn={nqn} />}
    </div>
  );
}

function NamespacesPanel({ nqn }: { nqn: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["nvmeof-ns", nqn],
    queryFn: () => shares.listNvmeofNamespaces(nqn),
  });
  const inval = () => qc.invalidateQueries({ queryKey: ["nvmeof-ns", nqn] });
  const delMut = useMutation({
    meta: { label: "Delete namespace failed" },
    mutationFn: (nsid: number) => shares.deleteNvmeofNamespace(nqn, nsid),
    onSuccess: () => { inval(); toastSuccess("Namespace deleted"); },
  });
  const [show, setShow] = useState(false);
  const [editNs, setEditNs] = useState<NvmeofNamespace | null>(null);
  return (
    <div className="sect">
      <div className="sect__head row gap-8">
        <div className="sect__title">Namespaces</div>
        <button className="btn btn--sm btn--primary" style={{ marginLeft: "auto" }} onClick={() => setShow(true)}>
          <Icon name="plus" size={9} />
          Add
        </button>
      </div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {q.data && q.data.length === 0 && <div className="muted">No namespaces.</div>}
        {q.data && q.data.length > 0 && (
          <table className="tbl tbl--compact">
            <thead>
              <tr>
                <th>NSID</th>
                <th>Device</th>
                <th className="num">Size</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((n) => (
                <tr key={n.nsid}>
                  <td className="mono">{n.nsid}</td>
                  <td className="mono" style={{ fontSize: 11 }}>{n.device ?? "—"}</td>
                  <td className="num mono">{n.size ? formatBytes(n.size) : "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm"
                      onClick={() => setEditNs(n)}
                    >
                      Edit
                    </button>{" "}
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={delMut.isPending}
                      onClick={() => {
                        if (window.confirm(`Delete namespace ${n.nsid}?`)) delMut.mutate(n.nsid);
                      }}
                    >
                      ×
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {show && <NamespaceModal nqn={nqn} onClose={() => setShow(false)} onDone={inval} />}
      {editNs && (
        <EditNamespaceModal
          nqn={nqn}
          ns={editNs}
          onClose={() => setEditNs(null)}
          onDone={inval}
        />
      )}
    </div>
  );
}

function EditNamespaceModal({
  nqn,
  ns,
  onClose,
  onDone,
}: {
  nqn: string;
  ns: NvmeofNamespace;
  onClose: () => void;
  onDone: () => void;
}) {
  const [device, setDevice] = useState(ns.device ?? "");
  const [uuid, setUuid] = useState(ns.uuid ?? "");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Update namespace failed" },
    mutationFn: () => shares.updateNvmeofNamespace(nqn, ns.nsid, {
      device: device || undefined,
      uuid: uuid || undefined,
    }),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Namespace updated", `nsid ${ns.nsid}`); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title={`Edit namespace ${ns.nsid}`} sub={nqn} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Device</label>
        <input className="input" value={device} onChange={(e) => setDevice(e.target.value)} />
      </div>
      <div className="field">
        <label className="field__label">UUID</label>
        <input className="input" value={uuid} onChange={(e) => setUuid(e.target.value)} />
      </div>
    </Modal>
  );
}

function NamespaceModal({
  nqn,
  onClose,
  onDone,
}: {
  nqn: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [nsid, setNsid] = useState<number | "">("");
  const [device, setDevice] = useState("");
  const [uuid, setUuid] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Add namespace failed" },
    mutationFn: () => shares.createNvmeofNamespace(nqn, {
      nsid: nsid === "" ? undefined : Number(nsid),
      device: device || undefined,
      uuid: uuid || undefined,
    }),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Namespace added", device); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Add namespace" sub={nqn} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending || !device} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Adding…" : "Add"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">NSID (optional)</label>
        <input
          className="input"
          type="number"
          value={nsid}
          onChange={(e) => setNsid(e.target.value === "" ? "" : Number(e.target.value))}
        />
      </div>
      <div className="field">
        <label className="field__label">Device</label>
        <input className="input" value={device} onChange={(e) => setDevice(e.target.value)} placeholder="/dev/zvol/tank/nvme0" />
      </div>
      <div className="field">
        <label className="field__label">UUID (optional)</label>
        <input className="input" value={uuid} onChange={(e) => setUuid(e.target.value)} />
      </div>
    </Modal>
  );
}

function HostsPanel({ nqn }: { nqn: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["nvmeof-hosts", nqn],
    queryFn: () => shares.listNvmeofHosts(nqn),
  });
  const inval = () => qc.invalidateQueries({ queryKey: ["nvmeof-hosts", nqn] });
  const delMut = useMutation({
    meta: { label: "Remove host failed" },
    mutationFn: (hostNqn: string) => shares.removeNvmeofHost(nqn, hostNqn),
    onSuccess: () => { inval(); toastSuccess("Host removed"); },
  });
  const [show, setShow] = useState(false);
  const [dhchapFor, setDhchapFor] = useState<string | null>(null);
  return (
    <div className="sect">
      <div className="sect__head row gap-8">
        <div className="sect__title">Hosts</div>
        <button className="btn btn--sm btn--primary" style={{ marginLeft: "auto" }} onClick={() => setShow(true)}>
          <Icon name="plus" size={9} />
          Add
        </button>
      </div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {q.data && q.data.length === 0 && <div className="muted">No hosts.</div>}
        {q.data && q.data.length > 0 && (
          <table className="tbl tbl--compact">
            <tbody>
              {q.data.map((h) => (
                <tr key={h.hostNqn}>
                  <td className="mono" style={{ fontSize: 11 }}>{h.hostNqn}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm"
                      onClick={() => setDhchapFor(h.hostNqn)}
                    >
                      DH-CHAP
                    </button>{" "}
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={delMut.isPending}
                      onClick={() => delMut.mutate(h.hostNqn)}
                    >
                      ×
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {show && <HostModal nqn={nqn} onClose={() => setShow(false)} onDone={inval} />}
      {dhchapFor && <DhchapModal hostNqn={dhchapFor} onClose={() => setDhchapFor(null)} />}
    </div>
  );
}

function HostModal({
  nqn,
  onClose,
  onDone,
}: {
  nqn: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [hostNqn, setHostNqn] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Add host failed" },
    mutationFn: () => shares.addNvmeofHost(nqn, hostNqn),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Host allowed", hostNqn); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Allow host" sub={nqn} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending || !hostNqn} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Adding…" : "Add"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Host NQN</label>
        <input className="input" value={hostNqn} onChange={(e) => setHostNqn(e.target.value)} placeholder="nqn.…" />
      </div>
    </Modal>
  );
}

function DhchapModal({ hostNqn, onClose }: { hostNqn: string; onClose: () => void }) {
  const qc = useQueryClient();
  const [secret, setSecret] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Set DH-CHAP failed" },
    mutationFn: () => shares.setNvmeofDhchap(hostNqn, { secret }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["nvmeof-subsystems"] });
      qc.invalidateQueries({ queryKey: ["nvmeof-hosts"] });
      onClose();
      toastSuccess("DH-CHAP secret set", hostNqn);
    },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Set DH-CHAP secret" sub={hostNqn} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending || !secret} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">DH-CHAP secret</label>
        <input className="input" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} />
      </div>
    </Modal>
  );
}

function PortsView() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["nvmeof-ports"], queryFn: () => shares.listNvmeofPorts() });
  const subQ = useQuery({
    queryKey: ["nvmeof-subsystems"],
    queryFn: () => shares.listNvmeofSubsystems(),
  });
  const ports = q.data ?? [];
  const subs = subQ.data ?? [];
  const [edit, setEdit] = useState<NvmeofPort | "new" | null>(null);
  const [bindFor, setBindFor] = useState<string | null>(null);
  const [unbindFor, setUnbindFor] = useState<string | null>(null);

  const inval = () => qc.invalidateQueries({ queryKey: ["nvmeof-ports"] });
  const delMut = useMutation({
    meta: { label: "Delete port failed" },
    mutationFn: (id: string) => shares.deleteNvmeofPort(id),
    onSuccess: () => { inval(); toastSuccess("Port deleted"); },
  });
  const unbindMut = useMutation({
    meta: { label: "Unbind failed" },
    mutationFn: ({ portId, nqn }: { portId: string; nqn: string }) =>
      shares.unbindNvmeofPort(portId, nqn),
    onSuccess: () => { inval(); toastSuccess("Subsystem unbound"); },
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setEdit("new")}>
          <Icon name="plus" size={11} />
          New port
        </button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading ports…</div>}
      {q.data && ports.length === 0 && <div className="empty-hint">No ports.</div>}
      {ports.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>ID</th>
              <th>Type</th>
              <th>Address</th>
              <th>Service</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {ports.map((p) => (
              <tr key={p.id}>
                <td className="mono">{p.id}</td>
                <td className="mono">{p.trtype ?? "—"}</td>
                <td className="mono" style={{ fontSize: 11 }}>{p.traddr ?? "—"}</td>
                <td className="mono">{p.trsvcid ?? "—"}</td>
                <td className="num">
                  <button className="btn btn--sm" onClick={() => setEdit(p)}>Edit</button>{" "}
                  <button className="btn btn--sm" onClick={() => setBindFor(p.id)}>Bind</button>{" "}
                  <button
                    className="btn btn--sm"
                    onClick={() => setUnbindFor(p.id)}
                    disabled={subs.length === 0}
                  >
                    Unbind
                  </button>{" "}
                  <button
                    className="btn btn--sm btn--danger"
                    disabled={delMut.isPending}
                    onClick={() => {
                      if (window.confirm(`Delete port ${p.id}?`)) delMut.mutate(p.id);
                    }}
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {edit && (
        <PortModal
          init={edit === "new" ? null : edit}
          onClose={() => setEdit(null)}
          onDone={inval}
        />
      )}
      {bindFor && (
        <BindPortModal
          portId={bindFor}
          subsystems={subs.map((s) => s.nqn)}
          onClose={() => setBindFor(null)}
        />
      )}
      {unbindFor && (
        <UnbindPortModal
          portId={unbindFor}
          subsystems={subs.map((s) => s.nqn)}
          onClose={() => setUnbindFor(null)}
          onSubmit={(nqn) => unbindMut.mutate({ portId: unbindFor, nqn })}
          pending={unbindMut.isPending}
        />
      )}
    </div>
  );
}

function UnbindPortModal({
  portId,
  subsystems,
  onClose,
  onSubmit,
  pending,
}: {
  portId: string;
  subsystems: string[];
  onClose: () => void;
  onSubmit: (nqn: string) => void;
  pending: boolean;
}) {
  const [nqn, setNqn] = useState(subsystems[0] ?? "");
  return (
    <Modal title="Unbind subsystem from port" sub={`port ${portId}`} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--danger"
            disabled={pending || !nqn}
            onClick={() => { onSubmit(nqn); onClose(); }}
          >
            {pending ? "Unbinding…" : "Unbind"}
          </button>
        </>
      }
    >
      <div className="field">
        <label className="field__label">Subsystem</label>
        <select className="input" value={nqn} onChange={(e) => setNqn(e.target.value)}>
          {subsystems.length === 0 && <option value="">— no subsystems —</option>}
          {subsystems.map((s) => <option key={s} value={s}>{s}</option>)}
        </select>
      </div>
    </Modal>
  );
}

function PortModal({
  init,
  onClose,
  onDone,
}: {
  init: NvmeofPort | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [trtype, setTrtype] = useState(init?.trtype ?? "tcp");
  const [traddr, setTraddr] = useState(init?.traddr ?? "");
  const [trsvcid, setTrsvcid] = useState(init?.trsvcid ?? "4420");
  const [adrfam, setAdrfam] = useState(init?.adrfam ?? "ipv4");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Save port failed" },
    mutationFn: () => init
      ? shares.updateNvmeofPort(init.id, { trtype, traddr, trsvcid, adrfam })
      : shares.createNvmeofPort({ trtype, traddr, trsvcid, adrfam }),
    onSuccess: () => { onDone(); onClose(); toastSuccess(init ? "Port updated" : "Port created"); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title={init ? `Edit port · ${init.id}` : "New NVMe-oF port"} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Transport</label>
        <select className="input" value={trtype} onChange={(e) => setTrtype(e.target.value)}>
          <option value="tcp">tcp</option>
          <option value="rdma">rdma</option>
          <option value="fc">fc</option>
        </select>
      </div>
      <div className="field">
        <label className="field__label">Address family</label>
        <select className="input" value={adrfam} onChange={(e) => setAdrfam(e.target.value)}>
          <option value="ipv4">ipv4</option>
          <option value="ipv6">ipv6</option>
        </select>
      </div>
      <div className="field">
        <label className="field__label">Address</label>
        <input className="input" value={traddr} onChange={(e) => setTraddr(e.target.value)} placeholder="0.0.0.0" />
      </div>
      <div className="field">
        <label className="field__label">Service ID (port)</label>
        <input className="input" value={trsvcid} onChange={(e) => setTrsvcid(e.target.value)} />
      </div>
    </Modal>
  );
}

function BindPortModal({
  portId,
  subsystems,
  onClose,
}: {
  portId: string;
  subsystems: string[];
  onClose: () => void;
}) {
  const [nqn, setNqn] = useState(subsystems[0] ?? "");
  const [err, setErr] = useState<string | null>(null);
  const qc = useQueryClient();
  const m = useMutation({
    meta: { label: "Bind failed" },
    mutationFn: () => shares.bindNvmeofPort(portId, nqn),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["nvmeof-ports"] });
      qc.invalidateQueries({ queryKey: ["nvmeof-subsystems"] });
      onClose();
      toastSuccess("Subsystem bound", nqn);
    },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Bind subsystem to port" sub={`port ${portId}`} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending || !nqn} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Binding…" : "Bind"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Subsystem</label>
        <select className="input" value={nqn} onChange={(e) => setNqn(e.target.value)}>
          {subsystems.length === 0 && <option value="">— no subsystems —</option>}
          {subsystems.map((s) => <option key={s} value={s}>{s}</option>)}
        </select>
      </div>
    </Modal>
  );
}

export default NVMEOF;
