import { useEffect, useState } from 'react';
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
  const [clearingTraces, setClearingTraces] = useState(false);
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    try {
      return window.localStorage.getItem('sim-theme') === 'dark' ? 'dark' : 'light';
    } catch {
      return 'light';
    }
  });

  useEffect(() => {
    try {
      window.localStorage.setItem('sim-theme', theme);
    } catch {
      /* localStorage may be unavailable; theme simply won't persist */
    }
  }, [theme]);
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

  async function clearConsole() {
    if (clearingTraces) {
      return;
    }
    setClearingTraces(true);
    try {
      await traceState.clear();
    } finally {
      setClearingTraces(false);
    }
  }

  const statusLabel = sim.status === 'live' ? 'онлайн' : sim.status === 'offline' ? 'офлайн' : 'подключение';
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
      <header className="ide-toolbar" aria-label="Панель рабочей области">
        <div className="ide-toolbar__brand">
          <strong>Local Telegram IDE</strong>
          <span>Рабочее место для тестирования ботов</span>
        </div>
        <div className="ide-toolbar__actions">
          <button className={showChats ? 'is-active' : ''} type="button" onClick={() => setShowChats((value) => !value)}>
            Чаты
          </button>
          <button className={showControls ? 'is-active' : ''} type="button" onClick={() => setShowControls((value) => !value)}>
            Гайд
          </button>
          <button className={showConsole ? 'is-active' : ''} type="button" onClick={() => setShowConsole((value) => !value)}>
            Консоль
          </button>
          <button type="button" onClick={() => setTheme((value) => (value === 'light' ? 'dark' : 'light'))}>
            {theme === 'light' ? 'Темная' : 'Светлая'}
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
        <section className={conversationClass} aria-label="Чат">
          <header className="conversation__header">
            <div>
              <p className="eyebrow">Local Telegram</p>
              <h1>Эмулятор бота-рецептов</h1>
              <p className="conversation__subtitle">Чат разработчика с витринным ботом для проверки Telegram Bot API</p>
            </div>
            <div className="conversation__status">
              {sim.callbackNotice ? <span className="status status--notice">{sim.callbackNotice}</span> : null}
              {sim.error ? <span className="status status--error">{sim.error}</span> : <span className="status">{statusLabel}</span>}
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
          <MessageList
            messages={sim.selectedMessages}
            chatAction={sim.selectedChatAction}
            draft={sim.selectedDraft}
            onCallback={sim.sendCallback}
            onReplyText={sim.sendText}
          />
          <Composer onSend={sim.sendText} onPhoto={sim.sendPhoto} />
        </section>
        {showConsole ? <TracePanel traces={traceState.traces} clearing={clearingTraces} onClear={clearConsole} /> : null}
      </div>
    </main>
  );
}
