import type { Chat } from '../types';

interface ChatListProps {
  chats: Chat[];
  selectedChatID: number;
  status: 'connecting' | 'live' | 'offline';
  onSelect: (chatID: number) => void;
}

function chatTitle(chat: Chat): string {
  return chat.first_name || chat.username || `Chat ${chat.id}`;
}

export function ChatList({ chats, selectedChatID, status, onSelect }: ChatListProps) {
  return (
    <aside className="chat-list" aria-label="Chats">
      <div className="chat-list__brand">
        <strong>Chats</strong>
        <span data-state={status}>{status}</span>
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
              <small>{chat.type}</small>
            </span>
          </button>
        ))}
      </nav>
    </aside>
  );
}
