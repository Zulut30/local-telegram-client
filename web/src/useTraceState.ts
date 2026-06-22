import { useEffect, useState } from 'react';
import { loadTraces } from './api';
import type { Trace, TraceEventPayload } from './types';

const maxTraces = 1000;

function upsertTrace(current: Trace[], trace: Trace): Trace[] {
  const withoutCurrent = current.filter((item) => item.id !== trace.id);
  return [...withoutCurrent, trace].slice(-maxTraces);
}

export function useTraceState() {
  const [traces, setTraces] = useState<Trace[]>([]);

  useEffect(() => {
    const controller = new AbortController();
    loadTraces(controller.signal)
      .then((snapshot) => setTraces(snapshot.slice(-maxTraces)))
      .catch(() => undefined);
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const source = new EventSource('/_sim/events');
    source.addEventListener('trace', (event) => {
      const payload = JSON.parse(event.data) as TraceEventPayload;
      setTraces((current) => upsertTrace(current, payload.trace));
    });
    return () => source.close();
  }, []);

  return traces;
}
