import { useState } from 'react';
import { ChatList } from './components/ChatList';
import { Composer } from './components/Composer';
import { MessageList } from './components/MessageList';
import { QuickStartPanel } from './components/QuickStartPanel';
import { TracePanel } from './components/TracePanel';
import { useSimState } from './useSimState';
import { useTraceState } from './useTraceState';

export function App() {
  const sim = useSimState();
  const traceState = useTraceState();
  const [resetting, setResetting] = useState(false);
  const [theme, setTheme] = useState<'light' | 'dark'>('light');
  const [showChats, setShowChats] = useState(true);
  const [showControls, setShowControls] = useState(true);
  const [showConsole, setShowConsole] = useState(true);

  async function resetEverything() {
    if (resetting) {
      return;
    }
    setResetting(true);
    try {
      await sim.reset();
      await traceState.refresh();
    } finally {
      setResetting(false);
    }
  }

  const workspaceClass = [
    'workspace',
    showChats ? '' : 'workspace--no-chats',
    showConsole ? '' : 'workspace--no-console',
  ]
    .filter(Boolean)
    .join(' ');
  const conversationClass = showControls ? 'conversation' : 'conversation conversation--no-controls';

  return (
    <main className="shell" data-theme={theme}>
      <header className="ide-toolbar" aria-label="Workspace controls">
        <div className="ide-toolbar__brand">
          <strong>Local Telegram IDE</strong>
          <span>Bot simulator workspace</span>
        </div>
        <div className="ide-toolbar__actions">
          <button className={showChats ? 'is-active' : ''} type="button" onClick={() => setShowChats((value) => !value)}>
            Chats
          </button>
          <button className={showControls ? 'is-active' : ''} type="button" onClick={() => setShowControls((value) => !value)}>
            Guide
          </button>
          <button className={showConsole ? 'is-active' : ''} type="button" onClick={() => setShowConsole((value) => !value)}>
            Console
          </button>
          <button type="button" onClick={() => setTheme((value) => (value === 'light' ? 'dark' : 'light'))}>
            {theme === 'light' ? 'Dark' : 'Light'}
          </button>
        </div>
      </header>
      <div className={workspaceClass}>
        {showChats ? (
          <ChatList
            chats={sim.chats}
            selectedChatID={sim.selectedChatID}
            status={sim.status}
            onSelect={sim.setSelectedChatID}
          />
        ) : null}
        <section className={conversationClass} aria-label="Chat">
          <header className="conversation__header">
            <div>
              <p className="eyebrow">Local Telegram</p>
              <h1>Recipe Bot Simulator</h1>
              <p className="conversation__subtitle">Developer chat with a food recipe showcase bot</p>
            </div>
            <div className="conversation__status">
              {sim.callbackNotice ? <span className="status status--notice">{sim.callbackNotice}</span> : null}
              {sim.error ? <span className="status status--error">{sim.error}</span> : <span className="status">{sim.status}</span>}
            </div>
          </header>
          {showControls ? (
            <QuickStartPanel
              hasMessages={sim.selectedMessages.length > 0}
              traceCount={traceState.traces.length}
              resetting={resetting}
              onSend={sim.sendText}
              onReset={resetEverything}
            />
          ) : null}
          <MessageList messages={sim.selectedMessages} onCallback={sim.sendCallback} onReplyText={sim.sendText} />
          <Composer onSend={sim.sendText} onPhoto={sim.sendPhoto} />
        </section>
        {showConsole ? <TracePanel traces={traceState.traces} /> : null}
      </div>
    </main>
  );
}
