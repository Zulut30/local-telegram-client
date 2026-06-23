import type { Chat } from '../types';

interface ChatListProps {
  chats: Chat[];
  selectedChatID: number;
  status: 'connecting' | 'live' | 'offline';
  onSelect: (chatID: number) => void;
}

function chatTitle(chat: Chat): string {
  return chat.first_name || chat.username || `Чат ${chat.id}`;
}

function statusLabel(status: ChatListProps['status']): string {
  switch (status) {
    case 'live':
      return 'онлайн';
    case 'offline':
      return 'офлайн';
    default:
      return 'подключение';
  }
}

function chatTypeLabel(type: string): string {
  return type === 'private' ? 'личный чат' : type;
}

export function ChatList({ chats, selectedChatID, status, onSelect }: ChatListProps) {
  return (
    <aside className="chat-list" aria-label="Чаты">
      <div className="chat-list__brand">
        <span>
          <strong>Local Telegram</strong>
          <small>Тестовые чаты</small>
        </span>
        <span className="chat-list__state" data-state={status}>{statusLabel(status)}</span>
      </div>
      <div className="chat-list__hint">
        Выберите сессию пользователя, которую хотите проверить.
      </div>
      <nav className="chat-list__items">
        {chats.map((chat) => (
          <button
            className={chat.id === selectedChatID ? 'chat-list__item chat-list__item--active' : 'chat-list__item'}
            key={chat.id}
            type="button"
            onClick={() => onSelect(chat.id)}
          >
            <span className="chat-list__avatar">{chatTitle(chat).slice(0, 1).toUpperCase()}</span>
            <span>
              <strong>{chatTitle(chat)}</strong>
              <small>{chatTypeLabel(chat.type)}</small>
            </span>
          </button>
        ))}
      </nav>
    </aside>
  );
}
