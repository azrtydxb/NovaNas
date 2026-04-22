/**
 * Small WebSocket connection-state indicator (issue #4).
 *
 * Renders a colored dot in the topbar:
 *   green  → open
 *   amber  → connecting
 *   red    → closed / error
 *
 * Click forces a reconnect by tearing down and re-creating the shared client.
 */
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useWsStatus } from '@/hooks/use-ws';
import { __setWsClientForTests, getWsClient } from '@/lib/ws';
import { Trans } from '@lingui/react';

function toneClass(status: ReturnType<typeof useWsStatus>): string {
  switch (status) {
    case 'open':
      return 'bg-success shadow-[0_0_0_2px_var(--bg-2),0_0_6px_var(--ok)]';
    case 'connecting':
      return 'bg-warning shadow-[0_0_0_2px_var(--bg-2),0_0_6px_var(--warn)]';
    default:
      return 'bg-danger shadow-[0_0_0_2px_var(--bg-2),0_0_6px_var(--err)]';
  }
}

function statusLabel(status: ReturnType<typeof useWsStatus>): string {
  switch (status) {
    case 'open':
      return 'Live updates connected';
    case 'connecting':
      return 'Connecting…';
    case 'closed':
      return 'Disconnected — click to reconnect';
    case 'error':
      return 'Connection error — click to reconnect';
    default:
      return status;
  }
}

export function WsStatusDot() {
  const status = useWsStatus();
  const reconnect = () => {
    try {
      getWsClient().close();
    } catch {
      // ignore
    }
    // Drop the singleton so the next getWsClient() call re-creates it.
    __setWsClientForTests(null);
    // Warm up a fresh client immediately.
    getWsClient();
  };
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type='button'
          onClick={reconnect}
          aria-label={`WebSocket: ${status}`}
          className='w-[30px] h-[30px] grid place-items-center rounded-md hover:bg-elevated'
        >
          <span className={`inline-block h-2 w-2 rounded-full ${toneClass(status)}`} />
        </button>
      </TooltipTrigger>
      <TooltipContent side='bottom'>
        <Trans id={statusLabel(status)} />
      </TooltipContent>
    </Tooltip>
  );
}
