import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Icon } from "../../components/Icon";
import {
  storage,
  type Dataset,
  type DatasetMetadata,
  type Snapshot,
} from "../../api/storage";
import { formatBytes } from "../../lib/format";
import { toastSuccess } from "../../store/toast";
import { Modal } from "./Modal";

function dsKey(d: Dataset): string {
  return d.fullname ?? d.name;
}

type TreeNodeKind = "dataset" | "snapshot";

type TreeNode = {
  kind: TreeNodeKind;
  // Display name. For snapshots we prefix with ".snap@" so they're
  // unmistakable in a list of dataset paths.
  display: string;
  fullname: string;    // full ZFS path: "tank/csi/pvc" or "tank/csi/pvc@snap"
  depth: number;
  dataset?: Dataset;   // for dataset nodes (or synthetic parents)
  snapshot?: Snapshot; // for snapshot nodes
  children: TreeNode[];
};

// snapshotKey returns the part of a snapshot's identifier after the
// "@". Backends may give us either the bare snap name or the full
// dataset@snap form; normalize.
function snapshotKey(s: Snapshot): string {
  const full = s.fullname ?? s.name;
  const at = full.indexOf("@");
  return at >= 0 ? full.slice(at + 1) : full;
}

// snapshotParent returns the dataset name a snapshot belongs to.
function snapshotParent(s: Snapshot): string | null {
  if (s.dataset) return s.dataset;
  const full = s.fullname ?? s.name;
  const at = full.indexOf("@");
  return at >= 0 ? full.slice(0, at) : null;
}

// buildTree groups datasets by their slash-separated path. Snapshots
// are attached as leaf children under their parent dataset node.
// Synthetic nodes are inserted for any missing intermediate path.
function buildTree(datasets: Dataset[], snapshots: Snapshot[]): TreeNode[] {
  const byPath = new Map<string, TreeNode>();
  const roots: TreeNode[] = [];
  const sorted = [...datasets].sort((a, b) =>
    (a.fullname ?? a.name).localeCompare(b.fullname ?? b.name)
  );
  for (const d of sorted) {
    const path = (d.fullname ?? d.name).split("/");
    for (let i = 0; i < path.length; i++) {
      const fullname = path.slice(0, i + 1).join("/");
      if (byPath.has(fullname)) {
        if (i === path.length - 1) byPath.get(fullname)!.dataset = d;
        continue;
      }
      const node: TreeNode = {
        kind: "dataset",
        display: path[i],
        fullname,
        depth: i,
        dataset: i === path.length - 1 ? d : undefined,
        children: [],
      };
      byPath.set(fullname, node);
      if (i === 0) roots.push(node);
      else byPath.get(path.slice(0, i).join("/"))?.children.push(node);
    }
  }
  // Attach snapshots under their parent dataset, sorted newest-first
  // (created descending, fall back to name compare).
  const snapsByParent = new Map<string, Snapshot[]>();
  for (const s of snapshots) {
    const parent = snapshotParent(s);
    if (!parent) continue;
    const arr = snapsByParent.get(parent) ?? [];
    arr.push(s);
    snapsByParent.set(parent, arr);
  }
  for (const [parent, snaps] of snapsByParent) {
    const node = byPath.get(parent);
    if (!node) continue;
    snaps.sort((a, b) => {
      const ta = a.created ? Date.parse(a.created) : 0;
      const tb = b.created ? Date.parse(b.created) : 0;
      if (ta && tb && ta !== tb) return tb - ta;
      return snapshotKey(a).localeCompare(snapshotKey(b));
    });
    for (const s of snaps) {
      node.children.push({
        kind: "snapshot",
        display: ".snap@" + snapshotKey(s),
        fullname: s.fullname ?? `${parent}@${snapshotKey(s)}`,
        depth: node.depth + 1,
        snapshot: s,
        children: [],
      });
    }
  }
  return roots;
}

// flattenTree walks the tree in display order, skipping branches whose
// ancestor is collapsed.
function flattenTree(roots: TreeNode[], expanded: Set<string>): TreeNode[] {
  const out: TreeNode[] = [];
  const walk = (nodes: TreeNode[]) => {
    for (const n of nodes) {
      out.push(n);
      if (n.children.length > 0 && expanded.has(n.fullname)) walk(n.children);
    }
  };
  walk(roots);
  return out;
}

