import { APP_LIST } from "../wm/registry";
import { useWM } from "../wm/store";

export function Launcher({ onClose }: { onClose: () => void }) {
  const open = useWM((s) => s.open);
  return (
    <div className="launcher" onClick={onClose}>
      <div className="launcher__grid" onClick={(e) => e.stopPropagation()}>
        {APP_LIST.map((a) => (
          <button
            key={a.id}
            className="launcher__item"
            onClick={() => {
              open(a.id);
              onClose();
            }}
          >
            <span className="launcher__icon">{a.title.charAt(0)}</span>
            <span className="launcher__lbl">{a.title}</span>
          </button>
        ))}
      </div>
    </div>
  );
}
