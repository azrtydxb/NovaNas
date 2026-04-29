import { useToasts } from "../store/toast";
import { Icon } from "./Icon";

export function Toasts() {
  const toasts = useToasts((s) => s.toasts);
  const dismiss = useToasts((s) => s.dismiss);
  if (toasts.length === 0) return null;
  return (
    <div className="toast-stack">
      {toasts.map((t) => (
        <div key={t.id} className={`toast toast--${t.kind}`}>
          <Icon
            name={t.kind === "error" ? "alert" : t.kind === "success" ? "check" : "info"}
            size={14}
          />
          <div className="toast__body">
            <div className="toast__title">{t.title}</div>
            {t.body && <div className="toast__sub">{t.body}</div>}
          </div>
          <button className="toast__close" onClick={() => dismiss(t.id)}>
            <Icon name="x" size={11} />
          </button>
        </div>
      ))}
    </div>
  );
}
