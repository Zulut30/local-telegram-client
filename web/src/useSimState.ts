import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { injectCallback, injectPhoto, injectText, loadState, resetSession } from './api';
import { withTokenURL } from './token';
import type {
  CallbackAnswerEventPayload,
  Chat,
  ChatActionEventPayload,
  Message,
  MessageDraftEventPayload,
  MessageEventPayload,
  SimState,
} from './types';

const fallbackChat: Chat = {
  id: 1,
  type: 'private',
  first_name: 'Чат 1',
};

function sortMessages(messages: Message[]): Message[] {
  return [...messages].sort((a, b) => a.message_id - b.message_id);
}

function applyMessage(state: SimState, payload: MessageEventPayload): SimState {
  const key = String(payload.message.chat.id);
  const existing = state.messages[key] ?? [];
  const withoutCurrent = existing.filter((message) => message.message_id !== payload.message.message_id);
  const nextMessages =
    payload.op === 'deleted' ? withoutCurrent : sortMessages([...withoutCurrent, payload.message]);
  const hasChat = state.chats.some((chat) => chat.id === payload.message.chat.id);
  return {
    chats: hasChat ? state.chats : [...state.chats, payload.message.chat],
    messages: {
      ...state.messages,
      [key]: nextMessages,
    },
  };
}

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

function parseEvent<T>(event: MessageEvent): T | null {
  try {
    return JSON.parse(event.data) as T;
  } catch {
    return null;
  }
}

function withoutChat<T>(items: Record<string, T>, chatID: number): Record<string, T> {
  const next = { ...items };
  delete next[String(chatID)];
  return next;
}

