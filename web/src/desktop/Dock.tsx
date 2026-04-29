import { APPS } from "../wm/registry";
import { useWM } from "../wm/store";
import { Icon } from "../components/Icon";
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
  const windows = useWM((s) => s.windows);
  return (
    <div className="dock">
      {PINNED.map((id) => {
        const def = APPS[id];
        const running = windows.some((w) => w.appId === id);
        return (
          <button
            key={id}
            className={`dock__btn${running ? " is-running" : ""}`}
            onClick={() => open(id)}
            title={def.title}
          >
            <Icon name={def.icon} size={20} sw={1.6} />
            <span className="dock__lbl">{def.title}</span>
          </button>
        );
      })}
    </div>
  );
}
