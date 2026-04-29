import { useEffect, useMemo, useRef, useState } from "react";
import { APP_LIST } from "../wm/registry";
import { useWM } from "../wm/store";

export function Palette({ onClose }: { onClose: () => void }) {
  const [q, setQ] = useState("");
  const [idx, setIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const open = useWM((s) => s.open);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const matches = useMemo(() => {
    const t = q.trim().toLowerCase();
    if (!t) return APP_LIST;
    return APP_LIST.filter((a) => a.title.toLowerCase().includes(t) || a.id.includes(t));
  }, [q]);

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") onClose();
    else if (e.key === "ArrowDown") {
      e.preventDefault();
      setIdx((i) => Math.min(matches.length - 1, i + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setIdx((i) => Math.max(0, i - 1));
    } else if (e.key === "Enter") {
      const m = matches[idx];
      if (m) {
        open(m.id);
        onClose();
      }
    }
  };

  return (
    <div className="palette-bg" onClick={onClose}>
      <div className="palette" onClick={(e) => e.stopPropagation()}>
        <input
          ref={inputRef}
          className="palette__input"
          placeholder="Search apps and actions…"
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setIdx(0);
          }}
          onKeyDown={onKeyDown}
        />
        <ul className="palette__list">
          {matches.map((a, i) => (
            <li
              key={a.id}
              className={`palette__item${i === idx ? " is-on" : ""}`}
              onMouseEnter={() => setIdx(i)}
              onClick={() => {
                open(a.id);
                onClose();
              }}
            >
              <span className="palette__icon">{a.title.charAt(0)}</span>
              <span className="palette__lbl">{a.title}</span>
              <span className="palette__hint">app</span>
            </li>
          ))}
          {matches.length === 0 && <li className="palette__empty">No matches.</li>}
        </ul>
      </div>
    </div>
  );
}
