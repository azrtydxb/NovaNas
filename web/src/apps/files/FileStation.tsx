// TODO: phase 3 — wire to backend file API
// File Station shell: renders the design's tree + grid layout structure
// so the eventual file browser drops in here. No live API yet.

import { useState } from "react";
import { Icon } from "../../components/Icon";

type View = "grid" | "list";

export function FileStation() {
  const [view, setView] = useState<View>("grid");

  return (
    <div className="app-files">
      <div className="files-toolbar">
        <div className="files-path mono">
          <span className="muted">/</span> home <Icon name="chev" size={11} /> pascal
        </div>
        <div className="row gap-8" style={{ marginLeft: "auto" }}>
          <div className="seg">
            <button
              className={view === "grid" ? "is-on" : ""}
              onClick={() => setView("grid")}
              aria-label="Grid view"
            >
              <Icon name="grid" size={12} />
            </button>
            <button
              className={view === "list" ? "is-on" : ""}
              onClick={() => setView("list")}
              aria-label="List view"
            >
              <Icon name="log" size={12} />
            </button>
          </div>
        </div>
      </div>

      <div className="files-split">
        <div className="files-tree">
          <div className="files-tree__group">FAVORITES</div>
          <div className="files-tree__item is-on">
            <Icon name="folder" size={12} />
            home
          </div>
          <div className="files-tree__item">
            <Icon name="folder" size={12} />
            documents
          </div>
          <div className="files-tree__item">
            <Icon name="folder" size={12} />
            downloads
          </div>
          <div className="files-tree__item">
            <Icon name="folder" size={12} />
            media
          </div>
        </div>

        <div
          className={`files-${view}`}
          style={{
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            gap: 10,
            padding: 32,
            textAlign: "center",
          }}
        >
          <div
            style={{
              width: 56,
              height: 56,
              display: "grid",
              placeItems: "center",
              borderRadius: "var(--r-lg)",
              background: "var(--accent-soft)",
              color: "var(--accent)",
            }}
          >
            <Icon name="files" size={28} />
          </div>
          <div style={{ fontSize: 13, color: "var(--fg-0)", fontWeight: 500 }}>
            File Station
          </div>
          <div className="muted" style={{ fontSize: 11, maxWidth: 360 }}>
            Coming in a later phase — backend file browser API is not yet
            available. Tree + grid layout shown here previews where the feature
            will live.
          </div>
        </div>
      </div>
    </div>
  );
}

export default FileStation;
