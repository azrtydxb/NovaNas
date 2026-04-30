import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import { shares, type SmbShare, type SmbUser } from "../../api/shares";
import { toastSuccess } from "../../store/toast";
import { Modal } from "./Modal";

export function SMB() {
  return <SharesView />;
}

function SharesView() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["smb-shares"], queryFn: () => shares.listSmb() });
  const list = q.data ?? [];

  const [edit, setEdit] = useState<SmbShare | "new" | null>(null);
  const [showGlobals, setShowGlobals] = useState(false);
  const [showUsers, setShowUsers] = useState(false);

  const inval = () => qc.invalidateQueries({ queryKey: ["smb-shares"] });
  const delMut = useMutation({
    meta: { label: "Delete share failed" },
    mutationFn: (n: string) => shares.deleteSmb(n),
    onSuccess: (_d, n) => { inval(); toastSuccess("SMB share deleted", n); },
  });
  const reloadMut = useMutation({
    meta: { label: "Reload failed" },
    mutationFn: () => shares.smbReload(),
    onSuccess: () => toastSuccess("Samba reloaded"),
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setEdit("new")}>
          <Icon name="plus" size={11} />
          New SMB share
        </button>
        <button className="btn" onClick={() => setShowGlobals(true)}>Globals…</button>
        <button className="btn" onClick={() => setShowUsers(true)}>Users…</button>
        <button
          className="btn"
          disabled={reloadMut.isPending}
          onClick={() => reloadMut.mutate()}
          style={{ marginLeft: "auto" }}
        >
          <Icon name="refresh" size={11} />
          Reload
        </button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading SMB shares…</div>}
      {q.isError && (
        <div className="empty-hint" style={{ color: "var(--err)" }}>
          Failed: {(q.error as Error).message}
        </div>
      )}
      {q.data && list.length === 0 && <div className="empty-hint">No SMB shares.</div>}
      {list.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Share</th>
              <th>Path</th>
              <th>Users</th>
              <th>Guest</th>
              <th>Recycle</th>
              <th>VFS</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.map((s) => (
              <tr key={s.name}>
                <td>{s.name}</td>
                <td className="mono muted" style={{ fontSize: 11 }}>{s.path ?? "—"}</td>
                <td className="mono">{s.users ?? "—"}</td>
                <td>{s.guest ? <Icon name="check" size={11} /> : <span className="muted">no</span>}</td>
                <td>{s.recycle ? <Icon name="check" size={11} /> : <span className="muted">no</span>}</td>
                <td className="mono muted" style={{ fontSize: 11 }}>{s.vfs ?? "—"}</td>
                <td className="num">
                  <button className="btn btn--sm" onClick={() => setEdit(s)}>Edit</button>{" "}
                  <button
                    className="btn btn--sm btn--danger"
                    disabled={delMut.isPending}
                    onClick={() => {
                      if (window.confirm(`Delete SMB share ${s.name}?`)) delMut.mutate(s.name);
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
        <SmbShareModal
          init={edit === "new" ? null : edit}
          onClose={() => setEdit(null)}
          onDone={inval}
        />
      )}
      {showGlobals && (
        <Modal
          title="Samba global settings"
          onClose={() => setShowGlobals(false)}
          footer={<button className="btn" onClick={() => setShowGlobals(false)}>Close</button>}
        >
          <GlobalsView />
        </Modal>
      )}
      {showUsers && (
        <Modal
          title="SMB users"
          onClose={() => setShowUsers(false)}
          footer={<button className="btn" onClick={() => setShowUsers(false)}>Close</button>}
        >
          <UsersView />
        </Modal>
      )}
    </div>
  );
}

function SmbShareModal({
  init,
  onClose,
  onDone,
}: {
  init: SmbShare | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(init?.name ?? "");
  const [path, setPath] = useState(init?.path ?? "");
  const [users, setUsers] = useState(init?.users ?? "");
  const [guest, setGuest] = useState(init?.guest ?? false);
  const [recycle, setRecycle] = useState(init?.recycle ?? false);
  const [vfs, setVfs] = useState(init?.vfs ?? "");
  const [comment, setComment] = useState(init?.comment ?? "");
  const [readOnly, setReadOnly] = useState(init?.readOnly ?? false);
  const [err, setErr] = useState<string | null>(null);

  const body = (): Partial<SmbShare> => ({
    name, path, users, guest, recycle, vfs, comment, readOnly,
  });

  const m = useMutation({
    meta: { label: "Save share failed" },
    mutationFn: () => init
      ? shares.updateSmb(init.name, body())
      : shares.createSmb(body()),
    onSuccess: () => { onDone(); onClose(); toastSuccess(init ? "SMB share updated" : "SMB share created", name); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal title={init ? `Edit SMB share · ${init.name}` : "New SMB share"} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !name || !path}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Name</label>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} disabled={!!init} />
      </div>
      <div className="field">
        <label className="field__label">Path</label>
        <input className="input" value={path} onChange={(e) => setPath(e.target.value)} placeholder="/tank/share" />
      </div>
      <div className="field">
        <label className="field__label">Users (comma list)</label>
        <input className="input" value={users} onChange={(e) => setUsers(e.target.value)} placeholder="alice, bob" />
      </div>
      <div className="field">
        <label className="field__label">Comment</label>
        <input className="input" value={comment} onChange={(e) => setComment(e.target.value)} />
      </div>
      <div className="field">
        <label className="field__label">VFS modules</label>
        <input className="input" value={vfs} onChange={(e) => setVfs(e.target.value)} placeholder="catia,fruit,streams_xattr" />
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={guest} onChange={(e) => setGuest(e.target.checked)} />
          Allow guest
        </label>
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={recycle} onChange={(e) => setRecycle(e.target.checked)} />
          Recycle bin
        </label>
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={readOnly} onChange={(e) => setReadOnly(e.target.checked)} />
          Read-only
        </label>
      </div>
    </Modal>
  );
}

function UsersView() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["smb-users"], queryFn: () => shares.listSmbUsers() });
  const list = q.data ?? [];
  const [edit, setEdit] = useState<SmbUser | "new" | null>(null);
  const [pwFor, setPwFor] = useState<string | null>(null);

  const inval = () => qc.invalidateQueries({ queryKey: ["smb-users"] });
  const delMut = useMutation({
    meta: { label: "Delete user failed" },
    mutationFn: (u: string) => shares.deleteSmbUser(u),
    onSuccess: (_d, u) => { inval(); toastSuccess("User deleted", u); },
  });

  return (
    <div style={{ padding: 14 }}>
      <div className="tbar">
        <button className="btn btn--primary" onClick={() => setEdit("new")}>
          <Icon name="plus" size={11} />
          New user
        </button>
      </div>
      {q.isLoading && <div className="empty-hint">Loading users…</div>}
      {q.data && list.length === 0 && <div className="empty-hint">No SMB users.</div>}
      {list.length > 0 && (
        <table className="tbl">
          <thead>
            <tr>
              <th>Username</th>
              <th>Full name</th>
              <th>Enabled</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.map((u) => (
              <tr key={u.username}>
                <td className="mono">{u.username}</td>
                <td>{u.fullname ?? "—"}</td>
                <td>{u.enabled ? <Icon name="check" size={11} /> : <span className="muted">no</span>}</td>
                <td className="num">
                  <button className="btn btn--sm" onClick={() => setEdit(u)}>Edit</button>{" "}
                  <button className="btn btn--sm" onClick={() => setPwFor(u.username)}>Password</button>{" "}
                  <button
                    className="btn btn--sm btn--danger"
                    disabled={delMut.isPending}
                    onClick={() => {
                      if (window.confirm(`Delete user ${u.username}?`)) delMut.mutate(u.username);
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
        <SmbUserModal
          init={edit === "new" ? null : edit}
          onClose={() => setEdit(null)}
          onDone={inval}
        />
      )}
      {pwFor && (
        <SmbPasswordModal username={pwFor} onClose={() => setPwFor(null)} />
      )}
    </div>
  );
}

function SmbUserModal({
  init,
  onClose,
  onDone,
}: {
  init: SmbUser | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [username, setUsername] = useState(init?.username ?? "");
  const [fullname, setFullname] = useState(init?.fullname ?? "");
  const [enabled, setEnabled] = useState(init?.enabled ?? true);
  const [password, setPassword] = useState("");
  const [err, setErr] = useState<string | null>(null);

  const m = useMutation({
    meta: { label: "Save user failed" },
    mutationFn: () => init
      ? shares.updateSmbUser(init.username, { fullname, enabled })
      : shares.createSmbUser({ username, fullname, enabled, password: password || undefined }),
    onSuccess: () => { onDone(); onClose(); toastSuccess(init ? "User updated" : "User created", username); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <Modal title={init ? `Edit user · ${init.username}` : "New SMB user"} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !username}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Saving…" : "Save"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Username</label>
        <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} disabled={!!init} />
      </div>
      <div className="field">
        <label className="field__label">Full name</label>
        <input className="input" value={fullname} onChange={(e) => setFullname(e.target.value)} />
      </div>
      {!init && (
        <div className="field">
          <label className="field__label">Initial password</label>
          <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
      )}
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          Enabled
        </label>
      </div>
    </Modal>
  );
}

function SmbPasswordModal({ username, onClose }: { username: string; onClose: () => void }) {
  const [pw, setPw] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Set password failed" },
    mutationFn: () => shares.setSmbUserPassword(username, pw),
    onSuccess: () => { onClose(); toastSuccess("Password updated", username); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title={`Set password · ${username}`} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !pw}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Setting…" : "Set"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">New password</label>
        <input className="input" type="password" value={pw} onChange={(e) => setPw(e.target.value)} />
      </div>
    </Modal>
  );
}

function GlobalsView() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["smb-globals"], queryFn: () => shares.getSmbGlobals() });
  const [draft, setDraft] = useState<Record<string, string> | null>(null);
  const data = q.data ?? {};
  const editing = draft !== null;
  const view = draft ?? data;

  const m = useMutation({
    meta: { label: "Save globals failed" },
    mutationFn: (body: Record<string, string>) => shares.updateSmbGlobals(body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["smb-globals"] });
      toastSuccess("Samba globals saved");
      setDraft(null);
    },
  });

  const setKey = (k: string, v: string) =>
    setDraft((d) => ({ ...(d ?? data), [k]: v }));
  const removeKey = (k: string) =>
    setDraft((d) => {
      const next = { ...(d ?? data) };
      delete next[k];
      return next;
    });
  const [newK, setNewK] = useState("");
  const [newV, setNewV] = useState("");

  return (
    <div style={{ padding: 14 }}>
      <div className="sect">
        <div className="sect__title">Samba global settings</div>
        <div className="sect__body">
          {q.isLoading && <div className="muted">Loading…</div>}
          {q.isError && (
            <div className="muted" style={{ color: "var(--err)" }}>
              {(q.error as Error).message}
            </div>
          )}
          {Object.keys(view).length === 0 && !q.isLoading && (
            <div className="muted">No globals returned.</div>
          )}
          {Object.keys(view).length > 0 && (
            <table className="tbl tbl--compact">
              <tbody>
                {Object.entries(view).map(([k, v]) => (
                  <tr key={k}>
                    <td className="mono" style={{ fontSize: 11 }}>{k}</td>
                    <td className="mono" style={{ fontSize: 11 }}>
                      {editing ? (
                        <input
                          className="input"
                          value={String(v)}
                          onChange={(e) => setKey(k, e.target.value)}
                        />
                      ) : (
                        <span className="muted">{String(v)}</span>
                      )}
                    </td>
                    {editing && (
                      <td className="num">
                        <button className="btn btn--sm" onClick={() => removeKey(k)}>
                          <Icon name="trash" size={11} />
                        </button>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {editing && (
            <div className="row gap-8" style={{ marginTop: 8 }}>
              <input
                className="input"
                placeholder="key (e.g. workgroup)"
                value={newK}
                onChange={(e) => setNewK(e.target.value)}
                style={{ flex: 1 }}
              />
              <input
                className="input"
                placeholder="value"
                value={newV}
                onChange={(e) => setNewV(e.target.value)}
                style={{ flex: 1 }}
              />
              <button
                className="btn btn--sm"
                disabled={!newK.trim()}
                onClick={() => {
                  setKey(newK.trim(), newV);
                  setNewK("");
                  setNewV("");
                }}
              >
                <Icon name="plus" size={11} />
              </button>
            </div>
          )}
          <div className="row gap-8" style={{ marginTop: 12 }}>
            {!editing && (
              <button className="btn btn--sm" onClick={() => setDraft({ ...data })}>
                <Icon name="edit" size={11} /> Edit globals
              </button>
            )}
            {editing && (
              <>
                <button className="btn btn--sm" onClick={() => setDraft(null)} disabled={m.isPending}>
                  Cancel
                </button>
                <button
                  className="btn btn--sm btn--primary"
                  disabled={m.isPending}
                  onClick={() => draft && m.mutate(draft)}
                >
                  {m.isPending ? "Saving…" : "Save"}
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export default SMB;