type ActionKind =
  | "rollback"
  | "clone"
  | "promote"
  | "rename"
  | "send"
  | "receive"
  | null;

export function DatasetsTab() {
  const [sel, setSel] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const q = useQuery({ queryKey: ["datasets"], queryFn: () => storage.listDatasets() });
  const datasets = q.data ?? [];
  const snapsQ = useQuery({
    queryKey: ["snapshots", "all"],
    queryFn: () => storage.listSnapshots(),
  });
  const snapshots = snapsQ.data ?? [];

  // Tree state. Top-level pool roots are expanded by default so
  // first-level children are visible without an extra click.
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const tree = buildTree(datasets, snapshots);
  // Auto-expand pool roots on first load.
  useEffect(() => {
    if (datasets.length === 0) return;
    setExpanded((prev) => {
      if (prev.size > 0) return prev;
      const next = new Set<string>();
      for (const root of tree) next.add(root.fullname);
      return next;
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [datasets.length]);
  const flat = flattenTree(tree, expanded);
  const toggle = (full: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(full)) next.delete(full);
      else next.add(full);
      return next;
    });
  };

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: sel ? "1fr 360px" : "1fr",
        height: "100%",
      }}
    >
      <div style={{ overflow: "auto", padding: 14 }}>
        <div className="tbar">
          <button className="btn btn--primary" onClick={() => setShowCreate(true)}>
            <Icon name="plus" size={11} />
            New dataset
          </button>
        </div>
        {showCreate && <CreateDatasetModal onClose={() => setShowCreate(false)} />}
        {q.isLoading && <div className="empty-hint">Loading datasets…</div>}
        {q.isError && (
          <div className="empty-hint" style={{ color: "var(--err)" }}>
            Failed: {(q.error as Error).message}
          </div>
        )}
        {q.data && datasets.length === 0 && <div className="empty-hint">No datasets.</div>}
        {datasets.length > 0 && (
          <table className="tbl">
            <thead>
              <tr>
                <th>Dataset</th>
                <th>Protocol</th>
                <th className="num">Used</th>
                <th>Quota</th>
                <th className="num">Snaps</th>
                <th>Comp</th>
                <th>Enc</th>
              </tr>
            </thead>
            <tbody>
              {flat.map((node) => {
                const isSnap = node.kind === "snapshot";
                const d = node.dataset;
                const s = node.snapshot;
                const k = node.fullname;
                const used = (isSnap ? (s?.used ?? s?.size ?? 0) : (d?.used ?? 0));
                const quota = d?.quota ?? 0;
                const pct = quota > 0 && !isSnap ? used / quota : 0;
                const enc = !isSnap && d ? (d.enc ?? d.encrypted ?? !!d.encryption) : false;
                const snapCount = d?.snap ?? d?.snapshots ?? 0;
                const hasChildren = node.children.length > 0;
                const isOpen = expanded.has(node.fullname);
                return (
                  <tr
                    key={k}
                    onClick={() => setSel(k)}
                    className={sel === k ? "is-on" : ""}
                    style={{ cursor: "pointer" }}
                  >
                    <td>
                      <span
                        style={{
                          display: "inline-flex",
                          alignItems: "center",
                          paddingLeft: node.depth * 16,
                        }}
                      >
                        {hasChildren ? (
                          <button
                            type="button"
                            className="tree-chevron"
                            onClick={(e) => {
                              e.stopPropagation();
                              toggle(node.fullname);
                            }}
                            aria-label={isOpen ? "Collapse" : "Expand"}
                            style={{
                              width: 16,
                              height: 16,
                              padding: 0,
                              border: 0,
                              background: "transparent",
                              cursor: "pointer",
                              color: "var(--fg-3)",
                              display: "inline-flex",
                              alignItems: "center",
                              justifyContent: "center",
                              transform: isOpen ? "rotate(90deg)" : "rotate(0deg)",
                              transition: "transform 80ms ease",
                              marginRight: 2,
                            }}
                          >
                            <Icon name="chev" size={10} />
                          </button>
                        ) : (
                          <span style={{ display: "inline-block", width: 16 }} />
                        )}
                        <Icon
                          name={isSnap ? "snapshot" : "storage"}
                          size={12}
                          style={{
                            marginRight: 6,
                            color: isSnap ? "var(--fg-3)" : "var(--accent)",
                          }}
                        />
                        <span
                          className="mono"
                          style={{
                            fontSize: 12,
                            color: isSnap ? "var(--fg-3)" : undefined,
                          }}
                        >
                          {node.display}
                        </span>
                      </span>
                    </td>
                    <td className="muted">{isSnap ? "—" : (d?.proto ?? "—")}</td>
                    <td className="num mono">{used > 0 ? formatBytes(used) : (isSnap || d ? "0 B" : "—")}</td>
                    <td>
                      {!isSnap && quota > 0 ? (
                        <div className="cap">
                          <div className="cap__bar">
                            <div style={{ width: `${pct * 100}%` }} />
                          </div>
                          <span className="mono" style={{ fontSize: 11, color: "var(--fg-3)" }}>
                            {formatBytes(quota)}
                          </span>
                        </div>
                      ) : (
                        <span className="muted">—</span>
                      )}
                    </td>
                    <td className="num mono">{isSnap ? "—" : (d ? snapCount : "—")}</td>
                    <td className="muted mono" style={{ fontSize: 11 }}>
                      {isSnap ? "—" : (d?.comp ?? d?.compression ?? "—")}
                    </td>
                    <td>
                      {isSnap ? (
                        <span className="muted">—</span>
                      ) : enc ? (
                        <Icon name="shield" size={12} />
                      ) : (
                        <span className="muted">—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
      {sel && (
        <DatasetDetail
          fullname={sel}
          fallback={datasets.find((d) => dsKey(d) === sel)}
          onClose={() => setSel(null)}
        />
      )}
    </div>
  );
}

type SubTab = "general" | "props" | "quota" | "policy" | "sharing" | "acl" | "bookmarks" | "meta";

function DatasetDetail({
  fullname,
  fallback,
  onClose,
}: {
  fullname: string;
  fallback?: Dataset;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [tab, setTab] = useState<SubTab>("general");
  const [action, setAction] = useState<ActionKind>(null);

  const q = useQuery({
    queryKey: ["dataset", fullname],
    queryFn: () => storage.getDataset(fullname),
  });
  const d = q.data ?? fallback;

  const inval = () => {
    qc.invalidateQueries({ queryKey: ["dataset", fullname] });
    qc.invalidateQueries({ queryKey: ["datasets"] });
  };

  const promoteMut = useMutation({
    meta: { label: "Promote failed" },
    mutationFn: () => storage.promoteDataset(fullname),
    onSuccess: () => { inval(); toastSuccess("Dataset promoted", fullname); },
  });

  if (!d) {
    return (
      <div className="side-detail">
        <div className="side-detail__head">
          <div>
            <div className="muted mono" style={{ fontSize: 10 }}>DATASET</div>
            <div className="side-detail__title">{fullname}</div>
          </div>
          <button className="btn btn--sm" onClick={onClose}>
            <Icon name="close" size={10} />
          </button>
        </div>
        <div className="empty-hint">{q.isLoading ? "Loading…" : "No data"}</div>
      </div>
    );
  }

  const used = d.used ?? 0;
  const quota = d.quota ?? 0;
  const pct = quota > 0 ? used / quota : 0;
  const enc = d.enc ?? d.encrypted ?? !!d.encryption;
  const snap = d.snap ?? d.snapshots ?? 0;

  return (
    <div className="side-detail">
      <div className="side-detail__head">
        <div>
          <div className="muted mono" style={{ fontSize: 10 }}>DATASET</div>
          <div className="side-detail__title">{d.name}</div>
        </div>
        <button className="btn btn--sm" onClick={onClose}>
          <Icon name="close" size={10} />
        </button>
      </div>

      <div className="win-tabs" style={{ overflowX: "auto" }}>
        {(["general", "props", "quota", "policy", "sharing", "acl", "bookmarks", "meta"] as const).map((t) => (
          <button key={t} className={tab === t ? "is-on" : ""} onClick={() => setTab(t)}>
            {t}
          </button>
        ))}
      </div>

      {tab === "general" && (
        <div className="sect">
          <div className="sect__title">Capacity</div>
          <div className="sect__body">
            <div className="bar">
              <div className="bar__fill" style={{ width: `${pct * 100}%` }} />
            </div>
            <div className="row" style={{ justifyContent: "space-between", fontSize: 11, marginTop: 4 }}>
              <span className="mono">{formatBytes(used)}</span>
              <span className="muted mono">/ {formatBytes(quota)}</span>
            </div>
            <dl className="kv" style={{ marginTop: 8 }}>
              <dt>Pool</dt><dd>{d.pool ?? "—"}</dd>
              <dt>Mountpoint</dt><dd className="mono" style={{ fontSize: 11 }}>{d.mountpoint ?? "—"}</dd>
              <dt>Protocol</dt><dd>{d.proto ?? "—"}</dd>
              <dt>Snapshots</dt><dd>{snap}</dd>
              <dt>Encrypted</dt><dd>{enc ? "yes" : "no"}</dd>
            </dl>
          </div>
        </div>
      )}

      {tab === "props" && (
        <div className="sect">
          <div className="sect__title">Properties</div>
          <div className="sect__body">
            <dl className="kv">
              <dt>Compression</dt><dd>{d.comp ?? d.compression ?? "—"}</dd>
              <dt>Recordsize</dt><dd>{d.recordsize ?? "—"}</dd>
              <dt>Atime</dt><dd>{d.atime ?? "—"}</dd>
              <dt>Encryption</dt><dd>{d.encryption ?? "—"}</dd>
              <dt>Referenced</dt><dd>{d.referenced ? formatBytes(d.referenced) : "—"}</dd>
              <dt>Available</dt><dd>{d.available ? formatBytes(d.available) : "—"}</dd>
            </dl>
          </div>
        </div>
      )}

      {tab === "quota" && (
        <div className="sect">
          <div className="sect__title">Quota</div>
          <div className="sect__body">
            <dl className="kv">
              <dt>Used</dt><dd>{formatBytes(used)}</dd>
              <dt>Quota</dt><dd>{quota > 0 ? formatBytes(quota) : "none"}</dd>
              <dt>Available</dt><dd>{d.available ? formatBytes(d.available) : "—"}</dd>
            </dl>
            <div className="muted small" style={{ marginTop: 6 }}>
              Editing quotas requires the property setter (TODO: backend missing for direct PUT).
            </div>
          </div>
        </div>
      )}

      {tab === "policy" && (
        <div className="sect">
          <div className="sect__title">Snapshot policy</div>
          <div className="sect__body">
            <div className="muted small">
              Snapshot schedules are managed in the <strong>Replication</strong> app.
              This dataset would inherit any schedule whose datasets list contains <code>{fullname}</code>.
            </div>
          </div>
        </div>
      )}

      {tab === "sharing" && (
        <div className="sect">
          <div className="sect__title">Sharing</div>
          <div className="sect__body">
            <div className="muted small">
              Manage SMB/NFS/iSCSI/NVMe-oF exports for this path in the <strong>Shares</strong> app.
            </div>
            <dl className="kv" style={{ marginTop: 6 }}>
              <dt>Path</dt><dd className="mono" style={{ fontSize: 11 }}>{d.mountpoint ?? "—"}</dd>
              <dt>Active proto</dt><dd>{d.proto ?? "—"}</dd>
            </dl>
          </div>
        </div>
      )}

      {tab === "acl" && <AclPanel fullname={fullname} />}
      {tab === "bookmarks" && <BookmarksPanel fullname={fullname} />}
      {tab === "meta" && <MetadataPanel fullname={fullname} />}

      <div
        className="row gap-8"
        style={{ padding: "10px 12px", borderTop: "1px solid var(--line)", flexWrap: "wrap" }}
      >
        <button className="btn btn--sm" onClick={() => setAction("rollback")}>Rollback</button>
        <button className="btn btn--sm" onClick={() => setAction("clone")}>Clone</button>
        <button
          className="btn btn--sm"
          disabled={promoteMut.isPending}
          onClick={() => {
            if (window.confirm(`Promote ${fullname}?`)) promoteMut.mutate();
          }}
        >
          Promote
        </button>
        <button className="btn btn--sm" onClick={() => setAction("rename")}>Rename</button>
        <button className="btn btn--sm" onClick={() => setAction("send")}>Send…</button>
        <button className="btn btn--sm" onClick={() => setAction("receive")}>Receive…</button>
      </div>

      {action === "rollback" && (
        <RollbackModal fullname={fullname} onClose={() => setAction(null)} onDone={inval} />
      )}
      {action === "clone" && (
        <CloneModal fullname={fullname} onClose={() => setAction(null)} onDone={inval} />
      )}
      {action === "rename" && (
        <RenameModal fullname={fullname} onClose={() => setAction(null)} onDone={inval} />
      )}
      {action === "send" && (
        <SendReceiveModal mode="send" fullname={fullname} onClose={() => setAction(null)} />
      )}
      {action === "receive" && (
        <SendReceiveModal mode="receive" fullname={fullname} onClose={() => setAction(null)} />
      )}
    </div>
  );
}

function AclPanel({ fullname }: { fullname: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["acl", fullname],
    queryFn: () => storage.getAcl(fullname),
  });
  const inval = () => qc.invalidateQueries({ queryKey: ["acl", fullname] });
  const removeMut = useMutation({
    meta: { label: "Remove ACL failed" },
    mutationFn: (i: number) => storage.removeAcl(fullname, i),
    onSuccess: () => { inval(); toastSuccess("ACL entry removed"); },
  });
  const [tag, setTag] = useState("user");
  const [who, setWho] = useState("");
  const [perms, setPerms] = useState("rwx");
  const [err, setErr] = useState<string | null>(null);

  const appendMut = useMutation({
    meta: { label: "Add ACL failed" },
    mutationFn: () => storage.appendAcl(fullname, { tag, who, permissions: perms }),
    onSuccess: () => { setWho(""); inval(); toastSuccess("ACL entry added"); },
    onError: (e: Error) => setErr(e.message),
  });

  return (
    <div className="sect">
      <div className="sect__title">ACL</div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {q.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>{(q.error as Error).message}</div>
        )}
        {q.data && q.data.length === 0 && <div className="muted">No ACL entries.</div>}
        {q.data && q.data.length > 0 && (
          <table className="tbl tbl--compact">
            <thead>
              <tr>
                <th>Tag</th>
                <th>Who</th>
                <th>Perms</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((a, i) => (
                <tr key={i}>
                  <td className="mono">{a.tag ?? "—"}</td>
                  <td className="mono" style={{ fontSize: 11 }}>{a.who ?? "—"}</td>
                  <td className="mono">{a.permissions ?? a.flags ?? "—"}</td>
                  <td className="num">
                    <button
                      className="btn btn--sm btn--danger"
                      disabled={removeMut.isPending}
                      onClick={() => removeMut.mutate(i)}
                    >
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="row gap-8" style={{ marginTop: 8, flexWrap: "wrap" }}>
          <select className="input" value={tag} onChange={(e) => setTag(e.target.value)} style={{ width: 90 }}>
            <option value="user">user</option>
            <option value="group">group</option>
            <option value="everyone">everyone@</option>
            <option value="owner">owner@</option>
          </select>
          <input
            className="input"
            placeholder="who"
            value={who}
            onChange={(e) => setWho(e.target.value)}
            style={{ flex: 1, minWidth: 90 }}
          />
          <input
            className="input"
            placeholder="rwx"
            value={perms}
            onChange={(e) => setPerms(e.target.value)}
            style={{ width: 80 }}
          />
          <button
            className="btn btn--sm"
            disabled={appendMut.isPending}
            onClick={() => { setErr(null); appendMut.mutate(); }}
          >
            Add
          </button>
        </div>
        {err && <div className="modal__err" style={{ marginTop: 6 }}>{err}</div>}
      </div>
    </div>
  );
}

function MetadataPanel({ fullname }: { fullname: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["meta", fullname],
    queryFn: () => storage.getMetadata(fullname),
  });
  const [draft, setDraft] = useState<DatasetMetadata>({});
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (q.data) setDraft({ ...q.data });
  }, [q.data]);

  const saveMut = useMutation({
    meta: { label: "Save metadata failed" },
    mutationFn: () => storage.putMetadata(fullname, draft),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["meta", fullname] }); toastSuccess("Metadata saved"); },
    onError: (e: Error) => setErr(e.message),
  });

  const [k, setK] = useState("");
  const [v, setV] = useState("");

  return (
    <div className="sect">
      <div className="sect__title">Metadata</div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {Object.keys(draft).length === 0 && !q.isLoading && (
          <div className="muted">No metadata.</div>
        )}
        {Object.keys(draft).length > 0 && (
          <table className="tbl tbl--compact">
            <tbody>
              {Object.entries(draft).map(([key, val]) => (
                <tr key={key}>
                  <td className="mono" style={{ fontSize: 11 }}>{key}</td>
                  <td>
                    <input
                      className="input"
                      value={val}
                      onChange={(e) => setDraft((d) => ({ ...d, [key]: e.target.value }))}
                    />
                  </td>
                  <td className="num">
                    <button
                      className="btn btn--sm btn--danger"
                      onClick={() => setDraft((d) => {
                        const c = { ...d }; delete c[key]; return c;
                      })}
                    >
                      ×
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="row gap-8" style={{ marginTop: 8 }}>
          <input className="input" placeholder="key" value={k} onChange={(e) => setK(e.target.value)} />
          <input className="input" placeholder="value" value={v} onChange={(e) => setV(e.target.value)} />
          <button
            className="btn btn--sm"
            disabled={!k}
            onClick={() => { setDraft((d) => ({ ...d, [k]: v })); setK(""); setV(""); }}
          >
            +
          </button>
        </div>
        <div className="row gap-8" style={{ marginTop: 8 }}>
          <button
            className="btn btn--sm btn--primary"
            disabled={saveMut.isPending}
            onClick={() => { setErr(null); saveMut.mutate(); }}
          >
            {saveMut.isPending ? "Saving…" : "Save"}
          </button>
        </div>
        {err && <div className="modal__err" style={{ marginTop: 6 }}>{err}</div>}
      </div>
    </div>
  );
}

function BookmarksPanel({ fullname }: { fullname: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["bookmarks", fullname],
    queryFn: () => storage.listBookmarks(fullname),
  });
  const inval = () => qc.invalidateQueries({ queryKey: ["bookmarks", fullname] });
  const [snap, setSnap] = useState("");
  const [bm, setBm] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const createMut = useMutation({
    meta: { label: "Create bookmark failed" },
    mutationFn: () => storage.createBookmark(fullname, { snapshot: snap, bookmark: bm }),
    onSuccess: () => { setSnap(""); setBm(""); inval(); toastSuccess("Bookmark created"); },
    onError: (e: Error) => setErr(e.message),
  });
  const destroyMut = useMutation({
    meta: { label: "Destroy bookmark failed" },
    mutationFn: (b: string) => storage.destroyBookmark(fullname, b),
    onSuccess: () => { inval(); toastSuccess("Bookmark destroyed"); },
  });
  return (
    <div className="sect">
      <div className="sect__title">Bookmarks</div>
      <div className="sect__body">
        {q.isLoading && <div className="muted">Loading…</div>}
        {q.isError && (
          <div className="muted" style={{ color: "var(--err)" }}>{(q.error as Error).message}</div>
        )}
        {q.data && q.data.length === 0 && <div className="muted">No bookmarks.</div>}
        {q.data && q.data.length > 0 && (
          <table className="tbl tbl--compact">
            <thead>
              <tr>
                <th>Bookmark</th>
                <th>Created</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((b) => {
                const k = b.fullname ?? b.name;
                const tail = k.includes("#") ? k.slice(k.indexOf("#") + 1) : k;
                return (
                  <tr key={k}>
                    <td className="mono" style={{ fontSize: 11 }}>{k}</td>
                    <td className="muted">{b.created ?? "—"}</td>
                    <td className="num">
                      <button
                        className="btn btn--sm btn--danger"
                        disabled={destroyMut.isPending}
                        onClick={() => {
                          if (window.confirm(`Destroy bookmark ${k}?`)) destroyMut.mutate(tail);
                        }}
                      >
                        Destroy
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
        <div className="row gap-8" style={{ marginTop: 8, flexWrap: "wrap" }}>
          <input
            className="input"
            placeholder="snapshot (e.g. snap-1)"
            value={snap}
            onChange={(e) => setSnap(e.target.value)}
            style={{ flex: 1, minWidth: 120 }}
          />
          <input
            className="input"
            placeholder="bookmark name"
            value={bm}
            onChange={(e) => setBm(e.target.value)}
            style={{ flex: 1, minWidth: 120 }}
          />
          <button
            className="btn btn--sm"
            disabled={createMut.isPending || !snap || !bm}
            onClick={() => { setErr(null); createMut.mutate(); }}
          >
            Create
          </button>
        </div>
        {err && <div className="modal__err" style={{ marginTop: 6 }}>{err}</div>}
      </div>
    </div>
  );
}

function RollbackModal({ fullname, onClose, onDone }: { fullname: string; onClose: () => void; onDone: () => void }) {
  const [snap, setSnap] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Rollback failed" },
    mutationFn: () => storage.rollbackDataset(fullname, snap || undefined),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Dataset rolled back", fullname); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Rollback dataset" sub={fullname} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn--primary" disabled={m.isPending} onClick={() => { setErr(null); m.mutate(); }}>
            {m.isPending ? "Rolling back…" : "Rollback"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Snapshot (optional, defaults to latest)</label>
        <input className="input" value={snap} onChange={(e) => setSnap(e.target.value)} placeholder="snap-name" />
      </div>
    </Modal>
  );
}

function CloneModal({ fullname, onClose, onDone }: { fullname: string; onClose: () => void; onDone: () => void }) {
  const [snap, setSnap] = useState("");
  const [target, setTarget] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Clone failed" },
    mutationFn: () => storage.cloneDataset(fullname, { snapshot: snap, target }),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Dataset cloned", target); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Clone dataset" sub={fullname} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !snap || !target}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Cloning…" : "Clone"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Source snapshot</label>
        <input className="input" value={snap} onChange={(e) => setSnap(e.target.value)} placeholder="snapname" />
      </div>
      <div className="field">
        <label className="field__label">Target dataset (full path)</label>
        <input className="input" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="pool/clone-name" />
      </div>
    </Modal>
  );
}

function RenameModal({ fullname, onClose, onDone }: { fullname: string; onClose: () => void; onDone: () => void }) {
  const [target, setTarget] = useState(fullname);
  const [err, setErr] = useState<string | null>(null);
  const m = useMutation({
    meta: { label: "Rename failed" },
    mutationFn: () => storage.renameDataset(fullname, target),
    onSuccess: () => { onDone(); onClose(); toastSuccess("Dataset renamed", target); },
    onError: (e: Error) => setErr(e.message),
  });
  return (
    <Modal title="Rename dataset" sub={fullname} onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !target || target === fullname}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Renaming…" : "Rename"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">New full name</label>
        <input className="input" value={target} onChange={(e) => setTarget(e.target.value)} />
      </div>
    </Modal>
  );
}

function SendReceiveModal({
  mode,
  fullname,
  onClose,
}: {
  mode: "send" | "receive";
  fullname: string;
  onClose: () => void;
}) {
  const [snapshot, setSnapshot] = useState("");
  const [target, setTarget] = useState("");
  const [incremental, setIncremental] = useState("");
  const [encrypted, setEncrypted] = useState(false);
  const [resumeToken, setResumeToken] = useState("");
  const [err, setErr] = useState<string | null>(null);

  const m = useMutation({
    meta: { label: `${mode === "send" ? "Send" : "Receive"} failed` },
    mutationFn: () => mode === "send"
      ? storage.sendDataset(fullname, {
          snapshot: snapshot || undefined,
          target: target || undefined,
          incremental: incremental || undefined,
          encrypted: encrypted || undefined,
          resume_token: resumeToken || undefined,
        })
      : storage.receiveDataset(fullname, {
          source: target || undefined,
          snapshot: snapshot || undefined,
          resume_token: resumeToken || undefined,
        }),
    onSuccess: () => {
      onClose();
      toastSuccess(mode === "send" ? "Send started" : "Receive started", fullname);
    },
    onError: (e: Error) => setErr(e.message),
  });

  const valid = mode === "send" ? !!snapshot : !!target;

  return (
    <Modal
      title={mode === "send" ? "Send dataset" : "Receive dataset"}
      sub={fullname}
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={m.isPending || !valid}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Working…" : (mode === "send" ? "Send" : "Receive")}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      {mode === "send" ? (
        <>
          <div className="field">
            <label className="field__label">Snapshot to send (required)</label>
            <input className="input" value={snapshot} onChange={(e) => setSnapshot(e.target.value)} placeholder="snap-2026-04-29" />
          </div>
          <div className="field">
            <label className="field__label">Target (replication-target id or remote dataset)</label>
            <input className="input" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="target-id or host:pool/ds" />
          </div>
          <div className="field">
            <label className="field__label">Incremental from (optional)</label>
            <input className="input" value={incremental} onChange={(e) => setIncremental(e.target.value)} placeholder="prev-snap" />
          </div>
          <div className="field">
            <label className="row gap-8" style={{ fontSize: 11 }}>
              <input type="checkbox" checked={encrypted} onChange={(e) => setEncrypted(e.target.checked)} />
              Send raw encrypted stream
            </label>
          </div>
          <div className="field">
            <label className="field__label">Resume token (optional)</label>
            <input className="input" value={resumeToken} onChange={(e) => setResumeToken(e.target.value)} />
          </div>
        </>
      ) : (
        <>
          <div className="field">
            <label className="field__label">Source (required)</label>
            <input className="input" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="target-id or host:pool/ds" />
          </div>
          <div className="field">
            <label className="field__label">Snapshot (optional)</label>
            <input className="input" value={snapshot} onChange={(e) => setSnapshot(e.target.value)} placeholder="snap-name" />
          </div>
          <div className="field">
            <label className="field__label">Resume token (optional)</label>
            <input className="input" value={resumeToken} onChange={(e) => setResumeToken(e.target.value)} />
          </div>
        </>
      )}
      <div className="muted small" style={{ marginTop: 6 }}>
        Streams are tracked by the replication engine; check the Replication app for live progress.
      </div>
    </Modal>
  );
}

function CreateDatasetModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [mountpoint, setMountpoint] = useState("");
  const [compression, setCompression] = useState("lz4");
  const [recordsize, setRecordsize] = useState("128K");
  const [atime, setAtime] = useState<"on" | "off">("off");
  const [encrypt, setEncrypt] = useState(false);
  const [passphrase, setPassphrase] = useState("");
  const [createParents, setCreateParents] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  const m = useMutation({
    meta: { label: "Create dataset failed" },
    mutationFn: () =>
      storage.createDataset({
        name,
        properties: { compression, recordsize, atime },
        mountpoint: mountpoint || undefined,
        createParents,
        encryption: encrypt
          ? { keyformat: "passphrase", passphrase }
          : undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["datasets"] });
      toastSuccess("Dataset created", name);
      onClose();
    },
    onError: (e: Error) => setErr(e.message),
  });

  const valid = name.trim() && (!encrypt || passphrase.length >= 8);
  return (
    <Modal
      title="New dataset"
      sub="Create a new ZFS dataset under an existing pool"
      onClose={onClose}
      footer={
        <>
          <button className="btn" onClick={onClose} disabled={m.isPending}>Cancel</button>
          <button
            className="btn btn--primary"
            disabled={!valid || m.isPending}
            onClick={() => { setErr(null); m.mutate(); }}
          >
            {m.isPending ? "Creating…" : "Create"}
          </button>
        </>
      }
    >
      {err && <div className="modal__err">{err}</div>}
      <div className="field">
        <label className="field__label">Name</label>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="tank/home" autoFocus />
        <div className="field__hint muted">Format: pool/path/to/dataset</div>
      </div>
      <div className="field">
        <label className="field__label">Mountpoint (optional)</label>
        <input className="input" value={mountpoint} onChange={(e) => setMountpoint(e.target.value)} placeholder="/mnt/tank/home" />
      </div>
      <div className="field">
        <label className="field__label">Compression</label>
        <select className="input" value={compression} onChange={(e) => setCompression(e.target.value)}>
          <option value="off">off</option>
          <option value="lz4">lz4</option>
          <option value="zstd">zstd</option>
          <option value="zstd-fast">zstd-fast</option>
          <option value="gzip">gzip</option>
        </select>
      </div>
      <div className="field">
        <label className="field__label">Record size</label>
        <select className="input" value={recordsize} onChange={(e) => setRecordsize(e.target.value)}>
          <option value="4K">4K</option>
          <option value="16K">16K</option>
          <option value="64K">64K</option>
          <option value="128K">128K (default)</option>
          <option value="256K">256K</option>
          <option value="1M">1M</option>
        </select>
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={atime === "on"} onChange={(e) => setAtime(e.target.checked ? "on" : "off")} />
          Track access times (atime)
        </label>
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={createParents} onChange={(e) => setCreateParents(e.target.checked)} />
          Create parent datasets if missing
        </label>
      </div>
      <div className="field">
        <label className="row gap-8" style={{ fontSize: 11 }}>
          <input type="checkbox" checked={encrypt} onChange={(e) => setEncrypt(e.target.checked)} />
          Encrypt with passphrase
        </label>
      </div>
      {encrypt && (
        <div className="field">
          <label className="field__label">Passphrase (≥ 8 chars)</label>
          <input
            type="password"
            className="input"
            value={passphrase}
            onChange={(e) => setPassphrase(e.target.value)}
            placeholder="••••••••"
            autoComplete="new-password"
          />
        </div>
      )}
    </Modal>
  );
}

export default DatasetsTab;
