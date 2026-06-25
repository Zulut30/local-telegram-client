import { tokenHeaders } from './token';
import type { Message, SimResponse, SimState, Trace } from './types';

async function readResponse<T>(response: Response): Promise<T> {
  if (response.status === 401) {
    throw new Error('Требуется токен доступа (remote-режим). Откройте интерфейс с ?token=...');
  }
  const payload = (await response.json()) as SimResponse<T>;
  if (!response.ok || !payload.ok || payload.result === undefined) {
    throw new Error(payload.description ?? `Запрос завершился ошибкой ${response.status}`);
  }
  return payload.result;
}

function jsonHeaders(): Record<string, string> {
  return { 'Content-Type': 'application/json', ...tokenHeaders() };
}

async function postJSON(path: string, body: unknown): Promise<Response> {
  return fetch(path, {
    method: 'POST',
    headers: jsonHeaders(),
    body: JSON.stringify(body),
  });
}

export async function loadState(signal?: AbortSignal): Promise<SimState> {
  const response = await fetch('/_sim/state', { signal, headers: tokenHeaders() });
  return readResponse<SimState>(response);
}

export async function loadTraces(signal?: AbortSignal): Promise<Trace[]> {
  const response = await fetch('/_sim/traces', { signal, headers: tokenHeaders() });
  return readResponse<Trace[]>(response);
}

export async function resetSession(): Promise<void> {
  await readResponse<boolean>(await postJSON('/_sim/reset', {}));
}

export async function clearTraces(): Promise<void> {
  await readResponse<boolean>(await postJSON('/_sim/traces/reset', {}));
}

export async function injectText(chatID: number, text: string): Promise<void> {
  await readResponse<unknown>(
    await postJSON('/_sim/inject', {
      type: 'message',
      chat_id: chatID,
      user_id: chatID,
      username: 'developer',
      text,
    }),
  );
}

export async function injectPhoto(chatID: number, photoURL: string, caption: string): Promise<void> {
  await readResponse<unknown>(
    await postJSON('/_sim/inject', {
      type: 'photo',
      chat_id: chatID,
      user_id: chatID,
      username: 'developer',
      photo_url: photoURL,
      caption,
    }),
  );
}

export async function injectCallback(message: Message, data: string): Promise<void> {
  await readResponse<unknown>(
    await postJSON('/_sim/inject', {
      type: 'callback_query',
      chat_id: message.chat.id,
      message_id: message.message_id,
      user_id: message.chat.id,
      username: 'developer',
      data,
    }),
  );
}
