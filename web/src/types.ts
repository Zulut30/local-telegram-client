export interface User {
  id: number;
  is_bot: boolean;
  first_name: string;
  last_name?: string;
  username?: string;
}

export interface Chat {
  id: number;
  type: string;
  username?: string;
  first_name?: string;
  last_name?: string;
}

export interface InlineKeyboardButton {
  text: string;
  callback_data?: string;
  url?: string;
}

export type ReplyKeyboardButton = string | { text: string };

export interface ReplyMarkup {
  inline_keyboard?: InlineKeyboardButton[][];
  keyboard?: ReplyKeyboardButton[][];
}

export interface Message {
  message_id: number;
  from?: User;
  chat: Chat;
  date: number;
  text?: string;
  entities?: MessageEntity[];
  parse_mode?: string;
  caption?: string;
  caption_entities?: MessageEntity[];
  caption_parse_mode?: string;
  photo?: PhotoSize[];
  photo_url?: string;
  media_kind?: string;
  media_url?: string;
  rich_message?: unknown;
  reply_markup?: ReplyMarkup;
}

export interface MessageEntity {
  type: string;
  offset: number;
  length: number;
  url?: string;
  user?: User;
  language?: string;
  custom_emoji_id?: string;
  unix_time?: number;
  date_time_format?: string;
  alternative_text?: string;
}

export interface PhotoSize {
  file_id: string;
  file_unique_id: string;
  width: number;
  height: number;
  file_size?: number;
}

export interface SimState {
  chats: Chat[];
  messages: Record<string, Message[]>;
}

export interface SimResponse<T> {
  ok: boolean;
  result?: T;
  description?: string;
}

export interface MessageEventPayload {
  op: 'created' | 'edited' | 'deleted';
  message: Message;
}

export interface CallbackAnswerEventPayload {
  callback_query_id: string;
  text?: string;
  show_alert?: boolean;
}

export interface ChatActionEventPayload {
  chat_id: number;
  action: string;
  from?: User;
  until: number;
}

export interface MessageDraftEventPayload {
  chat_id: number;
  draft_id: number;
  text?: string;
  entities?: MessageEntity[];
  parse_mode?: string;
  rich_message?: unknown;
  until: number;
}

export type TraceStatus = 'open' | 'ok' | 'error';

export interface InboundEvent {
  update_id: number;
  type: string;
  chat_id: number;
  text?: string;
  at: string;
}

export interface OutboundCall {
  method: string;
  params?: Record<string, unknown>;
  http_status: number;
  ok: boolean;
  error_code?: number;
  error_desc?: string;
  latency_ms: number;
  at: string;
  correlation: 'inferred';
}

export interface Trace {
  id: string;
  inbound?: InboundEvent;
  calls: OutboundCall[];
  started_at: string;
  finished_at?: string;
  status: TraceStatus;
  correlation: 'inferred';
  orphan?: boolean;
}

export interface TraceEventPayload {
  op: 'open' | 'update' | 'close';
  trace: Trace;
}
