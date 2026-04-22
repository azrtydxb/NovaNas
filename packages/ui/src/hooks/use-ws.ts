import { type WsStatus, getWsClient } from '@/lib/ws';
import { useEffect, useRef, useState } from 'react';

/**
 * Subscribe a React component to a single WS channel.
 *
 * The handler is stored in a ref so subscribing effects don't re-run on each
 * render — only the `channel` argument controls (un)subscription. Refcounting
 * at the `WsClient` layer ensures N consumers of the same channel share one
 * server-side subscription.
 */
export function useWsChannel<T = unknown>(
  channel: string | null | undefined,
  onEvent?: (data: T, event: string) => void
): { last: T | null; event: string | null; status: WsStatus } {
  const [last, setLast] = useState<T | null>(null);
  const [event, setEvent] = useState<string | null>(null);
  const [status, setStatus] = useState<WsStatus>(() => {
    try {
      return getWsClient().status;
    } catch {
      return 'closed';
    }
  });
  const handlerRef = useRef(onEvent);
  handlerRef.current = onEvent;

  useEffect(() => {
    if (!channel) return;
    const client = getWsClient();
    const unsub = client.subscribe(channel, (data, evt) => {
      setLast(data as T);
      setEvent(evt);
      handlerRef.current?.(data as T, evt);
    });
    return unsub;
  }, [channel]);

  useEffect(() => {
    const client = getWsClient();
    return client.onStatus(setStatus);
  }, []);

  return { last, event, status };
}

/** Standalone status subscription for UI indicators. */
export function useWsStatus(): WsStatus {
  const [status, setStatus] = useState<WsStatus>(() => {
    try {
      return getWsClient().status;
    } catch {
      return 'closed';
    }
  });
  useEffect(() => {
    const client = getWsClient();
    return client.onStatus(setStatus);
  }, []);
  return status;
}