export function useSimState() {
  const [state, setState] = useState<SimState>({ chats: [], messages: {} });
  const [selectedChatID, setSelectedChatID] = useState<number>(fallbackChat.id);
  const [status, setStatus] = useState<'connecting' | 'live' | 'offline'>('connecting');
  const [error, setError] = useState<string | null>(null);
  const [callbackNotice, setCallbackNotice] = useState<string | null>(null);
  const [chatActions, setChatActions] = useState<Record<string, ChatActionEventPayload>>({});
  const [drafts, setDrafts] = useState<Record<string, MessageDraftEventPayload>>({});
  const callbackNoticeTimer = useRef<number | null>(null);

  const refresh = useCallback(async (signal?: AbortSignal) => {
    const next = await loadState(signal);
    setState(next);
    if (next.chats.length > 0) {
      setSelectedChatID((current) => (next.chats.some((chat) => chat.id === current) ? current : next.chats[0].id));
    }
  }, []);

  const refreshLater = useCallback(
    (delayMs = 500) => {
      window.setTimeout(() => {
        refresh().catch((err: unknown) => {
          setError(errorMessage(err, 'Не удалось обновить состояние'));
        });
      }, delayMs);
    },
    [refresh],
  );

  useEffect(() => {
    const controller = new AbortController();
    refresh(controller.signal).catch((err: unknown) => {
      if (!controller.signal.aborted) {
        setError(err instanceof Error ? err.message : 'Не удалось загрузить состояние');
      }
    });
    return () => controller.abort();
  }, [refresh]);

  useEffect(() => {
    const source = new EventSource(withTokenURL('/_sim/events'));
    const expiryTimers = new Set<number>();
    let opened = false;

    const scheduleExpiry = (until: number, run: () => void) => {
      const handle = window.setTimeout(() => {
        expiryTimers.delete(handle);
        run();
      }, Math.max(0, until - Date.now()) + 50);
      expiryTimers.add(handle);
    };

    source.addEventListener('open', () => {
      setStatus('live');
      setError(null);
      // On every (re)connect, re-sync the full state so events missed during the
      // gap (e.g. after a network drop or server write-timeout) are recovered.
      if (opened) {
        refresh().catch((err: unknown) => setError(errorMessage(err, 'Не удалось пересинхронизировать состояние')));
      }
      opened = true;
    });
    source.addEventListener('error', () => {
      setStatus('offline');
    });
    source.addEventListener('message', (event) => {
      const payload = parseEvent<MessageEventPayload>(event);
      if (!payload) {
        return;
      }
      setState((current) => applyMessage(current, payload));
      setChatActions((current) => withoutChat(current, payload.message.chat.id));
      setDrafts((current) => withoutChat(current, payload.message.chat.id));
      setSelectedChatID((current) => current || payload.message.chat.id);
    });
    source.addEventListener('chat_action', (event) => {
      const payload = parseEvent<ChatActionEventPayload>(event);
      if (!payload) {
        return;
      }
      setChatActions((current) => ({ ...current, [String(payload.chat_id)]: payload }));
      scheduleExpiry(payload.until, () => {
        setChatActions((current) => {
          const active = current[String(payload.chat_id)];
          return active && active.until <= Date.now() ? withoutChat(current, payload.chat_id) : current;
        });
      });
    });
    source.addEventListener('message_draft', (event) => {
      const payload = parseEvent<MessageDraftEventPayload>(event);
      if (!payload) {
        return;
      }
      setDrafts((current) => ({ ...current, [String(payload.chat_id)]: payload }));
      setChatActions((current) => withoutChat(current, payload.chat_id));
      scheduleExpiry(payload.until, () => {
        setDrafts((current) => {
          const active = current[String(payload.chat_id)];
          return active && active.until <= Date.now() ? withoutChat(current, payload.chat_id) : current;
        });
      });
    });
    source.addEventListener('callback_answer', (event) => {
      const payload = parseEvent<CallbackAnswerEventPayload>(event);
      if (!payload) {
        return;
      }
      setCallbackNotice(payload.text || 'Callback обработан');
      if (callbackNoticeTimer.current !== null) {
        window.clearTimeout(callbackNoticeTimer.current);
      }
      callbackNoticeTimer.current = window.setTimeout(() => {
        setCallbackNotice(null);
        callbackNoticeTimer.current = null;
      }, payload.show_alert ? 5000 : 2400);
    });
    return () => {
      source.close();
      expiryTimers.forEach((handle) => window.clearTimeout(handle));
      expiryTimers.clear();
      if (callbackNoticeTimer.current !== null) {
        window.clearTimeout(callbackNoticeTimer.current);
        callbackNoticeTimer.current = null;
      }
    };
  }, [refresh]);

  useEffect(() => {
    if (state.chats.length > 0 && !state.chats.some((chat) => chat.id === selectedChatID)) {
      setSelectedChatID(state.chats[0].id);
    }
  }, [selectedChatID, state.chats]);

  const chats = useMemo(() => {
    return state.chats.length > 0 ? state.chats : [fallbackChat];
  }, [state.chats]);

  const selectedMessages = state.messages[String(selectedChatID)] ?? [];

  const sendText = useCallback(
    async (text: string) => {
      setError(null);
      try {
        await injectText(selectedChatID, text);
        await refresh();
        refreshLater();
      } catch (err) {
        setError(errorMessage(err, 'Не удалось отправить сообщение'));
      }
    },
    [refresh, refreshLater, selectedChatID],
  );

  const sendPhoto = useCallback(
    async (photoURL: string, caption: string) => {
      setError(null);
      try {
        await injectPhoto(selectedChatID, photoURL, caption);
        await refresh();
        refreshLater();
      } catch (err) {
        setError(errorMessage(err, 'Не удалось отправить фото'));
      }
    },
    [refresh, refreshLater, selectedChatID],
  );

  const sendCallback = useCallback(
    async (message: Message, data: string) => {
      setError(null);
      try {
        await injectCallback(message, data);
        await refresh();
        refreshLater();
      } catch (err) {
        setError(errorMessage(err, 'Не удалось отправить callback'));
      }
    },
    [refresh, refreshLater],
  );

  const reset = useCallback(async () => {
    setError(null);
    try {
      await resetSession();
      setCallbackNotice(null);
      setChatActions({});
      setDrafts({});
      setState({ chats: [], messages: {} });
      setSelectedChatID(fallbackChat.id);
      await refresh();
    } catch (err) {
      setError(errorMessage(err, 'Не удалось сбросить сессию'));
    }
  }, [refresh]);

  return {
    chats,
    selectedChatID,
    selectedMessages,
    selectedChatAction: chatActions[String(selectedChatID)],
    selectedDraft: drafts[String(selectedChatID)],
    status,
    error,
    callbackNotice,
    setSelectedChatID,
    sendText,
    sendPhoto,
    sendCallback,
    reset,
  };
}
