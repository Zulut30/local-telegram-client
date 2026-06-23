import { Keyboard } from './Keyboard';
import type { Message } from '../types';

interface MessageListProps {
  messages: Message[];
  onCallback: (message: Message, data: string) => Promise<void>;
  onReplyText: (text: string) => Promise<void>;
}

function senderName(message: Message): string {
  if (message.from?.is_bot) {
    return message.from.username || message.from.first_name || 'Bot';
  }
  return message.from?.first_name || message.chat.first_name || 'You';
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
          <strong>No messages here yet</strong>
          <span>Press "Send /start" or type below to begin.</span>
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
            {message.photo_url ? <img className="message__photo" src={message.photo_url} alt={body || 'Telegram photo'} /> : null}
            {body ? <p>{body}</p> : null}
            <Keyboard message={message} markup={message.reply_markup} onCallback={onCallback} onReplyText={onReplyText} />
          </article>
        );
      })}
    </div>
  );
}
