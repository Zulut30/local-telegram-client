import { useMemo, useState } from 'react';
import type { OutboundCall, Trace } from '../types';

interface TracePanelProps {
  traces: Trace[];
  clearing: boolean;
  onClear: () => Promise<void>;
}

function traceTitle(trace: Trace): string {
  if (trace.inbound) {
    const inboundType = trace.inbound.type === 'message' ? 'сообщение' : trace.inbound.type;
    return `${inboundType} #${trace.inbound.update_id}`;
  }
  return trace.orphan ? 'вызов без update' : trace.id;
}

function traceSubtitle(trace: Trace): string {
  if (trace.inbound) {
    return `чат ${trace.inbound.chat_id}${trace.inbound.text ? ` - ${trace.inbound.text}` : ''}`;
  }
  return (trace.calls ?? [])[0]?.method ?? 'нет вызовов';
}

function statusClass(status: string): string {
  return status === 'error' ? 'trace-card__status trace-card__status--error' : 'trace-card__status';
}

function callSummary(call: OutboundCall): string {
  if (call.ok) {
    return `${call.http_status} - ${call.latency_ms}ms`;
  }
  return `${call.http_status} - ${call.error_desc ?? 'ошибка'}`;
}

function statusLabel(status: Trace['status']): string {
  switch (status) {
    case 'open':
      return 'в работе';
    case 'error':
      return 'ошибка';
    default:
      return 'успешно';
  }
}

function ParamsPreview({ params }: { params?: Record<string, unknown> }) {
  if (!params || Object.keys(params).length === 0) {
    return null;
  }
  return <pre className="trace-call__params">{JSON.stringify(params, null, 2)}</pre>;
}

function traceLogPayload(traces: Trace[]): string {
  return JSON.stringify(
    {
      copied_at: new Date().toISOString(),
      trace_count: traces.length,
      traces,
    },
    null,
    2,
  );
}

async function copyText(text: string): Promise<void> {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }

  const area = document.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', 'true');
  area.style.position = 'fixed';
  area.style.left = '-9999px';
  document.body.appendChild(area);
  area.select();
  const copied = document.execCommand('copy');
  document.body.removeChild(area);
  if (!copied) {
    throw new Error('Не удалось скопировать текст');
  }
}

export function TracePanel({ traces, clearing, onClear }: TracePanelProps) {
  const ordered = [...traces].reverse();
  const [copyStatus, setCopyStatus] = useState<'idle' | 'copying' | 'copied' | 'failed'>('idle');
  const logPayload = useMemo(() => traceLogPayload(ordered), [ordered]);

  async function copyLogs() {
    setCopyStatus('copying');
    try {
      await copyText(logPayload);
      setCopyStatus('copied');
      window.setTimeout(() => setCopyStatus('idle'), 1800);
    } catch {
      setCopyStatus('failed');
      window.setTimeout(() => setCopyStatus('idle'), 2400);
    }
  }

  return (
    <aside className="trace-panel" aria-label="Поток trace-событий">
      <header className="trace-panel__header">
        <div>
          <p className="eyebrow">Консоль Bot API</p>
          <h2>Консоль</h2>
          <p>Каждая карточка связывает user update с Bot API вызовами, которые сделал бот.</p>
        </div>
        <div className="trace-panel__actions">
          <span>{ordered.length}</span>
          <button className="trace-panel__copy" type="button" onClick={copyLogs}>
            {copyStatus === 'copying'
              ? 'Копирую...'
              : copyStatus === 'copied'
                ? 'Скопировано'
                : copyStatus === 'failed'
                  ? 'Ошибка'
                  : 'Копировать логи'}
          </button>
          <button className="trace-panel__clear" type="button" disabled={clearing || ordered.length === 0} onClick={() => void onClear()}>
            {clearing ? 'Очистка...' : 'Очистить'}
          </button>
        </div>
      </header>
      <div className="trace-panel__body">
        <div className="trace-panel__legend" aria-label="Легенда trace">
          <span>в работе = бот обрабатывает update</span>
          <span>успешно = запрос выполнен</span>
          <span>ошибка = проверьте параметры</span>
        </div>
        {ordered.length === 0 ? (
          <div className="empty empty--compact">
            <strong>Trace пока пустой</strong>
            <span>Отправьте /start в чат. Вызовы бота появятся здесь.</span>
          </div>
        ) : null}
        {ordered.map((trace, index) => (
          <details className="trace-card" key={trace.id} open={index === 0 || trace.status === 'open' || trace.status === 'error'}>
            <summary>
              <span>
                <strong>{traceTitle(trace)}</strong>
                <small>{traceSubtitle(trace)}</small>
              </span>
              <span className={statusClass(trace.status)}>{statusLabel(trace.status)}</span>
            </summary>
            <div className="trace-card__body">
              <div className="trace-card__meta">
                <span>{trace.correlation}</span>
                {trace.orphan ? <span>без update</span> : null}
              </div>
              {(trace.calls ?? []).length === 0 ? <p className="trace-card__empty">Ждем вызовы бота</p> : null}
              {(trace.calls ?? []).map((call, index) => (
                <div className={call.ok ? 'trace-call' : 'trace-call trace-call--error'} key={`${trace.id}-${call.method}-${index}`}>
                  <div className="trace-call__top">
                    <strong>{call.method}</strong>
                    <span>{callSummary(call)}</span>
                  </div>
                  <div className="trace-card__meta">
                    <span>{call.correlation}</span>
                    {!call.ok && call.error_code ? <span>ошибка {call.error_code}</span> : null}
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
