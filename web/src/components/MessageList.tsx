import { Keyboard } from './Keyboard';
import type { Message } from '../types';

interface MessageListProps {
  messages: Message[];
  onCallback: (message: Message, data: string) => Promise<void>;
  onReplyText: (text: string) => Promise<void>;
}

function senderName(message: Message): string {
  if (message.from?.is_bot) {
    return message.from.username || message.from.first_name || 'Бот';
  }
  return message.from?.first_name || message.chat.first_name || 'Вы';
}

function messageTime(message: Message): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(message.date * 1000));
}

export function MessageList({ messages, onCallback, onReplyText }: MessageListProps) {
  return (
    <div className="messages" aria-live="polite">
      {messages.length === 0 ? (
        <div className="empty empty--chat">
          <strong>Здесь пока нет сообщений</strong>
          <span>Нажмите «Отправить /start» или напишите сообщение ниже.</span>
        </div>
      ) : null}
      {messages.map((message) => {
        const fromBot = Boolean(message.from?.is_bot);
        const body = message.text || message.caption;
        return (
          <article className={fromBot ? 'message message--bot' : 'message message--user'} key={message.message_id}>
            <div className="message__meta">
              <strong>{senderName(message)}</strong>
              <time dateTime={new Date(message.date * 1000).toISOString()}>{messageTime(message)}</time>
            </div>
            {message.photo_url ? <img className="message__photo" src={message.photo_url} alt={body || 'Фото Telegram'} /> : null}
            {body ? <p>{body}</p> : null}
            <Keyboard message={message} markup={message.reply_markup} onCallback={onCallback} onReplyText={onReplyText} />
          </article>
        );
      })}
    </div>
  );
}
