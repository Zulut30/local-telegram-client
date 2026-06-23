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

  return (
    <main className="shell">
      <ChatList
        chats={sim.chats}
        selectedChatID={sim.selectedChatID}
        status={sim.status}
        onSelect={sim.setSelectedChatID}
      />
      <section className="conversation" aria-label="Chat">
        <header className="conversation__header">
          <div>
            <p className="eyebrow">Local Telegram</p>
            <h1>Bot Simulator</h1>
            <p className="conversation__subtitle">Developer chat with the showcase bot</p>
          </div>
          <div className="conversation__status">
            {sim.callbackNotice ? <span className="status status--notice">{sim.callbackNotice}</span> : null}
            {sim.error ? <span className="status status--error">{sim.error}</span> : <span className="status">{sim.status}</span>}
          </div>
        </header>
        <QuickStartPanel
          hasMessages={sim.selectedMessages.length > 0}
          traceCount={traceState.traces.length}
          resetting={resetting}
          onSend={sim.sendText}
          onReset={resetEverything}
        />
        <MessageList messages={sim.selectedMessages} onCallback={sim.sendCallback} onReplyText={sim.sendText} />
        <Composer onSend={sim.sendText} />
      </section>
      <TracePanel traces={traceState.traces} />
    </main>
  );
}
