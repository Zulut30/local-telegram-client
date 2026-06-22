import { ChatList } from './components/ChatList';
import { Composer } from './components/Composer';
import { MessageList } from './components/MessageList';
import { useSimState } from './useSimState';

export function App() {
  const sim = useSimState();

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
          {sim.error ? <span className="status status--error">{sim.error}</span> : <span className="status">{sim.status}</span>}
        </header>
        <MessageList messages={sim.selectedMessages} onCallback={sim.sendCallback} onReplyText={sim.sendText} />
        <Composer onSend={sim.sendText} />
      </section>
    </main>
  );
}
