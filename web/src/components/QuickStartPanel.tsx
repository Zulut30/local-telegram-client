interface QuickStartPanelProps {
  hasMessages: boolean;
  traceCount: number;
  resetting: boolean;
  onSend: (text: string) => Promise<void>;
  onReset: () => Promise<void>;
}

const quickActions = [
  { label: 'Отправить /start', text: '/start' },
  { label: 'Пинг', text: 'Пинг' },
  { label: 'Ошибка trace', text: 'Ошибка trace' },
];

export function QuickStartPanel({ hasMessages, traceCount, resetting, onSend, onReset }: QuickStartPanelProps) {
  return (
    <section className="quick-start" aria-label="Быстрый старт">
      <div className="quick-start__main">
        <p className="eyebrow">Демо-панель</p>
        <h2>Проверьте инструменты бота</h2>
        <p>
          Отправьте update от пользователя. Ответы бота останутся в чате, а Bot API вызовы появятся в консоли.
        </p>
      </div>
      <div className="quick-start__steps" aria-label="Шаги демо">
        <span><b>1</b> Сообщение</span>
        <span><b>2</b> Кнопки</span>
        <span><b>3</b> Trace</span>
      </div>
      <div className="quick-start__actions">
        {quickActions.map((action) => (
          <button key={action.text} type="button" onClick={() => void onSend(action.text)}>
            {action.label}
          </button>
        ))}
        <button className="quick-start__reset" type="button" disabled={resetting} onClick={() => void onReset()}>
          {resetting ? 'Сбрасываю...' : 'Сбросить сессию'}
        </button>
      </div>
      <div className="quick-start__stats">
        <span>{hasMessages ? 'В чате есть сообщения' : 'Чат пуст'}</span>
        <span>{traceCount} trace</span>
      </div>
    </section>
  );
}
