import { APP_LIST } from "../wm/registry";
import { useWM } from "../wm/store";
import { Icon } from "../components/Icon";

export function Launcher({ onClose }: { onClose: () => void }) {
  const open = useWM((s) => s.open);
  return (
    <div className="launcher" onClick={onClose}>
      <div className="launcher__inner" onClick={(e) => e.stopPropagation()}>
        <div className="launcher__title">Applications</div>
        <div className="launcher__grid">
          {APP_LIST.map((a) => (
            <button
              key={a.id}
              className="launcher__item"
              onClick={() => {
                open(a.id);
                onClose();
              }}
            >
              <span className="launcher__icon">
                <Icon name={a.icon} size={26} sw={1.5} />
              </span>
              <span className="launcher__name">{a.title}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
