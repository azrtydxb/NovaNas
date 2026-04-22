import { Button } from '@/components/ui/button';
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import type { Vm } from '@novanas/schemas';
import RFB from '@novnc/novnc/lib/rfb';
import { Clipboard, Loader2, Maximize2, Minimize2, Power, WifiOff } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * VNC console component.
 *
 * Uses noVNC's `RFB` class (`@novnc/novnc`) to render a full RFB/VNC session
 * into a ref'd div. The WebSocket proxy at
 * `/api/v1/vms/:namespace/:name/console?type=vnc` forwards to KubeVirt's
 * `virtualmachineinstances/<name>/vnc` subresource.
 *
 * @remarks Replaces the prior SPICE scaffold — spice-html5 is unpublished on
 * npm. noVNC is the maintained de-facto browser VNC client and KubeVirt's
 * native graphics console protocol is VNC, so no upstream changes are needed.
 */

export type RfbFactory = (
  target: HTMLElement,
  url: string
) => {
  addEventListener(type: string, listener: (ev: CustomEvent) => void): void;
  removeEventListener(type: string, listener: (ev: CustomEvent) => void): void;
  disconnect(): void;
  sendCtrlAltDel(): void;
  clipboardPasteFrom(text: string): void;
  scaleViewport: boolean;
  resizeSession: boolean;
  focusOnClick: boolean;
  focus(): void;
};

interface VncConsoleProps {
  vm: Vm;
  /** Override the websocket URL (tests). */
  wsUrl?: string;
  /** Override the RFB factory (tests). */
  rfbFactory?: RfbFactory;
}

type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'error';

const defaultRfbFactory: RfbFactory = (target, url) =>
  new RFB(target, url) as unknown as ReturnType<RfbFactory>;

