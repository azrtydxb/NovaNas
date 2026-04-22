import { resolveTheme, useUiStore } from '@/stores/ui';
import { useEffect } from 'react';

/**
 * Reflects the persisted theme preference onto <html> by toggling
 * `theme-dark` / `theme-light` classes (bound in tokens.css).
 * Also syncs the density class.
 */
export function ThemeBridge() {
  const theme = useUiStore((s) => s.theme);
  const density = useUiStore((s) => s.density);

  useEffect(() => {
    const root = document.documentElement;
    const apply = () => {
      const resolved = resolveTheme(theme);
      root.classList.toggle('theme-dark', resolved === 'dark');
      root.classList.toggle('theme-light', resolved === 'light');
    };
    apply();
    if (theme === 'system' && typeof window !== 'undefined') {
      const mq = window.matchMedia('(prefers-color-scheme: light)');
      mq.addEventListener('change', apply);
      return () => mq.removeEventListener('change', apply);
    }
    return undefined;
  }, [theme]);

  useEffect(() => {
    const root = document.documentElement;
    root.classList.toggle('density-spacious', density === 'spacious');
  }, [density]);

  return null;
}
