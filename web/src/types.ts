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
