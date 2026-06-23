import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { injectCallback, injectPhoto, injectText, loadState, resetSession } from './api';
import type { CallbackAnswerEventPayload, Chat, Message, MessageEventPayload, SimState } from './types';

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

export function useSimState() {
  const [state, setState] = useState<SimState>({ chats: [], messages: {} });
  const [selectedChatID, setSelectedChatID] = useState<number>(fallbackChat.id);
  const [status, setStatus] = useState<'connecting' | 'live' | 'offline'>('connecting');
  const [error, setError] = useState<string | null>(null);
  const [callbackNotice, setCallbackNotice] = useState<string | null>(null);
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
    const source = new EventSource('/_sim/events');
    source.addEventListener('open', () => {
      setStatus('live');
      setError(null);
    });
    source.addEventListener('error', () => {
      setStatus('offline');
    });
    source.addEventListener('message', (event) => {
      const payload = JSON.parse(event.data) as MessageEventPayload;
      setState((current) => applyMessage(current, payload));
      setSelectedChatID((current) => current || payload.message.chat.id);
    });
    source.addEventListener('callback_answer', (event) => {
      const payload = JSON.parse(event.data) as CallbackAnswerEventPayload;
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
      if (callbackNoticeTimer.current !== null) {
        window.clearTimeout(callbackNoticeTimer.current);
      }
    };
  }, []);

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