export function VncConsole({ vm, wsUrl, rfbFactory }: VncConsoleProps) {
  const canvasRef = useRef<HTMLDivElement | null>(null);
  const rfbRef = useRef<ReturnType<RfbFactory> | null>(null);
  const [state, setState] = useState<ConnectionState>('connecting');
  const [lastMessage, setLastMessage] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);
  const [fullscreen, setFullscreen] = useState(false);

  const namespace = vm.metadata.namespace ?? '';
  const name = vm.metadata.name;

  useEffect(() => {
    // `nonce` is intentionally in the dep list — bumping it triggers reconnect.
    void nonce;
    const target = canvasRef.current;
    if (!target) return;

    const url =
      wsUrl ??
      `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}` +
        `/api/v1/vms/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/console?type=vnc`;

    setState('connecting');
    setLastMessage(null);

    let rfb: ReturnType<RfbFactory>;
    try {
      rfb = (rfbFactory ?? defaultRfbFactory)(target, url);
    } catch (err) {
      setLastMessage((err as Error)?.message ?? i18n._('connection failed'));
      setState('error');
      return;
    }
    rfb.scaleViewport = true;
    rfb.resizeSession = false;
    rfb.focusOnClick = true;
    rfbRef.current = rfb;

    const onConnect = () => {
      setState('connected');
      setLastMessage(null);
    };
    const onDisconnect = (evt: CustomEvent) => {
      const detail = (evt as CustomEvent<{ clean?: boolean }>).detail;
      setState(detail?.clean ? 'disconnected' : 'error');
      if (!detail?.clean) setLastMessage(i18n._('connection lost'));
    };
    const onCredentialsRequired = () => {
      setState('error');
      setLastMessage(i18n._('credentials required'));
    };
    const onSecurityFailure = (evt: CustomEvent) => {
      const detail = (evt as CustomEvent<{ reason?: string }>).detail;
      setState('error');
      setLastMessage(detail?.reason ?? i18n._('security failure'));
    };

    rfb.addEventListener('connect', onConnect);
    rfb.addEventListener('disconnect', onDisconnect);
    rfb.addEventListener('credentialsrequired', onCredentialsRequired);
    rfb.addEventListener('securityfailure', onSecurityFailure);

    return () => {
      rfb.removeEventListener('connect', onConnect);
      rfb.removeEventListener('disconnect', onDisconnect);
      rfb.removeEventListener('credentialsrequired', onCredentialsRequired);
      rfb.removeEventListener('securityfailure', onSecurityFailure);
      try {
        rfb.disconnect();
      } catch {
        /* ignore */
      }
      rfbRef.current = null;
    };
  }, [namespace, name, wsUrl, rfbFactory, nonce]);

  // Forward browser clipboard paste events into the guest.
  useEffect(() => {
    const onPaste = (e: ClipboardEvent) => {
      const rfb = rfbRef.current;
      if (!rfb) return;
      const text = e.clipboardData?.getData('text/plain');
      if (text) {
        try {
          rfb.clipboardPasteFrom(text);
        } catch {
          /* ignore */
        }
      }
    };
    window.addEventListener('paste', onPaste);
    return () => window.removeEventListener('paste', onPaste);
  }, []);

  const retry = () => setNonce((n) => n + 1);

  const sendCtrlAltDel = useCallback(() => {
    try {
      rfbRef.current?.sendCtrlAltDel();
    } catch {
      /* ignore */
    }
  }, []);

  const toggleFullscreen = useCallback(() => {
    const el = canvasRef.current?.parentElement;
    if (!el) return;
    if (!document.fullscreenElement) {
      el.requestFullscreen?.()
        .then(() => setFullscreen(true))
        .catch(() => {
          /* ignore */
        });
    } else {
      document
        .exitFullscreen?.()
        .then(() => setFullscreen(false))
        .catch(() => {
          /* ignore */
        });
    }
  }, []);

  return (
    <div className='flex flex-col gap-2'>
      <StateBanner state={state} lastMessage={lastMessage} onRetry={retry} />
      <div className='relative bg-black border border-border rounded-sm overflow-hidden min-h-[240px] h-full w-full'>
        <div
          ref={canvasRef}
          className='h-full w-full outline-none'
          aria-label={`${i18n._('VNC console for')} ${name}`}
        />
        <div className='absolute top-1 right-1 flex gap-1 pointer-events-auto'>
          <Button
            size='sm'
            variant='ghost'
            title={i18n._('Send Ctrl+Alt+Del')}
            onClick={sendCtrlAltDel}
            disabled={state !== 'connected'}
          >
            <Power size={11} /> Ctrl-Alt-Del
          </Button>
          <Button size='sm' variant='ghost' title={i18n._('Fullscreen')} onClick={toggleFullscreen}>
            {fullscreen ? <Minimize2 size={11} /> : <Maximize2 size={11} />}
          </Button>
        </div>
      </div>
      <div className='text-xs text-foreground-subtle flex items-center gap-2'>
        <Clipboard size={11} /> <Trans id='Paste (Ctrl/Cmd+V) forwards clipboard to guest.' />
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
        <span className='inline-block h-1.5 w-1.5 rounded-full bg-ok' /> <Trans id='connected' />
      </div>
    );
  }
  if (state === 'connecting') {
    return (
      <div className='text-[11px] mono flex items-center gap-2 text-foreground-subtle'>
        <Loader2 size={11} className='animate-spin' /> <Trans id='connecting…' />
      </div>
    );
  }
  return (
    <div className='text-[11px] mono flex items-center gap-2 text-warn'>
      <WifiOff size={11} /> {state === 'error' ? i18n._('error') : i18n._('disconnected')}
      {lastMessage ? ` — ${lastMessage}` : ''}
      <Button size='sm' variant='ghost' onClick={onRetry}>
        <Trans id='Reconnect' />
      </Button>
    </div>
  );
}

/**
 * @deprecated Use {@link VncConsole}. SPICE was never fully wired; the
 * console now uses noVNC against KubeVirt's VNC subresource.
 */
export const SpiceConsole = VncConsole;

export default VncConsole;
