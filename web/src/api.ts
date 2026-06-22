import type { Message, SimResponse, SimState, Trace } from './types';

async function readResponse<T>(response: Response): Promise<T> {
  const payload = (await response.json()) as SimResponse<T>;
  if (!response.ok || !payload.ok || payload.result === undefined) {
    throw new Error(payload.description ?? `Request failed with ${response.status}`);
  }
  return payload.result;
}

export async function loadState(signal?: AbortSignal): Promise<SimState> {
  const response = await fetch('/_sim/state', { signal });
  return readResponse<SimState>(response);
}

export async function loadTraces(signal?: AbortSignal): Promise<Trace[]> {
  const response = await fetch('/_sim/traces', { signal });
  return readResponse<Trace[]>(response);
}

export async function injectText(chatID: number, text: string): Promise<void> {
  const response = await fetch('/_sim/inject', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      type: 'message',
      chat_id: chatID,
      user_id: chatID,
      username: 'developer',
      text,
    }),
  });
  await readResponse<unknown>(response);
}

export async function injectCallback(message: Message, data: string): Promise<void> {
  const response = await fetch('/_sim/inject', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      type: 'callback_query',
      chat_id: message.chat.id,
      message_id: message.message_id,
      user_id: message.chat.id,
      username: 'developer',
      data,
    }),
  });
  await readResponse<unknown>(response);
}
