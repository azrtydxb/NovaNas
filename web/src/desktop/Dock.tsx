import { APPS } from "../wm/registry";
import { useWM } from "../wm/store";
import type { AppId } from "../wm/types";

const PINNED: AppId[] = [
  "package-center",
  "storage",
  "shares",
  "identity",
  "alerts",
  "logs",
  "files",
  "terminal",
  "control-panel",
];

export function Dock() {
  const open = useWM((s) => s.open);
  return (
    <div className="dock">
      {PINNED.map((id) => {
        const def = APPS[id];
        return (
          <button key={id} className="dock__btn" onClick={() => open(id)} title={def.title}>
            <span className="dock__glyph">{def.title.charAt(0)}</span>
            <span className="dock__lbl">{def.title}</span>
          </button>
        );
      })}
    </div>
  );
}
