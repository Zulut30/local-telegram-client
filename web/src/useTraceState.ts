import { useCallback, useEffect, useState } from 'react';
import { clearTraces, loadTraces } from './api';
import { withTokenURL } from './token';
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

  const clear = useCallback(async () => {
    await clearTraces();
    setTraces([]);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    refresh(controller.signal).catch(() => undefined);
    return () => controller.abort();
  }, [refresh]);

  useEffect(() => {
    const source = new EventSource(withTokenURL('/_sim/events'));
    let opened = false;
    source.addEventListener('open', () => {
      // Re-sync the trace ring on reconnect so nothing is lost across the gap.
      if (opened) {
        refresh().catch(() => undefined);
      }
      opened = true;
    });
    source.addEventListener('trace', (event) => {
      let payload: TraceEventPayload;
      try {
        payload = JSON.parse(event.data) as TraceEventPayload;
      } catch {
        return;
      }
      setTraces((current) => upsertTrace(current, payload.trace));
    });
    return () => source.close();
  }, [refresh]);

  return { traces, refresh, clear };
}
