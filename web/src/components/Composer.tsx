import { useState } from 'react';
import type { FormEvent } from 'react';

interface ComposerProps {
  onSend: (text: string) => Promise<void>;
}

export function Composer({ onSend }: ComposerProps) {
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const next = text.trim();
    if (!next || sending) {
      return;
    }
    setSending(true);
    try {
      await onSend(next);
      setText('');
    } finally {
      setSending(false);
    }
  }

  return (
    <form className="composer" onSubmit={submit}>
      <label className="sr-only" htmlFor="message-text">
        Message
      </label>
      <input
        id="message-text"
        value={text}
        onChange={(event) => setText(event.target.value)}
        placeholder="Type as user"
        autoComplete="off"
      />
      <button type="submit" disabled={sending || text.trim() === ''}>
        Send
      </button>
    </form>
  );
}
