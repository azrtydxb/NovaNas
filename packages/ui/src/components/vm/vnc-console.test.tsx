import type { Vm } from '@novanas/schemas';
import { act, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { type RfbFactory, VncConsole } from './vnc-console';

const vm: Vm = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'Vm',
  metadata: { name: 'vm-test', namespace: 'user-test' },
  spec: {
    os: { type: 'linux' },
    resources: { cpu: 2, memoryMiB: 2048 },
    graphics: { enabled: true, type: 'vnc' },
  },
};

class FakeRfb extends EventTarget {
  public scaleViewport = false;
  public resizeSession = false;
  public focusOnClick = false;
  public disconnected = false;
  public ctrlAltDelCount = 0;
  public pastedText: string[] = [];
  constructor(
    public target: HTMLElement,
    public url: string
  ) {
    super();
  }
  disconnect() {
    this.disconnected = true;
  }
  sendCtrlAltDel() {
    this.ctrlAltDelCount++;
  }
  clipboardPasteFrom(text: string) {
    this.pastedText.push(text);
  }
  focus() {}
  fireConnect() {
    this.dispatchEvent(new CustomEvent('connect'));
  }
  fireDisconnect(clean: boolean) {
    this.dispatchEvent(new CustomEvent('disconnect', { detail: { clean } }));
  }
}

function makeFactory() {
  const instances: FakeRfb[] = [];
  const factory: RfbFactory = (target, url) => {
    const r = new FakeRfb(target, url);
    instances.push(r);
    return r as unknown as ReturnType<RfbFactory>;
  };
  return { factory, instances };
}

describe('VncConsole', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('instantiates RFB with the computed WS URL from vm props', () => {
    const { factory, instances } = makeFactory();
    const { container } = render(<VncConsole vm={vm} rfbFactory={factory} />);
    expect(container.querySelector('[aria-label="VNC console for vm-test"]')).toBeTruthy();
    expect(instances).toHaveLength(1);
    const url = instances[0]!.url;
    expect(url).toMatch(/\/api\/v1\/vms\/user-test\/vm-test\/console\?type=vnc$/);
  });

  it('honours a wsUrl override (used for tests / injection)', () => {
    const { factory, instances } = makeFactory();
    render(<VncConsole vm={vm} wsUrl='ws://test/console?type=vnc' rfbFactory={factory} />);
    expect(instances[0]!.url).toBe('ws://test/console?type=vnc');
  });

  it('transitions to connected on the connect event', async () => {
    const { factory, instances } = makeFactory();
    const { findByText } = render(<VncConsole vm={vm} wsUrl='ws://t' rfbFactory={factory} />);
    await act(async () => {
      instances[0]!.fireConnect();
    });
    expect(await findByText(/connected/i)).toBeTruthy();
  });

  it('shows reconnect button after a dirty disconnect and reinstantiates on retry', async () => {
    const { factory, instances } = makeFactory();
    const { findByText, getByText } = render(
      <VncConsole vm={vm} wsUrl='ws://t' rfbFactory={factory} />
    );
    await act(async () => {
      instances[0]!.fireConnect();
      instances[0]!.fireDisconnect(false);
    });
    expect(await findByText(/error/i)).toBeTruthy();
    const retry = getByText('Reconnect');
    await act(async () => {
      retry.click();
    });
    // A new RFB instance was created on retry.
    expect(instances.length).toBeGreaterThanOrEqual(2);
    // The previous instance was disconnected.
    expect(instances[0]!.disconnected).toBe(true);
  });

  it('sends Ctrl-Alt-Del via the toolbar button when connected', async () => {
    const { factory, instances } = makeFactory();
    const { getByTitle } = render(<VncConsole vm={vm} wsUrl='ws://t' rfbFactory={factory} />);
    await act(async () => {
      instances[0]!.fireConnect();
    });
    const btn = getByTitle('Send Ctrl+Alt+Del') as HTMLButtonElement;
    btn.click();
    expect(instances[0]!.ctrlAltDelCount).toBe(1);
  });

  it('forwards window paste events to rfb.clipboardPasteFrom', () => {
    const { factory, instances } = makeFactory();
    render(<VncConsole vm={vm} wsUrl='ws://t' rfbFactory={factory} />);
    // jsdom does not implement ClipboardEvent/DataTransfer; fabricate a
    // minimal event that matches the handler's `clipboardData.getData` shape.
    const evt = new Event('paste') as Event & {
      clipboardData: { getData: (t: string) => string };
    };
    (evt as unknown as { clipboardData: unknown }).clipboardData = {
      getData: (_t: string) => 'hello guest',
    };
    window.dispatchEvent(evt);
    expect(instances[0]!.pastedText).toContain('hello guest');
  });
});
