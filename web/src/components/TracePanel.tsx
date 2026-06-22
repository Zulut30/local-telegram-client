import type { OutboundCall, Trace } from '../types';

interface TracePanelProps {
  traces: Trace[];
}

function traceTitle(trace: Trace): string {
  if (trace.inbound) {
    return `${trace.inbound.type} #${trace.inbound.update_id}`;
  }
  return trace.orphan ? 'orphan call' : trace.id;
}

function traceSubtitle(trace: Trace): string {
  if (trace.inbound) {
    return `chat ${trace.inbound.chat_id}${trace.inbound.text ? ` · ${trace.inbound.text}` : ''}`;
  }
  return (trace.calls ?? [])[0]?.method ?? 'no calls';
}

function statusClass(status: string): string {
  return status === 'error' ? 'trace-card__status trace-card__status--error' : 'trace-card__status';
}

function callSummary(call: OutboundCall): string {
  if (call.ok) {
    return `${call.http_status} · ${call.latency_ms}ms`;
  }
  return `${call.http_status} · ${call.error_desc ?? 'error'}`;
}

function ParamsPreview({ params }: { params?: Record<string, unknown> }) {
  if (!params || Object.keys(params).length === 0) {
    return null;
  }
  return <pre className="trace-call__params">{JSON.stringify(params, null, 2)}</pre>;
}

export function TracePanel({ traces }: TracePanelProps) {
  const ordered = [...traces].reverse();

  return (
    <aside className="trace-panel" aria-label="Trace stream">
      <header className="trace-panel__header">
        <div>
          <p className="eyebrow">X-ray</p>
          <h2>Traces</h2>
        </div>
        <span>{ordered.length}</span>
      </header>
      <div className="trace-panel__body">
        {ordered.length === 0 ? <div className="empty empty--compact">No traces yet</div> : null}
        {ordered.map((trace) => (
          <details className="trace-card" key={trace.id} open={trace.status === 'open' || trace.status === 'error'}>
            <summary>
              <span>
                <strong>{traceTitle(trace)}</strong>
                <small>{traceSubtitle(trace)}</small>
              </span>
              <span className={statusClass(trace.status)}>{trace.status}</span>
            </summary>
            <div className="trace-card__body">
              <div className="trace-card__meta">
                <span>{trace.correlation}</span>
                {trace.orphan ? <span>orphan</span> : null}
              </div>
              {(trace.calls ?? []).length === 0 ? <p className="trace-card__empty">Waiting for bot calls</p> : null}
              {(trace.calls ?? []).map((call, index) => (
                <div className={call.ok ? 'trace-call' : 'trace-call trace-call--error'} key={`${trace.id}-${call.method}-${index}`}>
                  <div className="trace-call__top">
                    <strong>{call.method}</strong>
                    <span>{callSummary(call)}</span>
                  </div>
                  <div className="trace-card__meta">
                    <span>{call.correlation}</span>
                    {!call.ok && call.error_code ? <span>error {call.error_code}</span> : null}
                  </div>
                  <ParamsPreview params={call.params} />
                </div>
              ))}
            </div>
          </details>
        ))}
      </div>
    </aside>
  );
}
