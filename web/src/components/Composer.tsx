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
    reader.addEventListener('error', () => reject(reader.error ?? new Error('Не удалось прочитать файл')));
    reader.readAsDataURL(file);
  });
}

export function Composer({ onSend, onPhoto }: ComposerProps) {
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const [attachError, setAttachError] = useState<string | null>(null);
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
    setAttachError(null);
    setSending(true);
    try {
      const photoURL = await readAsDataURL(file);
      await onPhoto(photoURL, text.trim());
      setText('');
    } catch (err) {
      setAttachError(err instanceof Error ? err.message : 'Не удалось прикрепить фото');
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
        aria-label="Прикрепить фото"
      />
      <button
        className="composer__attach"
        type="button"
        title="Прикрепить фото"
        aria-label="Прикрепить фото"
        disabled={sending}
        onClick={() => fileInputRef.current?.click()}
      >
        📎
      </button>
      <label className="sr-only" htmlFor="message-text">
        Сообщение
      </label>
      <input
        id="message-text"
        value={text}
        onChange={(event) => setText(event.target.value)}
        placeholder="Сообщение или подпись к фото"
        autoComplete="off"
      />
      <button type="submit" disabled={sending || text.trim() === ''}>
        Отправить
      </button>
      {attachError ? (
        <span className="composer__error" role="alert">
          {attachError}
        </span>
      ) : null}
    </form>
  );
}
