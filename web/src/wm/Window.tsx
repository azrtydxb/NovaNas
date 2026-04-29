import { useRef, type ReactNode } from "react";
import { useWM } from "./store";
import { Icon, type IconName } from "../components/Icon";
import type { WindowState } from "./types";

type Props = {
  state: WindowState;
  title: string;
  icon: IconName;
  children: ReactNode;
};

export function Window({ state, title, icon, children }: Props) {
  const { focus, close, toggleMinimize, toggleMaximize, move, resize } = useWM();
  const isActive = useWM((s) => s.activeId === state.id);
  const dragRef = useRef<{ startX: number; startY: number; origX: number; origY: number } | null>(
    null
  );

  if (state.minimized) return null;

  const onBarPointerDown = (e: React.PointerEvent) => {
    if ((e.target as HTMLElement).closest(".win-btn")) return;
    focus(state.id);
    if (state.maximized) return;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    dragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      origX: state.x,
      origY: state.y,
    };
  };
  const onBarPointerMove = (e: React.PointerEvent) => {
    if (!dragRef.current) return;
    const dx = e.clientX - dragRef.current.startX;
    const dy = e.clientY - dragRef.current.startY;
    move(state.id, dragRef.current.origX + dx, Math.max(0, dragRef.current.origY + dy));
  };
  const onBarPointerUp = (e: React.PointerEvent) => {
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
    dragRef.current = null;
  };

  const resizeRef = useRef<{ startX: number; startY: number; origW: number; origH: number } | null>(
    null
  );
  const onResizePointerDown = (e: React.PointerEvent) => {
    e.stopPropagation();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    resizeRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      origW: state.w,
      origH: state.h,
    };
  };
  const onResizePointerMove = (e: React.PointerEvent) => {
    if (!resizeRef.current) return;
    const w = Math.max(360, resizeRef.current.origW + (e.clientX - resizeRef.current.startX));
    const h = Math.max(240, resizeRef.current.origH + (e.clientY - resizeRef.current.startY));
    resize(state.id, w, h);
  };
  const onResizePointerUp = (e: React.PointerEvent) => {
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
    resizeRef.current = null;
  };

  const style: React.CSSProperties = state.maximized
    ? { left: 0, top: 0, width: "100%", height: "100%", zIndex: state.z }
    : {
        left: state.x,
        top: state.y,
        width: state.w,
        height: state.h,
        zIndex: state.z,
      };

  return (
    <div
      className={`win${isActive ? " is-active" : ""}`}
      style={style}
      onMouseDown={() => !isActive && focus(state.id)}
    >
      <div
        className="win__bar"
        onPointerDown={onBarPointerDown}
        onPointerMove={onBarPointerMove}
        onPointerUp={onBarPointerUp}
        onDoubleClick={() => toggleMaximize(state.id)}
      >
        <Icon name={icon} size={11} style={{ marginRight: 6, color: "var(--fg-2)" }} />
        <span className="win__title">{title}</span>
        <span className="win__btns">
          <button className="win-btn" onClick={() => toggleMinimize(state.id)} aria-label="Minimize">
            <Icon name="min" size={11} />
          </button>
          <button className="win-btn" onClick={() => toggleMaximize(state.id)} aria-label="Maximize">
            <Icon name="max" size={10} />
          </button>
          <button
            className="win-btn win-btn--close"
            onClick={() => close(state.id)}
            aria-label="Close"
          >
            <Icon name="x" size={11} />
          </button>
        </span>
      </div>
      <div className="win__body">{children}</div>
      {!state.maximized && (
        <div
          className="win__resize"
          onPointerDown={onResizePointerDown}
          onPointerMove={onResizePointerMove}
          onPointerUp={onResizePointerUp}
        />
      )}
    </div>
  );
}
