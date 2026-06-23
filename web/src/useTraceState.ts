import { useCallback, useEffect, useState } from 'react';
import { loadTraces } from './api';
import type { Trace, TraceEventPayload } from './types';

const maxTraces = 1000;

function upsertTrace(current: Trace[], trace: Trace): Trace[] {
  const withoutCurrent = current.filter((item) => item.id !== trace.id);
  return [...withoutCurrent, trace].slice(-maxTraces);
}

export function useTraceState() {
  const [traces, setTraces] = useState<Trace[]>([]);

  const refresh = useCallback(async (signal?: AbortSignal) => {
    const snapshot = await loadTraces(signal);
    setTraces(snapshot.slice(-maxTraces));
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    refresh(controller.signal).catch(() => undefined);
    return () => controller.abort();
  }, [refresh]);

  useEffect(() => {
    const source = new EventSource('/_sim/events');
    source.addEventListener('trace', (event) => {
      const payload = JSON.parse(event.data) as TraceEventPayload;
      setTraces((current) => upsertTrace(current, payload.trace));
    });
    return () => source.close();
  }, []);

  return { traces, refresh };
}
