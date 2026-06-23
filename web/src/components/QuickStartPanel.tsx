interface QuickStartPanelProps {
  hasMessages: boolean;
  traceCount: number;
  resetting: boolean;
  onSend: (text: string) => Promise<void>;
  onReset: () => Promise<void>;
}

const quickActions = [
  { label: 'Send /start', text: '/start' },
  { label: 'Ping', text: 'Ping' },
  { label: 'Trace error', text: 'Trace error' },
];

export function QuickStartPanel({ hasMessages, traceCount, resetting, onSend, onReset }: QuickStartPanelProps) {
  return (
    <section className="quick-start" aria-label="Quick start">
      <div className="quick-start__main">
        <p className="eyebrow">Demo controls</p>
        <h2>Try the bot tools</h2>
        <p>
          Send a user update here. Bot replies stay in the chat, and Bot API calls appear in Traces.
        </p>
      </div>
      <div className="quick-start__steps" aria-label="Demo steps">
        <span><b>1</b> Message</span>
        <span><b>2</b> Buttons</span>
        <span><b>3</b> Trace</span>
      </div>
      <div className="quick-start__actions">
        {quickActions.map((action) => (
          <button key={action.text} type="button" onClick={() => void onSend(action.text)}>
            {action.label}
          </button>
        ))}
        <button className="quick-start__reset" type="button" disabled={resetting} onClick={() => void onReset()}>
          {resetting ? 'Resetting...' : 'Reset session'}
        </button>
      </div>
      <div className="quick-start__stats">
        <span>{hasMessages ? 'Chat has messages' : 'Chat is empty'}</span>
        <span>{traceCount} traces</span>
      </div>
    </section>
  );
}
