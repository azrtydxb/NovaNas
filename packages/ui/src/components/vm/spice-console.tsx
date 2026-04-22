import { Button } from '@/components/ui/button';
import type { Vm } from '@novanas/schemas';
import { Download, Loader2, WifiOff } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';

/**
 * SPICE console component.
 *
 * spice-html5 (the canonical browser client for SPICE) is no longer
 * published on npm. Rather than silently faking a SPICE session, this
 * component establishes the full WebSocket proxy handshake against the
 * NovaNas API (`/api/v1/vms/:namespace/:name/console`), renders incoming
 * frames onto a canvas as raw byte activity, and surfaces a virt-viewer
 * download link for the complete SPICE protocol.
 *
 * TODO(a11-spice): integrate a real SPICE client. Options:
 *   1. Bundle a forked spice-html5 (the MIT source is still available at
 *      https://gitlab.freedesktop.org/spice/spice-html5).
 *   2. Switch the backend to VNC + noVNC (@novnc/novnc) which is maintained.
 */

interface SpiceConsoleProps {
  vm: Vm;
  /** Override the websocket URL (tests). */
  wsUrl?: string;
  /** Override the global WebSocket ctor (tests). */
  wsFactory?: (url: string) => WebSocket;
}

type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'error';

export function SpiceConsole({ vm, wsUrl, wsFactory }: SpiceConsoleProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [state, setState] = useState<ConnectionState>('connecting');
  const [frameCount, setFrameCount] = useState(0);
  const [lastMessage, setLastMessage] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const namespace = vm.metadata.namespace ?? '';
  const name = vm.metadata.name;
  const consoleType = vm.spec.graphics?.type ?? 'spice';

  // biome-ignore lint/correctness/useExhaustiveDependencies: `nonce` bump triggers reconnect.
  useEffect(() => {
    void nonce;
    const url =
      wsUrl ??
      `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}` +
        `/api/v1/vms/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/console?type=${consoleType}`;

    setState('connecting');
    setFrameCount(0);

    let ws: WebSocket;
    try {
      ws = wsFactory ? wsFactory(url) : new WebSocket(url);
    } catch (err) {
      setLastMessage((err as Error)?.message ?? 'connection failed');
      setState('error');
      return;
    }
    ws.binaryType = 'arraybuffer';
    wsRef.current = ws;

    const onOpen = () => {
      setState('connected');
      setLastMessage(null);
    };
    const onMessage = (evt: MessageEvent) => {
      setFrameCount((c) => c + 1);
      paintFrame(canvasRef.current, evt.data);
    };
    const onClose = () => {
      setState('disconnected');
    };
    const onError = () => {
      setState('error');
      setLastMessage('websocket error');
    };
    ws.addEventListener('open', onOpen);
    ws.addEventListener('message', onMessage);
    ws.addEventListener('close', onClose);
    ws.addEventListener('error', onError);

    return () => {
      ws.removeEventListener('open', onOpen);
      ws.removeEventListener('message', onMessage);
      ws.removeEventListener('close', onClose);
      ws.removeEventListener('error', onError);
      try {
        ws.close();
      } catch {
        /* ignore */
      }
    };
  }, [namespace, name, consoleType, wsUrl, wsFactory, nonce]);

  // Resize canvas to container on mount + on window resize.
  useEffect(() => {
    const container = containerRef.current;
    const canvas = canvasRef.current;
    if (!container || !canvas) return;
    const apply = () => {
      const rect = container.getBoundingClientRect();
      canvas.width = Math.max(320, Math.floor(rect.width));
      canvas.height = Math.max(200, Math.floor(rect.height));
    };
    apply();
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', apply);
      return () => window.removeEventListener('resize', apply);
    }
    const ro = new ResizeObserver(apply);
    ro.observe(container);
    return () => ro.disconnect();
  }, []);

  // Keyboard + mouse event forwarding (SPICE input opcodes live in the
  // protocol; this scaffolding forwards raw event data and will be replaced
  // by a real client).
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const send = (payload: Record<string, unknown>) => {
      const ws = wsRef.current;
      if (!ws || ws.readyState !== WebSocket.OPEN) return;
      try {
        ws.send(JSON.stringify(payload));
      } catch {
        /* ignore */
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (document.activeElement !== canvas) return;
      send({ t: 'key', code: e.code, down: e.type === 'keydown' });
      e.preventDefault();
    };
    const onMouseMove = (e: MouseEvent) => {
      const rect = canvas.getBoundingClientRect();
      send({ t: 'mouse', x: e.clientX - rect.left, y: e.clientY - rect.top });
    };
    const onMouseButton = (e: MouseEvent) => {
      send({ t: 'click', button: e.button, down: e.type === 'mousedown' });
    };
    canvas.tabIndex = 0;
    canvas.addEventListener('keydown', onKey);
    canvas.addEventListener('keyup', onKey);
    canvas.addEventListener('mousemove', onMouseMove);
    canvas.addEventListener('mousedown', onMouseButton);
    canvas.addEventListener('mouseup', onMouseButton);
    return () => {
      canvas.removeEventListener('keydown', onKey);
      canvas.removeEventListener('keyup', onKey);
      canvas.removeEventListener('mousemove', onMouseMove);
      canvas.removeEventListener('mousedown', onMouseButton);
      canvas.removeEventListener('mouseup', onMouseButton);
    };
  }, []);

  const retry = () => setNonce((n) => n + 1);

  const downloadVv = () => {
    const vv = [
      '[virt-viewer]',
      `type=${consoleType}`,
      'host=',
      'port=',
      `title=${name}`,
      'delete-this-file=1',
      '',
    ].join('\n');
    const blob = new Blob([vv], { type: 'application/x-virt-viewer' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `${name}.vv`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  return (
    <div className='flex flex-col gap-2' ref={containerRef}>
      <StateBanner state={state} lastMessage={lastMessage} onRetry={retry} />
      <div className='relative bg-black border border-border rounded-sm overflow-hidden min-h-[240px]'>
        <canvas
          ref={canvasRef}
          className='block w-full h-full outline-none'
          aria-label={`SPICE console for ${name}`}
        />
        <div className='absolute top-1 right-2 text-[10px] mono text-foreground-subtle/70 pointer-events-none'>
          frames: {frameCount}
        </div>
      </div>
      <div className='text-xs text-foreground-subtle flex items-center justify-between'>
        <span>
          TODO(a11-spice): full SPICE client not bundled (spice-html5 unpublished). Frames from the
          proxy are counted but not decoded.
        </span>
        <Button size='sm' variant='ghost' onClick={downloadVv} title='Download .vv'>
          <Download size={11} /> virt-viewer
        </Button>
      </div>
    </div>
  );
}

function StateBanner({
  state,
  lastMessage,
  onRetry,
}: {
  state: ConnectionState;
  lastMessage: string | null;
  onRetry: () => void;
}) {
  if (state === 'connected') {
    return (
      <div className='text-[11px] mono flex items-center gap-2 text-ok-fg'>
        <span className='inline-block h-1.5 w-1.5 rounded-full bg-ok' /> connected
      </div>
    );
  }
  if (state === 'connecting') {
    return (
      <div className='text-[11px] mono flex items-center gap-2 text-foreground-subtle'>
        <Loader2 size={11} className='animate-spin' /> connecting…
      </div>
    );
  }
  return (
    <div className='text-[11px] mono flex items-center gap-2 text-warn'>
      <WifiOff size={11} /> {state === 'error' ? 'error' : 'disconnected'}
      {lastMessage ? ` — ${lastMessage}` : ''}
      <Button size='sm' variant='ghost' onClick={onRetry}>
        Retry
      </Button>
    </div>
  );
}

function paintFrame(canvas: HTMLCanvasElement | null, data: unknown) {
  if (!canvas) return;
  let ctx: CanvasRenderingContext2D | null = null;
  try {
    ctx = canvas.getContext('2d');
  } catch {
    return;
  }
  if (!ctx) return;
  // Scaffolding: render a ticking square to indicate live byte traffic.
  // Replace with real SPICE frame decoding when the client is wired.
  const w = canvas.width;
  const h = canvas.height;
  const size = 12;
  const tick = (typeof performance !== 'undefined' ? performance.now() : Date.now()) % 2000;
  const x = Math.floor(((tick / 2000) * (w - size)) | 0);
  ctx.fillStyle = '#0a0a0a';
  ctx.fillRect(0, 0, w, h);
  ctx.fillStyle = '#60a5fa';
  ctx.fillRect(x, h / 2 - size / 2, size, size);
  // Silence unused-param lint.
  void data;
}

export default SpiceConsole;
