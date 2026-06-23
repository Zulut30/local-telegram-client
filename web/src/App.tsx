import { ChatList } from './components/ChatList';
import { Composer } from './components/Composer';
import { MessageList } from './components/MessageList';
import { TracePanel } from './components/TracePanel';
import { useSimState } from './useSimState';
import { useTraceState } from './useTraceState';

export function App() {
  const sim = useSimState();
  const traces = useTraceState();

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
          </div>
          <div className="conversation__status">
            {sim.callbackNotice ? <span className="status status--notice">{sim.callbackNotice}</span> : null}
            {sim.error ? <span className="status status--error">{sim.error}</span> : <span className="status">{sim.status}</span>}
          </div>
        </header>
        <MessageList messages={sim.selectedMessages} onCallback={sim.sendCallback} onReplyText={sim.sendText} />
        <Composer onSend={sim.sendText} />
      </section>
      <TracePanel traces={traces} />
    </main>
  );
}
