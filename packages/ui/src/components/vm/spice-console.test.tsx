import type { Vm } from '@novanas/schemas';
import { render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { SpiceConsole } from './spice-console';

const vm: Vm = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'Vm',
  metadata: { name: 'vm-test', namespace: 'user-test' },
  spec: {
    os: { type: 'linux' },
    resources: { cpu: 2, memoryMiB: 2048 },
    graphics: { enabled: true, type: 'spice' },
  },
};

class FakeWebSocket {
  public binaryType = 'arraybuffer';
  public readyState = 0;
  public url: string;
  private listeners = new Map<string, Set<(e: unknown) => void>>();
  constructor(url: string) {
    this.url = url;
  }
  addEventListener(event: string, cb: (e: unknown) => void) {
    if (!this.listeners.has(event)) this.listeners.set(event, new Set());
    this.listeners.get(event)!.add(cb);
  }
  removeEventListener(event: string, cb: (e: unknown) => void) {
    this.listeners.get(event)?.delete(cb);
  }
  close() {
    this.readyState = 3;
    for (const cb of this.listeners.get('close') ?? []) cb({});
  }
  send() {
    /* no-op */
  }
  fireOpen() {
    this.readyState = 1;
    for (const cb of this.listeners.get('open') ?? []) cb({});
  }
  fireMessage(data: unknown) {
    for (const cb of this.listeners.get('message') ?? []) cb({ data });
  }
}

describe('SpiceConsole', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders without crashing and opens the right URL', () => {
    const sockets: FakeWebSocket[] = [];
    const factory = (url: string) => {
      const s = new FakeWebSocket(url);
      sockets.push(s);
      return s as unknown as WebSocket;
    };
    const { container } = render(
      <SpiceConsole vm={vm} wsFactory={factory} wsUrl='ws://test/console?type=spice' />
    );
    expect(container.querySelector('canvas')).toBeTruthy();
    expect(sockets).toHaveLength(1);
    expect(sockets[0]!.url).toBe('ws://test/console?type=spice');
  });

  it('transitions to connected on open and counts frames', async () => {
    const sockets: FakeWebSocket[] = [];
    const factory = (url: string) => {
      const s = new FakeWebSocket(url);
      sockets.push(s);
      return s as unknown as WebSocket;
    };
    const { getByText, rerender } = render(
      <SpiceConsole vm={vm} wsFactory={factory} wsUrl='ws://t' />
    );
    sockets[0]!.fireOpen();
    sockets[0]!.fireMessage(new ArrayBuffer(4));
    sockets[0]!.fireMessage(new ArrayBuffer(4));
    // Force a re-render so state updates flush.
    rerender(<SpiceConsole vm={vm} wsFactory={factory} wsUrl='ws://t' />);
    // Wait a tick for state effects.
    await Promise.resolve();
    expect(getByText(/frames:/)).toBeTruthy();
  });
});
