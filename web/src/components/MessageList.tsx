import { Keyboard } from './Keyboard';
import { FormattedText, RichMessageView } from './FormattedText';
import type { ChatActionEventPayload, Message, MessageDraftEventPayload } from '../types';

interface MessageListProps {
  messages: Message[];
  chatAction?: ChatActionEventPayload;
  draft?: MessageDraftEventPayload;
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

function actionLabel(action: string): string {
  switch (action) {
    case 'typing':
      return 'печатает';
    case 'upload_photo':
      return 'загружает фото';
    case 'record_video':
      return 'записывает видео';
    case 'upload_video':
      return 'загружает видео';
    case 'record_voice':
      return 'записывает голос';
    case 'upload_voice':
      return 'загружает голос';
    case 'upload_document':
      return 'загружает документ';
    case 'choose_sticker':
      return 'выбирает стикер';
    case 'find_location':
      return 'ищет локацию';
    case 'record_video_note':
      return 'записывает видеосообщение';
    case 'upload_video_note':
      return 'загружает видеосообщение';
    default:
      return action;
  }
}

function MessageBody({ message }: { message: Message }) {
  const body = message.text || message.caption;
  const entities = message.text ? message.entities : message.caption_entities;
  const parseMode = message.text ? message.parse_mode : message.caption_parse_mode;
  const mediaLabel = message.media_kind?.replaceAll('_', ' ');
  return (
    <>
      {message.media_kind && !message.photo_url && message.media_url ? (
        <a className="message__media-kind" href={message.media_url} rel="noreferrer" target="_blank">
          {mediaLabel}
        </a>
      ) : null}
      {message.media_kind && !message.photo_url && !message.media_url ? <div className="message__media-kind">{mediaLabel}</div> : null}
      {message.photo_url ? <img className="message__photo" src={message.photo_url} alt={body || 'Фото Telegram'} /> : null}
      {message.rich_message ? <RichMessageView value={message.rich_message} /> : null}
      {body ? (
        <p className="formatted-text">
          <FormattedText entities={entities} parseMode={parseMode} text={body} />
        </p>
      ) : null}
    </>
  );
}

export function MessageList({ messages, chatAction, draft, onCallback, onReplyText }: MessageListProps) {
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
        return (
          <article className={fromBot ? 'message message--bot' : 'message message--user'} key={message.message_id}>
            <div className="message__meta">
              <strong>{senderName(message)}</strong>
              <time dateTime={new Date(message.date * 1000).toISOString()}>{messageTime(message)}</time>
            </div>
            <MessageBody message={message} />
            <Keyboard message={message} markup={message.reply_markup} onCallback={onCallback} onReplyText={onReplyText} />
          </article>
        );
      })}
      {draft ? (
        <article className="message message--bot message--draft">
          <div className="message__meta">
            <strong>Бот печатает</strong>
            <span className="message__typing-dots"><i /><i /><i /></span>
          </div>
          {draft.rich_message ? <RichMessageView value={draft.rich_message} /> : null}
          {draft.text ? (
            <p className="formatted-text">
              <FormattedText entities={draft.entities} parseMode={draft.parse_mode} text={draft.text} />
            </p>
          ) : null}
        </article>
      ) : chatAction ? (
        <div className="typing-indicator">
          <span className="message__typing-dots"><i /><i /><i /></span>
          <span>Бот {actionLabel(chatAction.action)}</span>
        </div>
      ) : null}
    </div>
  );
}
