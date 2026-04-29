import type { ReactNode } from "react";
import { Icon } from "../../components/Icon";

export function Modal({
  title,
  sub,
  onClose,
  children,
  footer,
  width,
}: {
  title: string;
  sub?: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  width?: number;
}) {
  return (
    <div
      className="modal-bg"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="modal" style={width ? { width } : undefined}>
        <div className="modal__head">
          <div className="modal__head-meta">
            <div className="modal__title">{title}</div>
            {sub && <div className="modal__sub muted">{sub}</div>}
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="close" size={14} />
          </button>
        </div>
        <div className="modal__body">{children}</div>
        {footer && <div className="modal__foot">{footer}</div>}
      </div>
    </div>
  );
}

export default Modal;
