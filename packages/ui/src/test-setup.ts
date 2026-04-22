import '@testing-library/jest-dom/vitest';
import { vi } from 'vitest';

/**
 * Mock `@novnc/novnc/lib/rfb` globally for tests.
 *
 * The published noVNC 1.6 bundle uses top-level await in its CommonJS module
 * graph, which Node's `require` refuses to load under vitest. Tests never
 * need the real RFB client — they exercise `VncConsole` through its
 * `rfbFactory` override. This stub is only here to keep the import resolvable.
 */
vi.mock('@novnc/novnc/lib/rfb', () => {
  class RFB extends EventTarget {
    constructor(
      public target: HTMLElement,
      public url: string
    ) {
      super();
    }
    disconnect() {}
    sendCtrlAltDel() {}
    clipboardPasteFrom() {}
    focus() {}
  }
  return { default: RFB };
});
