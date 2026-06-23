import { useRef, useState } from 'react';
import type { ChangeEvent, FormEvent } from 'react';

interface ComposerProps {
  onSend: (text: string) => Promise<void>;
  onPhoto: (photoURL: string, caption: string) => Promise<void>;
}

function readAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.addEventListener('load', () => resolve(String(reader.result ?? '')));
    reader.addEventListener('error', () => reject(reader.error ?? new Error('Failed to read file')));
    reader.readAsDataURL(file);
  });
}

export function Composer({ onSend, onPhoto }: ComposerProps) {
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

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

  async function attachPhoto(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file || sending) {
      return;
    }
    setSending(true);
    try {
      const photoURL = await readAsDataURL(file);
      await onPhoto(photoURL, text.trim());
      setText('');
    } finally {
      setSending(false);
    }
  }

  return (
    <form className="composer" onSubmit={submit}>
      <input
        ref={fileInputRef}
        className="sr-only"
        type="file"
        accept="image/*"
        onChange={attachPhoto}
        aria-label="Attach photo file"
      />
      <button
        className="composer__attach"
        type="button"
        title="Attach photo"
        aria-label="Attach photo"
        disabled={sending}
        onClick={() => fileInputRef.current?.click()}
      >
        📎
      </button>
      <label className="sr-only" htmlFor="message-text">
        Message
      </label>
      <input
        id="message-text"
        value={text}
        onChange={(event) => setText(event.target.value)}
        placeholder="Message or photo caption"
        autoComplete="off"
      />
      <button type="submit" disabled={sending || text.trim() === ''}>
        Send
      </button>
    </form>
  );
}
