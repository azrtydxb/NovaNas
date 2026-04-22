import { useEffect, useRef, useState } from 'react';
import { getWsClient } from '@/lib/ws';

export function useWsChannel<T = unknown>(
  channel: string | null,
  onEvent?: (data: T, event: string) => void
): { last: T | null; event: string | null } {
  const [last, setLast] = useState<T | null>(null);
  const [event, setEvent] = useState<string | null>(null);
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

  return { last, event };
}
