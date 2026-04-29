import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares, type IscsiTarget } from "../../api/shares";
import { formatBytes } from "../../lib/format";
import { Modal } from "./Modal";

export function ISCSI() {
  const qc = useQueryClient();
  const [sel, setSel] = useState<string | null>(null);
  const [edit, setEdit] = useState<IscsiTarget | "new" | null>(null);
  const q = useQuery({ queryKey: ["iscsi-targets"], queryFn: () => shares.listIscsi() });
  const list = q.data ?? [];
  const cur = list.find((t) => t.iqn === sel);

  const inval = () => qc.invalidateQueries({ queryKey: ["iscsi-targets"] });
  const delMut = useMutation({
    mutationFn: (iqn: string) => shares.deleteIscsi(iqn),
    onSuccess: inval,
  });
  const saveMut = useMutation({ mutationFn: () => shares.iscsiSaveConfig() });

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
            New target
          </button>
          <button className="btn" disabled={saveMut.isPending} onClick={() => saveMut.mutate()}>
            <Icon name="download" size={11} />
            Save config
          </button>
        </div>
        {q.isLoading && <div className="empty-hint">Loading targets…</div>}
        {q.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(q.error as Error).message}
          </div>
        )}
        {q.data && list.length === 0 && <div className="empty-hint">No iSCSI targets.</div>}
        {list.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>IQN</th>
                <th className="num">LUNs</th>
                <th>Portals</th>
                <th className="num">ACLs</th>
                <th>State</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {list.map((t) => (
                <tr
                  key={t.iqn}
                  className={sel === t.iqn ? "is-on" : ""}
                  onClick={() => setSel(t.iqn)}
                  style={{ cursor: "pointer" }}
                >
                  <td className="mono" style={{ fontSize: 11 }}>{t.iqn}</td>
                  <td className="num mono">{t.luns ?? 0}</td>
                  <td className="mono muted" style={{ fontSize: 11 }}>
                    {(t.portals ?? []).join(", ")}
                  </td>
                  <td className="num mono">{t.acls ?? 0}</td>
                  <td>
                    <span className="pill pill--ok">
                      <span className="dot" />
                      {t.state ?? "up"}
                    </span>
                  </td>
                  <td className="num" onClick={(e) => e.stopPropagation()}>
                    <button className="btn btn--sm" onClick={() => setEdit(t)}>Edit</button>{" "}
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={delMut.isPending}
                      onClick={() => {
                        if (window.confirm(`Delete target ${t.iqn}?`)) delMut.mutate(t.iqn);
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
      {cur && <TargetDetail iqn={cur.iqn} onClose={() => setSel(null)} />}
      {edit && (
        <IscsiTargetModal
          init={edit === "new" ? null : edit}
          onClose={() => setEdit(null)}
          onDone={inval}
        />
      )}
    </div>
  );
}

function IscsiTargetModal({
  init,
  onClose,
  onDone,
}: {
  init: IscsiTarget | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [iqn, setIqn] = useState(init?.iqn ?? "");
  const [alias, setAlias] = useState(init?.alias ?? "");
  const [err, setErr] = useState<string | null>(null);

  const m = useMutation({
    mutationFn: () => init
      ? shares.updateIscsi(init.iqn, { alias })
      : shares.createIscsi({ iqn, alias }),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal title={init ? `Edit target · ${init.iqn}` : "New iSCSI target"} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || (!init && !iqn)}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">IQN</label>
        <input
          className="input"
          value={iqn}
          onChange={(e) => setIqn(e.target.value)}
          disabled={!!init}
          placeholder="iqn.2026-04.com.example:storage.target0"
        />
      </div>
      <div className="field">
        <label className="field__label">Alias</label>
        <input className="input" value={alias} onChange={(e) => setAlias(e.target.value)} />
      </div>
    </Modal>
  );
}

type SubView = "luns" | "portals" | "acls";

function TargetDetail({ iqn, onClose }: { iqn: string; onClose: () => void }) {
  const [view, setView] = useState<SubView>("luns");
  return (
    <div className="side-detail">
      <div className="side-detail__head">
        <div>
          <div className="muted mono" style={{ fontSize: 10 }}>ISCSI TARGET</div>
          <div className="side-detail__title" style={{ wordBreak: "break-all" }}>{iqn}</div>
        </div>
        <button className="btn btn--sm" onClick={onClose}>
          <Icon name="close" size={10} />
        </button>
      </div>
      <div className="win-tabs">
        {(["luns", "portals", "acls"] as const).map((v) => (
          <button key={v} className={view === v ? "is-on" : ""} onClick={() => setView(v)}>
            {v}
          </button>
        ))}
      </div>
      {view === "luns" && <LunsPanel iqn={iqn} />}
      {view === "portals" && <PortalsPanel iqn={iqn} />}
      {view === "acls" && <AclsPanel iqn={iqn} />}
    </div>
  );
}

function LunsPanel({ iqn }: { iqn: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["iscsi-luns", iqn],
    queryFn: () => shares.listIscsiLuns(iqn),
  });
  const inval = () => qc.invalidateQueries({ queryKey: ["iscsi-luns", iqn] });
  const delMut = useMutation({
    mutationFn: (id: string) => shares.deleteIscsiLun(iqn, id),
    onSuccess: inval,
  });
  const [show, setShow] = useState(false);

  return (
    <div className="sect">
      <div className="sect__head row gap-8">
        <div className="sect__title">LUNs</div>
        <button className="btn btn--sm btn--primary" style={{ marginLeft: "auto" }} onClick={() => setShow(true)}>
          <Icon name="plus" size={9} />
          Add
        </button>
      </div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {q.data && q.data.length === 0 && <div className="muted">No LUNs.</div>}
        {q.data && q.data.length > 0 && (
          <table className="tbl tbl--compact">
            <thead>
              <tr>
                <th>LUN</th>
                <th>Backing</th>
                <th className="num">Size</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((l) => (
                <tr key={l.id}>
                  <td className="mono">{l.lun ?? "—"}</td>
                  <td className="mono" style={{ fontSize: 11 }}>{l.backing ?? "—"}</td>
                  <td className="num mono">{l.size ? formatBytes(l.size) : "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={delMut.isPending}
                      onClick={() => delMut.mutate(l.id)}
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
      {show && <LunModal iqn={iqn} onClose={() => setShow(false)} onDone={inval} />}
    </div>
  );
}

function LunModal({
  iqn,
  onClose,
  onDone,
}: {
  iqn: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [lun, setLun] = useState<number | "">("");
  const [backing, setBacking] = useState("");
  const [size, setSize] = useState<number | "">("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: () => shares.createIscsiLun(iqn, {
      lun: lun === "" ? undefined : Number(lun),
      backing: backing || undefined,
      size: size === "" ? undefined : Number(size),
    }),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Add LUN" sub={iqn} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Adding…" : "Add"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">LUN number</label>
        <input
          className="input"
          type="number"
          value={lun}
          onChange={(e) => setLun(e.target.value === "" ? "" : Number(e.target.value))}
        />
      </div>
      <div className="field">
        <label className="field__label">Backing path</label>
        <input className="input" value={backing} onChange={(e) => setBacking(e.target.value)} placeholder="/dev/zvol/tank/iscsi0" />
      </div>
      <div className="field">
        <label className="field__label">Size (bytes, optional)</label>
        <input
          className="input"
          type="number"
          value={size}
          onChange={(e) => setSize(e.target.value === "" ? "" : Number(e.target.value))}
        />
      </div>
    </Modal>
  );
}

function PortalsPanel({ iqn }: { iqn: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["iscsi-portals", iqn],
    queryFn: () => shares.listIscsiPortals(iqn),
  });
  const inval = () => qc.invalidateQueries({ queryKey: ["iscsi-portals", iqn] });
  const delMut = useMutation({
    mutationFn: ({ ip, port }: { ip: string; port: number }) =>
      shares.deleteIscsiPortal(iqn, ip, port),
    onSuccess: inval,
  });
  const [show, setShow] = useState(false);
  return (
    <div className="sect">
      <div className="sect__head row gap-8">
        <div className="sect__title">Portals</div>
        <button className="btn btn--sm btn--primary" style={{ marginLeft: "auto" }} onClick={() => setShow(true)}>
          <Icon name="plus" size={9} />
          Add
        </button>
      </div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {q.data && q.data.length === 0 && <div className="muted">No portals.</div>}
        {q.data && q.data.length > 0 && (
          <table className="tbl tbl--compact">
            <tbody>
              {q.data.map((p, i) => (
                <tr key={p.id ?? i}>
                  <td className="mono">{p.ip ?? "—"}</td>
                  <td className="mono">{p.port ?? "—"}</td>
                  <td className="muted mono">tag {p.tag ?? "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={delMut.isPending || !p.ip || p.port == null}
                      onClick={() => p.ip && p.port != null && delMut.mutate({ ip: p.ip, port: p.port })}
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
      {show && <PortalModal iqn={iqn} onClose={() => setShow(false)} onDone={inval} />}
    </div>
  );
}

function PortalModal({
  iqn,
  onClose,
  onDone,
}: {
  iqn: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [ip, setIp] = useState("");
  const [port, setPort] = useState<number | "">(3260);
  const [tag, setTag] = useState<number | "">(1);
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: () => shares.createIscsiPortal(iqn, {
      ip,
      port: port === "" ? undefined : Number(port),
      tag: tag === "" ? undefined : Number(tag),
    }),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Add portal" sub={iqn} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending || !ip} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Adding…" : "Add"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">IP</label>
        <input className="input" value={ip} onChange={(e) => setIp(e.target.value)} placeholder="0.0.0.0" />
      </div>
      <div className="field">
        <label className="field__label">Port</label>
        <input
          className="input"
          type="number"
          value={port}
          onChange={(e) => setPort(e.target.value === "" ? "" : Number(e.target.value))}
        />
      </div>
      <div className="field">
        <label className="field__label">Portal group tag</label>
        <input
          className="input"
          type="number"
          value={tag}
          onChange={(e) => setTag(e.target.value === "" ? "" : Number(e.target.value))}
        />
      </div>
    </Modal>
  );
}

function AclsPanel({ iqn }: { iqn: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["iscsi-acls", iqn],
    queryFn: () => shares.listIscsiAcls(iqn),
  });
  const inval = () => qc.invalidateQueries({ queryKey: ["iscsi-acls", iqn] });
  const delMut = useMutation({
    mutationFn: (initiator: string) => shares.deleteIscsiAcl(iqn, initiator),
    onSuccess: inval,
  });
  const [show, setShow] = useState(false);

  return (
    <div className="sect">
      <div className="sect__head row gap-8">
        <div className="sect__title">ACLs</div>
        <button className="btn btn--sm btn--primary" style={{ marginLeft: "auto" }} onClick={() => setShow(true)}>
          <Icon name="plus" size={9} />
          Add
        </button>
      </div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {q.data && q.data.length === 0 && <div className="muted">No ACLs.</div>}
        {q.data && q.data.length > 0 && (
          <table className="tbl tbl--compact">
            <tbody>
              {q.data.map((a, i) => (
                <tr key={i}>
                  <td className="mono" style={{ fontSize: 11 }}>{a.initiator}</td>
                  <td className="muted">{a.user ?? "—"}</td>
                  <td className="muted">{a.authMethod ?? "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={delMut.isPending}
                      onClick={() => delMut.mutate(a.initiator)}
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
      {show && <AclModal iqn={iqn} onClose={() => setShow(false)} onDone={inval} />}
    </div>
  );
}

function AclModal({
  iqn,
  onClose,
  onDone,
}: {
  iqn: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [initiator, setInitiator] = useState("");
  const [user, setUser] = useState("");
  const [authMethod, setAuthMethod] = useState("CHAP");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: () => shares.createIscsiAcl(iqn, {
      initiator,
      user: user || undefined,
      authMethod,
    }),
    onSuccess: () => { onDone(); onClose(); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Add ACL" sub={iqn} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending || !initiator} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Adding…" : "Add"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Initiator IQN</label>
        <input className="input" value={initiator} onChange={(e) => setInitiator(e.target.value)} placeholder="iqn.…" />
      </div>
      <div className="field">
        <label className="field__label">User</label>
        <input className="input" value={user} onChange={(e) => setUser(e.target.value)} />
      </div>
      <div className="field">
        <label className="field__label">Auth method</label>
        <select className="input" value={authMethod} onChange={(e) => setAuthMethod(e.target.value)}>
          <option value="CHAP">CHAP</option>
          <option value="None">None</option>
        </select>
      </div>
    </Modal>
  );
}

export default ISCSI;
