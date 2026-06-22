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
  reply_markup?: ReplyMarkup;
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
