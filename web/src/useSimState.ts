import { useCallback, useEffect, useMemo, useState } from 'react';
import { injectCallback, injectText, loadState } from './api';
import type { Chat, Message, MessageEventPayload, SimState } from './types';

const fallbackChat: Chat = {
  id: 1,
  type: 'private',
  first_name: 'Chat 1',
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

export function useSimState() {
  const [state, setState] = useState<SimState>({ chats: [], messages: {} });
  const [selectedChatID, setSelectedChatID] = useState<number>(fallbackChat.id);
  const [status, setStatus] = useState<'connecting' | 'live' | 'offline'>('connecting');
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (signal?: AbortSignal) => {
    const next = await loadState(signal);
    setState(next);
    if (next.chats.length > 0) {
      setSelectedChatID((current) => (next.chats.some((chat) => chat.id === current) ? current : next.chats[0].id));
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    refresh(controller.signal).catch((err: unknown) => {
      if (!controller.signal.aborted) {
        setError(err instanceof Error ? err.message : 'Failed to load state');
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
    return () => source.close();
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
      await injectText(selectedChatID, text);
      await refresh();
    },
    [refresh, selectedChatID],
  );

  const sendCallback = useCallback(
    async (message: Message, data: string) => {
      setError(null);
      await injectCallback(message, data);
    },
    [],
  );

  return {
    chats,
    selectedChatID,
    selectedMessages,
    status,
    error,
    setSelectedChatID,
    sendText,
    sendCallback,
  };
}
