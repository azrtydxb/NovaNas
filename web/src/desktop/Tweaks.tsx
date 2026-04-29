import { useState } from "react";
import { useTheme, type ThemeVariant, type Density } from "../store/theme";
import { Icon } from "../components/Icon";

export function Tweaks() {
  const { variant, density, setVariant, setDensity } = useTheme();
  const [open, setOpen] = useState(false);

  return (
    <>
      <button
        className="tweaks-fab"
        onClick={() => setOpen((v) => !v)}
        aria-label="Toggle tweaks panel"
      >
        <Icon name="settings" size={16} />
      </button>
      {open && (
        <div className="tweaks">
          <div className="tweaks__head">Tweaks</div>
          <div className="tweaks__group">
            <div className="tweaks__lbl">Theme</div>
            <div className="tweaks__row">
              {(["aurora", "graphite", "aether"] as ThemeVariant[]).map((v) => (
                <button
                  key={v}
                  className={variant === v ? "is-on" : ""}
                  onClick={() => setVariant(v)}
                >
                  {v}
                </button>
              ))}
            </div>
          </div>
          <div className="tweaks__group">
            <div className="tweaks__lbl">Density</div>
            <div className="tweaks__row">
              {(["compact", "default", "spacious"] as Density[]).map((d) => (
                <button
                  key={d}
                  className={density === d ? "is-on" : ""}
                  onClick={() => setDensity(d)}
                >
                  {d}
                </button>
              ))}
            </div>
          </div>
        </div>
      )}
    </>
  );
}
