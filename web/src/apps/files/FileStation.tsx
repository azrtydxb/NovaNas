// File Station — design-faithful skeleton.
// Backend file API is not yet wired; tree + grid layout match the design.

import { useState } from "react";
import { Icon } from "../../components/Icon";

type View = "grid" | "list";

type FileEntry = {
  name: string;
  kind: "folder" | "image" | "video" | "doc";
  size?: number;
  mod?: string;
};

const FILES: FileEntry[] = [
  { name: "trip-2024", kind: "folder", mod: "2 d ago" },
  { name: "wedding", kind: "folder", mod: "1 w ago" },
  { name: "raw-imports", kind: "folder", mod: "3 w ago" },
  { name: "IMG_4821.heic", kind: "image", size: 3.2e6, mod: "yesterday" },
  { name: "IMG_4822.heic", kind: "image", size: 3.4e6, mod: "yesterday" },
  { name: "IMG_4823.heic", kind: "image", size: 3.1e6, mod: "yesterday" },
  { name: "clip-01.mov", kind: "video", size: 142e6, mod: "2 d ago" },
  { name: "notes.md", kind: "doc", size: 2.1e3, mod: "1 h ago" },
];

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

export function FileStation() {
  const [view, setView] = useState<View>("grid");
  const [sel, setSel] = useState<string | null>(null);

  return (
    <div className="app-files">
      <div className="files-toolbar">
        <div className="files-path mono">
          <span className="muted">/</span> family-media <Icon name="chev" size={11} /> Photos
        </div>
        <div className="row gap-8" style={{ marginLeft: "auto" }}>
          <div className="seg">
            <button className={view === "grid" ? "is-on" : ""} onClick={() => setView("grid")}>
              <Icon name="grid" size={12} />
            </button>
            <button className={view === "list" ? "is-on" : ""} onClick={() => setView("list")}>
              <Icon name="log" size={12} />
            </button>
          </div>
        </div>
      </div>
      <div className="files-split">
        <div className="files-tree">
          <div className="files-tree__group">VOLUMES</div>
          <div className="files-tree__item is-on"><Icon name="storage" size={12} />family-media</div>
          <div className="files-tree__item"><Icon name="storage" size={12} />pascal/docs</div>
          <div className="files-tree__item"><Icon name="storage" size={12} />pascal/photos</div>
          <div className="files-tree__item"><Icon name="storage" size={12} />backups</div>
          <div className="files-tree__group">SHARED</div>
          <div className="files-tree__item"><Icon name="user" size={12} />family</div>
          <div className="files-tree__item"><Icon name="user" size={12} />pascal</div>
        </div>
        <div className={`files-${view}`}>
          {FILES.map((f) => (
            <div
              key={f.name}
              className={`file-item ${sel === f.name ? "is-on" : ""}`}
              onClick={() => setSel(f.name)}
            >
              <div className="file-item__icon" data-kind={f.kind}>
                <Icon
                  name={f.kind === "folder" ? "folder" : f.kind === "image" ? "image" : f.kind === "video" ? "video" : "doc"}
                  size={view === "grid" ? 28 : 14}
                />
              </div>
              <div className="file-item__name">{f.name}</div>
              {view === "list" && (
                <>
                  <div className="muted mono">{f.size ? fmtBytes(f.size) : "—"}</div>
                  <div className="muted">{f.mod}</div>
                </>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

export default FileStation;
